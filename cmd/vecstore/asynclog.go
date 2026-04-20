package main

import (
	"bufio"
	"io"
	"sync"
	"time"
)

// logBufPool reuses per-record byte buffers between asyncWriter.Write and
// the drain goroutine. Amortizes the per-log-line allocation to near zero.
// Stores *[]byte (not []byte) so Put/Get don't box the slice header.
var logBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 512)
		return &b
	},
}

// asyncWriter buffers writes to an underlying io.Writer and flushes them
// from a background goroutine. Safe for concurrent Write calls. Write never
// invokes the underlying Writer directly, so callers on the hot path avoid
// syscall latency at the cost of best-effort delivery on crash.
type asyncWriter struct {
	ch chan *[]byte
	wg sync.WaitGroup
}

func newAsyncWriter(out io.Writer, queueSize, bufSize int, flushInterval time.Duration) *asyncWriter {
	a := &asyncWriter{ch: make(chan *[]byte, queueSize)}
	a.wg.Add(1)
	go a.drain(out, bufSize, flushInterval)
	return a
}

func (a *asyncWriter) drain(out io.Writer, bufSize int, flushInterval time.Duration) {
	defer a.wg.Done()
	bw := bufio.NewWriterSize(out, bufSize)
	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		select {
		case bp, ok := <-a.ch:
			if !ok {
				_ = bw.Flush()
				return
			}
			_, _ = bw.Write(*bp)
			*bp = (*bp)[:0]
			logBufPool.Put(bp)
		case <-t.C:
			_ = bw.Flush()
		}
	}
}

// Write copies p (slog reuses its buffer after Write returns) into a pooled
// buffer and enqueues it. Blocks if the queue is full — back-pressure is
// preferable to silent log loss under sustained overload.
func (a *asyncWriter) Write(p []byte) (int, error) {
	bp := logBufPool.Get().(*[]byte)
	*bp = append((*bp)[:0], p...)
	a.ch <- bp
	return len(p), nil
}

// Close drains all queued writes and flushes the underlying writer.
func (a *asyncWriter) Close() error {
	close(a.ch)
	a.wg.Wait()
	return nil
}
