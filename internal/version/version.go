package version

type BuildInfo struct {
	DaemonVersion string
	ProtocolMajor int
	ProtocolMinor int
	BuildCommit   string
	Capabilities  []string
}

func Info() BuildInfo {
	return BuildInfo{
		DaemonVersion: "0.1.0-dev",
		ProtocolMajor: 1,
		ProtocolMinor: 0,
		BuildCommit:   buildCommit,
		Capabilities:  []string{"headscale", "device_snapshots"},
	}
}

var buildCommit = "unknown"
