package auth

import (
	"sync"
	"time"
)

// limiter counts failures per key over a sliding window. In memory: one server process,
// and a restart clearing the counters is acceptable.
type limiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	failures map[string][]time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, failures: map[string][]time.Time{}}
}

// blocked reports whether key is locked out at now, and until when.
func (l *limiter) blocked(key string, now time.Time) (bool, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.prune(key, now)
	if len(recent) < l.max {
		return false, time.Time{}
	}
	return true, recent[0].Add(l.window)
}

func (l *limiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[key] = append(l.prune(key, now), now)
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

// prune drops failures older than the window; the caller holds the lock.
func (l *limiter) prune(key string, now time.Time) []time.Time {
	list := l.failures[key]
	i := 0
	for i < len(list) && now.Sub(list[i]) >= l.window {
		i++
	}
	list = list[i:]
	if len(list) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = list
	return list
}
