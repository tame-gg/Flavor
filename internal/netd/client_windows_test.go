package netd

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestServerAllowed(t *testing.T) {
	cases := []struct {
		name       string
		state      uint32
		servicePid uint32
		serverPid  uint32
		startName  string
		want       bool
	}{
		{"running local system", windows.SERVICE_RUNNING, 4242, 4242, "LocalSystem", true},
		{"running nt authority system", windows.SERVICE_RUNNING, 4242, 4242, `NT AUTHORITY\SYSTEM`, true},
		{"case insensitive", windows.SERVICE_RUNNING, 4242, 4242, "localsystem", true},
		{"pid mismatch", windows.SERVICE_RUNNING, 4242, 4243, "LocalSystem", false},
		{"stopped", windows.SERVICE_STOPPED, 0, 4242, "LocalSystem", false},
		{"stopped with stale pid", windows.SERVICE_STOPPED, 4242, 4242, "LocalSystem", false},
		{"running without pid", windows.SERVICE_RUNNING, 0, 0, "LocalSystem", false},
		{"local service", windows.SERVICE_RUNNING, 4242, 4242, `NT AUTHORITY\LocalService`, false},
		{"user account", windows.SERVICE_RUNNING, 4242, 4242, `.\marco`, false},
		{"empty account", windows.SERVICE_RUNNING, 4242, 4242, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := serverAllowed(c.state, c.servicePid, c.serverPid, c.startName); got != c.want {
				t.Fatalf("serverAllowed = %v, want %v", got, c.want)
			}
		})
	}
}
