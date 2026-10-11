package cli

import (
	"bytes"
	"strings"
	"testing"

	"ahv-console/internal/prism"
)

type answerPrompt string

func (p answerPrompt) Ask(string, bool) (string, error) { return string(p), nil }

func TestDuplicateSelection(t *testing.T) {
	vms := []prism.VM{{ID: "id-a", Name: "same", ClusterName: "cluster-a"}, {ID: "id-b", Name: "same", ClusterName: "cluster-b"}}
	var out bytes.Buffer
	if _, err := Select(vms, nil, &out); err == nil {
		t.Fatal("unattended ambiguity accepted")
	}
	for _, s := range []string{"cluster-a", "cluster-b", "id-a", "id-b"} {
		if !strings.Contains(out.String(), s) {
			t.Fatal("missing candidate detail", s)
		}
	}
	for _, s := range []string{"", "0", "3", "-1", "+1", "bad", "1.0"} {
		if _, err := Select(vms, answerPrompt(s), &out); err == nil {
			t.Fatal("invalid selection accepted", s)
		}
	}
	vm, err := Select(vms, answerPrompt("2"), &out)
	if err != nil || vm.ID != "id-b" {
		t.Fatal(vm, err)
	}
}
