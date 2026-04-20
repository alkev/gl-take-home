package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/alkev/gl_take_home/internal/store"
)

// NewRouter wires handlers + middleware into a single http.Handler without
// snapshot support.
func NewRouter(s *store.Store, logger *slog.Logger, maxBody int64, maxBatch int) http.Handler {
	return NewRouterWithSnapshot(s, logger, maxBody, maxBatch, nil)
}

// NewRouterWithSnapshot is like NewRouter but also accepts a snapshot callback.
func NewRouterWithSnapshot(
	s *store.Store,
	logger *slog.Logger,
	maxBody int64,
	maxBatch int,
	snapshotFn func() (string, int64, error),
) http.Handler {
	h := &handlers{
		store:        s,
		startTime:    time.Now(),
		snapshotFn:   snapshotFn,
		maxBatchSize: maxBatch,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /vectors", h.handleInsert)
	mux.HandleFunc("GET /vector", h.handleGet)
	mux.HandleFunc("GET /compare/{metric}", h.handleCompare)
	mux.HandleFunc("GET /nearest", h.handleNearest)
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("POST /snapshot", h.handleSnapshot)

	var handler http.Handler = mux
	handler = bodyLimitMiddleware(maxBody)(handler)
	handler = recoverMiddleware(logger)(handler)
	handler = loggingMiddleware(logger)(handler)
	return handler
}
