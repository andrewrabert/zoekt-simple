package reload

import (
	"net/http"
	"sync/atomic"
)

// SwappableHandler is an http.Handler that delegates to an inner handler
// which can be atomically replaced at runtime.
type SwappableHandler struct {
	handler atomic.Value // stores http.Handler
}

// NewSwappableHandler creates a SwappableHandler with the given initial handler.
func NewSwappableHandler(h http.Handler) *SwappableHandler {
	sh := &SwappableHandler{}
	sh.handler.Store(h)
	return sh
}

// Swap replaces the inner handler atomically.
func (sh *SwappableHandler) Swap(h http.Handler) {
	sh.handler.Store(h)
}

// ServeHTTP delegates to the current inner handler.
func (sh *SwappableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sh.handler.Load().(http.Handler).ServeHTTP(w, r)
}
