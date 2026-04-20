# Session optimizations — 2026-04-20

Three rounds of work landed during this session. Round one tightened the semcheck / bench harnesses so they measure what they claim to measure; rounds two and three attacked hot-path tail latency on `GET /vector`.

## Code changes

### 1. Scoped access for `GET /vector`

**Problem.** `store.readLocked` allocated a fresh `[]float32` and `copy`'d the row into it on every call (400 B/req for the 100-dim GloVe corpus). At 10 k RPS that's ~4 MB/s of short-lived garbage feeding GC pressure, and the stop-the-world pauses surfaced directly in p99.

**Change.** Added `Store.WithByLabel` and `Store.WithByUUID` in `internal/store/get.go`. They acquire the `RLock`, build an `Embedding` whose `Data` is a direct view of the row inside the chunk slab (no copy), and invoke a caller-supplied closure while the lock is held. `handleGet` now passes its JSON-encode closure into these methods, so the lock releases only after the response has been written to the socket.

**Safety.** Chunks are allocated once and never moved (`insert.appendRow` appends only; `chunks.rowSlice` returns a stable view into an immutable slab). The only mutation path is `Store.Load`, which takes the write lock and replaces `s.chunks` wholesale — the `RLock` held across the handler blocks it, so a reader can never observe a torn chunk.

**Trade-off.** The `RLock` is held across JSON encode + socket write. Concurrent readers don't contend with each other (RWMutex), so throughput is unaffected; only `Insert` / `Load` queue behind in-flight reads, which is fine for the query-heavy workload.

**Files.** `internal/store/get.go`, `internal/api/handlers.go`.

### 2. Async buffered logger

**Problem.** `slog` was configured with `slog.NewJSONHandler(os.Stdout, …)`. Every request log line triggered a synchronous stdout `Write` syscall on the request-handling goroutine. At 10 k RPS that's 10 k syscalls/sec on the critical path — a direct contributor to tail-latency spikes.

**Change.** New `cmd/vecstore/asynclog.go` wraps any `io.Writer` with a channel-buffered background flusher:

- a `chan *[]byte` queue (capacity 4096),
- one drain goroutine that writes into a 64 KB `bufio.Writer` and flushes every 50 ms (or when the buffer fills),
- `Write` copies the record into a pooled buffer and enqueues the pointer — the hot path never hits a syscall,
- `Close` drains and flushes, wired to a `defer` in `main` so final log lines aren't dropped on SIGINT.

**Files.** `cmd/vecstore/asynclog.go`, `cmd/vecstore/main.go`.

### 3. `sync.Pool` for log byte buffers

**Problem.** The first cut of the async writer did `make([]byte, len(p)) + copy` on every `Write`. At 10 k RPS × ~200 B/record that's ~2 MB/s of new garbage — GC pressure moved, not eliminated. Under single-client load the allocation was visible as a fresh ~1 % tail cluster in the 10–50 ms band.

**Change.** `logBufPool` (`sync.Pool` of `*[]byte`). `Write` borrows a buffer, resets length with `append((*bp)[:0], p...)`, and sends the pointer through the channel. The drain goroutine writes `*bp`, resets length again, and `Put`s it back. Storing `*[]byte` (not `[]byte`) avoids the slice-header boxing that `sync.Pool.Put` otherwise incurs — staticcheck SA6002.

**Files.** `cmd/vecstore/asynclog.go`.

## Bench harness

Several harness changes landed alongside the code. None are performance changes — they correct what we measure so the before/after numbers below are comparable:

- Dropped a broken warmup (`vegeta attack -rate=0 -duration=0 … | head -c 0` — `head -c 0` is an illegal byte count on BSD `head`, and `rate=0 duration=0` is unbounded in vegeta). Replaced with a single 3 s / 500 RPS warmup at the top of the script covering both `/vector` and `/nearest`.
- Switched GET benches from `uuid=$SAMPLE` to `word=king`. Drops the `curl | jq` preamble and measures a more realistic client path. `jq` removed from documented requirements.
- Corrected the nearest-latency rate from 2000 RPS (well above the measured saturation of ~63 RPS, so every sample queued for seconds) to 100 RPS. Added a separate uncapped saturation probe (`-rate=0 -max-workers=64 -duration=30s`) to measure peak RPS.
- Extended histogram buckets (`10 ms`, `50 ms`, `100 ms`, `500 ms`) so the tail is visible instead of collapsed into one `+Inf` overflow.
- Text summaries (`*_summary.txt`) written alongside histograms; a final `spec_summary.txt` prints the `Latencies` row from the queue-depth-1 bench against the target thresholds.

## Measured impact

`GET /vector` throughput bench (10 k RPS × 60 s, 32 vegeta workers — the concurrent hot path):

| Percentile | Baseline  | After all changes | Improvement |
|------------|-----------|-------------------|-------------|
| mean       | 2.305 ms  | 0.293 ms          | 7.9×        |
| p50        | 103.7 µs  | 88.6 µs           | 1.2×        |
| p90        | 177.2 µs  | 143.1 µs          | 1.2×        |
| p95        | 946.3 µs  | 225.1 µs          | 4.2×        |
| p99        | 70.58 ms  | 6.45 ms           | 11×         |
| max        | 345.9 ms  | 66.96 ms          | 5.2×        |

The win concentrates in the tail, which is exactly where the work targeted: stdout syscalls + short-lived response-copy garbage were the two biggest sources of GC / scheduler stalls. p50 moved modestly because the median was already cache-hot and not GC-bound.

`GET /nearest` is **unchanged** by the above work — its bottleneck is memory bandwidth in the brute-force scan over ~520 MB of embeddings, not response copying or logging. Peak `/nearest` throughput remains ~63 RPS per the saturation probe.

## Open items

- Single-client p99 at 5 k RPS is still ~5 ms — above the 1 ms "no concurrency" spec target. The remaining tail is `json.NewEncoder` (reflection-based) plus `slog`'s own per-record allocations. Next lever: hand-rolled float32 encoder using `strconv.AppendFloat` into a pooled `[]byte`.
- `/nearest` optimization path: (a) cap per-request worker fan-out with a semaphore so concurrent requests don't thrash the scheduler, (b) choose a smaller worker count for small `k`, (c) introduce an approximate-nearest-neighbour index (HNSW or IVF) for sub-linear search.
