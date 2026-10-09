package service

import (
	"bytes"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"testing"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/logging"
)

func TestLogForwardRecordsListenerAndConnections(t *testing.T) {
	var buf bytes.Buffer
	var seen []ForwardEventKind
	emit, stopped := logForward(logging.New(&buf, slog.LevelDebug).With("forward", "postgres.home:5432"), func(e ForwardEvent) { seen = append(seen, e.Kind) })
	route := Route{Network: domain.Network{ID: "01NET"}, Target: netip.MustParseAddrPort("100.64.0.9:5432")}
	emit(ForwardEvent{Kind: ForwardStarted, Listen: netip.MustParseAddrPort("127.0.0.1:15432"), Route: route})
	emit(ForwardEvent{Kind: ForwardRefused, ConnID: 1, Client: "127.0.0.1:50000", Err: errors.New("connection from another local user refused")})
	emit(ForwardEvent{Kind: ForwardOpened, ConnID: 2, Client: "127.0.0.1:50001", Route: route})
	emit(ForwardEvent{Kind: ForwardClosed, ConnID: 2, Client: "127.0.0.1:50001", Route: route, BytesSent: 5, BytesReceived: 7})
	stopped()
	if len(seen) != 4 {
		t.Fatalf("events must still reach the caller: %v", seen)
	}
	out := buf.String()
	for _, want := range []string{
		`level=INFO msg="listener started" forward=postgres.home:5432 listen=127.0.0.1:15432 network_id=01NET target=100.64.0.9:5432`,
		`level=WARN msg="connection refused" forward=postgres.home:5432 conn=1 client=127.0.0.1:50000 err="connection from another local user refused"`,
		`level=DEBUG msg="connection opened" forward=postgres.home:5432 conn=2`,
		`sent=5 received=7`,
		`level=INFO msg="listener stopped" forward=postgres.home:5432 listen=127.0.0.1:15432`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
