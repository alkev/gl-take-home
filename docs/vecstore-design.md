# GL-AI-simple-vecstore — Design

**Assignment brief:** [`assignment.md`](assignment.md)

## 1. Goals

Deliver a single-node, in-memory, production-grade vector store that serves the fixed REST API in the brief, beats the stated performance targets with headroom, and includes all bonus artifacts (Docker, compose, benchmark report, persistence).

**Non-goals:** distributed operation, approximate search, auth, metadata filtering.

## 2. Language & runtime

**Go.** Rationale:

- Standard-library HTTP server is production-grade and sub-millisecond on simple routes.
- Goroutines + no GIL allow the search hot loop to scale across cores for parallel top-k.
- Scalar float32 loops in Go don't auto-SIMD on arm64 today, but on our workload the scan is close enough to the memory-bandwidth ceiling that compute-side wins (BLAS, NEON asm) would buy only 1.5–2× wall-clock. Kept hermetic; no cgo/BLAS.
- Fast build → small, scratch-based deployable image for the bonus.

Alternatives considered: Rust (better raw perf, longer dev time), Node/TS (viable but needs BLAS binding for the hot path), Python (GIL fights the GET latency targets).

## 3. System overview

A single binary `vecstore` holds all embeddings in memory, serves the REST API, and persists via atomic binary snapshots. A separate CLI `loader` downloads GloVe and bulk-inserts via `POST /vectors`.

```
┌──────────────┐   POST /vectors    ┌──────────────────┐
│  loader CLI  │ ─────────────────▶ │                  │
└──────────────┘                     │                  │
                                     │   vecstore       │
┌──────────────┐   GET /nearest      │   (Go binary)    │
│   client     │ ─────────────────▶ │                  │
└──────────────┘                     │                  │
                                     └────────┬─────────┘
                                              │ atomic write
                                              ▼
                                         snapshot.bin
```

## 4. Repository layout

```
cmd/
  vecstore/         main.go          # service entrypoint
                    asynclog.go      # channel-buffered async writer wrapping stdout
  loader/           main.go          # loader CLI
                    upload.go        # batch upload with retries
  semcheck/         main.go          # 100-query semantic-plausibility harness
internal/
  config/           config.go        # env var parsing + validation
  store/            store.go         # in-memory store, public API
                    chunks.go        # chunked slab indexing
                    search.go        # parallel top-k
                    insert.go        # InsertOne / InsertBatch
                    get.go           # GetByLabel / GetByUUID + scoped WithByLabel/WithByUUID
                    compare.go       # pairwise cosine
                    heap.go          # small-k top-k via min-heap
                    snapshot.go      # persistence (Save/Load with dirty flag)
                    errors.go        # sentinel errors
  api/              router.go        # stdlib http.ServeMux wiring + middleware
                    handlers.go      # HTTP handlers
                    requests.go      # request types
                    responses.go     # response types
                    errors.go        # error response helper (writeError)
                    middleware.go    # structured JSON logging, recover, body limit
  vecmath/          vecmath.go       # Norm, Dot, InvNorm
testdata/
  semantic_queries.txt               # fixture for semcheck (word → plausible set)
bench/
  RESULTS.md                         # benchmark report
  run.sh                             # driver script running all scenarios
  raw/                               # vegeta binary results (for replay/plot)
  results-on-host/                   # text reports from native bin run
  results-in-docker/                 # text reports from docker compose run
Dockerfile
docker-compose.yml
docs/
  assignment.md                      # original take-home brief
  assumptions.md                     # assumptions and interpretations
  vecstore-design.md                 # this file
  optimizations-1.md                 # perf optimisation retrospective
```

## 5. Data model

### 5.1 Public record

The record surfaced in responses:

```go
type Embedding struct {
    UUID      uuid.UUID `json:"uuid"`
    Label     string    `json:"label"`      // original case preserved
    Dimension int       `json:"dimension"`
    Data      []float32 `json:"data"`       // length == Dimension; original values as submitted
}
```

