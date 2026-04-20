package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alkev/gl_take_home/internal/store"
)

func newTestHandler(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	s := store.New(3, 4, 0)
	h := NewRouter(s, slog.New(slog.NewTextHandler(io.Discard, nil)), 1<<20, 100)
	return h, s
}

func TestPostVectorsHappyPath(t *testing.T) {
	h, s := newTestHandler(t)
	body := `{"embeddings":[{"label":"king","data":[1,0,0]},{"label":"queen","data":[0.9,0.1,0]}]}`
	req := httptest.NewRequest("POST", "/vectors", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if s.Len() != 2 {
		t.Fatalf("store Len = %d, want 2", s.Len())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["inserted"] != float64(2) {
		t.Fatalf("inserted = %v", out["inserted"])
	}
}

func TestPostVectorsBadDimension(t *testing.T) {
	h, _ := newTestHandler(t)
	body := `{"embeddings":[{"label":"x","data":[1,2]}]}`
	req := httptest.NewRequest("POST", "/vectors", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("status = %d, want 400, body = %s", rr.Code, rr.Body.String())
	}
}

func TestPostVectorsMalformedJSON(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest("POST", "/vectors", strings.NewReader("{not-json"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestPostVectorsOversizedBody(t *testing.T) {
	s := store.New(3, 4, 0)
	h := NewRouter(s, slog.New(slog.NewTextHandler(io.Discard, nil)), 100, 100)
	big := bytes.Repeat([]byte("x"), 500)
	req := httptest.NewRequest("POST", "/vectors", bytes.NewReader(big))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestGetByWordAndUUID(t *testing.T) {
	h, s := newTestHandler(t)
	id, _ := s.InsertOne("King", []float32{1, 0, 0})
	req := httptest.NewRequest("GET", "/vector?word=king", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("by word: %d %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest("GET", "/vector?uuid="+id.String(), nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("by uuid: %d %s", rr.Code, rr.Body.String())
	}
}

func TestGetMissingParam(t *testing.T) {
	h, _ := newTestHandler(t)
	for _, q := range []string{"", "?word=x&uuid=00000000-0000-0000-0000-000000000000"} {
		req := httptest.NewRequest("GET", "/vector"+q, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != 400 {
			t.Fatalf("q=%q: status = %d, want 400", q, rr.Code)
		}
	}
}

func TestGetNotFound(t *testing.T) {
	h, s := newTestHandler(t)
	// Populate so the store is non-empty; absence of the queried key then
	// maps to 404 (not the empty-store 503).
	_, _ = s.InsertOne("present", []float32{1, 0, 0})
	req := httptest.NewRequest("GET", "/vector?word=missing", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestGetEmptyStoreReturns503(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/vector?word=anything", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 503 {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestCompareEmptyStoreReturns503(t *testing.T) {
	h, _ := newTestHandler(t)
	// Valid-looking uuids so we reach the empty-store check, not a 400.
	req := httptest.NewRequest("GET",
		"/compare/cosine_similarity?uuid1=00000000-0000-0000-0000-000000000001"+
			"&uuid2=00000000-0000-0000-0000-000000000002", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 503 {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestCompareCosine(t *testing.T) {
	h, s := newTestHandler(t)
	a, _ := s.InsertOne("a", []float32{1, 0, 0})
	b, _ := s.InsertOne("b", []float32{0, 1, 0})
	url := "/compare/cosine_similarity?uuid1=" + a.String() + "&uuid2=" + b.String()
	req := httptest.NewRequest("GET", url, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestCompareUnknownMetric(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/compare/jaccard?uuid1=a&uuid2=b", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestNearestStoreEmpty(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/nearest?word=x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 503 {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestNearestHappy(t *testing.T) {
	h, s := newTestHandler(t)
	_, _ = s.InsertOne("q", []float32{1, 0, 0})
	_, _ = s.InsertOne("near", []float32{0.9, 0.1, 0})
	_, _ = s.InsertOne("far", []float32{0, 1, 0})
	req := httptest.NewRequest("GET", "/nearest?word=q&k=2", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["query"] != "q" {
		t.Fatalf("query echoed wrong")
	}
}

func TestHealth(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
}
