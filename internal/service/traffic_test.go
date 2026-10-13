package service

import (
	"testing"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/session"
)

func peer(id domain.NodeID, rx, tx int64) session.EnginePeer {
	return session.EnginePeer{NodeID: id, RxBytes: rx, TxBytes: tx}
}

func TestTrafficRingRates(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	var r trafficRing
	last := func() TrafficSample { return r.samples[len(r.samples)-1] }

	r.observe(t0, []session.EnginePeer{peer("a", 1000, 500)})
	if len(r.samples) != 0 {
		t.Fatalf("the first read must only prime: %+v", r.samples)
	}

	r.observe(t0.Add(2*time.Second), []session.EnginePeer{peer("a", 5000, 700), peer("b", 9000, 9000)})
	if s := last(); s.Rx != 2000 || s.Tx != 100 || !s.At.Equal(t0.Add(2*time.Second)) {
		t.Fatalf("a new peer must only prime and rates must use the elapsed time: %+v", s)
	}

	r.observe(t0.Add(3*time.Second), []session.EnginePeer{peer("a", 300, 800), peer("b", 9500, 9000)})
	if s := last(); s.Rx != 800 || s.Tx != 100 {
		t.Fatalf("a reset counter must count its new value: %+v", s)
	}

	r.observe(t0.Add(4*time.Second), []session.EnginePeer{peer("b", 9600, 9100)})
	if s := last(); s.Rx != 100 || s.Tx != 100 {
		t.Fatalf("%+v", s)
	}

	r.observe(t0.Add(5*time.Second), []session.EnginePeer{peer("a", 50, 60), peer("b", 9600, 9100)})
	if s := last(); s.Rx != 0 || s.Tx != 0 {
		t.Fatalf("a peer that came back must only prime: %+v", s)
	}

	r.observe(t0.Add(5500*time.Millisecond), []session.EnginePeer{peer("a", 50, 60), peer("b", 0, 0)})
	if s := last(); s.Rx != 0 || s.Tx != 0 {
		t.Fatalf("a counter reset to zero must not wrap: %+v", s)
	}

	r.observe(t0.Add(6*time.Second), []session.EnginePeer{peer("", 900, 900), peer("", 100, 100)})
	r.observe(t0.Add(7*time.Second), []session.EnginePeer{peer("", 100, 100), peer("", 900, 900)})
	if s := last(); s.Rx != 0 || s.Tx != 0 {
		t.Fatalf("peers without a node id must not count: %+v", s)
	}
}

func TestTrafficRingKeepsTheLatestSamples(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	var r trafficRing
	for i := range 131 {
		r.observe(t0.Add(time.Duration(i)*time.Second), []session.EnginePeer{peer("a", int64(i)*10, 0)})
	}
	if len(r.samples) != trafficSamples || !r.samples[0].At.Equal(t0.Add(11*time.Second)) || !r.samples[trafficSamples-1].At.Equal(t0.Add(130*time.Second)) || r.samples[0].Rx != 10 {
		t.Fatalf("%d samples from %v to %v", len(r.samples), r.samples[0].At, r.samples[len(r.samples)-1].At)
	}
}

func TestRecordTrafficDropsStoppedNetworks(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	s := &Service{traffic: map[domain.NetworkID]*trafficRing{}}
	s.recordTraffic(map[domain.NetworkID]trafficRead{"a": {at: t0}, "b": {at: t0}})
	s.recordTraffic(map[domain.NetworkID]trafficRead{"b": {at: t0.Add(time.Second)}})
	if _, ok := s.traffic["a"]; ok {
		t.Fatal("a stopped network must lose its samples")
	}
	if b := s.traffic["b"]; b == nil || len(b.samples) != 1 {
		t.Fatalf("%+v", b)
	}
	s.recordTraffic(map[domain.NetworkID]trafficRead{"b": {}})
	if b := s.traffic["b"]; b == nil || len(b.samples) != 1 {
		t.Fatalf("a failed read must keep the samples: %+v", b)
	}
	s.recordTraffic(map[domain.NetworkID]trafficRead{"b": {at: t0.Add(3 * time.Second)}})
	if b := s.traffic["b"]; len(b.samples) != 2 || !b.samples[1].At.Equal(t0.Add(3*time.Second)) {
		t.Fatalf("%+v", b)
	}
}
