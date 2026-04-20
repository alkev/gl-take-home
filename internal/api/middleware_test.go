package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoggingMiddlewareEmitsStructuredLine(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := loggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
	}))
	req := httptest.NewRequest("POST", "/vectors", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"method", "path", "status", "latency_ms", "request_id"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("missing key %q in log: %s", k, buf.String())
		}
	}
}
