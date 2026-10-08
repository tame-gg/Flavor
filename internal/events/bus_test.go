package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/events"
)

func publish(t *testing.T, bus *events.Bus, label string) events.Event {
	t.Helper()
	ev, err := bus.Publish(events.Marker{Label: label})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func collect(t *testing.T, sub *events.Subscription, n int, timeout time.Duration) []events.Event {
	t.Helper()
	out := make([]events.Event, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timeout waiting for %d events, got %d", n, len(out))
		}
	}
	return out
}

func TestPublishSequenceAndTimestamp(t *testing.T) {
	bus := events.NewBus(8, 8)
	if bus.Sequence() != 0 {
		t.Fatal("expected zero before publish")
	}
	ev := publish(t, bus, "a")
	if ev.Sequence != 1 {
		t.Fatalf("seq=%d", ev.Sequence)
	}
	if ev.OccurredAt.IsZero() {
		t.Fatal("timestamp required")
	}
	ev2 := publish(t, bus, "b")
	if ev2.Sequence != 2 {
		t.Fatalf("seq=%d", ev2.Sequence)
	}
	if bus.Sequence() != 2 {
		t.Fatalf("current=%d", bus.Sequence())
	}
}

func TestMultipleSubscribers(t *testing.T) {
	bus := events.NewBus(8, 8)
	ctx := context.Background()
	s1, err := bus.Subscribe(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := bus.Subscribe(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	publish(t, bus, "x")
	a := collect(t, s1, 1, time.Second)
	b := collect(t, s2, 1, time.Second)
	if a[0].Sequence != 1 || b[0].Sequence != 1 {
		t.Fatalf("%v %v", a, b)
	}
}

func TestReplayWindow(t *testing.T) {
	bus := events.NewBus(3, 8)
	publish(t, bus, "1")
	publish(t, bus, "2")
	publish(t, bus, "3")
	ctx := context.Background()
	sub, err := bus.Subscribe(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := collect(t, sub, 2, time.Second)
	if got[0].Sequence != 2 || got[1].Sequence != 3 {
		t.Fatalf("%v", got)
	}
	publish(t, bus, "4")
	got = append(got, collect(t, sub, 1, time.Second)...)
	if got[2].Sequence != 4 {
		t.Fatalf("%v", got)
	}
	sub2, err := bus.Subscribe(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	got2 := collect(t, sub2, 3, time.Second)
	if got2[0].Sequence != 2 || got2[2].Sequence != 4 {
		t.Fatalf("%v", got2)
	}
	publish(t, bus, "5")
	_, err = bus.Subscribe(ctx, 1)
	if !errors.Is(err, events.ErrResyncRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestAfterZeroRequiresHistoryFromOne(t *testing.T) {
	bus := events.NewBus(2, 4)
	publish(t, bus, "1")
	publish(t, bus, "2")
	publish(t, bus, "3")
	_, err := bus.Subscribe(context.Background(), 0)
	if !errors.Is(err, events.ErrResyncRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestAfterGreaterThanCurrent(t *testing.T) {
	bus := events.NewBus(8, 8)
	publish(t, bus, "1")
	_, err := bus.Subscribe(context.Background(), 5)
	if !errors.Is(err, events.ErrResyncRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestSnapshotBaselineRace(t *testing.T) {
	bus := events.NewBus(256, 64)
	baseline := bus.Sequence()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = bus.Publish(events.Marker{Label: "x"})
		}(i)
	}
	wg.Wait()
	sub, err := bus.Subscribe(context.Background(), baseline)
	if err != nil {
		t.Fatal(err)
	}
	want := int(bus.Sequence() - baseline)
	got := collect(t, sub, want, 2*time.Second)
	if len(got) != want {
		t.Fatalf("got %d want %d", len(got), want)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Sequence != got[i-1].Sequence+1 {
			t.Fatalf("order break: %v", got)
		}
	}
	if got[0].Sequence != baseline+1 {
		t.Fatalf("first=%d baseline=%d", got[0].Sequence, baseline)
	}
}

func TestSlowSubscriberTerminated(t *testing.T) {
	bus := events.NewBus(32, 16)
	ctx := context.Background()
	slow, err := bus.SubscribeQueue(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	fast, err := bus.Subscribe(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for i := 0; i < 8; i++ {
		publish(t, bus, "p")
	}
	if time.Since(start) > time.Second {
		t.Fatal("publish blocked on slow subscriber")
	}
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-slow.Events:
			if !ok {
				if !errors.Is(slow.Err(), events.ErrResyncRequired) {
					t.Fatalf("err=%v", slow.Err())
				}
				goto slowDone
			}
		case <-deadline:
			t.Fatal("slow subscriber did not terminate")
		}
	}
slowDone:
	got := collect(t, fast, 8, time.Second)
	if len(got) != 8 || got[0].Sequence != 1 || got[7].Sequence != 8 {
		t.Fatalf("%v", got)
	}
}

func TestClose(t *testing.T) {
	bus := events.NewBus(8, 8)
	sub, err := bus.Subscribe(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = bus.Publish(events.Marker{Label: "x"})
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		bus.Close()
	}()
	wg.Wait()
	if _, err := bus.Publish(events.Marker{Label: "z"}); !errors.Is(err, events.ErrClosed) {
		t.Fatalf("publish after close: %v", err)
	}
	if _, err := bus.Subscribe(context.Background(), 0); !errors.Is(err, events.ErrClosed) {
		t.Fatalf("subscribe after close: %v", err)
	}
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-sub.Events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("subscription did not end")
		}
	}
}

func TestContextCancel(t *testing.T) {
	bus := events.NewBus(8, 8)
	ctx, cancel := context.WithCancel(context.Background())
	sub, err := bus.Subscribe(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-sub.Events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("cancel did not end subscription")
		}
	}
}

func TestSubscribePublishRaceNoGaps(t *testing.T) {
	bus := events.NewBus(10_000, 10_000)
	ctx := context.Background()
	baseline := bus.Sequence()
	sub, err := bus.Subscribe(ctx, baseline)
	if err != nil {
		t.Fatal(err)
	}
	const publishers = 8
	const perPub = 100
	var wg sync.WaitGroup
	wg.Add(publishers)
	for p := 0; p < publishers; p++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perPub; i++ {
				_, _ = bus.Publish(events.Marker{Label: "x"})
			}
		}()
	}
	wg.Wait()
	final := bus.Sequence()
	want := int(final - baseline)
	got := collect(t, sub, want, 3*time.Second)
	seen := map[uint64]bool{}
	for i, ev := range got {
		if seen[ev.Sequence] {
			t.Fatalf("duplicate %d", ev.Sequence)
		}
		seen[ev.Sequence] = true
		if i > 0 && ev.Sequence != got[i-1].Sequence+1 {
			t.Fatalf("gap/order at %d: %d then %d", i, got[i-1].Sequence, ev.Sequence)
		}
	}
	if len(got) != want {
		t.Fatalf("got %d want %d", len(got), want)
	}
}
