package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ROHAN-BHUTANI/ttl-kv-service/internal/store"
)

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	return rec
}

func TestPutGetDelete(t *testing.T) {
	srv := New(store.New())
	handler := srv.Handler()

	put := do(t, handler, http.MethodPut, "/v1/keys/name", "Rohan")
	if put.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want %d", put.Code, http.StatusNoContent)
	}

	get := do(t, handler, http.MethodGet, "/v1/keys/name", "")
	if get.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", get.Code, http.StatusOK)
	}

	if get.Body.String() != "Rohan" {
		t.Fatalf("GET body = %q, want Rohan", get.Body.String())
	}

	del := do(t, handler, http.MethodDelete, "/v1/keys/name", "")
	if del.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want %d", del.Code, http.StatusNoContent)
	}
}

func TestBadTTL(t *testing.T) {
	srv := New(store.New())

	rec := do(t, srv.Handler(), http.MethodPut, "/v1/keys/name?ttl=invalid", "Rohan")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestMissingKeyIs404(t *testing.T) {
	srv := New(store.New())

	rec := do(t, srv.Handler(), http.MethodGet, "/v1/keys/missing", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHealthAndReadiness(t *testing.T) {
	srv := New(store.New())

	health := do(t, srv.Handler(), http.MethodGet, "/healthz", "")
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", health.Code, http.StatusOK)
	}

	ready := do(t, srv.Handler(), http.MethodGet, "/readyz", "")
	if ready.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want %d", ready.Code, http.StatusOK)
	}

	srv.SetReady(false)

	notReady := do(t, srv.Handler(), http.MethodGet, "/readyz", "")
	if notReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("not-ready status = %d, want %d", notReady.Code, http.StatusServiceUnavailable)
	}
}

func TestOversizedValueRejected(t *testing.T) {
	srv := New(store.New())

	body := strings.Repeat("a", maxValueBytes+1)
	rec := do(t, srv.Handler(), http.MethodPut, "/v1/keys/large", body)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestListKeys(t *testing.T) {
	srv := New(store.New())
	handler := srv.Handler()

	do(t, handler, http.MethodPut, "/v1/keys/name", "Rohan")
	do(t, handler, http.MethodPut, "/v1/keys/city", "Dehradun")

	rec := do(t, handler, http.MethodGet, "/v1/keys", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	body := rec.Body.String()

	if !strings.Contains(body, `"name"`) {
		t.Fatalf("response does not contain name key: %s", body)
	}

	if !strings.Contains(body, `"city"`) {
		t.Fatalf("response does not contain city key: %s", body)
	}
}

func TestListKeysEmptyStore(t *testing.T) {
	srv := New(store.New())

	rec := do(t, srv.Handler(), http.MethodGet, "/v1/keys", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if !strings.Contains(rec.Body.String(), `"keys":[]`) {
		t.Fatalf("expected empty keys array, got: %s", rec.Body.String())
	}
}

// TestMetricsEndpoint verifies that /metrics returns HTTP 200 and exposes expvar counters.
func TestMetricsEndpoint(t *testing.T) {
	srv := New(store.New())
	handler := srv.Handler()

	rec := do(t, handler, http.MethodGet, "/metrics", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "requests_total") {
		t.Fatalf("metrics body does not contain requests_total: %s", body)
	}
}

// TestReadinessTransitions verifies toggling readiness between ready and not-ready states.
func TestReadinessTransitions(t *testing.T) {
	srv := New(store.New())
	handler := srv.Handler()

	// Initial state: ready
	rec1 := do(t, handler, http.MethodGet, "/readyz", "")
	if rec1.Code != http.StatusOK {
		t.Fatalf("initial readyz status = %d, want %d", rec1.Code, http.StatusOK)
	}

	// Transition to shutdown: failing readiness
	srv.SetReady(false)
	rec2 := do(t, handler, http.MethodGet, "/readyz", "")
	if rec2.Code != http.StatusServiceUnavailable {
		t.Fatalf("after SetReady(false) status = %d, want %d", rec2.Code, http.StatusServiceUnavailable)
	}

	// Liveness must remain healthy even when readiness is false
	recHealth := do(t, handler, http.MethodGet, "/healthz", "")
	if recHealth.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want %d", recHealth.Code, http.StatusOK)
	}

	// Transition back to ready (recovery scenario)
	srv.SetReady(true)
	rec3 := do(t, handler, http.MethodGet, "/readyz", "")
	if rec3.Code != http.StatusOK {
		t.Fatalf("after SetReady(true) status = %d, want %d", rec3.Code, http.StatusOK)
	}
}
