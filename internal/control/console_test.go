package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ahv-console/internal/prism"
	"github.com/coder/websocket"
)

func TestDemandAuthorizationReuseAndOffInvalidation(t *testing.T) {
	var tokens atomic.Int32
	var rejected atomic.Bool
	var power atomic.Value
	power.Store("OFF")
	taskID := "ZXJnb24=:00000000-0000-4000-8000-000000000001"
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/console/") {
			if rejected.Load() {
				w.WriteHeader(403)
				return
			}
			ws, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer ws.CloseNow()
			ws.Read(r.Context())
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "NTNX_SESSION_ID", Value: "synthetic-cookie", Path: "/", Secure: true})
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			tokens.Add(1)
			w.WriteHeader(202)
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"extId": taskID}})
			return
		}
		if strings.Contains(r.URL.Path, "/tasks/") {
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": "SUCCEEDED", "completionDetails": []any{map[string]string{"name": "WsUri", "value": "/console/launch/" + vmID}, map[string]string{"name": "VmConsoleToken", "value": "synthetic-token"}}}})
			return
		}
		var vm prism.VM
		vm.ID = vmID
		vm.Cluster.ID = clusterID
		vm.PowerState = power.Load().(string)
		json.NewEncoder(w).Encode(map[string]any{"data": vm})
	}))
	defer s.Close()
	pc, err := prism.New(prism.Options{BaseURL: s.URL, Insecure: true, SetupTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	var vm prism.VM
	vm.ID = vmID
	vm.Cluster.ID = clusterID
	c := New(context.Background(), pc, vm, time.Second)
	defer c.Close()
	if _, err = c.Dial(context.Background()); err == nil || tokens.Load() != 0 {
		t.Fatal("off-state acquisition")
	}
	power.Store("ON")
	for i := 0; i < 2; i++ {
		conn, e := c.Dial(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		conn.CloseNow()
	}
	if tokens.Load() != 1 {
		t.Fatal("authorization not reused")
	}
	rejected.Store(true)
	_, err = c.Dial(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rerun") || tokens.Load() != 1 {
		t.Fatal("renewed rejected token", err)
	}
	power.Store("OFF")
	c.State(context.Background())
	rejected.Store(false)
	power.Store("ON")
	conn, err := c.Dial(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	conn.CloseNow()
	if tokens.Load() != 2 {
		t.Fatal("startup authorization not reacquired")
	}
}
