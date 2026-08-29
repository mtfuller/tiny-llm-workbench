package cmd

import "testing"

func TestDisplayHost(t *testing.T) {
	cases := map[string]string{
		"":            "localhost",
		"0.0.0.0":     "localhost",
		"::":          "localhost",
		"[::]":        "localhost",
		"127.0.0.1":   "127.0.0.1",
		"192.168.1.5": "192.168.1.5",
		"tlw.local":   "tlw.local",
	}
	for in, want := range cases {
		if got := displayHost(in); got != want {
			t.Errorf("displayHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestServeCmdDefaultsToLoopback(t *testing.T) {
	flag := serveCmd.Flags().Lookup("host")
	if flag == nil {
		t.Fatal("serve command has no --host flag")
	}
	if flag.DefValue != "127.0.0.1" {
		t.Errorf("--host default = %q, want 127.0.0.1 (loopback-only by default)", flag.DefValue)
	}
}

func TestServeCmdOpenFlagDefaultsOff(t *testing.T) {
	flag := serveCmd.Flags().Lookup("open")
	if flag == nil {
		t.Fatal("serve command has no --open flag")
	}
	if flag.DefValue != "false" {
		t.Errorf("--open default = %q, want false (don't auto-open a browser unless asked)", flag.DefValue)
	}
}
