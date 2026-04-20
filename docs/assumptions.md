# Assumptions

Things we took as given — either stated in the brief, or chosen by us where the brief was silent. Grouped by topic.

## 1. API semantics the brief leaves open

| #    | Assumption                                                                                         |
|------|----------------------------------------------------------------------------------------------------|
| 1.1  | Word lookups (`/vector?word=`, `/nearest?word=`) are case-insensitive; GloVe itself is uncased.    |
| 1.2  | Duplicate labels are rejected at insert with 400 — word-keyed lookups must be deterministic.       |
| 1.3  | The query word is excluded from `/nearest` results (otherwise `k=1` always returns the query).     |
| 1.4  | `distance = 1 − cosine_similarity` in `/nearest`. Range `[0, 2]`, smaller = more similar.          |
| 1.5  | `POST /vectors` is all-or-nothing — any validation failure rejects the whole batch.                |
| 1.6  | Response `data` echoes the caller's original (non-normalized) values.                              |
| 1.7  | UUIDs are RFC 9562 v4, generated server-side.                                                      |
| 1.8  | Zero vectors rejected at insert with 400 (cosine is undefined).                                    |
| 1.9  | NaN / Inf floats rejected at insert with 400.                                                      |
| 1.10 | `k` in `/nearest` must satisfy `1 ≤ k ≤ n-1` (query itself is excluded).                           |
| 1.11 | `/compare/{metric}` with any metric other than `cosine_similarity` → 404 (path not matched).       |
| 1.12 | `GET /vector` with neither or both of `word` / `uuid` → 400.                                       |

## 2. Storage

| #   | Assumption                                                                                                                                    |
|-----|-----------------------------------------------------------------------------------------------------------------------------------------------|
| 2.1 | Vectors stored as `float32`. Halves memory vs `float64`; GloVe's precision doesn't need 64-bit.                                               |
| 2.2 | Rows live in a chunked slab (`[][]float32`, default `CHUNK_SIZE=16384`). Contiguous scan, no 520 MB memmoves on growth.                       |
| 2.3 | `invNorms[i] = 1/‖row_i‖` cached per row as `float32`. Lets us echo original `data` while the search hot loop costs one extra scalar mul.     |
| 2.4 | Rows are never mutated after insert. Callers can return direct slice views without copying — `store.WithByLabel` relies on this.              |
| 2.5 | A single `sync.RWMutex` on the store is sufficient. Read-heavy workload; inserts are rare (loader only).                                      |

## 3. Performance-target interpretation

| #   | Assumption                                                                                                                                                                       |
|-----|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 3.1 | "GET latency p50 < 0.5 ms, p99 < 1 ms under no concurrency" is measured against `GET /vector` with a single client at queue depth 1 — separate from "search latency" in brief §8. |
| 3.2 | Reported latencies are end-to-end wall-clock from the client (vegeta), not server-side only — matches real-caller experience.                                                     |
| 3.3 | Hardware / OS used for benchmarks is documented in `bench/RESULTS.md` (mandated by brief §8).                                                                                     |

## 4. Loader

| #   | Assumption                                                                                                                                      |
|-----|-------------------------------------------------------------------------------------------------------------------------------------------------|
| 4.1 | Loader is a separate CLI (`cmd/loader`) that hits `POST /vectors` on the running service — brief says "bulk-inserts into a running instance".   |
| 4.2 | Loader streams the zipped archive line-by-line; never decompresses the full 560 MB into RAM.                                                    |
| 4.3 | Failed batches retry with exponential backoff before the loader exits non-zero.                                                                 |
| 4.4 | GloVe archive format is `<word> <f1> ... <f100>\n`, UTF-8, no header — per Stanford NLP docs.                                                   |

## 5. Persistence (bonus)

| #   | Assumption                                                                                                                                |
|-----|-------------------------------------------------------------------------------------------------------------------------------------------|
| 5.1 | Snapshots use a custom binary format (design §9). JSON at 520 MB of floats is tens of seconds; binary is disk-bandwidth bound.            |
| 5.2 | Snapshots store original vectors only; inverse norms are recomputed at load (derivable; no reason to persist and risk format drift).      |
| 5.3 | Snapshots are written atomically via `<path>.tmp` + `fsync` + `rename` (POSIX-guaranteed).                                                |
| 5.4 | A corrupt snapshot (bad magic / version / CRC32C) makes the service start empty and log the error — not crash.                            |
| 5.5 | `SNAPSHOT_PATH` unset disables snapshotting entirely; opt-in.                                                                             |
| 5.6 | Only the current snapshot is kept (no rolling or timestamped copies).                                                                     |
| 5.7 | Periodic snapshots skip if the previous one is still in flight.                                                                           |

## 6. Observability

| #   | Assumption                                                                                                                                        |
|-----|---------------------------------------------------------------------------------------------------------------------------------------------------|
| 6.1 | Structured JSON logs to stdout, one line per request, via `log/slog`. Writes are async-buffered off the request path (see `docs/optimizations-*`).|
| 6.2 | `X-Request-ID` is honored when supplied; otherwise a UUID v4 is generated server-side.                                                            |
| 6.3 | No Prometheus / OTel metrics endpoint — out of scope for this brief.                                                                              |

## 7. Testing

| #   | Assumption                                                                                                                                                    |
|-----|---------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 7.1 | Semantic correctness is a pass/fail harness (`cmd/semcheck`) over ~100 query words; PASS if *any* of the returned top-5 is in the expected set — GloVe's neighbourhood is broader than a single handwritten top-1 answer. |
| 7.2 | Benchmarks (bulk load, GET / search latency, throughput, memory) are run manually on the documented machine — brief §8 requires hardware-specific reporting. |
| 7.3 | Persistence round-trip test: insert → snapshot → restart → verify identical UUIDs and vectors.                                                                |

## 8. Explicitly out of scope

- Approximate-NN indexing (HNSW / IVF / PQ) — per brief §2.
- DELETE / PATCH / bulk-replace semantics — not in the API contract.
- Metadata filtering, multi-tenancy, TLS termination — gateway's job.
- Rate limiting / quotas beyond `http.MaxBytesReader` — gateway's job.
