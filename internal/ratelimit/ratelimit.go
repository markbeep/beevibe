// Package ratelimit provides a small in-memory sliding-window limiter.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter allows at most max events per window per key.
type Limiter struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	hits   map[string][]time.Time
}

// New returns a limiter allowing max events per window for each key.
func New(window time.Duration, max int) *Limiter {
	return &Limiter{window: window, max: max, hits: make(map[string][]time.Time)}
}

// Allow records an event for key and reports whether it is within the limit.
// When it is not, the second return value is how long the caller should wait
// before retrying (used for Retry-After).
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		retry := kept[0].Sub(cutoff)
		if retry < 0 {
			retry = 0
		}
		l.hits[key] = kept
		return false, retry
	}
	l.hits[key] = append(kept, now)
	return true, 0
}

// Reset forgets every recorded event for key.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}
