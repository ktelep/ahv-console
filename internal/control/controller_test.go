package control

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ahv-console/internal/fault"
	"ahv-console/internal/prism"
)

const vmID = "00000000-0000-4000-8000-000000000002"
const clusterID = "00000000-0000-4000-8000-000000000003"

type fakeBackend struct {
	power   atomic.Value
	calls   atomic.Int32
	tokens  atomic.Int32
	pending bool
	badRead atomic.Bool
}

func (b *fakeBackend) VMState(ctx context.Context, id string) (prism.VM, error) {
	if b.badRead.Load() {
		return prism.VM{}, fault.New(fault.Permission, "PC denied VM state read.")
	}
	var vm prism.VM
	vm.ID = id
	vm.Name = "fixture VM"
	vm.Cluster.ID = clusterID
	vm.PowerState = b.power.Load().(string)
	return vm, nil
}
func (b *fakeBackend) Drives(context.Context, string) ([]prism.Drive, error) {
	return []prism.Drive{}, nil
}
func (b *fakeBackend) Images(context.Context, string) ([]prism.Image, error) {
	return []prism.Image{}, nil
}
func (b *fakeBackend) Console(context.Context, string) (*prism.Session, error) {
	b.tokens.Add(1)
	return nil, fault.New(fault.Permission, "Console permission denied.")
}
func (b *fakeBackend) Power(ctx context.Context, id, action string) prism.MutationResult {
	b.calls.Add(1)
	if b.pending {
		<-ctx.Done()
		return prism.MutationResult{Status: "unconfirmed", Message: "Outcome unconfirmed."}
	}
	if action == "power-on" {
		b.power.Store("ON")
	}
	return prism.MutationResult{Status: "succeeded", Message: "Task succeeded."}
}
func (b *fakeBackend) Media(ctx context.Context, id, drive, image string, eject bool) prism.MutationResult {
	return b.Power(ctx, id, "media")
}
func testController(t *testing.T, power string, pending bool, timeout time.Duration) (*Controller, *fakeBackend) {
	t.Helper()
	b := &fakeBackend{pending: pending}
	b.power.Store(power)
	var vm prism.VM
	vm.ID = vmID
	vm.Name = "fixture VM"
	vm.Cluster.ID = clusterID
	c := New(context.Background(), b, vm, timeout)
	t.Cleanup(c.Close)
	return c, b
}
func terminal(t *testing.T, c *Controller, id string) Operation {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		op, _ := c.Operation(id)
		if op.Status != "pending" {
			return op
		}
		select {
		case <-deadline:
			t.Fatal("operation did not stop")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestIndependentState(t *testing.T) {
	c, b := testController(t, "OFF", false, time.Second)
	s := c.State(context.Background())
	if s.Power != "OFF" || s.Stale || b.tokens.Load() != 0 {
		t.Fatal(s)
	}
	if _, err := c.Dial(context.Background()); err == nil || b.tokens.Load() != 0 {
		t.Fatal("off token generation")
	}
	b.power.Store("ON")
	if _, err := c.Dial(context.Background()); err == nil {
		t.Fatal("expected console denial")
	}
	s = c.State(context.Background())
	if s.Stale || s.ManagementError != "" || s.ConsoleError == "" {
		t.Fatal("console broke management", s)
	}
	if _, err := c.Dial(context.Background()); err == nil || b.tokens.Load() != 1 {
		t.Fatal("silently renewed rejected authorization")
	}
	b.power.Store("OFF")
	c.State(context.Background())
	b.power.Store("ON")
	c.Dial(context.Background())
	if b.tokens.Load() != 2 {
		t.Fatal("off transition did not clear acquisition guard")
	}
	b.badRead.Store(true)
	s = c.State(context.Background())
	if !s.Stale || s.ManagementError == "" {
		t.Fatal("stale state not surfaced")
	}
}

func TestSerializedOperationsTimeoutAndCancel(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		c, b := testController(t, "ON", true, 40*time.Millisecond)
		var accepted atomic.Int32
		var id string
		var mu sync.Mutex
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				op, err := c.Submit(Action{Action: "reset"})
				if err == nil {
					accepted.Add(1)
					mu.Lock()
					id = op.ID
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if accepted.Load() != 1 {
			t.Fatal("accepted conflicting operations", accepted.Load())
		}
		if cancel {
			c.Close()
		}
		op := terminal(t, c, id)
		if op.Status != "unconfirmed" || b.calls.Load() > 1 {
			t.Fatal(op, b.calls.Load())
		}
		if _, ok := c.Operation("other-session"); ok {
			t.Fatal("foreign operation")
		}
	}
}

func TestMountAndStartupIndependent(t *testing.T) {
	c, b := testController(t, "OFF", false, time.Second)
	op, err := c.Submit(Action{Action: "insert", DriveID: clusterID, ImageID: vmID})
	if err != nil {
		t.Fatal(err)
	}
	terminal(t, c, op.ID)
	if c.State(context.Background()).Power != "OFF" || b.tokens.Load() != 0 {
		t.Fatal("mount powered on or generated console")
	}
	op, err = c.Submit(Action{Action: "power-on"})
	if err != nil {
		t.Fatal(err)
	}
	terminal(t, c, op.ID)
	if c.State(context.Background()).Power != "ON" || b.calls.Load() != 2 {
		t.Fatal("startup failed")
	}
}