### 5.2 Internal store

```go
type Store struct {
    dim       int
    chunkSize int                   // rows per chunk, default 16384 (power of 2 for fast div/mod)
    chunks    [][]float32           // each chunk = chunkSize * dim float32s; ORIGINAL values
    invNorms  []float32             // invNorms[i] = 1 / ‖row_i‖; pre-computed at insert
    meta      []rowMeta             // meta[i] is row i's UUID + original-case label
    byUUID    map[uuid.UUID]int     // UUID → row
    byLabel   map[string]int        // lowercased label → row
    n         int                   // active row count
    mu        sync.RWMutex
}

type rowMeta struct {
    UUID  uuid.UUID
    Label string  // preserves original case for responses
}
```

### 5.3 Invariants

1. `meta[i].UUID` is unique across `i ∈ [0, n)`.
2. `strings.ToLower(meta[i].Label)` is unique across `i ∈ [0, n)`.
3. `byUUID[meta[i].UUID] == i` and `byLabel[strings.ToLower(meta[i].Label)] == i`.
4. Stored vectors are kept as **original values** the caller submitted. In parallel we cache `invNorms[i] = 1 / ‖row_i‖` so cosine similarity is computed as `dot(qNorm, row_i) * invNorms[i]` — one extra scalar multiply per row compared to fully-pre-normalized storage.
5. `len(chunks[c])` is always `chunkSize * dim` for every chunk `c`. Row `i` occupies `chunks[i / chunkSize][(i % chunkSize) * dim : ((i % chunkSize) + 1) * dim]`.
6. `dim == VECTOR_DIMENSION` at process startup; mismatched inserts are rejected with 400.

### 5.4 Storage layout rationale

**Chunked slab** (`[][]float32` with fixed-size inner slices) gives us:

- Contiguous memory per chunk → CPU prefetcher walks 100-dim rows sequentially during search.
- Growth without copy: exceeding a chunk appends one new ~6.4 MB chunk (16384 × 100 × 4 bytes) — never a 480 MB `memmove`.
- Predictable memory growth (GC-friendly, no transient 1 GB peaks during resize).

Hash maps (`byUUID`, `byLabel`) are **not** used for storing vectors — they index into the chunked slab. This keeps the search hot path cache-friendly while giving O(1) lookup by UUID and case-folded label.

## 6. Search algorithm

We store original (non-normalized) vectors plus a cached `invNorms[i] = 1/‖row_i‖` per row. Because the query word is itself a stored vector (looked up by label), its inverse norm is already cached as `invNorms[q_row]` — we never recompute it.

**Key observation:** `invNorms[q_row]` is constant across all rows for a single query, so it doesn't affect top-k ranking. We can drop it from the hot loop entirely and apply it only to the `k` results we keep. The scan's ranking key is:

```
scoreKey_i = dot(q, row_i) * invNorms[i]     // proportional to cosine
```

Algorithm for `GET /nearest?word=w&k=k`:

1. `q_row = byLabel[ToLower(w)]`; load raw `q` vector and cached `invNormQ = invNorms[q_row]`.
2. **Partition the `chunks` list across `P = GOMAXPROCS` workers** (not the row range). Each worker gets a contiguous slice of chunks; worst-case load imbalance is one chunk.
3. Each worker iterates **chunk-by-chunk**. For each chunk it resolves the chunk slice once, then sweeps rows 0..rowCount-1 inside that chunk computing `scoreKey_i = dot(q, row_i) * invNorms[globalRowIdx]`, maintaining a **local** min-heap of size `k` keyed on `scoreKey`. Only the final chunk may have `rowCount < chunkSize` (when `n % chunkSize != 0`); this is hoisted out of the inner loop.
4. Merge local heaps → global top-k (merging P heaps of size k is negligible: P·k·log(P·k) ≈ a few hundred ops at P=8, k=10).
5. Exclude `q_row` itself from the results.
6. For each surviving result, convert the scoreKey to true cosine: `cosine = scoreKey * invNormQ`, then `distance = 1 - cosine`.
7. Sort most-similar-first and return.

