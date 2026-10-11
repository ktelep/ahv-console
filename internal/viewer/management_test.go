package viewer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"ahv-console/internal/control"
	"ahv-console/internal/prism"
	"github.com/coder/websocket"
)

type testManager struct{ submits atomic.Int32 }

func (*testManager) Dial(context.Context) (*websocket.Conn, error) {
	return nil, errors.New("fixture-secret")
}
func (*testManager) State(context.Context) control.State {
	return control.State{ID: "selected", Power: "OFF"}
}
func (*testManager) Drives(context.Context) ([]prism.Drive, error) {
	return []prism.Drive{{ID: "selected-drive"}}, nil
}
func (*testManager) Images(context.Context) ([]prism.Image, error) {
	return nil, errors.New("fixture-secret password cookie token")
}
func (m *testManager) Submit(a control.Action) (control.Operation, error) {
	m.submits.Add(1)
	return control.Operation{ID: strings.Repeat("a", 32), Status: "pending"}, nil
}
func (*testManager) Operation(id string) (control.Operation, bool) { return control.Operation{}, false }

func TestManagementProtection(t *testing.T) {
	m := &testManager{}
	s, err := Start(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := bootstrap(t, s)
	other, _ := Start(context.Background(), m)
	defer other.Close()
	for _, tc := range []struct {
		name, path, method, body, origin, host string
		authorized                             bool
		want                                   int
	}{
		{"no cookie", "/api/actions", "POST", `{"action":"reset"}`, s.origin, "", false, 403},
		{"wrong origin", "/api/actions", "POST", `{"action":"reset"}`, "https://evil.example", "", true, 403},
		{"missing origin", "/api/actions", "POST", `{"action":"reset"}`, "", "", true, 403},
		{"wrong host", "/api/actions", "POST", `{"action":"reset"}`, s.origin, "evil.example", true, 403},
		{"wrong method", "/api/actions", "GET", "", s.origin, "", true, 405},
		{"cross VM", "/api/actions", "POST", `{"action":"reset","vmId":"other"}`, s.origin, "", true, 400},
		{"cross device", "/api/actions", "POST", `{"action":"eject","driveId":"other"}`, s.origin, "", true, 400},
		{"malformed", "/api/actions", "POST", `{`, s.origin, "", true, 400},
		{"duplicate", "/api/actions", "POST", `{"action":"reset","action":"power-off"}`, s.origin, "", true, 400},
		{"oversized", "/api/actions", "POST", `{"action":"` + strings.Repeat("x", 5000) + `"}`, s.origin, "", true, 400},
		{"trailing", "/api/actions", "POST", `{"action":"reset"}{}`, s.origin, "", true, 400},
		{"foreign operation", "/api/operations/" + strings.Repeat("b", 32), "GET", "", "", "", true, 404},
		{"read origin", "/api/state", "GET", "", "https://evil.example", "", true, 403},
		{"read method", "/api/state", "POST", "", s.origin, "", true, 405},
		{"state", "/api/state", "GET", "", "", "", true, 200},
		{"inventory denied", "/api/images", "GET", "", "", "", true, 502},
		{"accepted", "/api/actions", "POST", `{"action":"reset"}`, s.origin, "", true, 202},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, s.origin+tc.path, strings.NewReader(tc.body))
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Content-Type", "application/json")
			if tc.host != "" {
				req.Host = tc.host
			}
			client := c
			if !tc.authorized {
				client = http.DefaultClient
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatal(resp.StatusCode, string(b))
			}
			if strings.Contains(string(b), "fixture-secret") || strings.Contains(string(b), s.cookie) || strings.Contains(string(b), s.bootstrap) {
				t.Fatal("secret response")
			}
		})
	}
	if m.submits.Load() != 1 {
		t.Fatal("unauthorized submission", m.submits.Load())
	}
	// A different invocation cookie cannot authorize these endpoints.
	req, _ := http.NewRequest("GET", s.origin+"/api/state", nil)
	req.AddCookie(&http.Cookie{Name: s.cookieName, Value: other.cookie})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("cross-session request")
	}
}
