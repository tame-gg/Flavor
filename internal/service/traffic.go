package service

import (
	"context"
	"slices"
	"sync"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/session"
)

const (
	trafficSamples   = 120
	trafficIdleTicks = 10
)

type TrafficSample struct {
	At time.Time
	Rx uint64
	Tx uint64
}

type NetworkTraffic struct {
	NetworkID domain.NetworkID
	Samples   []TrafficSample
}

type trafficRing struct {
	at      time.Time
	peers   map[domain.NodeID]session.EnginePeer
	samples []TrafficSample
}

type trafficRead struct {
	at    time.Time
	peers []session.EnginePeer
}

func (s *Service) GetTrafficStats(ctx context.Context) ([]NetworkTraffic, time.Duration, error) {
	s.watchTraffic()
	live, err := s.trafficSessions(ctx)
	if err != nil {
		return nil, 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]NetworkTraffic, 0, len(live))
	for _, sess := range live {
		n := NetworkTraffic{NetworkID: sess.ID()}
		if r := s.traffic[sess.ID()]; r != nil {
			n.Samples = slices.Clone(r.samples)
		}
		out = append(out, n)
	}
	return out, s.cfg.TrafficTick, nil
}

func (s *Service) watchTraffic() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trafficAsked = time.Now()
	if s.trafficStop != nil || s.shuttingDown {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.trafficStop = cancel
	s.traffic = make(map[domain.NetworkID]*trafficRing)
	s.trafficDone.Add(1)
	go s.sampleTraffic(ctx)
}

func (s *Service) sampleTraffic(ctx context.Context) {
	defer s.trafficDone.Done()
	tick := time.NewTicker(s.cfg.TrafficTick)
	defer tick.Stop()
	for {
		if reads, err := s.readTraffic(ctx); err == nil {
			s.recordTraffic(reads)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if s.trafficIdle() {
			return
		}
	}
}

func (s *Service) trafficIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.trafficAsked) < trafficIdleTicks*s.cfg.TrafficTick {
		return false
	}
	s.trafficStop()
	s.trafficStop, s.traffic = nil, nil
	return true
}

func (s *Service) trafficSessions(ctx context.Context) ([]*session.Session, error) {
	all, err := s.cfg.Store.Networks().List(ctx)
	if err != nil {
		return nil, s.storeErr(err)
	}
	var out []*session.Session
	for _, n := range all {
		sess, ok := s.cfg.Sessions.Get(n.ID)
		if !ok {
			continue
		}
		if st := sess.State(); st == domain.StateConnected || st == domain.StateDegraded {
			out = append(out, sess)
		}
	}
	return out, nil
}

func (s *Service) readTraffic(ctx context.Context) (map[domain.NetworkID]trafficRead, error) {
	live, err := s.trafficSessions(ctx)
	if err != nil {
		return nil, err
	}
	rctx, cancel := context.WithTimeout(ctx, s.cfg.TrafficTick)
	defer cancel()
	results := make([]trafficRead, len(live))
	var wg sync.WaitGroup
	for i, sess := range live {
		wg.Go(func() {
			if peers, err := sess.PeerCounters(rctx); err == nil {
				results[i] = trafficRead{at: time.Now(), peers: peers}
			}
		})
	}
	wg.Wait()
	reads := make(map[domain.NetworkID]trafficRead, len(live))
	for i, sess := range live {
		reads[sess.ID()] = results[i]
	}
	return reads, nil
}

func (s *Service) recordTraffic(reads map[domain.NetworkID]trafficRead) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.traffic {
		if _, ok := reads[id]; !ok {
			delete(s.traffic, id)
		}
	}
	for id, r := range reads {
		if r.at.IsZero() {
			continue
		}
		ring := s.traffic[id]
		if ring == nil {
			ring = &trafficRing{}
			s.traffic[id] = ring
		}
		ring.observe(r.at, r.peers)
	}
}

func (r *trafficRing) observe(at time.Time, peers []session.EnginePeer) {
	prev, elapsed := r.peers, at.Sub(r.at).Seconds()
	r.at, r.peers = at, make(map[domain.NodeID]session.EnginePeer, len(peers))
	var rx, tx uint64
	for _, p := range peers {
		if p.NodeID == "" {
			continue
		}
		if old, ok := prev[p.NodeID]; ok {
			rx += grown(old.RxBytes, p.RxBytes)
			tx += grown(old.TxBytes, p.TxBytes)
		}
		r.peers[p.NodeID] = p
	}
	if prev == nil || elapsed <= 0 {
		return
	}
	r.samples = append(r.samples, TrafficSample{At: at, Rx: uint64(float64(rx) / elapsed), Tx: uint64(float64(tx) / elapsed)})
	if len(r.samples) > trafficSamples {
		r.samples = r.samples[1:]
	}
}

func grown(prev, cur int64) uint64 {
	if cur < prev {
		prev = 0
	}
	return uint64(max(cur-prev, 0))
}
