package ratelimit_test

import (
	"testing"
	"time"

	"github.com/mark/beevibe/internal/ratelimit"
)

// TestExceededDoesNotSpendBudget guards the property the login endpoint depends
// on: Exceeded is a pure peek, so a caller that checks it before doing the work
// and records with Allow afterwards cannot charge an event twice.
func TestExceededDoesNotSpendBudget(t *testing.T) {
	l := ratelimit.New(time.Minute, 2)

	if exceeded, _ := l.Exceeded("ip"); exceeded {
		t.Fatal("a fresh key is already at its limit")
	}
	for i := range 2 {
		if ok, _ := l.Allow("ip"); !ok {
			t.Fatalf("event %d within the limit was rejected", i+1)
		}
	}
	if exceeded, _ := l.Exceeded("ip"); !exceeded {
		t.Fatal("a key at its limit is not reported as exceeded")
	}

	// Repeated peeks must not consume anything: the key is at its limit, so an
	// Allow still reports the key as over budget rather than sliding it along.
	if exceeded, _ := l.Exceeded("ip"); !exceeded {
		t.Fatal("peeking twice changed the verdict")
	}
	if ok, retry := l.Allow("ip"); ok || retry <= 0 {
		t.Fatalf("Allow past the limit returned (%v, %v), want (false, >0)", ok, retry)
	}
}

// TestKeysAreIndependent pins the per-key bucketing: one address being out of
// budget must not affect another.
func TestKeysAreIndependent(t *testing.T) {
	l := ratelimit.New(time.Minute, 1)

	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("first event for a was rejected")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("first event for b was rejected")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("a exceeded its limit but was allowed")
	}
}
