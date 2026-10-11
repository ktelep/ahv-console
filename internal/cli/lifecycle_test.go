package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Synthetic PC lifecycle fixture; never performs live VM mutations.
func TestViewerLifecycle(t *testing.T) {
	for _, mode := range []string{"off", "console-denied", "browser-fallback", "pending-cancel"} {
		t.Run(mode, func(t *testing.T) {
			var tokens atomic.Int32
			var posts atomic.Int32
			vmID := "00000000-0000-4000-8000-000000000002"
			clusterID := "00000000-0000-4000-8000-000000000003"
			pc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts.Add(1)
					if strings.Contains(r.URL.Path, "generate-console") {
						tokens.Add(1)
						w.WriteHeader(403)
						return
					}
					w.WriteHeader(202)
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"extId": "ZXJnb24=:" + vmID}})
					return
				}
				if strings.Contains(r.URL.Path, "/tasks/") {
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "RUNNING"}})
					return
				}
				if strings.Contains(r.URL.Path, "/clusters/") {
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"extId": clusterID, "name": "test-cluster"}})
					return
				}
				power := "OFF"
				if mode == "console-denied" {
					power = "ON"
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"extId": vmID, "name": "test-vm", "cluster": map[string]string{"extId": clusterID}, "powerState": power}})
			}))
			defer pc.Close()
			u, _ := url.Parse(pc.URL)
			host, port, _ := net.SplitHostPort(u.Host)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			launched := make(chan string, 1)
			done := make(chan error, 1)
			go func() {
				done <- run(ctx, []string{"--pc-host", host, "--pc-port", port, "--username", "fixture-user", "--insecure", "--non-interactive", "--vm-id", vmID}, func(key string) string {
					if key == "AHV_CONSOLE_PASSWORD" {
						return "fixture-password"
					}
					return ""
				}, io.Discard, io.Discard, func(address string) error {
					launched <- address
					if mode == "browser-fallback" {
						return errors.New("synthetic launcher failure")
					}
					return nil
				})
			}()
			var address string
			select {
			case address = <-launched:
			case err := <-done:
				t.Fatal("setup failed", err)
			case <-time.After(3 * time.Second):
				t.Fatal("no viewer")
			}
			local, _ := url.Parse(address)
			origin := local.Scheme + "://" + local.Host
			jar, _ := cookiejar.New(nil)
			client := &http.Client{Jar: jar}
			req, _ := http.NewRequest("POST", origin+"/bootstrap", strings.NewReader(`{"secret":"`+local.Fragment+`"}`))
			req.Header.Set("Origin", origin)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 204 {
				t.Fatal(resp.StatusCode)
			}
			if tokens.Load() != 0 {
				t.Fatal("startup acquired console")
			}
			if mode == "console-denied" {
				ws, _, err := websocket.Dial(ctx, "ws://"+local.Host+"/ws", &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": {origin}}})
				if err != nil {
					t.Fatal(err)
				}
				ws.Read(ctx)
				ws.CloseNow()
				if tokens.Load() != 1 {
					t.Fatal("console demand absent")
				}
			}
			if mode == "pending-cancel" {
				req, _ = http.NewRequest("POST", origin+"/api/actions", strings.NewReader(`{"action":"power-on"}`))
				req.Header.Set("Origin", origin)
				req.Header.Set("Content-Type", "application/json")
				resp, err = client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if resp.StatusCode != 202 {
					t.Fatal(resp.StatusCode)
				}
				deadline := time.Now().Add(time.Second)
				for posts.Load() == 0 && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if posts.Load() == 0 {
					t.Fatal("pending work not submitted")
				}
			}
			resp, err = client.Get(origin + "/api/state")
			if err != nil {
				t.Fatal("viewer exited", err)
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatal(resp.StatusCode)
			}
			select {
			case err := <-done:
				t.Fatal("exited before cancellation", err)
			default:
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("shutdown blocked")
			}
			if _, err = client.Get(origin); err == nil {
				t.Fatal("listener remains")
			}
		})
	}
}
