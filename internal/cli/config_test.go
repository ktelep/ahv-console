package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCAFlagPrecedence(t *testing.T) {
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, []byte("validated by client"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Parse([]string{"vm", "--ca-cert", ca}, func(k string) string {
		if k == "AHV_CONSOLE_CA_CERT" {
			return "/missing"
		}
		return ""
	})
	if err != nil || c.CACert != ca {
		t.Fatal(c, err)
	}
}

func TestConfig(t *testing.T) {
	for _, tc := range []struct {
		name               string
		args               []string
		env                map[string]string
		wantHost, wantPort string
		bad                bool
	}{
		{"defaults", []string{"vm"}, nil, "", "9440", false},
		{"empty env", []string{"vm"}, map[string]string{"AHV_CONSOLE_PC_PORT": ""}, "", "9440", false},
		{"overrides after name", []string{"vm", "--pc-host", "new", "--pc-port", "443"}, map[string]string{"AHV_CONSOLE_PC_HOST": "old", "AHV_CONSOLE_PC_PORT": "bad"}, "new", "443", false},
		{"bad env port", []string{"vm"}, map[string]string{"AHV_CONSOLE_PC_PORT": "abc"}, "", "", true},
		{"port zero", []string{"vm", "--pc-port=0"}, nil, "", "", true},
		{"port range", []string{"vm", "--pc-port=65536"}, nil, "", "", true},
		{"port sign", []string{"vm", "--pc-port=+9440"}, nil, "", "", true},
		{"explicit empty", []string{"vm", "--pc-host="}, nil, "", "", true},
		{"URL host", []string{"vm", "--pc-host=https://pc"}, nil, "", "", true},
		{"IPv6", []string{"vm", "--pc-host=::1"}, nil, "::1", "9440", false},
		{"missing selector", nil, nil, "", "", true},
		{"conflicting selectors", []string{"vm", "--vm-id=00000000-0000-4000-8000-000000000001"}, nil, "", "", true},
		{"UUID cluster", []string{"--vm-id=00000000-0000-4000-8000-000000000001", "--cluster=x"}, nil, "", "", true},
		{"UUID", []string{"--vm-id=00000000-0000-4000-8000-000000000001"}, nil, "", "9440", false},
		{"bad UUID", []string{"--vm-id=bad"}, nil, "", "", true},
		{"CA missing", []string{"vm", "--ca-cert=/nonexistent/ca.pem"}, nil, "", "", true},
		{"no password flag", []string{"vm", "--password=SECRET"}, nil, "", "", true},
		{"information bypasses config", []string{"--help"}, map[string]string{"AHV_CONSOLE_PC_PORT": "bad"}, "", "bad", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Parse(tc.args, func(k string) string { return tc.env[k] })
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
			if err != nil {
				if strings.Contains(err.Error(), "SECRET") {
					t.Fatal("secret exposed")
				}
				return
			}
			if c.Host != tc.wantHost || c.Port != tc.wantPort {
				t.Fatalf("host=%q port=%q", c.Host, c.Port)
			}
		})
	}
}

type prompt struct{ calls []string }

func (p *prompt) Ask(label string, secret bool) (string, error) {
	p.calls = append(p.calls, label)
	if label == "Password" && !secret {
		panic("password not hidden")
	}
	return "test", nil
}

func TestComplete(t *testing.T) {
	c := Config{Host: "pc", Port: "9440"}
	p := &prompt{}
	if err := c.Complete(p); err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.calls, ",") != "Username,Password" {
		t.Fatal(p.calls)
	}
	for _, noninteractive := range []bool{false, true} {
		c = Config{Host: "pc", Port: "9440", Username: "user", NonInteractive: noninteractive}
		p = &prompt{}
		var pr Prompter
		if noninteractive {
			pr = p
		}
		if err := c.Complete(pr); err == nil || !strings.Contains(err.Error(), "AHV_CONSOLE_PASSWORD") {
			t.Fatalf("expected missing password: %v", err)
		}
		if len(p.calls) != 0 {
			t.Fatal("unattended prompt")
		}
	}
}
