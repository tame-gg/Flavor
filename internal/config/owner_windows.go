package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"git.lunarlabs.dev/flavor/flavor/internal/winsid"
	"golang.org/x/sys/windows"
)

func checkOwner(path string, _ os.FileInfo) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	self, err := winsid.Current()
	if err != nil {
		return err
	}
	if !owner.Equals(self) {
		return fmt.Errorf("unexpected owner %s", owner)
	}
	return nil
}

func socketPath(runtime string) string {
	sum := sha256.Sum256([]byte(runtime))
	return `\\.\pipe\flavor-` + hex.EncodeToString(sum[:16])
}
