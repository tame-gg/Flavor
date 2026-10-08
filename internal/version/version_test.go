package version_test

import (
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
