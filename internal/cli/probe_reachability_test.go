package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestProbeReachabilityCmd_InvalidProbeOnlyValue(t *testing.T) {
	cmd := newProbeReachabilityCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--probe-only", "garbage", "https://example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid --probe-only value")
	}
	if !strings.Contains(err.Error(), "probe-only") {
		t.Errorf("expected error to mention probe-only; got: %v", err)
	}
}

func TestProbeReachabilityCmd_HelpFlags(t *testing.T) {
	cmd := newProbeReachabilityCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"--json", "--timeout", "--probe-only"} {
		if !strings.Contains(help, want) {
			t.Errorf("help output missing %q:\n%s", want, help)
		}
	}
}
