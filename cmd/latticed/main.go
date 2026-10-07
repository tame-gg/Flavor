package main

import (
	"fmt"
	"os"

	"git.lunarlabs.dev/lattice/lattice/internal/version"
)

func main() {
	info := version.Info()
	fmt.Fprintf(os.Stdout, "latticed %s protocol %d.%d\n", info.DaemonVersion, info.ProtocolMajor, info.ProtocolMinor)
}
