package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alkev/gl_take_home/internal/api"
	"github.com/alkev/gl_take_home/internal/store"
)

func TestEndToEnd(t *testing.T) {
	s := store.New(3, 4, 0)
	h := api.NewRouter(s, slog.New(slog.NewTextHandler(io.Discard, nil)), 1<<20, 100)
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Insert a small batch.
	body := `{"embeddings":[
		{"label":"king","data":[1,0,0]},
		{"label":"queen","data":[0.95,0.05,0]},
		{"label":"broccoli","data":[0,1,0]}
	]}`
	resp, err := http.Post(srv.URL+"/vectors", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /vectors: %d %s", resp.StatusCode, b)
	}
	_ = resp.Body.Close()

	// GET by word (case-insensitive).
	resp, err = http.Get(srv.URL + "/vector?word=KING")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("GET /vector?word=KING: err=%v status=%d", err, resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Nearest: expect queen as top-1 neighbour of king, not broccoli.
	resp, err = http.Get(srv.URL + "/nearest?word=king&k=1")
	if err != nil || resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("nearest: err=%v status=%d body=%s", err, resp.StatusCode, b)
	}
	var out struct {
		Query   string `json:"query"`
		Results []struct {
			Label    string  `json:"label"`
			Distance float32 `json:"distance"`
		} `json:"results"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	_ = resp.Body.Close()
	if len(out.Results) != 1 || out.Results[0].Label != "queen" {
		t.Fatalf("expected queen as top-1 neighbour of king, got %+v", out)
	}

	// /snapshot without SNAPSHOT_PATH → 400.
	resp, err = http.Post(srv.URL+"/snapshot", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("snapshot without path should be 400, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}