**Why partition by chunks, not by row range:** if we split `[0, n)` into equal-size stripes, stripes cross chunk boundaries and the inner loop pays chunk-index arithmetic (`i/chunkSize`, `i%chunkSize`) on every row. Partitioning by chunks resolves the chunk pointer once per chunk, so the inner loop sees a single contiguous 6.4 MB float slice — the CPU prefetcher walks it perfectly.

**Why local heaps, not a shared one:** a shared heap requires a lock on every push; at 1.2M candidate rows the lock overhead + cache-line bouncing across cores dominates the cost of the scan itself. Local heaps are embarrassingly parallel with zero coordination during the scan and a trivial merge at the end.

**Why we don't normalize the query on the fly:** we already paid for that math at insert time. `invNorms[q_row]` is the cached `1/‖q‖`. Re-normalizing the query each search would duplicate that work and produce a scratch buffer we don't need.

**`GET /compare/cosine_similarity`** uses the same trick: `cosine(a, b) = dot(a, b) * invNorms[a_row] * invNorms[b_row]` — both factors from the cache, zero on-the-fly normalization.

**Why exclude the query word:** otherwise `k=1` always returns the query itself with similarity 1.0, which is useless semantically.

**Why partial heap:** O(N log k) vs O(N log N) full sort. Negligible for k ≤ 10 but matters for larger k.

**Distance field in response:** the brief shows `"distance": 0.043` without defining it. We return `distance = 1 - cosine_similarity` (standard cosine distance, `[0, 2]`, smaller = more similar). Documented in the API reference and OpenAPI spec.

## 7. Concurrency model

Single `sync.RWMutex` on the `Store`:

- `GET /vector`, `/nearest`, `/compare/*`: `RLock`. `handleGet` uses `Store.WithByLabel` / `WithByUUID` — the RLock is held across the handler's JSON encode + socket write so the `Embedding.Data` returned is a direct view of the row (no copy). Readers don't contend with each other, only writers queue.
- `POST /vectors`: `Lock`. Validation happens outside the lock; only the commit of `meta` / `byUUID` / `byLabel` entries + `appendRow` is under lock.
- `Store.Save` (snapshot): holds `RLock` for the serialisation pass (header + label table + vector bytes), then releases before fsync + rename. Short write-lock re-acquisition at the end to clear the `dirty` flag iff `s.n` hasn't grown during the serialise (concurrent inserts simply leave dirty set, so the next Save picks them up).
- `Store.Load`: `Lock` (wipes and rebuilds).

Read-heavy workload → RWMutex is near-zero overhead. No lock-free gymnastics needed.

## 8. API contract details

All error responses: `{"error": "<human message>", "code": <status>}`. Status-code table matches §9 of the brief.

### 8.1 `POST /vectors` → 201

Validation applied to the batch before any mutation:

1. JSON well-formed; body ≤ `MAX_REQUEST_BYTES`.
2. `len(embeddings)` between 1 and `MAX_BATCH_SIZE`.
3. For each embedding: non-empty `label`, `data` is exactly `VECTOR_DIMENSION` floats, all finite (no NaN/Inf).
4. Within the batch: no duplicate labels (case-insensitive).
5. Against the store: no label (case-insensitive) already present.
6. Zero vectors (`‖v‖ == 0`) rejected — cosine similarity is undefined.

All-or-nothing: if any embedding fails, the entire batch is rejected with 400 and a message identifying the first offending item. UUIDs are v4. Response echoes the caller's original `data` as stored.

### 8.2 `GET /vector`

- Exactly one of `word`, `uuid` must be present → 400 otherwise.
- Case-insensitive word lookup via `byLabel`.
- 404 if not found.

