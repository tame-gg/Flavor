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
		DaemonVersion: daemonVersion,
		ProtocolMajor: 1,
		ProtocolMinor: 3,
		BuildCommit:   buildCommit,
		Capabilities:  []string{"headscale", "device_snapshots", "connection_inspector", "conflict_center", "workspaces", "destination_preferences", "forwarding", "socks_proxy"},
	}
}

var (
	daemonVersion = "0.1.0-dev"
	buildCommit   = "unknown"
)
