package config

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestPipeNameMatchesDesktopClient(t *testing.T) {
	got := socketPath(`C:\Users\a\AppData\Local\flavor`)
	if want := `\\.\pipe\flavor-71bc6c60eb8a6c5e9d15707bbc888c1a`; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestOwnerCheckAcceptsAdministratorsAndRejectsOthers(t *testing.T) {
	dir := t.TempDir()
	if err := checkOwner(dir, nil); err != nil {
		t.Fatalf("own temp dir: %v", err)
	}
	for _, tc := range []struct {
		sidType windows.WELL_KNOWN_SID_TYPE
		want    bool
	}{
		{windows.WinBuiltinAdministratorsSid, true},
		{windows.WinLocalSystemSid, true},
		{windows.WinBuiltinUsersSid, false},
	} {
		sid, err := windows.CreateWellKnownSid(tc.sidType)
		if err != nil {
			t.Fatal(err)
		}
		if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION, sid, nil, nil, nil); err != nil {
			t.Skipf("cannot change the owner here: %v", err)
		}
		if got := checkOwner(dir, nil) == nil; got != tc.want {
			t.Fatalf("owner %s accepted=%v want %v", sid, got, tc.want)
		}
	}
}
