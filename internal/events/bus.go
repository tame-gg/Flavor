package events

import (
	"context"
	"sync"
	"time"
)

const (
	DefaultReplayCapacity = 512
	DefaultSubQueue       = 64
)

type Bus struct {
	mu       sync.Mutex
	seq      uint64
	capacity int
	subQueue int
	history  []Event
	subs     map[*Subscription]struct{}
	closed   bool
}

func NewBus(replayCapacity, subQueue int) *Bus {
	if replayCapacity <= 0 {
		replayCapacity = DefaultReplayCapacity
	}
	if subQueue <= 0 {
		subQueue = DefaultSubQueue
	}
	return &Bus{
		capacity: replayCapacity,
		subQueue: subQueue,
		subs:     make(map[*Subscription]struct{}),
	}
}

func (b *Bus) Sequence() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

func (b *Bus) Publish(payload Payload) (Event, error) {
	if payload == nil {
		return Event{}, ErrInvalidSequence
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Event{}, ErrClosed
	}
	b.seq++
	ev := Event{
		Sequence:   b.seq,
		OccurredAt: time.Now().UTC(),
		Payload:    payload,
	}
	b.history = append(b.history, ev)
	if len(b.history) > b.capacity {
		b.history = append([]Event(nil), b.history[len(b.history)-b.capacity:]...)
	}
	for sub := range b.subs {
		b.deliverLocked(sub, ev)
	}
	return ev, nil
}

func (b *Bus) Subscribe(ctx context.Context, afterSequence uint64) (*Subscription, error) {
	return b.SubscribeQueue(ctx, afterSequence, b.subQueue)
}

func (b *Bus) SubscribeQueue(ctx context.Context, afterSequence uint64, queue int) (*Subscription, error) {
	if queue <= 0 {
		queue = b.subQueue
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	if err := b.validateSubscribeLocked(afterSequence); err != nil {
		return nil, err
	}
	replay := b.replayAfterLocked(afterSequence)
	ch := make(chan Event, len(replay)+queue)
	subCtx, cancel := context.WithCancel(ctx)
	sub := &Subscription{
		Events: ch,
		send:   ch,
		cancel: cancel,
		bus:    b,
	}
	for _, ev := range replay {
		ch <- ev
	}
	b.subs[sub] = struct{}{}
	go sub.watch(subCtx)
	return sub, nil
}

func (b *Bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	cancels := make([]context.CancelFunc, 0, len(b.subs))
	for sub := range b.subs {
		if c := sub.failLocked(ErrClosed); c != nil {
			cancels = append(cancels, c)
		}
	}
	clear(b.subs)
	b.history = nil
	b.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

func (b *Bus) validateSubscribeLocked(after uint64) error {
	current := b.seq
	if after > current {
		return ErrResyncRequired
	}
	if after == 0 {
		if current == 0 {
			return nil
		}
		if len(b.history) == 0 || b.history[0].Sequence != 1 {
			return ErrResyncRequired
		}
		return nil
	}
	need := after + 1
	if need > current {
		return nil
	}
	if len(b.history) == 0 || b.history[0].Sequence > need {
		return ErrResyncRequired
	}
	return nil
}

func (b *Bus) replayAfterLocked(after uint64) []Event {
	if len(b.history) == 0 {
		return nil
	}
	out := make([]Event, 0, len(b.history))
	for _, ev := range b.history {
		if ev.Sequence > after {
			out = append(out, ev)
		}
	}
	return out
}

func (b *Bus) deliverLocked(sub *Subscription, ev Event) {
	if sub.finished() {
		return
	}
	select {
	case sub.send <- ev:
	default:
		if c := sub.failLocked(ErrResyncRequired); c != nil {
			go c()
		}
	}
}
