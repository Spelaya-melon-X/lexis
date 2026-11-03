package eventbus

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
)

func newTestBus() *Bus {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// collector — потокобезопасный сборщик событий для проверок после Close.
type collector struct {
	mu     sync.Mutex
	events []Event
}

func (c *collector) handle(_ context.Context, e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *collector) get() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.events
}

func TestBus_DeliversToAllInOrder(t *testing.T) {
	b := newTestBus()
	var a, s collector
	b.Subscribe("achievements", a.handle)
	b.Subscribe("stats", s.handle)

	// больше, чем queueSize — проверяем, что backpressure не теряет события
	const n = queueSize * 3
	for i := range n {
		b.Publish(WordLevelChanged{WordID: int64(i)})
	}
	b.Close()

	for name, c := range map[string]*collector{"achievements": &a, "stats": &s} {
		got := c.get()
		if len(got) != n {
			t.Fatalf("%s: got %d events, want %d", name, len(got), n)
		}
		for i, e := range got {
			if id := e.(WordLevelChanged).WordID; id != int64(i) {
				t.Fatalf("%s: event %d has WordID %d — order broken", name, i, id)
			}
		}
	}
}

func TestBus_PanicIsolated(t *testing.T) {
	b := newTestBus()
	var ok collector
	calls := 0
	b.Subscribe("broken", func(context.Context, Event) {
		calls++ // читается только после Close → wg.Wait даёт happens-before
		panic("boom")
	})
	b.Subscribe("ok", ok.handle)

	b.Publish(WordsAdded{Created: 1})
	b.Publish(WordsAdded{Created: 2})
	b.Close()

	if calls != 2 {
		t.Errorf("broken handler called %d times, want 2 (must survive its own panic)", calls)
	}
	if got := len(ok.get()); got != 2 {
		t.Errorf("ok handler got %d events, want 2", got)
	}
}

func TestBus_PublishAfterCloseIsNoop(t *testing.T) {
	b := newTestBus()
	var c collector
	b.Subscribe("c", c.handle)
	b.Close()
	b.Close() // повторный Close не паникует

	b.Publish(SessionFinished{SessionID: 1}) // не паникует и не блокируется
	if got := len(c.get()); got != 0 {
		t.Errorf("got %d events after Close, want 0", got)
	}
}

func TestBus_ConcurrentPublish(t *testing.T) {
	b := newTestBus()
	var c collector
	b.Subscribe("c", c.handle)

	const workers, per = 8, 50
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range per {
				b.Publish(WordsAdded{Created: 1})
			}
		})
	}
	wg.Wait()
	b.Close()

	if got := len(c.get()); got != workers*per {
		t.Errorf("got %d events, want %d", got, workers*per)
	}
}
