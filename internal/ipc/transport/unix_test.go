package transport

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestListenerReportsPeerUIDAndPrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %o", st.Mode().Perm())
	}
	go func() {
		c, err := net.Dial("unix", path)
		if err == nil {
			defer c.Close()
			_, _ = c.Read(make([]byte, 1))
		}
	}()
	c, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	uid, ok := PeerUID(ConnContext(context.Background(), c))
	if !ok || uid != os.Getuid() {
		t.Fatalf("peer uid %d ok=%v", uid, ok)
	}
}

func TestRequirePeerRejectsForeignOrUnknownUID(t *testing.T) {
	h := RequirePeer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	for name, ctx := range map[string]context.Context{
		"unknown": context.Background(),
		"foreign": context.WithValue(context.Background(), peerKey{}, os.Getuid()+1),
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d", name, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	ok := context.WithValue(context.Background(), peerKey{}, os.Getuid())
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ok))
	if rec.Code != http.StatusOK {
		t.Fatalf("own uid: got %d", rec.Code)
	}
}
