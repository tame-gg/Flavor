package app_test

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/app"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"git.lunarlabs.dev/flavor/flavor/internal/session/sessiontest"
)

func TestTrafficStats(t *testing.T) {
	engines := &sessiontest.Sequence{Prepare: func(_ int, _ provider.ResolvedSessionConfig, e *sessiontest.Engine) {
		e.SetStatus(sessiontest.StatusWithPeer("peer", "100.64.0.20"))
	}}
	d := start(t, testEnv(t), app.Options{EngineFactory: engines.Factory, TrafficTick: 100 * time.Millisecond})
	ctx := context.Background()
	info, err := d.client.Daemon.GetDaemonInfo(ctx, connect.NewRequest(&v1.GetDaemonInfoRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(info.Msg.Capabilities, v1.Capability_CAPABILITY_TRAFFIC_STATS) {
		t.Fatal("traffic capability not advertised")
	}
	a := addHeadscale(t, d.client, "Lab", "https://lab.example.com", false)
	addHeadscale(t, d.client, "Office", "https://office.example.com", false)
	connectNetworks(t, d.client, a)

	stats := func() *v1.GetTrafficStatsResponse {
		t.Helper()
		res, err := d.client.Traffic.GetTrafficStats(ctx, connect.NewRequest(&v1.GetTrafficStatsRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}
	samples := func() []*v1.TrafficSample {
		t.Helper()
		r := stats()
		if r.IntervalMs != 100 || len(r.Networks) != 1 || r.Networks[0].NetworkId != a.Id {
			t.Fatalf("only the connected network must be reported: %+v", r)
		}
		return r.Networks[0].Samples
	}
	setCounters := func(rx, tx int64) {
		st := sessiontest.StatusWithPeer("peer", "100.64.0.20")
		st.Peers[0].RxBytes, st.Peers[0].TxBytes = rx, tx
		engines.Engine(0).SetStatus(st)
	}
	busy := func(after int64) (prev, cur *v1.TrafficSample) {
		t.Helper()
		eventually(t, "a sample with traffic", func() bool {
			s := samples()
			for i := 1; i < len(s); i++ {
				if s[i].AtUnixMs > after && s[i].RxBytesPerSecond > 0 {
					prev, cur = s[i-1], s[i]
					return true
				}
			}
			return false
		})
		return prev, cur
	}
	rateOf := func(rate uint64, bytes int64, prev, cur *v1.TrafficSample) bool {
		dt := float64(cur.AtUnixMs - prev.AtUnixMs)
		hi := math.Inf(1)
		if dt > 1 {
			hi = float64(bytes) * 1000 / (dt - 1)
		}
		return float64(rate) >= float64(bytes)*1000/(dt+1)-1 && float64(rate) <= hi
	}

	if s := samples(); len(s) != 0 {
		t.Fatalf("nothing is sampled before the first request: %+v", s)
	}
	eventually(t, "a first sample", func() bool { return len(samples()) > 0 })
	if s := samples()[0]; s.RxBytesPerSecond != 0 || s.TxBytesPerSecond != 0 {
		t.Fatalf("idle counters must report no traffic: %+v", s)
	}

	setCounters(1_000_000, 250_000)
	prev, cur := busy(0)
	if !rateOf(cur.RxBytesPerSecond, 1_000_000, prev, cur) || !rateOf(cur.TxBytesPerSecond, 250_000, prev, cur) {
		t.Fatalf("rates do not match the counters over %d ms: %+v", cur.AtUnixMs-prev.AtUnixMs, cur)
	}

	setCounters(4_000, 0)
	prev, reset := busy(cur.AtUnixMs)
	if !rateOf(reset.RxBytesPerSecond, 4_000, prev, reset) || reset.TxBytesPerSecond != 0 {
		t.Fatalf("a reset counter must count its new value and never wrap: %+v", reset)
	}

	time.Sleep(3 * time.Second)
	if s := samples(); len(s) != 0 {
		t.Fatalf("the sampler must stop and forget samples when nobody asks: %d samples", len(s))
	}
	eventually(t, "sampling to resume", func() bool { return len(samples()) > 0 })

	if _, err := d.client.Networks.DisconnectNetwork(ctx, connect.NewRequest(&v1.DisconnectNetworkRequest{NetworkId: a.Id})); err != nil {
		t.Fatal(err)
	}
	if r := stats(); len(r.Networks) != 0 {
		t.Fatalf("a disconnected network must not be reported: %+v", r)
	}
}
