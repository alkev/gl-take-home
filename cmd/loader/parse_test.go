package main

import (
	"strings"
	"testing"
)

func TestParseLine(t *testing.T) {
	line := "king 0.1 0.2 0.3"
	label, vec, err := parseLine(line, 3)
	if err != nil {
		t.Fatal(err)
	}
	if label != "king" {
		t.Fatalf("label = %q", label)
	}
	if len(vec) != 3 {
		t.Fatalf("len(vec) = %d, want 3", len(vec))
	}
}

func TestParseLineBadDim(t *testing.T) {
	if _, _, err := parseLine("king 1 2", 3); err == nil {
		t.Fatal("expected error")
	}
}

// Regression: GloVe 2024 labels may legitimately contain non-breaking
// space (U+00A0), e.g. "2\u00a01/2" (the mixed fraction "2 1/2"). ASCII
// tools treat NBSP as a label byte; a naive strings.Fields call would
// split on it and fail. The parser must treat only ASCII space/tab as
// separators.
func TestParseLineKeepsNBSPInLabel(t *testing.T) {
	line := "2\u00a01/2 0.1 0.2 0.3"
	label, vec, err := parseLine(line, 3)
	if err != nil {
		t.Fatal(err)
	}
	if label != "2\u00a01/2" {
		t.Fatalf("label = %q, want %q", label, "2\u00a01/2")
	}
	if len(vec) != 3 {
		t.Fatalf("len(vec) = %d, want 3", len(vec))
	}
}

func TestParseLineStripsTrailingCR(t *testing.T) {
	label, _, err := parseLine("king 1 2 3\r", 3)
	if err != nil {
		t.Fatal(err)
	}
	if label != "king" {
		t.Fatalf("label = %q", label)
	}
}

func TestStreamLines(t *testing.T) {
	input := "a 1 2 3\nb 4 5 6\n"
	r := strings.NewReader(input)
	got := []string{}
	err := streamLines(r, 3, func(label string, vec []float32) error {
		got = append(got, label)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
}
