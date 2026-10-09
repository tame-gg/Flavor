//go:build !windows

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func checkOwner(_ string, st os.FileInfo) error {
	stat, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("unexpected owner uid=%d", stat.Uid)
	}
	return nil
}

func socketPath(runtime string) string { return filepath.Join(runtime, "flavord.sock") }
