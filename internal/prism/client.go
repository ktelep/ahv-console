package prism

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"ahv-console/internal/fault"
	"github.com/coder/websocket"
)

const vmsPath = "/api/vmm/v4.3/ahv/config/vms"
const maxResponse = 8 << 20
const MaxFrame = 8 << 20

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var taskPattern = regexp.MustCompile(`^[a-zA-Z0-9/+]*={0,2}:[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

type Options struct {
	BaseURL, Username, Password, CACert string
	Insecure                            bool
	SetupTimeout                        time.Duration
}

type Client struct {
	base                       *url.URL
	http                       *http.Client
	username, password         string
	setupTimeout, pollInterval time.Duration
}

func New(o Options) (*Client, error) {
	u, err := url.Parse(o.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, fault.New(fault.Configuration, "PC address must be an HTTPS origin.")
	}
	u.Path = ""
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if o.CACert != "" {
		pem, err := os.ReadFile(o.CACert)
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return nil, fault.New(fault.Configuration, "--ca-cert must contain readable PEM CA certificates.")
		}
	}
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, InsecureSkipVerify: o.Insecure}, // Explicit, upstream-only opt-in.
		TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout: 60 * time.Second, MaxIdleConns: 4,
	}
	c := &Client{base: u, username: o.Username, password: o.Password, setupTimeout: o.SetupTimeout, pollInterval: 250 * time.Millisecond}
	if c.setupTimeout <= 0 {
		c.setupTimeout = 2 * time.Minute
	}
	c.http = &http.Client{Transport: transport, Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || !sameOrigin(req.URL, u) || req.URL.User != nil {
			return fault.New(fault.Protocol, "PC redirected authentication to an unexpected destination; redirect refused.")
		}
		if len(via) >= 3 {
			return fault.New(fault.Protocol, "PC returned too many redirects.")
		}
		return nil
	}}
	return c, nil
}

func sameOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if u.Port() != "" {
			return u.Port()
		}
		return "443"
	}
	return strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

func transportError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var known *fault.Error
	if errors.As(err, &known) {
		return known
	}
	var cert *tls.CertificateVerificationError
	if errors.As(err, &cert) {
		return fault.New(fault.TLS, "PC certificate verification failed; configure --ca-cert (or explicitly opt into --insecure for a trusted test endpoint).")
	}
	return fault.New(fault.Connection, "could not reach PC; check the host, port, TLS configuration, and network connectivity.")
}

func responseError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return fault.New(fault.Authentication, "PC rejected credentials or the session; check username/password and rerun.")
	case http.StatusForbidden:
		return fault.New(fault.Permission, "PC denied access; verify VM visibility, task-read access, and Generate VM Console Token permission.")
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return fault.New(fault.Protocol, "required PC 7.6 API operation is unavailable; check API compatibility and access.")
	default:
		return fault.New(fault.Connection, fmt.Sprintf("PC returned HTTP %d; check PC availability and access.", status))
	}
}

func (c *Client) request(ctx context.Context, method, path string, query url.Values, requestID string, out any) (int, error) {
	u := *c.base
	// path is assembled only from fixed API paths and validated identifiers.
	u.Path, _ = url.PathUnescape(path)
	u.RawPath = path
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return 0, fault.New(fault.Protocol, "could not construct PC request.")
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	if requestID != "" {
		req.Header.Set("NTNX-Request-Id", requestID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, transportError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, responseError(resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return resp.StatusCode, transportError(ctx, err)
	}
	if len(b) > maxResponse || json.Unmarshal(b, out) != nil {
		return resp.StatusCode, fault.New(fault.Protocol, "PC returned an oversized or malformed JSON response.")
	}
	return resp.StatusCode, nil
}

type VM struct {
	ID      string `json:"extId"`
	Name    string `json:"name"`
	Cluster struct {
		ID string `json:"extId"`
	} `json:"cluster"`
	ClusterName string `json:"-"`
}

func (c *Client) Resolve(ctx context.Context, name, id, cluster string) ([]VM, error) {
	ctx, cancel := context.WithTimeout(ctx, c.setupTimeout)
	defer cancel()
	var candidates []VM
	if id != "" {
		if !uuidPattern.MatchString(id) {
			return nil, fault.New(fault.Configuration, "VM identifier must be a UUID.")
		}
		var r struct {
			Data VM `json:"data"`
		}
		status, err := c.request(ctx, "GET", vmsPath+"/"+id, nil, "", &r)
		if status == 404 {
			return nil, fault.New(fault.Lookup, "no accessible VM matches the requested UUID.")
		}
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(r.Data.ID, id) {
			return nil, fault.New(fault.Protocol, "PC returned a different or missing VM identifier.")
		}
		candidates = []VM{r.Data}
	} else {
		filter := "name eq '" + strings.ReplaceAll(name, "'", "''") + "'"
		seen := map[string]bool{}
		for page := 0; ; page++ {
			var r struct {
				Data     []VM `json:"data"`
				Metadata struct {
					Total *int `json:"totalAvailableResults"`
				} `json:"metadata"`
			}
			q := url.Values{"$filter": {filter}, "$limit": {"100"}, "$page": {fmt.Sprint(page)}}
			_, err := c.request(ctx, "GET", vmsPath, q, "", &r)
			if err != nil {
				return nil, err
			}
			// PC 7.6 omits data entirely for an empty filtered collection.
			// An explicit zero total distinguishes that from a malformed list.
			if r.Data == nil && (r.Metadata.Total == nil || *r.Metadata.Total != 0) {
				return nil, fault.New(fault.Protocol, "PC VM list is missing its data array.")
			}
			for _, vm := range r.Data {
				if !uuidPattern.MatchString(vm.ID) {
					return nil, fault.New(fault.Protocol, "PC VM list contains an invalid identifier.")
				}
				if seen[vm.ID] {
					return nil, fault.New(fault.Protocol, "PC pagination repeated a VM; inventory may have changed. Rerun the command.")
				}
				seen[vm.ID] = true
				if vm.Name == name {
					candidates = append(candidates, vm)
				}
			}
			if r.Metadata.Total != nil {
				if *r.Metadata.Total < 0 || (len(r.Data) == 0 && len(seen) < *r.Metadata.Total) {
					return nil, fault.New(fault.Protocol, "PC pagination ended before the reported total.")
				}
				if len(seen) >= *r.Metadata.Total {
					break
				}
			} else if len(r.Data) == 0 {
				break
			}
			if page >= 10000 {
				return nil, fault.New(fault.Protocol, "PC inventory pagination exceeded its limit; narrow the selection with --vm-id.")
			}
		}
	}
	clusterNames := map[string]string{}
	var result []VM
	for _, vm := range candidates {
		if !uuidPattern.MatchString(vm.Cluster.ID) {
			return nil, fault.New(fault.Protocol, "VM is missing a valid cluster identifier.")
		}
		if _, ok := clusterNames[vm.Cluster.ID]; !ok {
			var r struct {
				Data struct {
					ID   string `json:"extId"`
					Name string `json:"name"`
				} `json:"data"`
			}
			_, err := c.request(ctx, "GET", "/api/clustermgmt/v4.3/config/clusters/"+vm.Cluster.ID, nil, "", &r)
			if err != nil {
				return nil, err
			}
			if r.Data.ID != vm.Cluster.ID || r.Data.Name == "" {
				return nil, fault.New(fault.Protocol, "PC returned malformed cluster details.")
			}
			clusterNames[vm.Cluster.ID] = r.Data.Name
		}
		vm.ClusterName = clusterNames[vm.Cluster.ID]
		if cluster == "" || vm.ClusterName == cluster {
			result = append(result, vm)
		}
	}
	if len(result) == 0 {
		return nil, fault.New(fault.Lookup, "no accessible VM matches the exact name and cluster selection.")
	}
	return result, nil
}

func requestUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fault.New(fault.Connection, "secure random generation failed.")
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Session holds upstream authorization only in memory. Its fields are private
// so callers cannot accidentally serialize a console URL to the frontend.
type Session struct {
	client      *Client
	destination string
}

func (c *Client) Console(ctx context.Context, vmID string) (*Session, error) {
	if !uuidPattern.MatchString(vmID) {
		return nil, fault.New(fault.Configuration, "VM identifier must be a UUID.")
	}
	ctx, cancel := context.WithTimeout(ctx, c.setupTimeout)
	defer cancel()
	id, err := requestUUID()
	if err != nil {
		return nil, err
	}
	var r struct {
		Data struct {
			ID string `json:"extId"`
		} `json:"data"`
	}
	for attempt := 0; ; attempt++ {
		status, e := c.request(ctx, "POST", vmsPath+"/"+vmID+"/$actions/generate-console-token", nil, id, &r)
		if e == nil {
			if status != 202 {
				return nil, fault.New(fault.Protocol, "PC did not accept console generation as an asynchronous task.")
			}
			break
		}
		var f *fault.Error
		retryable := status == 502 || status == 503 || status == 504 || (status == 0 && errors.As(e, &f) && f.Kind == fault.Connection)
		if attempt >= 2 || !retryable {
			return nil, e
		}
		if err := wait(ctx, time.Duration(attempt+1)*c.pollInterval); err != nil {
			return nil, err
		}
	}
	if !taskPattern.MatchString(r.Data.ID) {
		return nil, fault.New(fault.Protocol, "PC returned an invalid console task reference.")
	}
	delay := c.pollInterval
	for {
		var task struct {
			Data struct {
				Status string `json:"status"`
				Errors []struct {
					Code  string `json:"code"`
					Group string `json:"errorGroup"`
				} `json:"errorMessages"`
				Details []struct {
					Name  string          `json:"name"`
					Value json.RawMessage `json:"value"`
				} `json:"completionDetails"`
			} `json:"data"`
		}
		_, err := c.request(ctx, "GET", "/api/prism/v4.4/config/tasks/"+url.PathEscape(r.Data.ID), nil, "", &task)
		if err != nil {
			return nil, err
		}
		switch task.Data.Status {
		case "SUCCEEDED":
			values := map[string]string{}
			for _, pair := range task.Data.Details {
				if pair.Name != "WsUri" && pair.Name != "VmConsoleToken" {
					continue
				}
				var s string
				if _, exists := values[pair.Name]; exists || json.Unmarshal(pair.Value, &s) != nil || s == "" {
					return nil, fault.New(fault.Protocol, "console task result contains duplicate, empty, or non-string fields.")
				}
				values[pair.Name] = s
			}
			destination, err := c.consoleURL(values["WsUri"], values["VmConsoleToken"])
			if err != nil {
				return nil, err
			}
			parsed, _ := url.Parse(destination)
			if !strings.EqualFold(strings.TrimPrefix(parsed.Path, "/console/launch/"), vmID) {
				return nil, fault.New(fault.Protocol, "console task returned a different VM destination.")
			}
			return &Session{client: c, destination: destination}, nil
		case "FAILED", "CANCELED":
			for _, message := range task.Data.Errors {
				if message.Code == "VMM-30500" || message.Group == "VM_UNAUTHORIZED_REQUEST" {
					return nil, fault.New(fault.Permission, "PC denied console-token generation; verify Generate VM Console Token permission for the selected VM.")
				}
			}
			return nil, fault.New(fault.Task, "PC console-token task failed or was canceled; inspect the task in Prism and check console permissions.")
		case "QUEUED", "RUNNING", "CANCELING", "SUSPENDED":
		default:
			return nil, fault.New(fault.Protocol, "PC returned an unknown console task status.")
		}
		if err := wait(ctx, delay); err != nil {
			return nil, err
		}
		delay = min(delay*2, 2*time.Second)
	}
}

func (c *Client) consoleURL(uri, token string) (string, error) {
	bad := func() (string, error) {
		return "", fault.New(fault.Protocol, "console result has an invalid or unexpected destination; refusing to forward authorization.")
	}
	if uri == "" || token == "" {
		return bad()
	}
	u, err := url.Parse(uri)
	if err != nil || u.User != nil || u.Fragment != "" {
		return bad()
	}
	base := *c.base
	base.Scheme = "wss"
	base.Path = "/"
	u = base.ResolveReference(u)
	if u.Scheme != "wss" || !sameOrigin(u, c.base) || !strings.HasPrefix(u.Path, "/console/launch/") {
		return bad()
	}
	vmID := strings.TrimPrefix(u.Path, "/console/launch/")
	if !uuidPattern.MatchString(vmID) || u.RawQuery != "" {
		return bad()
	}
	u.RawQuery = url.Values{"VmConsoleToken": {token}}.Encode()
	return u.String(), nil
}

func (s *Session) Dial(ctx context.Context) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	c := s.client
	if len(c.http.Jar.Cookies(c.base)) == 0 {
		return nil, fault.New(fault.Authentication, "PC did not establish a user session cookie; verify PC compatibility and credentials.")
	}
	conn, resp, err := websocket.Dial(ctx, s.destination, &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: http.Header{"Origin": {c.base.String()}}, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403) {
			return nil, fault.New(fault.Connection, "console authorization expired or was rejected; rerun the command and check console permissions.")
		}
		return nil, transportError(ctx, err)
	}
	conn.SetReadLimit(MaxFrame)
	return conn, nil
}
