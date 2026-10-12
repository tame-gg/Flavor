package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/netd"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

var installed = []string{"flavor-netd.exe", "wintun.dll"}

func installDir() (string, error) {
	pf, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(pf, "Flavor"), nil
}

func current() bool {
	dir, err := installDir()
	if err != nil || !netd.Installed() {
		return false
	}
	for _, name := range installed {
		src, err1 := sourceFile(name)
		a, err2 := os.ReadFile(src)
		b, err3 := os.ReadFile(filepath.Join(dir, name))
		if err1 != nil || err2 != nil || err3 != nil || !bytes.Equal(a, b) {
			return false
		}
	}
	return true
}

func sourceFile(name string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if name == installed[0] {
		return self, nil
	}
	return filepath.Join(filepath.Dir(self), name), nil
}

func install() error {
	dir, err := installDir()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(netd.ServiceName)
	if err == nil {
		defer s.Close()
		if err := stop(s); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, name := range installed {
		src, err := sourceFile(name)
		if err != nil {
			return err
		}
		if err := copyFile(src, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	exe := filepath.Join(dir, installed[0])
	if s == nil {
		s, err = m.CreateService(netd.ServiceName, exe, mgr.Config{
			StartType:   mgr.StartAutomatic,
			DisplayName: "Flavor network helper",
			Description: "Gives Flavor names and addresses to apps for signed-in users.",
		})
		if err != nil {
			return err
		}
		defer s.Close()
	} else {
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		cfg.BinaryPathName = syscall.EscapeArg(exe)
		if err := s.UpdateConfig(cfg); err != nil {
			return err
		}
	}
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}}, 86400); err != nil {
		return err
	}
	return s.Start()
}

func copyFile(src, dst string) error {
	if same, err := sameFile(src, dst); err != nil || same {
		return err
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func sameFile(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil && os.SameFile(sa, sb), err
}

func stop(s *mgr.Service) error {
	st, err := s.Query()
	if err != nil {
		return err
	}
	if st.State != svc.Stopped && st.State != svc.StopPending {
		if _, err := s.Control(svc.Stop); err != nil {
			return err
		}
	}
	for range 60 {
		if st, err = s.Query(); err != nil || st.State == svc.Stopped {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("flavor-netd did not stop within 30 seconds")
}

func uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if s, err := m.OpenService(netd.ServiceName); err == nil {
		err = stop(s)
		if err == nil {
			err = s.Delete()
		}
		s.Close()
		if err != nil {
			return err
		}
	}
	if err := (netd.NRPT{}).Revert(context.Background(), 0); err != nil {
		return err
	}
	dir, err := installDir()
	if err != nil {
		return err
	}
	for _, name := range append(installed, "flavor-netd.log") {
		path := filepath.Join(dir, name)
		for i := 0; i < 20; i++ {
			if err = os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
