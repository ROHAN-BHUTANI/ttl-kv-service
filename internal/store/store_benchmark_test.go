package store

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkStoreSet measures sequential Set throughput with a realistic 256-byte payload.
func BenchmarkStoreSet(b *testing.B) {
	s := New()
	val := make([]byte, 256)
	for i := range val {
		val[i] = byte(i % 256)
	}

	// Pre-generate keys to isolate Store.Set performance from string formatting.
	const keyPoolSize = 10000
	keys := make([]string, keyPoolSize)
	for i := 0; i < keyPoolSize; i++ {
		keys[i] = "key:" + strconv.Itoa(i)
	}

	b.SetBytes(int64(len(val)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s.Set(keys[i%keyPoolSize], val, 5*time.Minute)
	}
}

// BenchmarkStoreGet measures sequential Get throughput for existing keys.
func BenchmarkStoreGet(b *testing.B) {
	s := New()
	val := make([]byte, 256)
	for i := range val {
		val[i] = byte(i % 256)
	}

	const numKeys = 10000
	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		k := "key:" + strconv.Itoa(i)
		keys[i] = k
		s.Set(k, val, 10*time.Minute)
	}

	b.SetBytes(int64(len(val)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = s.Get(keys[i%numKeys])
	}
}

// BenchmarkStoreDelete measures sequential Delete throughput of existing keys.
func BenchmarkStoreDelete(b *testing.B) {
	val := []byte("benchmark-value")
	s := New()

	// Pre-populate b.N keys so every measured iteration performs an authentic delete.
	keys := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		keys[i] = "del:" + strconv.Itoa(i)
		s.Set(keys[i], val, 0)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s.Delete(keys[i])
	}
}

// BenchmarkStoreKeys measures enumeration and sorting of active keys across varying store sizes.
func BenchmarkStoreKeys(b *testing.B) {
	sizes := []int{10, 100, 1000}
	val := []byte("val")

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			s := New()
			for i := 0; i < size; i++ {
				// Store keys in reverse order to ensure sort.Strings is exercised
				k := fmt.Sprintf("k:%06d", size-i)
				s.Set(k, val, 0)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = s.Keys()
			}
		})
	}
}

// BenchmarkStoreParallel measures concurrent read/write performance under goroutine contention.
func BenchmarkStoreParallel(b *testing.B) {
	s := New()
	val := make([]byte, 256)
	for i := range val {
		val[i] = byte(i % 256)
	}

	const keyPool = 1000
	keys := make([]string, keyPool)
	for i := 0; i < keyPool; i++ {
		keys[i] = "par:" + strconv.Itoa(i)
		s.Set(keys[i], val, 10*time.Minute)
	}

	b.SetBytes(int64(len(val)))
	b.ReportAllocs()
	b.ResetTimer()

	var counter uint64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := int(atomic.AddUint64(&counter, 1) % keyPool)
			k := keys[idx]
			if idx%5 == 0 {
				s.Set(k, val, 10*time.Minute)
			} else {
				_, _ = s.Get(k)
			}
		}
	})
}

// BenchmarkStoreMixedWorkload simulates a realistic production service profile:
// 80% Reads (Get), 15% Writes (Set), 5% Deletes (Delete).
func BenchmarkStoreMixedWorkload(b *testing.B) {
	s := New()
	val := make([]byte, 256)
	for i := range val {
		val[i] = byte(i % 256)
	}

	const keyPool = 2000
	keys := make([]string, keyPool)
	for i := 0; i < keyPool; i++ {
		keys[i] = "mix:" + strconv.Itoa(i)
		s.Set(keys[i], val, 10*time.Minute)
	}

	b.SetBytes(int64(len(val)))
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pb.Next() {
			op := r.Intn(100)
			k := keys[r.Intn(keyPool)]

			switch {
			case op < 80:
				_, _ = s.Get(k)
			case op < 95:
				s.Set(k, val, 10*time.Minute)
			default:
				s.Delete(k)
			}
		}
	})
}
