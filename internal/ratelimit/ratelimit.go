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

// Exceeded reports whether key has already reached its limit, without recording
// an event. Callers that charge only the events which matter — the login
// endpoint, which counts rejected credentials and never successful ones — check
// this before doing the work and record with Allow on the way out.
func (l *Limiter) Exceeded(key string) (bool, time.Duration) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.exceeded(key, now)
}

// Allow records an event for key and reports whether it is within the limit.
// When it is not, the second return value is how long the caller should wait
// before retrying (used for Retry-After).
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if ok, retry := l.exceeded(key, now); ok {
		return false, retry
	}
	l.hits[key] = append(l.hits[key], now)
	return true, 0
}

// exceeded drops key's events that have left the window and reports whether
// what remains is at the limit. It mutates only the pruned slice.
func (l *Limiter) exceeded(key string, now time.Time) (bool, time.Duration) {
	cutoff := now.Add(-l.window)

	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.hits[key] = kept

	if len(kept) >= l.max {
		retry := kept[0].Sub(cutoff)
		if retry < 0 {
			retry = 0
		}
		return true, retry
	}
	return false, 0
}
