package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

type Terminal struct {
	Context context.Context
	In, Out *os.File
}

func (t *Terminal) Available() bool {
	return term.IsTerminal(int(t.In.Fd())) && term.IsTerminal(int(t.Out.Fd()))
}

func (t *Terminal) Ask(label string, secret bool) (string, error) {
	if !t.Available() {
		return "", configError("interactive input requires a terminal; supply configuration through flags and environment variables.")
	}
	state, err := term.GetState(int(t.In.Fd()))
	if err != nil {
		return "", configError("cannot access terminal state.")
	}
	defer term.Restore(int(t.In.Fd()), state)
	if secret {
		// Disable echo before displaying the prompt: fast pasted input must not
		// race a later ReadPassword call. Raw input handles Ctrl+C explicitly.
		if _, err := term.MakeRaw(int(t.In.Fd())); err != nil {
			return "", configError("cannot disable password echo.")
		}
		defer fmt.Fprint(t.Out, "\r\n")
	}
	fmt.Fprint(t.Out, label+": ")
	type answer struct {
		value string
		err   error
	}
	done := make(chan answer, 1)
	go func() {
		if secret {
			b := make([]byte, 0, 128)
			one := make([]byte, 1)
			for len(b) < 65536 {
				_, e := t.In.Read(one)
				if e != nil {
					done <- answer{"", e}
					return
				}
				switch one[0] {
				case 3:
					done <- answer{"", context.Canceled}
					return
				case '\r', '\n':
					done <- answer{string(b), nil}
					return
				case 8, 127:
					if len(b) > 0 {
						b = b[:len(b)-1]
					}
				default:
					b = append(b, one[0])
				}
			}
			done <- answer{"", configError("password input is too long.")}
			return
		}
		var b strings.Builder
		one := make([]byte, 1)
		for b.Len() < 65536 {
			n, e := t.In.Read(one)
			if e != nil {
				done <- answer{"", e}
				return
			}
			if n == 1 {
				if one[0] == '\n' {
					done <- answer{strings.TrimSuffix(b.String(), "\r"), nil}
					return
				}
				b.WriteByte(one[0])
			}
		}
		done <- answer{"", configError("terminal input is too long.")}
	}()
	select {
	case <-t.Context.Done():
		fmt.Fprintln(t.Out)
		return "", t.Context.Err()
	case a := <-done:
		if a.err != nil {
			if errors.Is(a.err, context.Canceled) {
				return "", a.err
			}
			return "", configError("could not read terminal input.")
		}
		return a.value, nil
	}
}
