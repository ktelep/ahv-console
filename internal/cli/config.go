package cli

import (
	"flag"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"ahv-console/internal/fault"
)

type Config struct {
	Host, Port, Username, Password, CACert string
	VMName, VMID, Cluster                  string
	Insecure, NonInteractive, NoBrowser    bool
	Help, Version, Licenses                bool
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

func configError(message string) error { return fault.New(fault.Configuration, message) }

// Parse permits flags before or after the positional name. Explicit empty flags
// are errors; empty environment values are missing and may be prompted for.
func Parse(args []string, env func(string) string) (Config, error) {
	c := Config{Host: env("AHV_CONSOLE_PC_HOST"), Port: env("AHV_CONSOLE_PC_PORT"), Username: env("AHV_CONSOLE_USERNAME"), Password: env("AHV_CONSOLE_PASSWORD"), CACert: env("AHV_CONSOLE_CA_CERT")}
	if c.Port == "" {
		c.Port = "9440"
	}
	f := flag.NewFlagSet("ahv-console", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&c.Host, "pc-host", c.Host, "")
	f.StringVar(&c.Port, "pc-port", c.Port, "")
	f.StringVar(&c.Username, "username", c.Username, "")
	f.StringVar(&c.CACert, "ca-cert", c.CACert, "")
	f.StringVar(&c.VMID, "vm-id", "", "")
	f.StringVar(&c.Cluster, "cluster", "", "")
	f.BoolVar(&c.Insecure, "insecure", false, "")
	f.BoolVar(&c.NonInteractive, "non-interactive", false, "")
	f.BoolVar(&c.NoBrowser, "no-browser", false, "")
	f.BoolVar(&c.Help, "help", false, "")
	f.BoolVar(&c.Help, "h", false, "")
	f.BoolVar(&c.Version, "version", false, "")
	f.BoolVar(&c.Licenses, "licenses", false, "")
	var flags, names []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			names = append(names, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			names = append(names, a)
			continue
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		ff := f.Lookup(name)
		if ff == nil {
			return c, configError("unknown flag; use --help for supported options (there is no password flag).")
		}
		flags = append(flags, a)
		_, isBool := ff.Value.(interface{ IsBoolFlag() bool })
		if !isBool && !hasValue {
			i++
			if i == len(args) {
				return c, configError("a flag is missing its value; use --help.")
			}
			flags = append(flags, args[i])
		}
	}
	if err := f.Parse(flags); err != nil {
		return c, configError("invalid flag value; use --help.")
	}
	if c.Help || c.Version || c.Licenses {
		return c, nil
	}
	var empty bool
	f.Visit(func(ff *flag.Flag) {
		if ff.Value.String() == "" {
			empty = true
		}
	})
	if empty {
		return c, configError("explicit flags must not be empty.")
	}
	if len(names) > 1 {
		return c, configError("supply one exact VM name, or --vm-id instead.")
	}
	if len(names) == 1 {
		c.VMName = names[0]
	}
	if (c.VMName == "") == (c.VMID == "") {
		return c, configError("supply exactly one VM name or --vm-id.")
	}
	if c.VMID != "" && !uuidPattern.MatchString(c.VMID) {
		return c, configError("--vm-id must be a UUID.")
	}
	if c.VMID != "" && c.Cluster != "" {
		return c, configError("--cluster narrows name lookup and cannot be combined with --vm-id.")
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 || strings.Trim(c.Port, "0123456789") != "" {
		return configError("PC port must be a number from 1 to 65535 (--pc-port).")
	}
	if c.Host != "" {
		h := strings.TrimSuffix(strings.TrimPrefix(c.Host, "["), "]")
		if net.ParseIP(h) == nil {
			if len(c.Host) > 253 {
				return configError("--pc-host must be an IP address or hostname, without a URL or port.")
			}
			for _, label := range strings.Split(strings.TrimSuffix(c.Host, "."), ".") {
				if len(label) > 63 || !hostnamePattern.MatchString(label) {
					return configError("--pc-host must be an IP address or hostname, without a URL or port.")
				}
			}
		}
	}
	if strings.ContainsAny(c.Username, "\r\n:") {
		return configError("username must not contain a colon or line break.")
	}
	if c.CACert != "" {
		info, err := os.Stat(c.CACert)
		if err != nil || !info.Mode().IsRegular() {
			return configError("--ca-cert must name a readable PEM certificate file.")
		}
	}
	return nil
}

type Prompter interface {
	Ask(label string, secret bool) (string, error)
}

func (c *Config) Complete(p Prompter) error {
	fields := []struct {
		label, env string
		value      *string
		secret     bool
	}{
		{"PC host", "AHV_CONSOLE_PC_HOST", &c.Host, false},
		{"Username", "AHV_CONSOLE_USERNAME", &c.Username, false},
		{"Password", "AHV_CONSOLE_PASSWORD", &c.Password, true},
	}
	for _, field := range fields {
		if *field.value != "" {
			continue
		}
		if c.NonInteractive || p == nil {
			return configError("missing " + field.label + "; set " + field.env + " or run in an interactive terminal.")
		}
		value, err := p.Ask(field.label, field.secret)
		if err != nil {
			return err
		}
		if value == "" {
			return configError(field.label + " cannot be empty.")
		}
		*field.value = value
	}
	return c.Validate()
}
