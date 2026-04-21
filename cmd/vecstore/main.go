// Command vecstore runs the in-memory vector store HTTP service.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/alkev/gl_take_home/internal/api"
	"github.com/alkev/gl_take_home/internal/config"
	"github.com/alkev/gl_take_home/internal/store"
)

func main() {
	cfg, err := config.Parse(os.Getenv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	logger, logCloser := newLogger(cfg.LogLevel)
	defer func() { _ = logCloser.Close() }()

	s := store.New(cfg.VectorDimension, cfg.ChunkSize, cfg.InitialCapacity)

	if cfg.SnapshotPath != "" {
		if _, statErr := os.Stat(cfg.SnapshotPath); statErr == nil {
			start := time.Now()
			if err := s.Load(cfg.SnapshotPath); err != nil {
				logger.Warn("snapshot load failed; starting empty",
					slog.String("path", cfg.SnapshotPath),
					slog.Any("err", err))
			} else {
				// Snapshot Load mmap'd / buffered the whole file (~520 MB)
				// then discarded it. Force a GC + scavenge so that transient
				// heap doesn't stay resident as HeapIdle for minutes. Brings
				// post-load RSS close to the true steady-state floor.
				runtime.GC()
				debug.FreeOSMemory()
				logger.Info("snapshot loaded",
					slog.String("path", cfg.SnapshotPath),
					slog.Int("vectors", s.Len()),
					slog.Int64("duration_ms", time.Since(start).Milliseconds()))
			}
		}
	}

	snapshotFn := makeSnapshotFn(cfg, s, logger)

	mux := api.NewRouterWithSnapshot(s, logger, cfg.MaxRequestBytes, cfg.MaxBatchSize, snapshotFn)
	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if snapshotFn != nil && cfg.SnapshotInterval > 0 {
		go runPeriodicSnapshot(ctx, cfg.SnapshotInterval, snapshotFn, logger)
	}

	go func() {
		logger.Info("listening",
			slog.Int("port", cfg.Port),
			slog.Int("vectors_loaded", s.Len()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", slog.Any("err", err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown initiated")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Warn("shutdown error", slog.Any("err", err))
	}
	if snapshotFn != nil {
		if _, _, err := snapshotFn(); err != nil {
			logger.Warn("final snapshot failed", slog.Any("err", err))
		}
	}
	logger.Info("goodbye")
}

func makeSnapshotFn(cfg *config.Config, s *store.Store, logger *slog.Logger) func() (string, int64, error) {
	if cfg.SnapshotPath == "" {
		return nil
	}
	return func() (string, int64, error) {
		start := time.Now()
		if err := s.Save(cfg.SnapshotPath); err != nil {
			return cfg.SnapshotPath, 0, err
		}
		fi, err := os.Stat(cfg.SnapshotPath)
		var size int64
		if err == nil {
			size = fi.Size()
		}
		logger.Info("snapshot written",
			slog.String("path", cfg.SnapshotPath),
			slog.Int64("bytes", size),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()))
		return cfg.SnapshotPath, size, nil
	}
}

func runPeriodicSnapshot(ctx context.Context, interval time.Duration, fn func() (string, int64, error), logger *slog.Logger) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var inFlight atomic.Bool
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if !inFlight.CompareAndSwap(false, true) {
				continue
			}
			go func() {
				defer inFlight.Store(false)
				if _, _, err := fn(); err != nil {
					logger.Warn("periodic snapshot failed", slog.Any("err", err))
				}
			}()
		}
	}
}

func newLogger(level string) (*slog.Logger, io.Closer) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	aw := newAsyncWriter(os.Stdout, 4096, 64*1024, 50*time.Millisecond)
	return slog.New(slog.NewJSONHandler(aw, &slog.HandlerOptions{Level: lvl})), aw
}
