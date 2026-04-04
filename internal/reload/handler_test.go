package reload

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSwappableHandler(t *testing.T) {
	h1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("handler1"))
	})
	h2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("handler2"))
	})

	sh := NewSwappableHandler(h1)

	// Should serve h1 initially.
	rec := httptest.NewRecorder()
	sh.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "handler1" {
		t.Fatalf("expected handler1, got %q", rec.Body.String())
	}

	// Swap to h2.
	sh.Swap(h2)
	rec = httptest.NewRecorder()
	sh.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "handler2" {
		t.Fatalf("expected handler2, got %q", rec.Body.String())
	}
}
