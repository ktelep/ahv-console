package platform

import "testing"

func TestArgumentSafeLaunch(t *testing.T) {
	for _, os := range []string{"windows", "darwin", "linux"} {
		url := "http://127.0.0.1:1234/#a&b"
		name, args := Command(os, url)
		if name == "" || args[len(args)-1] != url {
			t.Fatal(name, args)
		}
		for _, a := range args {
			if a == "-c" || a == "/c" {
				t.Fatal("shell interpolation")
			}
		}
	}
}
