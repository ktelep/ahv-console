package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestOfflineInformationAndUnattended(t *testing.T) {
	for _, flag := range []string{"--help", "--version", "--licenses"} {
		var out, errs bytes.Buffer
		e := Run(context.Background(), []string{flag}, func(string) string { return "" }, &out, &errs)
		if e != nil || out.Len() == 0 || errs.Len() != 0 {
			t.Fatal(flag, e)
		}
	}
	var out, errs bytes.Buffer
	e := Run(context.Background(), []string{"vm", "--non-interactive"}, func(k string) string {
		if k == "AHV_CONSOLE_PC_HOST" {
			return "pc"
		}
		if k == "AHV_CONSOLE_USERNAME" {
			return "user"
		}
		return ""
	}, &out, &errs)
	if e == nil || !strings.Contains(e.Error(), "AHV_CONSOLE_PASSWORD") || out.Len() != 0 || errs.Len() != 0 {
		t.Fatal("unattended invocation did not fail without prompts", e)
	}
}
