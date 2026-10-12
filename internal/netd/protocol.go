//go:build linux || windows

package netd

import (
	"errors"
	"os"
)

const (
	ProtocolMajor = 1
	ProtocolMinor = 1
	maxPacket     = 4096
)

var errProtocol = errors.New("netd protocol violation")

func closeAll(files []*os.File) {
	for _, f := range files {
		f.Close()
	}
}
