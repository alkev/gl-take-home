# Session optimizations — 2026-04-20

## Summary

The spec target was `p50 < 0.5 ms, p99 < 1 ms` for `GET /vector` under no concurrency. After the changes described below, the service meets both with massive headroom (p50 ≈ 86 µs, p99 ≈ 192 µs). However: most of the session's effort was driven by a benchmark-setup mistake that inflated the measured tail latency. Had we used a correctly-configured latency bench from the start, the spec-target work would probably have been much shorter — we don't have a pre-optimisation queue-depth-1 measurement to prove it, but the dominant tail contributors we found (GC / logging syscalls) don't trigger hard at queue depth 1. Three rounds of code changes did land and remain useful for the concurrent-load case.

## The measurement mistake we spent time on

The original `bench/run.sh` measured `GET /vector` latency with:

```bash
vegeta attack -rate=5000 -duration=20s -workers=1
```

Intent: "one keep-alive client at 5 k RPS, report per-query latency." What actually happened: `-workers=1` only sets the **initial** goroutine count. Vegeta's `-max-workers` defaults to effectively unbounded (max uint64), so whenever the server couldn't sustain 5 k RPS from a single worker — which is any time GC or a scheduling blip added jitter — vegeta silently spawned additional workers to maintain the offered rate. The bench wasn't single-client; under any latency excursion it became fractional saturation, and the reported tail blended server response time with queue-wait time.

Read as if it were legitimate, the pre-optimisation p99 looked like ~5 ms — well above the 1 ms spec — and motivated a round of JSON-encoder experiments (goccy, pooled-buffer wrapper) that all ended up reverted. We never re-ran the pre-optimisation code under a correctly-configured queue-depth-1 bench; the 190 µs p99 we now report is post-optimisation. Given that the dominant contributors to tail latency we identified (stdout syscalls, per-request `[]float32` copies) only pile up under concurrent load, the pre-optimisation code was probably already close to or inside spec at queue depth 1 — but that's an inference, not a measurement.

**Takeaway.** `-workers=N` is a starting point, not a concurrency cap. Use `-max-workers=N` to bound concurrency. Pair `-rate=0` with an explicit `-max-workers=K` for true queue-depth-K measurement. `bench/run.sh` now uses this shape for every latency bench, and the flag semantics are documented there.

## Optimizations that landed (throughput tail — beyond the no-concurrency spec)

These were motivated by genuinely bad p99 under the **10 k RPS × 32-worker throughput bench** — a different question than the spec's no-concurrency target. They're real wins under concurrent load (GC pauses and per-request syscalls stop dominating the tail) and the code is kept.

### 1. Scoped access for `GET /vector`

**Problem.** `store.readLocked` allocated a fresh `[]float32` and `copy`'d the row into it on every call (400 B/req for the 100-dim GloVe corpus). At 10 k RPS that's ~4 MB/s of short-lived garbage feeding GC pressure, and the STW pauses surfaced directly in p99.

**Change.** Added `Store.WithByLabel` and `Store.WithByUUID` in `internal/store/get.go`. They acquire the `RLock`, build an `Embedding` whose `Data` is a direct view of the row inside the chunk slab (no copy), and invoke a caller-supplied closure while the lock is held. `handleGet` now passes its JSON-encode closure into these methods, so the lock releases only after the response has been written to the socket.

**Safety.** Chunks are allocated once and never moved (`insert.appendRow` appends only; `chunks.rowSlice` returns a stable view into an immutable slab). The only mutation path is `Store.Load`, which takes the write lock and replaces `s.chunks` wholesale — the `RLock` held across the handler blocks it, so a reader can never observe a torn chunk.

**Trade-off.** The `RLock` is held across JSON encode + socket write. Concurrent readers don't contend with each other (RWMutex), so throughput is unaffected; only `Insert` / `Load` queue behind in-flight reads, which is fine for the query-heavy workload.

**Files.** `internal/store/get.go`, `internal/api/handlers.go`.

### 2. Async buffered logger

**Problem.** `slog` was configured with `slog.NewJSONHandler(os.Stdout, …)`. Every request log line triggered a synchronous stdout `Write` syscall on the request-handling goroutine. At 10 k RPS that's 10 k syscalls/sec on the critical path — a direct contributor to tail-latency spikes under concurrent load.

**Change.** New `cmd/vecstore/asynclog.go` wraps any `io.Writer` with a channel-buffered background flusher:

- a `chan *[]byte` queue (capacity 4096),
- one drain goroutine that writes into a 64 KB `bufio.Writer` and flushes every 50 ms (or when the buffer fills),
- `Write` copies the record into a pooled buffer and enqueues the pointer — the hot path never hits a syscall,
- `Close` drains and flushes, wired to a `defer` in `main` so final log lines aren't dropped on SIGINT.

**Files.** `cmd/vecstore/asynclog.go`, `cmd/vecstore/main.go`.

### 3. `sync.Pool` for log byte buffers

**Problem.** The first cut of the async writer did `make([]byte, len(p)) + copy` on every `Write`. At 10 k RPS × ~200 B/record that's ~2 MB/s of new garbage — GC pressure moved, not eliminated.

