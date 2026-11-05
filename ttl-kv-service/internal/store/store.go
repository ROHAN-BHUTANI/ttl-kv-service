// Package store implements a concurrency-safe in-memory key-value store with
// per-key TTL. Expired keys are removed lazily on read and by a background
// janitor so memory does not grow without bound.
package store

import (
	"context"
	"sync"
	"time"
)

type entry struct {
	value     []byte
	expiresAt time.Time // zero value means "never expires"
}

func (e entry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && !now.Before(e.expiresAt)
}

// Store is safe for concurrent use.
type Store struct {
	mu   sync.RWMutex
	data map[string]entry
	now  func() time.Time // injectable clock for tests
}

// New returns an empty Store.
func New() *Store {
	return &Store{data: make(map[string]entry), now: time.Now}
}

// Set stores value under key. A ttl <= 0 means the key never expires.
func (s *Store) Set(key string, value []byte, ttl time.Duration) {
	e := entry{value: append([]byte(nil), value...)}
	if ttl > 0 {
		e.expiresAt = s.now().Add(ttl)
	}
	s.mu.Lock()
	s.data[key] = e
	s.mu.Unlock()
}

// Get returns the value and true if the key exists and has not expired.
func (s *Store) Get(key string) ([]byte, bool) {
	s.mu.RLock()
	e, ok := s.data[key]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if e.expired(s.now()) {
		s.Delete(key)
		return nil, false
	}
	return append([]byte(nil), e.value...), true
}

// Delete removes key and reports whether it was present.
func (s *Store) Delete(key string) bool {
	s.mu.Lock()
	_, ok := s.data[key]
	delete(s.data, key)
	s.mu.Unlock()
	return ok
}

// Len returns the number of stored (possibly not yet swept) entries.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}

// sweep removes all expired entries and returns how many were removed.
func (s *Store) sweep() int {
	now := s.now()
	removed := 0
	s.mu.Lock()
	for k, e := range s.data {
		if e.expired(now) {
			delete(s.data, k)
			removed++
		}
	}
	s.mu.Unlock()
	return removed
}

// RunJanitor sweeps expired keys every interval until ctx is cancelled.
func (s *Store) RunJanitor(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweep()
		}
	}
}
