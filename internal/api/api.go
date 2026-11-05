// Package api exposes the store over HTTP.
package api

import (
	"encoding/json"
	"expvar"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ROHAN-BHUTANI/ttl-kv-service/internal/store"
)

const maxValueBytes = 1 << 20 // 1 MiB

var (
	reqCount = expvar.NewInt("requests_total")
	errCount = expvar.NewInt("errors_total")
)

// Server wires HTTP routes to a Store.
type Server struct {
	store *store.Store
	ready atomic.Bool
}

func New(s *store.Store) *Server {
	srv := &Server{store: s}
	srv.ready.Store(true)
	return srv
}

// SetReady flips the readiness signal (used during graceful shutdown so the
// load balancer stops sending traffic before the process exits).
func (s *Server) SetReady(v bool) { s.ready.Store(v) }

// Handler returns the routed handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/keys/{key}", s.put)
	mux.HandleFunc("GET /v1/keys/{key}", s.get)
	mux.HandleFunc("DELETE /v1/keys/{key}", s.del)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // liveness: process is up
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if s.ready.Load() { // readiness: willing to take traffic
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.Handle("GET /metrics", expvar.Handler())
	return count(mux)
}

func count(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) put(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var ttl time.Duration
	if q := r.URL.Query().Get("ttl"); q != "" {
		secs, err := strconv.Atoi(q)
		if err != nil || secs < 0 {
			httpError(w, "ttl must be a non-negative integer (seconds)", http.StatusBadRequest)
			return
		}
		ttl = time.Duration(secs) * time.Second
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxValueBytes))
	if err != nil {
		httpError(w, "value too large or unreadable (max 1 MiB)", http.StatusRequestEntityTooLarge)
		return
	}
	s.store.Set(key, body, ttl)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	v, ok := s.store.Get(r.PathValue("key"))
	if !ok {
		httpError(w, "key not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(v)
}

func (s *Server) del(w http.ResponseWriter, r *http.Request) {
	if !s.store.Delete(r.PathValue("key")) {
		httpError(w, "key not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func httpError(w http.ResponseWriter, msg string, code int) {
	errCount.Add(1)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