### 8.3 `GET /compare/{metric}`

- Only `cosine_similarity` supported initially; unknown metric → 404 (path not matched).
- Both UUIDs must exist → 404 otherwise.
- Same UUID for both → valid (returns 1.0); no special-case.

### 8.4 `GET /nearest`

- `word` required; `k` defaults to 1.
- `k` must satisfy `1 ≤ k ≤ n - 1` (excluding the query itself). `k` larger than that → 400.
- `n == 0` → 503 (store empty).
- Query word not found → 404.

### 8.5 `POST /snapshot` → 200

Synchronous; returns after the snapshot is durable on disk. Body: `{"path": "...", "vectors": N, "bytes": B, "duration_ms": D}`.

### 8.6 `GET /health` → 200

Body: `{"status": "ok", "vectors_loaded": N, "uptime_s": U}`. Used by Docker HEALTHCHECK.

## 9. Persistence

### 9.1 Binary snapshot format

Little-endian throughout.

```
magic          8   bytes   "VECSTORE"
version        4   uint32  = 1
dim            4   uint32
count          8   uint64  N = number of rows
for i in 0..N:
    uuid      16   bytes
    label_len  2   uint16
    label          label_len bytes (UTF-8, original case)
for i in 0..N:
    vector    400  bytes (100 float32, little-endian, original values)
crc32          4   uint32  CRC32C of all preceding bytes
```

Layout rationale: label table and vector table are separate so we can `mmap` or bulk-read vectors as a single contiguous block on restore. `invNorms` are **not** snapshotted — they are recomputed during load (1.2M sqrt+reciprocal operations = ~30 ms).

### 9.2 Write path

1. Acquire `RLock`, copy `meta` into a local slice, snapshot `n` and chunk pointers, release lock.
2. Open `${SNAPSHOT_PATH}.tmp`, buffered write of header + label table + vector bytes + CRC.
3. `f.Sync()`, close.
4. `os.Rename(tmp, final)` — atomic on POSIX.
5. Log size, duration, vector count.

### 9.3 Read path (boot)

