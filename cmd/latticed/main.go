package main

import (
	"fmt"
	"os"

	"git.lunarlabs.dev/lattice/lattice/internal/session"
	"git.lunarlabs.dev/lattice/lattice/internal/version"
)

func main() {
	if err := session.PrepareProcessEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "latticed: prepare environment: %v\n", err)
		os.Exit(1)
	}
	info := version.Info()
	fmt.Fprintf(os.Stdout, "latticed %s protocol %d.%d\n", info.DaemonVersion, info.ProtocolMajor, info.ProtocolMinor)
}
