package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func newWithClock(t *time.Time) *Store {
	s := New()
	s.now = func() time.Time { return *t }
	return s
}

func TestSetGetDelete(t *testing.T) {
	s := New()
	s.Set("a", []byte("1"), 0)
	if v, ok := s.Get("a"); !ok || string(v) != "1" {
		t.Fatalf("got %q,%v", v, ok)
	}
	if !s.Delete("a") {
		t.Fatal("expected delete to report true")
	}
	if _, ok := s.Get("a"); ok {
		t.Fatal("key should be gone")
	}
	if s.Delete("a") {
		t.Fatal("second delete should report false")
	}
}

func TestTTLExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	s := newWithClock(&now)
	s.Set("k", []byte("v"), 10*time.Second)

	now = now.Add(9 * time.Second)
	if _, ok := s.Get("k"); !ok {
		t.Fatal("key should still be alive at 9s")
	}
	now = now.Add(1 * time.Second) // exactly at expiry
	if _, ok := s.Get("k"); ok {
		t.Fatal("key should be expired at 10s")
	}
	if s.Len() != 0 {
		t.Fatalf("lazy delete should have removed the key, len=%d", s.Len())
	}
}

func TestSweepRemovesOnlyExpired(t *testing.T) {
	now := time.Unix(0, 0)
	s := newWithClock(&now)
	s.Set("short", []byte("x"), time.Second)
	s.Set("long", []byte("y"), time.Hour)
	s.Set("forever", []byte("z"), 0)

	now = now.Add(2 * time.Second)
	if n := s.sweep(); n != 1 {
		t.Fatalf("swept %d, want 1", n)
	}
	if s.Len() != 2 {
		t.Fatalf("len=%d, want 2", s.Len())
	}
}

func TestValueIsCopied(t *testing.T) {
	s := New()
	in := []byte("abc")
	s.Set("k", in, 0)
	in[0] = 'X'
	out, _ := s.Get("k")
	if string(out) != "abc" {
		t.Fatalf("store must copy on write, got %q", out)
	}
	out[0] = 'Y'
	again, _ := s.Get("k")
	if string(again) != "abc" {
		t.Fatalf("store must copy on read, got %q", again)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				k := fmt.Sprintf("k%d", i%20)
				s.Set(k, []byte("v"), time.Minute)
				s.Get(k)
				if i%7 == 0 {
					s.Delete(k)
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestKeysReturnsActiveKeys verifies that active keys are returned alphabetically.
func TestKeysReturnsActiveKeys(t *testing.T) {
	s := New()

	s.Set("banana", []byte("1"), 0)
	s.Set("apple", []byte("2"), 0)
	s.Set("cherry", []byte("3"), 0)

	keys := s.Keys()

	expected := []string{"apple", "banana", "cherry"}

	if len(keys) != len(expected) {
		t.Fatalf("got %d keys, want %d", len(keys), len(expected))
	}

	for i, key := range expected {
		if keys[i] != key {
			t.Fatalf("keys[%d] = %q, want %q", i, keys[i], key)
		}
	}
}

// TestKeysExcludesExpiredKeys verifies expired keys are excluded.
func TestKeysExcludesExpiredKeys(t *testing.T) {
	now := time.Unix(1000, 0)
	s := newWithClock(&now)

	s.Set("active", []byte("1"), time.Hour)
	s.Set("expired", []byte("2"), time.Second)

	now = now.Add(2 * time.Second)

	keys := s.Keys()

	if len(keys) != 1 || keys[0] != "active" {
		t.Fatalf("got keys %v, want [active]", keys)
	}
}

// TestKeysEmptyStore verifies an empty store returns no keys.
func TestKeysEmptyStore(t *testing.T) {
	s := New()

	keys := s.Keys()

	if len(keys) != 0 {
		t.Fatalf("got %v, want empty keys", keys)
	}
}

// TestRunJanitorStopsOnCancel verifies that the background janitor goroutine
// terminates immediately and cleanly when its context is cancelled.
func TestRunJanitorStopsOnCancel(t *testing.T) {
	s := New()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		s.RunJanitor(ctx, 10*time.Millisecond)
		close(done)
	}()

	// Signal cancellation
	cancel()

	select {
	case <-done:
		// Janitor stopped cleanly as expected
	case <-time.After(1 * time.Second):
		t.Fatal("RunJanitor did not terminate after context cancellation")
	}
}

// TestRunJanitorPeriodicSweep verifies that the background janitor automatically
// sweeps and deletes expired keys on its ticker interval.
func TestRunJanitorPeriodicSweep(t *testing.T) {
	now := time.Unix(1000, 0)
	s := newWithClock(&now)

	s.Set("temp", []byte("val"), 10*time.Millisecond)
	if s.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", s.Len())
	}

	// Advance clock past expiration
	now = now.Add(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.RunJanitor(ctx, 10*time.Millisecond)

	// Poll until swept or timeout
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if s.Len() == 0 {
			return // Successfully swept
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("janitor failed to sweep expired keys, store len=%d", s.Len())
}
