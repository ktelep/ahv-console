package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"ahv-console/internal/fault"
	"ahv-console/internal/prism"
)

func Select(vms []prism.VM, p Prompter, out io.Writer) (prism.VM, error) {
	if len(vms) == 1 {
		return vms[0], nil
	}
	if len(vms) == 0 {
		return prism.VM{}, fault.New(fault.Lookup, "no accessible VM matched the selection.")
	}
	fmt.Fprintln(out, "Multiple VMs match. Select explicitly:")
	for i, vm := range vms {
		fmt.Fprintf(out, "  %d. %q  cluster=%q  UUID=%s\n", i+1, vm.Name, vm.ClusterName, vm.ID)
	}
	if p == nil {
		return prism.VM{}, fault.New(fault.Lookup, "ambiguous VM name; use --cluster or --vm-id to select a candidate.")
	}
	text, err := p.Ask("Candidate number", false)
	if err != nil {
		return prism.VM{}, err
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > len(vms) || strings.Trim(text, "0123456789") != "" {
		return prism.VM{}, fault.New(fault.Lookup, "invalid candidate number; rerun and select a listed candidate.")
	}
	return vms[n-1], nil
}
