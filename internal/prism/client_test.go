package prism

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ahv-console/internal/fault"
	"github.com/coder/websocket"
)

const testVM = "00000000-0000-4000-8000-000000000002"
const testCluster = "00000000-0000-4000-8000-000000000003"
const testTask = "ZXJnb24=:00000000-0000-4000-8000-000000000001"

func clientFor(t *testing.T, s *httptest.Server) *Client {
	t.Helper()
	c, err := New(Options{BaseURL: s.URL, Username: "fixture-user", Password: "fixture-secret", Insecure: true, SetupTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	c.pollInterval = time.Millisecond
	t.Cleanup(c.Close)
	return c
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pc76", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func kind(t *testing.T, err error, want fault.Kind) {
	t.Helper()
	var f *fault.Error
	if !errors.As(err, &f) || f.Kind != want {
		t.Fatalf("wanted %s, got %v", want, err)
	}
}

func TestTLSAndAuth(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "fixture-user" || p != "fixture-secret" {
			w.WriteHeader(401)
			w.Write(fixture(t, "authentication.failed.json"))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "NTNX_SESSION_ID", Value: "fixture-cookie", Path: "/", Secure: true, HttpOnly: true})
		writeJSON(w, map[string]any{"data": []any{}})
	}))
	defer s.Close()
	c, err := New(Options{BaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var out any
	_, err = c.request(context.Background(), "GET", vmsPath, nil, "", &out)
	kind(t, err, fault.TLS)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	c, err = New(Options{BaseURL: s.URL, CACert: ca, Username: "fixture-user", Password: "fixture-secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.request(context.Background(), "GET", vmsPath, nil, "", &out); err != nil {
		t.Fatal(err)
	}
	if len(c.http.Jar.Cookies(c.base)) != 1 {
		t.Fatal("session cookie not retained")
	}
	c.username = "wrong"
	_, err = c.request(context.Background(), "GET", vmsPath, nil, "", &out)
	kind(t, err, fault.Authentication)
	bad := filepath.Join(t.TempDir(), "bad.pem")
	os.WriteFile(bad, []byte("not a certificate"), 0600)
	_, err = New(Options{BaseURL: s.URL, CACert: bad})
	kind(t, err, fault.Configuration)
}

func TestRedirectDoesNotForwardSecrets(t *testing.T) {
	var hits atomic.Int32
	dest := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer dest.Close()
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL+"/?VmConsoleToken=fixture-secret", http.StatusTemporaryRedirect)
	}))
	defer s.Close()
	c := clientFor(t, s)
	var out any
	_, err := c.request(context.Background(), "GET", vmsPath, nil, "", &out)
	kind(t, err, fault.Protocol)
	if hits.Load() != 0 {
		t.Fatal("cross-origin request escaped")
	}
	if strings.Contains(err.Error(), "fixture-secret") {
		t.Fatal("secret in redirect diagnostic")
	}
}

func TestResolvePaginationAndEscaping(t *testing.T) {
	name := "Bob's VM"
	var pages []string
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/clustermgmt/") {
			writeJSON(w, map[string]any{"data": map[string]string{"extId": testCluster, "name": "cluster-a"}})
			return
		}
		if r.URL.Path == vmsPath+"/"+testVM {
			writeJSON(w, map[string]any{"data": map[string]any{"extId": testVM, "name": name, "cluster": map[string]string{"extId": testCluster}}})
			return
		}
		if r.URL.Query().Get("$filter") != "name eq 'Bob''s VM'" {
			t.Error("filter escaping:", r.URL.RawQuery)
		}
		p := r.URL.Query().Get("$page")
		pages = append(pages, p)
		id := testVM
		if p == "1" {
			id = "00000000-0000-4000-8000-000000000004"
		}
		writeJSON(w, map[string]any{"data": []any{map[string]any{"extId": id, "name": name, "cluster": map[string]string{"extId": testCluster}}}, "metadata": map[string]int{"totalAvailableResults": 2}})
	}))
	defer s.Close()
	c := clientFor(t, s)
	vms, err := c.Resolve(context.Background(), name, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(vms) != 2 || strings.Join(pages, ",") != "0,1" || vms[1].ClusterName != "cluster-a" {
		t.Fatalf("%+v pages %v", vms, pages)
	}
	pages = nil
	vms, err = c.Resolve(context.Background(), "", testVM, "")
	if err != nil || len(vms) != 1 || len(pages) != 0 {
		t.Fatalf("UUID lookup: %v %v", vms, err)
	}
	_, err = c.Resolve(context.Background(), name, "", "other-cluster")
	kind(t, err, fault.Lookup)
}

