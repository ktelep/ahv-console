// Package control owns one selected VM for the foreground process lifetime.
package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"ahv-console/internal/fault"
	"ahv-console/internal/prism"
	"github.com/coder/websocket"
)

type Backend interface {
	VMState(context.Context, string) (prism.VM, error)
	Drives(context.Context, string) ([]prism.Drive, error)
	Images(context.Context, string) ([]prism.Image, error)
	Power(context.Context, string, string) prism.MutationResult
	Media(context.Context, string, string, string, bool) prism.MutationResult
	Console(context.Context, string) (*prism.Session, error)
}

type State struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Power           string     `json:"power"`
	ObservedAt      time.Time  `json:"observedAt"`
	Stale           bool       `json:"stale"`
	ManagementError string     `json:"managementError"`
	ConsoleError    string     `json:"consoleError"`
	Operation       *Operation `json:"operation,omitempty"`
}

type Action struct {
	Action  string `json:"action"`
	DriveID string `json:"driveId,omitempty"`
	ImageID string `json:"imageId,omitempty"`
}

type Operation struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type Controller struct {
	ctx              context.Context
	cancel           context.CancelFunc
	backend          Backend
	vm               prism.VM
	timeout          time.Duration
	mu               sync.Mutex
	readGate         chan struct{}
	consoleGate      chan struct{}
	state            State
	operation        *Operation
	operations       map[string]Operation
	session          *prism.Session
	consoleAttempted bool
	consoleEpoch     uint64
	console          *websocket.Conn
	wg               sync.WaitGroup
}

func New(ctx context.Context, backend Backend, vm prism.VM, timeout time.Duration) *Controller {
	ctx, cancel := context.WithCancel(ctx)
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	c := &Controller{ctx: ctx, cancel: cancel, backend: backend, vm: vm, timeout: timeout, readGate: make(chan struct{}, 1), consoleGate: make(chan struct{}, 1), operations: map[string]Operation{}}
	c.state = State{ID: vm.ID, Name: vm.Name, Power: "UNKNOWN", Stale: true}
	return c
}

func (c *Controller) Close() {
	c.cancel()
	c.mu.Lock()
	if c.console != nil {
		c.console.CloseNow()
	}
	c.mu.Unlock()
	c.wg.Wait()
}

func (c *Controller) context(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	stop := context.AfterFunc(parent, cancel)
	if parent.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}

func (c *Controller) snapshot() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.state
	if c.operation != nil {
		op := *c.operation
		s.Operation = &op
	}
	return s
}

func (c *Controller) State(parent context.Context) State {
	ctx, cancel := c.context(parent)
	defer cancel()
	select {
	case c.readGate <- struct{}{}:
	case <-ctx.Done():
		return c.snapshot()
	}
	defer func() { <-c.readGate }()
	vm, err := c.backend.VMState(ctx, c.vm.ID)
	c.mu.Lock()
	if err != nil {
		c.state.Stale = true
		c.state.ManagementError, _ = fault.Diagnostic(err)
	} else {
		if vm.PowerState == "OFF" && c.state.Power != "OFF" {
			c.consoleEpoch++
			c.session = nil
			c.consoleAttempted = false
			c.state.ConsoleError = ""
			if c.console != nil {
				c.console.CloseNow()
				c.console = nil
			}
		}
		c.state.Power = vm.PowerState
		c.state.ObservedAt = time.Now().UTC()
		c.state.Stale = false
		c.state.ManagementError = ""
		c.vm.Cluster = vm.Cluster
	}
	c.mu.Unlock()
	return c.snapshot()
}

func (c *Controller) Drives(parent context.Context) ([]prism.Drive, error) {
	ctx, cancel := c.context(parent)
	defer cancel()
	return c.backend.Drives(ctx, c.vm.ID)
}

func (c *Controller) Images(parent context.Context) ([]prism.Image, error) {
	ctx, cancel := c.context(parent)
	defer cancel()
	s := c.State(ctx)
	if s.Stale {
		return nil, fault.New(fault.Connection, "VM cluster state is unavailable; refresh before loading ISO inventory.")
	}
	c.mu.Lock()
	cluster := c.vm.Cluster.ID
	c.mu.Unlock()
	return c.backend.Images(ctx, cluster)
}

