// Package cli owns foreground command execution and terminal interaction.
package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"

	"ahv-console/internal/control"
	"ahv-console/internal/fault"
	"ahv-console/internal/platform"
	"ahv-console/internal/prism"
	"ahv-console/internal/viewer"
)

var Version = "dev"

const Help = `Usage: ahv-console [options] <exact-vm-name>
       ahv-console [options] --vm-id <UUID>

Options:
  --pc-host HOST       PC IP/hostname (AHV_CONSOLE_PC_HOST)
  --pc-port PORT       Default 9440 (AHV_CONSOLE_PC_PORT)
  --username USER      PC user (AHV_CONSOLE_USERNAME)
  --ca-cert FILE       Add PEM CA trust (AHV_CONSOLE_CA_CERT)
  --cluster NAME       Narrow exact-name lookup by cluster
  --vm-id UUID         Select directly instead of a name
  --insecure           Disable upstream TLS verification, with a warning
  --non-interactive    Never prompt
  --no-browser         Print URL without launching a browser
  --help, -h           Show this help offline
  --version            Show build version offline
  --licenses           Show embedded notices offline

Password: AHV_CONSOLE_PASSWORD, or a hidden interactive prompt.
No password flag is supported. Empty environment values count as missing.
Keep the command running; Ctrl+C closes the viewer and console sockets.
`

func Main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := Run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	if err != nil {
		message, code := fault.Diagnostic(err)
		fmt.Fprintln(os.Stderr, message)
		os.Exit(code)
	}
}

func Run(ctx context.Context, args []string, env func(string) string, out, errOut io.Writer) error {
	return run(ctx, args, env, out, errOut, platform.Open)
}

func run(ctx context.Context, args []string, env func(string) string, out, errOut io.Writer, launch func(string) error) error {
	c, err := Parse(args, env)
	if err != nil {
		return err
	}
	switch {
	case c.Help:
		fmt.Fprint(out, Help)
		return nil
	case c.Version:
		fmt.Fprintln(out, "ahv-console", Version)
		return nil
	case c.Licenses:
		fmt.Fprint(out, viewer.Licenses())
		return nil
	}
	var p Prompter
	terminal := &Terminal{Context: ctx, In: os.Stdin, Out: os.Stderr}
	if !c.NonInteractive && terminal.Available() {
		p = terminal
	}
	if err := c.Complete(p); err != nil {
		return err
	}
	if c.Insecure {
		fmt.Fprintln(errOut, "WARNING: upstream PC TLS certificate verification is disabled (--insecure).")
	}
	host := strings.TrimSuffix(strings.TrimPrefix(c.Host, "["), "]")
	pc, err := prism.New(prism.Options{BaseURL: "https://" + net.JoinHostPort(host, c.Port), Username: c.Username, Password: c.Password, CACert: c.CACert, Insecure: c.Insecure})
	if err != nil {
		return err
	}
	defer pc.Close()
	setupCtx, setupCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer setupCancel()
	candidates, err := pc.Resolve(setupCtx, c.VMName, c.VMID, c.Cluster)
	if err != nil {
		return err
	}
	vm, err := Select(candidates, p, out)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Opening %q (cluster %q, UUID %s)\n", vm.Name, vm.ClusterName, vm.ID)
	setupCancel()
	controller := control.New(ctx, pc, vm, 2*time.Minute)
	defer controller.Close()
	server, err := viewer.Start(ctx, controller)
	if err != nil {
		return err
	}
	defer server.Close()
	fmt.Fprintln(out, "Console URL:", server.URL())
	fmt.Fprintln(out, "Keep this command running. Press Ctrl+C to close the session.")
	if !c.NoBrowser {
		go func() {
			if launch(server.URL()) != nil {
				fmt.Fprintln(errOut, "Browser launch failed. Open the printed Console URL manually; the viewer remains available.")
			}
		}()
	}
	<-ctx.Done()
	return ctx.Err()
}
