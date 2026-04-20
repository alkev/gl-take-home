package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// statusCapturer wraps http.ResponseWriter to observe status and byte counts.
type statusCapturer struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusCapturer) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
func (s *statusCapturer) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = 200
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// loggingMiddleware logs one JSON line per request with method, path, status,
// latency_ms, and request_id.
func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = uuid.NewString()
			}
			w.Header().Set("X-Request-ID", reqID)
			start := time.Now()
			sw := &statusCapturer{ResponseWriter: w}
			next.ServeHTTP(sw, r)
			logger.Info("request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000.0),
				slog.String("request_id", reqID),
				slog.Int("bytes_out", sw.bytes),
			)
		})
	}
}

// recoverMiddleware converts panics in handlers into 500 responses with the
// standard error body.
func recoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic", slog.Any("err", rec), slog.String("path", r.URL.Path))
					writeError(w, http.StatusInternalServerError, "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// bodyLimitMiddleware wraps the request body with http.MaxBytesReader so that
// oversized payloads fail cleanly with 400 before a handler reads them.
func bodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
