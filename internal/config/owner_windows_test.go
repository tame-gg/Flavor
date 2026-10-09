package config

import "testing"

func TestPipeNameMatchesDesktopClient(t *testing.T) {
	got := socketPath(`C:\Users\a\AppData\Local\flavor`)
	if want := `\\.\pipe\flavor-71bc6c60eb8a6c5e9d15707bbc888c1a`; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
