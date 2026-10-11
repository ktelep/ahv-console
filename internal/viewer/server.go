package viewer

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"ahv-console/internal/fault"
	"ahv-console/internal/prism"
	"github.com/coder/websocket"
)

type Dialer interface {
	Dial(context.Context) (*websocket.Conn, error)
}
type Server struct {
	listener                        net.Listener
	http                            *http.Server
	origin, host, bootstrap, cookie string
	cookieName                      string
	dialer                          Dialer
	ctx                             context.Context
	cancel                          context.CancelFunc
	mu                              sync.Mutex
	used                            bool
	active                          bool
	lastError                       string
	sockets                         map[*websocket.Conn]bool
	wg                              sync.WaitGroup
}

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", fault.New(fault.Connection, "secure random generation failed.")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func Start(ctx context.Context, d Dialer) (*Server, error) {
	boot, e := randomSecret()
	if e != nil {
		return nil, e
	}
	cookie, e := randomSecret()
	if e != nil {
		return nil, e
	}
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return nil, fault.New(fault.Connection, "cannot bind the local viewer to loopback.")
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Server{listener: ln, host: ln.Addr().String(), bootstrap: boot, cookie: cookie, dialer: d, ctx: ctx, cancel: cancel, sockets: map[*websocket.Conn]bool{}}
	s.origin = "http://" + s.host
	// Cookies are scoped by host, not TCP port. A unique name keeps two local
	// invocations from overwriting each other's browser authorization.
	s.cookieName = "ahv_session_" + boot[:16]
	s.http = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0)}
	go s.http.Serve(ln)
	go func() { <-ctx.Done(); s.Close() }()
	return s, nil
}
func (s *Server) URL() string { return s.origin + "/#" + s.bootstrap }
func (s *Server) Close() {
	s.cancel()
	s.mu.Lock()
	for ws := range s.sockets {
		ws.CloseNow()
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.http.Shutdown(ctx)
	s.http.Close()
	s.wg.Wait()
}
func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func (s *Server) authorized(r *http.Request) bool {
	c, e := r.Cookie(s.cookieName)
	return e == nil && equal(c.Value, s.cookie)
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	if r.Host != s.host {
		http.Error(w, "Unexpected Host", 403)
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "Unexpected query", 400)
		return
	}
	switch r.URL.Path {
	case "/bootstrap":
		if r.Method != "POST" || r.Header.Get("Origin") != s.origin {
			http.Error(w, "Forbidden", 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var body struct {
			Secret string `json:"secret"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "Invalid bootstrap", 400)
			return
		}
		s.mu.Lock()
		valid := !s.used && equal(body.Secret, s.bootstrap)
		if valid {
			s.used = true
		}
		s.mu.Unlock()
		if !valid {
			http.Error(w, "Invalid or used bootstrap", 403)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: s.cookieName, Value: s.cookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		w.WriteHeader(204)
	case "/session":
		if r.Method != "GET" || !s.authorized(r) || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.origin) {
			http.Error(w, "Forbidden", 403)
			return
		}
		s.mu.Lock()
		message := s.lastError
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": message})
	case "/ws":
		if r.Method != "GET" || r.Header.Get("Origin") != s.origin || !s.authorized(r) {
			http.Error(w, "Forbidden", 403)
			return
		}
		s.mu.Lock()
		if s.active {
			s.mu.Unlock()
			http.Error(w, "Console already connected", 409)
			return
		}
		s.active = true
		s.wg.Add(1)
		s.mu.Unlock()
		defer func() { s.mu.Lock(); s.active = false; s.mu.Unlock(); s.wg.Done() }()
		s.proxy(w, r)
	default:
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		http.FileServer(http.FS(assets())).ServeHTTP(w, r)
	}
}
func (s *Server) track(ws *websocket.Conn) {
	s.mu.Lock()
	s.sockets[ws] = true
	if s.ctx.Err() != nil {
		ws.CloseNow()
	}
	s.mu.Unlock()
}
func (s *Server) untrack(ws *websocket.Conn) {
	ws.CloseNow()
	s.mu.Lock()
	delete(s.sockets, ws)
	s.mu.Unlock()
}
func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	local, e := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if e != nil {
		return
	}
	s.track(local)
	defer s.untrack(local)
	local.SetReadLimit(prism.MaxFrame)
	upstream, e := s.dialer.Dial(ctx)
	if e != nil {
		message, _ := fault.Diagnostic(e)
		s.mu.Lock()
		s.lastError = message
		s.mu.Unlock()
		local.Close(websocket.StatusPolicyViolation, "Console unavailable; rerun the command.")
		return
	}
	s.track(upstream)
	defer s.untrack(upstream)
	s.mu.Lock()
	s.lastError = ""
	s.mu.Unlock()
	done := make(chan error, 2)
	copyFrames := func(dst, src *websocket.Conn) {
		for {
			typ, b, e := src.Read(ctx)
			if e != nil {
				done <- e
				return
			}
			if typ != websocket.MessageBinary {
				done <- fault.New(fault.Protocol, "Console returned nonbinary data.")
				return
			}
			writeCtx, stop := context.WithTimeout(ctx, 30*time.Second)
			e = dst.Write(writeCtx, websocket.MessageBinary, b)
			stop()
			if e != nil {
				done <- e
				return
			}
		}
	}
	go copyFrames(local, upstream)
	go copyFrames(upstream, local)
	<-done
	cancel()
	local.CloseNow()
	upstream.CloseNow()
	<-done
}