func (c *Controller) Submit(a Action) (Operation, error) {
	validPower := a.Action == "power-on" || a.Action == "power-off" || a.Action == "reset" || a.Action == "guest-reboot" || a.Action == "guest-shutdown"
	validMedia := a.Action == "insert" || a.Action == "eject"
	if (!validPower && !validMedia) || (validPower && (a.DriveID != "" || a.ImageID != "")) || (validMedia && a.DriveID == "") || (a.Action == "insert" && a.ImageID == "") || (a.Action == "eject" && a.ImageID != "") {
		return Operation{}, fault.New(fault.Configuration, "Invalid selected-VM action.")
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return Operation{}, fault.New(fault.Connection, "Secure operation identifier generation failed.")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx.Err() != nil {
		return Operation{}, c.ctx.Err()
	}
	if c.operation != nil && c.operation.Status == "pending" {
		return Operation{}, fault.New(fault.Task, "Another power or media operation is pending.")
	}
	op := Operation{ID: hex.EncodeToString(b[:]), Action: a.Action, Status: "pending", Message: "Operation accepted locally; waiting for PC."}
	c.operation = &op
	c.operations[op.ID] = op
	// Bound history to the most recent operation; other invocation IDs are never accepted.
	for id := range c.operations {
		if id != op.ID {
			delete(c.operations, id)
		}
	}
	c.wg.Add(1)
	go c.execute(a, op)
	return op, nil
}

func (c *Controller) execute(a Action, op Operation) {
	defer c.wg.Done()
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()
	var result prism.MutationResult
	if a.Action == "insert" || a.Action == "eject" {
		result = c.backend.Media(ctx, c.vm.ID, a.DriveID, a.ImageID, a.Action == "eject")
	} else {
		s := c.State(ctx)
		if s.Stale || (a.Action == "power-on" && s.Power != "OFF") || (a.Action != "power-on" && s.Power != "ON") {
			result = prism.MutationResult{Status: "failed", Message: "Power state changed or is unknown; refresh before retrying."}
		} else {
			result = c.backend.Power(ctx, c.vm.ID, a.Action)
		}
	}
	if c.ctx.Err() != nil {
		result = prism.MutationResult{Status: "unconfirmed", Message: "Local monitoring stopped; already accepted upstream work may still complete."}
	}
	op.Status = result.Status
	op.Message = result.Message
	c.mu.Lock()
	c.operations[op.ID] = op
	c.operation = &op
	c.mu.Unlock()
	// Publish the terminal outcome promptly, then refresh with a fresh bounded
	// process context even if the mutation exhausted its own deadline.
	if c.ctx.Err() == nil {
		c.State(c.ctx)
	}
}

func (c *Controller) Operation(id string) (Operation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	op, ok := c.operations[id]
	return op, ok
}

func (c *Controller) Dial(parent context.Context) (*websocket.Conn, error) {
	ctx, cancel := c.context(parent)
	defer cancel()
	select {
	case c.consoleGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.consoleGate }()
	s := c.State(ctx)
	if s.Stale || s.Power != "ON" {
		return nil, fault.New(fault.Connection, "Console unavailable until the VM is observed powered on; power/media controls remain available.")
	}
	c.mu.Lock()
	session := c.session
	attempted := c.consoleAttempted
	epoch := c.consoleEpoch
	c.mu.Unlock()
	if session == nil {
		if attempted {
			return nil, fault.New(fault.Connection, "Console authorization expired or was rejected; rerun the command.")
		}
		c.mu.Lock()
		c.consoleAttempted = true
		c.mu.Unlock()
		var err error
		session, err = c.backend.Console(ctx, c.vm.ID)
		if err != nil {
			message, _ := fault.Diagnostic(err)
			c.mu.Lock()
			if epoch == c.consoleEpoch {
				c.state.ConsoleError = message
				var f *fault.Error
				if !errors.As(err, &f) || (f.Kind != fault.Authentication && f.Kind != fault.Permission && f.Kind != fault.Protocol) {
					c.consoleAttempted = false
				}
			}
			c.mu.Unlock()
			return nil, err
		}
		c.mu.Lock()
		if epoch != c.consoleEpoch {
			c.mu.Unlock()
			return nil, fault.New(fault.Connection, "VM powered off during console acquisition; reconnect after startup.")
		}
		c.session = session
		c.mu.Unlock()
	}
	conn, err := session.Dial(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if epoch != c.consoleEpoch {
		if conn != nil {
			conn.CloseNow()
		}
		return nil, fault.New(fault.Connection, "VM powered off during console connection.")
	}
	if err != nil {
		c.state.ConsoleError, _ = fault.Diagnostic(err)
		return nil, err
	}
	c.console = conn
	c.state.ConsoleError = ""
	return conn, nil
}
