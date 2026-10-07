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
		ProtocolMinor: 1,
		BuildCommit:   buildCommit,
		Capabilities:  []string{"headscale", "device_snapshots", "connection_inspector", "conflict_center", "workspaces"},
	}
}

var buildCommit = "unknown"
