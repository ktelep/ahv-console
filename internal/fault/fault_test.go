package fault

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDiagnostic(t *testing.T) {
	seen := map[int]bool{}
	for _, kind := range []Kind{Configuration, Authentication, Permission, Lookup, TLS, Task, Protocol, Connection} {
		text, code := Diagnostic(New(kind, "actionable advice"))
		if code == 0 || seen[code] || !strings.Contains(text, string(kind)) {
			t.Fatalf("%s %d", text, code)
		}
		seen[code] = true
	}
	text, _ := Diagnostic(errors.New("https://server/?VmConsoleToken=SECRET; password=SECRET; cookie=SECRET"))
	if strings.Contains(text, "SECRET") {
		t.Fatal("raw error exposed")
	}
	_, code := Diagnostic(context.Canceled)
	if code != 130 {
		t.Fatal(code)
	}
}
