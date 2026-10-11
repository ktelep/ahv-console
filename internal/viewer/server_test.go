package viewer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type testDialer struct {
	url    string
	closed chan struct{}
}

func (d testDialer) Dial(ctx context.Context) (*websocket.Conn, error) {
	c, _, e := websocket.Dial(ctx, d.url, nil)
	return c, e
}

func bootstrap(t *testing.T, s *Server) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	body, _ := json.Marshal(map[string]string{"secret": s.bootstrap})
	req, _ := http.NewRequest("POST", s.origin+"/bootstrap", bytes.NewReader(body))
	req.Header.Set("Origin", s.origin)
	r, e := c.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 204 {
		t.Fatal(r.StatusCode)
	}
	return c
}

func TestProtectionAndAssets(t *testing.T) {
	s, e := Start(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if !strings.HasPrefix(s.host, "127.0.0.1:") {
		t.Fatal(s.host)
	}
	for _, tc := range []struct {
		path, method, origin, host string
		want                       int
	}{
		{"/session", "GET", "", "", 403}, {"/ws", "GET", s.origin, "", 403}, {"/bootstrap", "POST", "https://evil", "", 403}, {"/bootstrap", "POST", s.origin, "evil", 403}, {"/ws?upstream=wss://evil", "GET", s.origin, "", 400},
	} {
		req, _ := http.NewRequest(tc.method, s.origin+tc.path, strings.NewReader(`{"secret":"`+s.bootstrap+`"}`))
		req.Header.Set("Origin", tc.origin)
		if tc.host != "" {
			req.Host = tc.host
		}
		r, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
		if r.StatusCode != tc.want {
			t.Errorf("%s: %d", tc.path, r.StatusCode)
		}
	}
	c := bootstrap(t, s)
	wrongOrigin, _ := http.NewRequest("GET", s.origin+"/ws", nil)
	wrongOrigin.Header.Set("Origin", "https://unrelated.example")
	denied, err := c.Do(wrongOrigin)
	if err != nil {
		t.Fatal(err)
	}
	denied.Body.Close()
	if denied.StatusCode != 403 {
		t.Fatal("authorized cookie bypassed Origin check")
	}
	req, _ := http.NewRequest("POST", s.origin+"/bootstrap", strings.NewReader(`{"secret":"`+s.bootstrap+`"}`))
	req.Header.Set("Origin", s.origin)
	r, e := c.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal("reused bootstrap accepted")
	}
	r, e = c.Get(s.origin + "/session")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || strings.Contains(string(b), s.bootstrap) || strings.Contains(string(b), s.cookie) {
		t.Fatal("session leak")
	}
	for _, path := range []string{"/", "/app.js", "/novnc/core/rfb.js", "/novnc/vendor/pako/lib/zlib/inflate.js", "/novnc/SOURCE.tar.gz"} {
		r, e := c.Get(s.origin + path)
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Error("missing asset", path, r.StatusCode)
		}
	}
}

func TestProxyAndCleanup(t *testing.T) {
	closed := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := websocket.Accept(w, r, nil)
		if e != nil {
			return
		}
		defer ws.CloseNow()
		defer close(closed)
		for {
			typ, b, e := ws.Read(r.Context())
			if e != nil {
				return
			}
			if ws.Write(r.Context(), typ, b) != nil {
				return
			}
		}
	}))
	defer up.Close()
	s, e := Start(context.Background(), testDialer{url: "ws" + strings.TrimPrefix(up.URL, "http")})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c := bootstrap(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, e := websocket.Dial(ctx, "ws://"+s.host+"/ws", &websocket.DialOptions{HTTPClient: c, HTTPHeader: http.Header{"Origin": {s.origin}}})
	if e != nil {
		t.Fatal(e)
	}
	if e := conn.Write(ctx, websocket.MessageBinary, []byte{0, 1, 2, 255}); e != nil {
		t.Fatal(e)
	}
	typ, b, e := conn.Read(ctx)
	if e != nil || typ != websocket.MessageBinary || !bytes.Equal(b, []byte{0, 1, 2, 255}) {
		t.Fatal(b, e)
	}
	s.Close()
	conn.CloseNow()
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("upstream not released")
	}
	if _, e := c.Get(s.origin); e == nil {
		t.Fatal("listener remains open")
	}
}

func TestIndependentSessions(t *testing.T) {
	a, _ := Start(context.Background(), nil)
	defer a.Close()
	b, _ := Start(context.Background(), nil)
	defer b.Close()
	if a.host == b.host || a.cookie == b.cookie || a.bootstrap == b.bootstrap {
		t.Fatal("session collision")
	}
	c := bootstrap(t, a)
	u, _ := url.Parse(b.origin)
	c.Jar.SetCookies(u, []*http.Cookie{{Name: b.cookieName, Value: a.cookie}})
	r, e := c.Get(b.origin + "/session")
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal("cross-session access")
	}
	// One browser cookie jar must retain both sessions despite sharing 127.0.0.1.
	body, _ := json.Marshal(map[string]string{"secret": b.bootstrap})
	req, _ := http.NewRequest("POST", b.origin+"/bootstrap", bytes.NewReader(body))
	req.Header.Set("Origin", b.origin)
	r, e = c.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 204 {
		t.Fatal(r.StatusCode)
	}
	for _, origin := range []string{a.origin, b.origin} {
		r, e := c.Get(origin + "/session")
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatal("browser cookie collision", r.StatusCode)
		}
	}
}