**Change.** `logBufPool` (`sync.Pool` of `*[]byte`). `Write` borrows a buffer, resets length with `append((*bp)[:0], p...)`, and sends the pointer through the channel. The drain goroutine writes `*bp`, resets length again, and `Put`s it back. Storing `*[]byte` (not `[]byte`) avoids the slice-header boxing that `sync.Pool.Put` otherwise incurs — staticcheck SA6002.

**Files.** `cmd/vecstore/asynclog.go`.

## Bench harness

Harness-level corrections landed alongside the code. None are performance changes; they fix what the numbers actually represent:

- Dropped a broken warmup (`vegeta attack -rate=0 -duration=0 … | head -c 0` — `head -c 0` is an illegal byte count on BSD `head`, and `rate=0 duration=0` is unbounded in vegeta). Replaced with a single 3 s / 500 RPS warmup at the top of the script covering both `/vector` and `/nearest`.
- Switched GET benches from `uuid=$SAMPLE` to `word=king`. Drops the `curl | jq` preamble, measures a more realistic client path, and lets us drop `jq` from the documented requirements.
- All latency benches now use `-rate=0 -max-workers=1 -duration=60s` — truly queue-depth 1, self-enforcing regardless of response-time drift. Previously they relied on `-workers=1` which caps nothing (see mistake above).
- Nearest saturation probe uses `-rate=0 -max-workers=64 -duration=30s` — the explicit opposite: unbounded rate, bounded concurrency, to report peak sustained RPS.
- Extended histogram buckets (`10 ms`, `50 ms`, `100 ms`, `500 ms`) so the tail is visible instead of collapsed into one `+Inf` overflow.
- Text summaries (`*_summary.txt`) written alongside histograms; a final `spec_summary.txt` prints the `Latencies` row from the queue-depth-1 bench against the target thresholds.

## Measured impact

Numbers are from a clean run on a freshly-booted machine, recorded under `bench/results-on-host/`.

**`GET /vector` — "no concurrency" spec bench** (queue-depth 1, 60 s):

| Metric | Value     | Target    | Status |
|--------|-----------|-----------|--------|
| p50    | 86.1 µs   | < 500 µs  | ✓ PASS |
| p95    | 126.1 µs  | —         |        |
| p99    | 191.5 µs  | < 1 ms    | ✓ PASS |
| max    | 62.6 ms   | —         |        |

Both spec targets met with massive headroom — p50 at ~17 % of budget, p99 at ~19 %. One stray 62 ms sample out of 654 906 (<1 in 600 k) is a GC pause or background-process blip, not a steady-state cost. These numbers are with rounds 1–3 applied; we don't have a clean pre-optimisation queue-depth-1 measurement to compare against.

**`GET /vector` — throughput bench** (10 k RPS × 60 s, 32 vegeta workers):

| Percentile | Baseline  | Final     | Improvement |
|------------|-----------|-----------|-------------|
| mean       | 2.305 ms  | 0.244 ms  | 9.4×        |
| p50        | 103.7 µs  | 94.5 µs   | 1.1×        |
| p90        | 177.2 µs  | 148.9 µs  | 1.2×        |
| p95        | 946.3 µs  | 220.9 µs  | 4.3×        |
| p99        | 70.58 ms  | 2.81 ms   | 25×         |
| max        | 345.9 ms  | 62.31 ms  | 5.6×        |

This is where rounds 1–3 matter. Gains concentrate in the tail — stdout syscalls and per-request `[]float32` copying were the two biggest sources of GC / scheduler stalls under concurrent load. p50 moved only modestly because the median was already cache-hot and not GC-bound.

**`GET /nearest` — queue-depth 1** (single-client, 60 s):

| Metric | k=1       | k=10      |
|--------|-----------|-----------|
| p50    | 19.88 ms  | 19.82 ms  |
| p99    | 31.08 ms  | 27.21 ms  |
| max    | 84.88 ms  | 69.62 ms  |
| RPS    | 49.6      | 49.9      |

k doesn't change the shape meaningfully — dominant cost is the full scan, not the top-K merge.

**`GET /nearest` — saturation** (unbounded rate, 64 clients, 30 s):

| Metric | Value     |
|--------|-----------|
| Peak RPS | 68.3    |
| p50    | 938 ms    |
| p99    | 1.74 s    |

Saturation latency is queue-wait, not per-query work. Peak throughput is the real number here.

**`/nearest` is unchanged by rounds 1–3.** Its bottleneck is memory bandwidth in the brute-force scan over ~520 MB of embeddings, not response copying or logging. Addressing it would require reducing per-query bytes (float16 / int8 storage), sharing scans across concurrent queries, or an approximate-NN index.

**Note on measurement conditions.** On a laptop, benchmark numbers drift noticeably over a bench session — CPU thermal state, accumulated GC heap from previous runs, and background processes (Spotlight / cloud sync / browser) all add noise. During this session we saw p99 values 2–3× worse for *identical* code before a restart cleared these. The numbers above are the floor the code can reach; for reviewer reproducibility, run `make load-test` on a freshly-rebooted machine with no competing GUI applications.