func TestResolveEmptyAndRepeatedPage(t *testing.T) {
	for _, repeat := range []bool{false, true} {
		t.Run(fmt.Sprint(repeat), func(t *testing.T) {
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data := []any{}
				total := 0
				if repeat {
					data = append(data, map[string]any{"extId": testVM, "name": "vm"})
					total = 2
				}
				writeJSON(w, map[string]any{"data": data, "metadata": map[string]int{"totalAvailableResults": total}})
			}))
			defer s.Close()
			_, err := clientFor(t, s).Resolve(context.Background(), "vm", "", "")
			if repeat {
				kind(t, err, fault.Protocol)
			} else {
				kind(t, err, fault.Lookup)
			}
		})
	}
}

func TestResolveMissingDataEmptyContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
		want fault.Kind
	}{
		{"observed empty collection", fixture(t, "list-vms.empty.observed.json"), fault.Lookup},
		{"missing count", []byte(`{"metadata":{}}`), fault.Protocol},
		{"nonempty count", []byte(`{"metadata":{"totalAvailableResults":1}}`), fault.Protocol},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(tc.body)
			}))
			defer s.Close()
			_, err := clientFor(t, s).Resolve(context.Background(), "missing-vm", "", "")
			kind(t, err, tc.want)
		})
	}
}

func TestConsoleTasks(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		want         fault.Kind
	}{
		{"success", "task.succeeded.json", ""},
		{"failure", "task-failed.synthetic.json", fault.Task},
		{"live asynchronous permission failure", "task.permission-denied.observed.json", fault.Permission},
		{"malformed", `{"data":{"status":"SUCCEEDED","completionDetails":[{"name":"VmConsoleToken","value":1}]}}`, fault.Protocol},
		{"unknown", `{"data":{"status":"$UNKNOWN"}}`, fault.Protocol},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var posts int
			var ids []string
			var polls int
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					ids = append(ids, r.Header.Get("NTNX-Request-Id"))
					if r.ContentLength > 0 {
						t.Error("unexpected body")
					}
					if posts == 1 {
						w.WriteHeader(503)
						return
					}
					w.WriteHeader(202)
					w.Write(fixture(t, "generate-console-token.accepted.json"))
					return
				}
				if r.URL.Path != "/api/prism/v4.4/config/tasks/"+testTask {
					t.Error("wrong task path", r.URL.Path)
				}
				polls++
				if polls == 1 {
					writeJSON(w, map[string]any{"data": map[string]string{"status": "RUNNING"}})
					return
				}
				if strings.HasSuffix(tc.result, ".json") {
					w.Write(fixture(t, tc.result))
				} else {
					fmt.Fprint(w, tc.result)
				}
			}))
			defer s.Close()
			session, err := clientFor(t, s).Console(context.Background(), testVM)
			if tc.want == "" {
				if err != nil || session == nil {
					t.Fatal(err)
				}
			} else {
				kind(t, err, tc.want)
			}
			if posts != 2 || len(ids) != 2 || ids[0] != ids[1] || !uuidPattern.MatchString(ids[0]) {
				t.Fatal("retry did not preserve request UUID", ids)
			}
			if polls != 2 {
				t.Fatal("incorrect polling", polls)
			}
		})
	}
}

