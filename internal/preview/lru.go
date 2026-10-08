// Package preview holds the preview pipeline: the bounded image cache, the
// renderer client and the debouncing capture manager.
package preview

import (
	"container/list"
	"sync"
	"time"
)

// entry is one cached preview image together with its revision.
type entry struct {
	userID int64
	data   []byte
	// rev is a monotonically increasing revision used as the ETag. A page that
	// sees the same revision can answer 304 Not Modified.
	rev int64
	at  time.Time
}

// Store is a bounded LRU cache keyed by user id. It is safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	cap   int
	items map[int64]*list.Element
	order *list.List
	seq   int64
}

// NewStore returns a Store holding at most cap entries, evicting the least
// recently used one first. A cap below 1 is treated as 1 so the cache is always
// bounded.
func NewStore(cap int) *Store {
	if cap < 1 {
		cap = 1
	}
	return &Store{
		cap:   cap,
		items: make(map[int64]*list.Element),
		order: list.New(),
	}
}

// Put stores the image for a user and returns its new revision. The revision
// counter never decreases, so it is usable as an ETag value.
func (s *Store) Put(userID int64, data []byte) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	rev := s.seq
	e := &entry{userID: userID, data: data, rev: rev, at: time.Now()}

	if el, ok := s.items[userID]; ok {
		el.Value = e
		s.order.MoveToFront(el)
		return rev
	}

	el := s.order.PushFront(e)
	s.items[userID] = el
	for s.order.Len() > s.cap {
		back := s.order.Back()
		if back == nil {
			break
		}
		s.order.Remove(back)
		delete(s.items, back.Value.(*entry).userID)
	}
	return rev
}

// Get returns the cached image and its revision. The returned slice is shared
// and must not be modified by the caller.
func (s *Store) Get(userID int64) ([]byte, int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	el, ok := s.items[userID]
	if !ok {
		return nil, 0, false
	}
	s.order.MoveToFront(el)
	e := el.Value.(*entry)
	return e.data, e.rev, true
}

// Drop removes a user's cached image.
func (s *Store) Drop(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	el, ok := s.items[userID]
	if !ok {
		return
	}
	s.order.Remove(el)
	delete(s.items, userID)
}

// Len reports how many entries the cache currently holds.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.order.Len()
}
