package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

// progressWriter is an io.Writer that counts bytes it sees and prints a
// throttled one-line progress update to stderr. Intended to be used with
// io.MultiWriter so the bytes are also written to the real destination.
type progressWriter struct {
	total    int64 // 0 when Content-Length is unknown
	written  int64
	label    string
	lastLog  time.Time
	interval time.Duration
}

func newProgressWriter(label string, total int64) *progressWriter {
	return &progressWriter{
		label:    label,
		total:    total,
		interval: 500 * time.Millisecond,
	}
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n := len(b)
	p.written += int64(n)
	if time.Since(p.lastLog) >= p.interval {
		p.print()
		p.lastLog = time.Now()
	}
	return n, nil
}

// Done prints a final line with the total bytes transferred and a newline
// so subsequent output starts on its own line.
func (p *progressWriter) Done() {
	p.print()
	fmt.Fprintln(os.Stderr)
}

func (p *progressWriter) print() {
	w := float64(p.written) / (1 << 20) // MiB
	if p.total > 0 {
		t := float64(p.total) / (1 << 20)
		pct := 100 * float64(p.written) / float64(p.total)
		fmt.Fprintf(os.Stderr, "\r%s: %.1f / %.1f MiB (%.1f%%)    ", p.label, w, t, pct)
	} else {
		fmt.Fprintf(os.Stderr, "\r%s: %.1f MiB    ", p.label, w)
	}
}

// teeToProgress returns an io.Reader that passes through r while accumulating
// progress in pw. Wrapping the reader rather than the writer lets us capture
// stream-based sources (HTTP response bodies) without modifying the destination.
func teeToProgress(r io.Reader, pw *progressWriter) io.Reader {
	return io.TeeReader(r, pw)
}
