package version_test

import (
	"strings"
	"testing"

	"git.lunarlabs.dev/flavor/flavor/internal/version"
)

func TestInfoHasProtocolV1(t *testing.T) {
	info := version.Info()
	if info.ProtocolMajor != 1 {
		t.Fatalf("ProtocolMajor=%d want 1", info.ProtocolMajor)
	}
	if info.DaemonVersion == "" {
		t.Fatal("DaemonVersion empty")
	}
}

func TestLineIncludesVersionCommitAndProtocol(t *testing.T) {
	info := version.Info()
	line := info.Line("flavorctl")
	for _, want := range []string{"flavorctl ", info.DaemonVersion, "(commit " + info.BuildCommit + ")", "protocol 1."} {
		if !strings.Contains(line, want) {
			t.Fatalf("%q does not contain %q", line, want)
		}
	}
}