func TestConsolePermissionTimeoutCancellation(t *testing.T) {
	for _, mode := range []string{"permission", "timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "permission" {
					w.WriteHeader(403)
					w.Write(fixture(t, "permission-denied.synthetic.json"))
					return
				}
				if r.Method == "POST" {
					w.WriteHeader(202)
					w.Write(fixture(t, "generate-console-token.accepted.json"))
					return
				}
				writeJSON(w, map[string]any{"data": map[string]string{"status": "RUNNING"}})
			}))
			defer s.Close()
			c := clientFor(t, s)
			c.setupTimeout = 30 * time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			_, err := c.Console(ctx, testVM)
			switch mode {
			case "permission":
				kind(t, err, fault.Permission)
			case "timeout":
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal(err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestConsoleDestinations(t *testing.T) {
	c, err := New(Options{BaseURL: "https://pc.example:9440"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, uri := range []string{"console/launch/" + testVM, "/console/launch/" + testVM, "wss://pc.example:9440/console/launch/" + testVM} {
		u, e := c.consoleURL(uri, "secret+&")
		if e != nil {
			t.Fatal(e)
		}
		parsed, _ := url.Parse(u)
		if parsed.Query().Get("VmConsoleToken") != "secret+&" {
			t.Fatal("query not escaped")
		}
	}
	for _, uri := range []string{"wss://evil/console/launch/" + testVM, "ws://pc.example:9440/console/launch/" + testVM, "https://pc.example:9440/console/launch/" + testVM, "//pc.example:443/console/launch/" + testVM, "/console/launch/" + testVM + "?x=1", "/console/launch/" + testVM + "#fragment", "/other", "wss://user@pc.example:9440/console/launch/" + testVM} {
		if _, e := c.consoleURL(uri, "secret"); e == nil {
			t.Fatal("accepted unexpected destination", uri)
		}
	}
}

func TestWSSCookieBinaryAndExpiration(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(fmt.Sprint(expire), func(t *testing.T) {
			closed := make(chan struct{})
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if expire {
					http.Error(w, "fixture-secret", 403)
					return
				}
				cookie, err := r.Cookie("NTNX_SESSION_ID")
				if err != nil || cookie.Value != "fixture-cookie" || r.URL.Query().Get("VmConsoleToken") != "fixture-token" {
					t.Error("missing console authorization")
					w.WriteHeader(401)
					return
				}
				if r.Header.Get("Origin") != "https://"+r.Host {
					t.Error("wrong upstream origin")
				}
				ws, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer ws.CloseNow()
				defer close(closed)
				ws.Write(r.Context(), websocket.MessageBinary, []byte("RFB 003.008\n"))
				typ, b, e := ws.Read(r.Context())
				if e == nil {
					ws.Write(r.Context(), typ, b)
				}
			}))
			defer s.Close()
			c := clientFor(t, s)
			c.http.Jar.SetCookies(c.base, []*http.Cookie{{Name: "NTNX_SESSION_ID", Value: "fixture-cookie", Path: "/", Secure: true}})
			u, err := c.consoleURL("/console/launch/"+testVM, "fixture-token")
			if err != nil {
				t.Fatal(err)
			}
			session := &Session{client: c, destination: u}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			conn, err := session.Dial(ctx)
			if expire {
				kind(t, err, fault.Connection)
				if strings.Contains(err.Error(), "fixture-secret") {
					t.Fatal("error body leaked")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer conn.CloseNow()
			typ, b, err := conn.Read(ctx)
			if err != nil || typ != websocket.MessageBinary || string(b) != "RFB 003.008\n" {
				t.Fatalf("%s %v", b, err)
			}
			if err := conn.Write(ctx, websocket.MessageBinary, []byte{0, 1, 2}); err != nil {
				t.Fatal(err)
			}
			_, b, err = conn.Read(ctx)
			if err != nil || string(b) != string([]byte{0, 1, 2}) {
				t.Fatalf("echo %v %v", b, err)
			}
			conn.CloseNow()
			select {
			case <-closed:
			case <-ctx.Done():
				t.Fatal("socket did not close")
			}
		})
	}
}
