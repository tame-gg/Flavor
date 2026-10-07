package events

import (
	"context"
	"sync"
	"sync/atomic"
)

type Subscription struct {
	Events <-chan Event

	send   chan Event
	cancel context.CancelFunc
	bus    *Bus

	once sync.Once
	err  atomic.Value
}

func (s *Subscription) watch(ctx context.Context) {
	<-ctx.Done()
	s.terminate(ctx.Err())
}

func (s *Subscription) Close() {
	s.terminate(ErrClosed)
}

func (s *Subscription) Err() error {
	v := s.err.Load()
	if v == nil {
		return nil
	}
	return v.(error)
}

func (s *Subscription) finished() bool {
	return s.err.Load() != nil
}

func (s *Subscription) terminate(err error) {
	cancel := s.fail(err)
	if cancel != nil {
		cancel()
	}
}

func (s *Subscription) fail(err error) context.CancelFunc {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	return s.failLocked(err)
}

func (s *Subscription) failLocked(err error) context.CancelFunc {
	var cancel context.CancelFunc
	s.once.Do(func() {
		if err == nil {
			err = ErrClosed
		}
		s.err.Store(err)
		delete(s.bus.subs, s)
		close(s.send)
		cancel = s.cancel
	})
	return cancel
}