1. If `SNAPSHOT_PATH` unset → start empty.
2. If file exists and magic+version+CRC valid → load. Pre-allocate chunks, stream vector bytes directly into them, rebuild `byUUID` / `byLabel`.
3. If corrupt → log error and start empty (don't crash; loader can repopulate).

### 9.4 Periodic snapshots

A background goroutine ticks every `SNAPSHOT_INTERVAL` and triggers §9.2. If a snapshot is already in flight, skip.

## 10. Loader CLI

`cmd/loader` responsibilities:

1. Download `glove.2024.wikigiga.100d.zip` (skipped if already on disk).
2. Stream the zipped archive line by line (no 560 MB decompress into RAM).
3. Parse each line: `word f1 f2 ... f100`.
4. Batch into groups of `BATCH_SIZE` (default 1000) and post to `${VECSTORE_URL}/vectors`.
5. Use a bounded pool of concurrent workers (default 8) to saturate the server.
6. Progress bar: lines parsed, batches sent, errors, vectors/sec.
7. Exit non-zero if any batch fails after N retries.

Flags: `--url`, `--file`, `--dim`, `--batch-size`, `--workers`, `--retries`, `--skip-download` (see `./bin/loader --help`).

## 11. Configuration

| Variable            | Default   | Notes                                       |
|---------------------|-----------|---------------------------------------------|
| `PORT`              | `8888`    |                                             |
| `VECTOR_DIMENSION`  | `100`     | Validated on every insert                   |
| `LOG_LEVEL`         | `info`    | `debug`/`info`/`warn`/`error`               |
| `SNAPSHOT_PATH`     | *(unset)* | Unset disables persistence                  |
| `SNAPSHOT_INTERVAL` | `300s`    | Go `time.Duration` parse                    |
| `INITIAL_CAPACITY`  | `0`       | Hint: pre-allocate chunks to cover N rows   |
| `CHUNK_SIZE`        | `16384`   | Rows per chunk (power of 2 for fast div/mod)|
| `MAX_BATCH_SIZE`    | `10000`   | Embeddings per `POST /vectors`              |
| `MAX_REQUEST_BYTES` | `64MB`    | Request body limit                          |

Config is parsed once at boot; invalid values → fast-fail with a clear error.

## 12. Observability

- Structured JSON logs via `log/slog` to stdout.
- Every request: `method`, `path`, `status`, `latency_ms`, `request_id`, `bytes_out`.
- **Async log writer** (`cmd/vecstore/asynclog.go`): stdout is wrapped in a channel-buffered flusher with a `sync.Pool` of byte buffers. Request-path `slog.Info` never pays a stdout syscall; a drain goroutine batches into a `bufio.Writer` and flushes every 50 ms. Kept throughput p99 under concurrent load from being dominated by 10 k syscalls/s. `Close` called from `defer` in `main` so final lines flush on SIGTERM.
- **Post-load scavenge**: right after a successful `Store.Load`, `runtime.GC()` + `debug.FreeOSMemory()` run once so the ~524 MB snapshot read buffer is returned to the OS immediately instead of sitting in `HeapIdle` for minutes. Makes the reported RSS reflect steady-state.
- `/health` for Docker HEALTHCHECK.

## 13. Error handling

- Central `writeError(w, code, msg)` used by every error path. No ad-hoc `http.Error` calls.
- Middleware `recover`s panics, converts to 500 with a generic message, logs the stack.
- `http.MaxBytesReader` enforces `MAX_REQUEST_BYTES` → malformed/oversized payloads return 400 without reading the full body.
- All validation errors map to 400 with actionable messages (e.g., `"embedding 3: data length 50, want 100"`).

## 14. Testing strategy

| Layer            | Tooling             | Coverage                                                                                                                                                                                                                 |
|------------------|---------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Unit             | `testing`           | vecmath (normalize, dot, edge cases like zero vectors), chunks (indexing, bounds), store invariants                                                                                                                      |
| HTTP contract    | `httptest`          | Every row of the brief's §7 acceptance checklist: UUID uniqueness, dim enforcement, read-your-writes, exact top-k correctness, bad-input safety (malformed JSON, oversized body, invalid floats), status codes, error-body shape |
| Semantic         | `cmd/semcheck` + Go test | 100 prepared queries (king, paris, computer, …) with a curated set of plausible neighbours; PASS if *any* of the returned top-5 is in the expected set — GloVe's neighbourhoods are too broad to pin to a single top-1. Pass/fail table printed by `./bin/semcheck`; exit-code 0 on 100 % pass. |
| Round-trip       | Go test             | Insert → snapshot → restart → GET/nearest yields identical UUIDs and vectors                                                                                                                                             |
| Load / benchmark | `testing.B` + vegeta | Every performance target in the brief's §8. vegeta for HTTP-level load (constant-arrival-rate, HDR histograms); `testing.B` for in-process micro-benchmarks (dot product, heap merge).                                 |

CI runs unit + HTTP contract + round-trip on every commit. Semantic and load benchmarks run on demand.

## 15. Performance plan

Target recap (brief §8): GET p50 < 0.5 ms / p99 < 1 ms; GET ≥ 10k RPS at 32 clients; search k=1 p99 < 10 ms.

### Back-of-envelope

Search hot path: 1.29 M × 100 FMAs = ~130 M ops per query. A modern x86 core with AVX2 FMA (16 FLOPs/cycle × 3.5 GHz = ~56 GFLOPS) does this in ~2 ms. 520 MB / 30 GB/s ≈ 17 ms single-threaded memory-bound. Pre-build estimate was 3–6 ms p99 with 4 cores. Actual measured p99 on arm64 M1 Pro is ~31 ms for `k=1` — Go's scalar loop doesn't auto-SIMD on arm64, and goroutine fan-out across cores buys nothing because the scan is already memory-bandwidth-pinned at ~28 GB/s per query. Misses the <10 ms target; see `bench/RESULTS.md` for the list of levers and why each wasn't pursued.

GET p50 < 0.5 ms: lookup is a single map access + memcpy of 400 bytes of float data + JSON encode of 100 floats. Stdlib `encoding/json` encodes this in ~15-25 µs, Go net/http adds ~30-50 µs. Achievable with 50-70% headroom.

### Micro-optimizations we'll apply

- Log byte-buffers pooled via `sync.Pool` (in the async log writer). Response-buffer pooling was tried and reverted: `encoding/json.Encoder` already pools its own scratch buffer internally, so wrapping it adds a memcpy with no benefit.
- Tight inner loops kept free of interface dispatch.

### Benchmark methodology (`bench/RESULTS.md`)

- Hardware/OS captured at top of report.
- Bulk load: wall-clock time of loader end-to-end → vectors/sec.
- GET latency: single client, 100k sequential requests, per-request wall time, report p50 / p99.
- GET throughput: `vegeta attack -rate=10000 -duration=60s -workers=32` with a fixed target URL; report from the resulting binary via `vegeta report`. Constant-arrival-rate attack is coordinated-omission-safe.
- Search latency k=1 and k=10: queue-depth-1 vegeta attack (`-rate=0 -max-workers=1 -duration=60s`) hitting `/nearest?word=king`; throughput = 1 / mean_latency. Repeating `king` keeps the harness deterministic — a cache would trivially win and not measure the scan.
- Memory: `ps -o rss=` on the running process after full load.

## 16. Design decisions worth flagging for the reviewer

1. **We store original vectors and cache inverse L2 norms.** Responses return the caller's exact `data`. Search cost is one extra scalar multiply per row vs. fully-pre-normalized storage — negligible on modern CPUs. Extra memory: 4 bytes × N = ~4.8 MB for 1.2M rows.

2. **`distance = 1 - cosine_similarity`** in `/nearest` responses. The brief shows `0.043` without defining "distance"; cosine distance is the standard pairing.

3. **The query word is excluded from `/nearest` results.** Without this, `k=1` always returns the query itself at similarity 1.0, which is useless.

4. **One third-party dependency: `google/uuid`.** HTTP routing uses stdlib `net/http` with Go 1.22+ method-aware patterns. JSON uses stdlib `encoding/json` — we experimented with `goccy/go-json` but reverted after it regressed concurrent p99 (its runtime type cache contends under heavy fan-out). A pooled `bytes.Buffer` wrapper around the stdlib encoder was also tried and reverted: `encoding/json.Encoder` already pools its own scratch buffer internally, so adding ours only added a second pool lookup and a memcpy.

## 17. Out-of-scope items explicitly deferred

Per the brief §2:

- Sharding / distribution
- Auth (gateway layer)
- Metadata filtering
- Approximate search
- DELETE endpoint (no API slot)
- Update endpoint (no API slot)

## 18. Risk register

| Risk                                                   | Mitigation                                                                                                                                                              |
|--------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Go GC pauses spike under load                          | Preallocated chunks via `INITIAL_CAPACITY`; hot paths return direct slab views (`Store.WithByLabel`) to avoid per-request row-copy allocs; async log writer with pooled buffers so logging doesn't churn GC |
| Cache pressure from the 480 MB vector set on small VMs | Benchmark on target hardware; document min RSS                                                                                                                          |
| Loader overwhelms the server with concurrency          | Worker pool is bounded; 429 responses trigger backoff (brief doesn't include 429 — we'd use 503 or let the server slow down via lock contention)                        |
| Snapshot corruption during write                       | CRC32C + atomic rename + fall-through to empty on corrupt load                                                                                                          |
| Duplicate-label race during concurrent inserts         | Single write lock serializes insert-time uniqueness check with the commit                                                                                               |
