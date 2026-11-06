package widgets

import "sync"

// Bus tells the open pages that a service has something new. It only names the service: a
// page reads the news through the API, behind its session, like any other read.
type Bus struct {
	mu     sync.Mutex
	subs   map[chan string]struct{}
	closed bool
}

func NewBus() *Bus { return &Bus{subs: map[chan string]struct{}{}} }

// Publish never waits: a page too slow to follow misses a signal, and its polling catches up.
func (b *Bus) Publish(slug string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- slug:
		default:
		}
	}
}

// Subscribe returns the signals and how to stop them. The channel closes with the bus.
func (b *Bus) Subscribe() (<-chan string, func()) {
	ch := make(chan string, 8)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(ch)
		return ch, func() {}
	}
	b.subs[ch] = struct{}{}
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
}

// Close ends every subscription, so that open streams let the server shut down.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}
