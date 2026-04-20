package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestProgressWriterCountsBytes(t *testing.T) {
	pw := newProgressWriter("x", 0)
	if _, err := pw.Write(make([]byte, 1234)); err != nil {
		t.Fatal(err)
	}
	if pw.written != 1234 {
		t.Fatalf("written = %d, want 1234", pw.written)
	}
}

func TestTeeToProgressPassesBytesThrough(t *testing.T) {
	src := strings.NewReader("hello world")
	pw := newProgressWriter("x", int64(src.Len()))
	var sink bytes.Buffer
	n, err := io.Copy(&sink, teeToProgress(src, pw))
	if err != nil {
		t.Fatal(err)
	}
	if n != 11 || sink.String() != "hello world" {
		t.Fatalf("tee dropped bytes: n=%d, body=%q", n, sink.String())
	}
	if pw.written != 11 {
		t.Fatalf("progress counted %d, want 11", pw.written)
	}
}
