package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFixture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queries.txt")
	content := "# comment\n\nking\tqueen,prince,monarch\ncat dog,kitten\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	qs, err := loadFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 2 {
		t.Fatalf("got %d queries, want 2", len(qs))
	}
	if qs[0].word != "king" || qs[0].expected[0] != "queen" {
		t.Fatalf("first query wrong: %+v", qs[0])
	}
	if qs[1].word != "cat" || qs[1].expected[1] != "kitten" {
		t.Fatalf("second query wrong: %+v", qs[1])
	}
}

func TestRunOnePassTop1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nearest" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(nearestResp{
			Query: r.URL.Query().Get("word"),
			Results: []struct {
				Label string `json:"label"`
			}{{Label: "queen"}, {Label: "prince"}},
		})
	}))
	defer srv.Close()
	client := &http.Client{Timeout: time.Second}
	got := runOne(client, srv.URL, query{word: "king", expected: []string{"queen", "prince"}}, 5)
	if !got.pass {
		t.Fatalf("expected pass, got %+v", got)
	}
	if got.hit != "queen" {
		t.Fatalf("hit = %q, want queen", got.hit)
	}
}

func TestRunOnePassWithinTopK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(nearestResp{
			Query: r.URL.Query().Get("word"),
			Results: []struct {
				Label string `json:"label"`
			}{{Label: "really"}, {Label: "feel"}, {Label: "glad"}},
		})
	}))
	defer srv.Close()
	client := &http.Client{Timeout: time.Second}
	got := runOne(client, srv.URL, query{word: "happy", expected: []string{"glad", "joyful"}}, 5)
	if !got.pass {
		t.Fatalf("expected pass (glad in top-3), got %+v", got)
	}
	if got.hit != "glad" {
		t.Fatalf("hit = %q, want glad", got.hit)
	}
}

func TestRunOneFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(nearestResp{
			Query: "king",
			Results: []struct {
				Label string `json:"label"`
			}{{Label: "broccoli"}, {Label: "asparagus"}},
		})
	}))
	defer srv.Close()
	client := &http.Client{Timeout: time.Second}
	got := runOne(client, srv.URL, query{word: "king", expected: []string{"queen"}}, 5)
	if got.pass {
		t.Fatalf("expected fail, got pass")
	}
	if got.note != "none of top-K in expected set" {
		t.Fatalf("note = %q", got.note)
	}
}
