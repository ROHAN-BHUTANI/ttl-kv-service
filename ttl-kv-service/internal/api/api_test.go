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
	h := New(store.New()).Handler()

	if rec := do(t, h, "PUT", "/v1/keys/greeting", "hello"); rec.Code != http.StatusNoContent {
		t.Fatalf("put: %d", rec.Code)
	}
	rec := do(t, h, "GET", "/v1/keys/greeting", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("get: %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "DELETE", "/v1/keys/greeting", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/keys/greeting", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: %d", rec.Code)
	}
}

func TestBadTTL(t *testing.T) {
	h := New(store.New()).Handler()
	for _, q := range []string{"abc", "-5"} {
		if rec := do(t, h, "PUT", "/v1/keys/k?ttl="+q, "v"); rec.Code != http.StatusBadRequest {
			t.Fatalf("ttl=%s: got %d, want 400", q, rec.Code)
		}
	}
}

func TestMissingKeyIs404(t *testing.T) {
	h := New(store.New()).Handler()
	if rec := do(t, h, "GET", "/v1/keys/nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/v1/keys/nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestHealthAndReadiness(t *testing.T) {
	srv := New(store.New())
	h := srv.Handler()
	if rec := do(t, h, "GET", "/healthz", ""); rec.Code != http.StatusOK {
		t.Fatalf("healthz: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/readyz", ""); rec.Code != http.StatusOK {
		t.Fatalf("readyz: %d", rec.Code)
	}
	srv.SetReady(false)
	if rec := do(t, h, "GET", "/readyz", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz while draining: %d", rec.Code)
	}
}

func TestOversizedValueRejected(t *testing.T) {
	h := New(store.New()).Handler()
	big := strings.Repeat("x", maxValueBytes+1)
	if rec := do(t, h, "PUT", "/v1/keys/big", big); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
}
