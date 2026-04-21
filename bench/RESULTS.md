# Benchmark results

**Hardware:** Apple M1 Pro — 10 cores (8 performance + 2 efficiency), 32 GB unified memory
**OS:** macOS 26.2 (Tahoe), build 25C56
**Go version:** go1.26.2 darwin/arm64
**Dataset:** GloVe Wikipedia + Gigaword 100d, 1,291,147 vectors
**Bench environment:** native `./bin/vecstore` on the host. Docker-based runs (under `docker compose up`) are also collected in [`results-in-docker/`](results-in-docker/), but see the [environment note](#environment-note-native-vs-docker) below.

## Summary

| Benchmark                  | Target            | Achieved                           | Status  |
|----------------------------|-------------------|------------------------------------|---------|
| Bulk load                  | report vec/s      | 60,209 vec/s                       | —       |
| GET latency p50            | < 0.5 ms          | 86 µs                              | ✓ PASS  |
| GET latency p99            | < 1 ms            | 192 µs                             | ✓ PASS  |
| GET throughput (32 workers)| ≥ 10,000 RPS      | 10,000 RPS sustained, 100 % 2xx    | ✓ PASS  |
| Search latency k=1 p99     | < 10 ms           | 31 ms                              | ✗ FAIL  |
| Search latency k=10        | report p50 / p99  | p50 19.8 ms, p99 27.2 ms           | —       |
| Memory footprint (RSS)     | report            | 1.30 GB                            | —       |

Detailed numbers and raw outputs: [`results-on-host/`](results-on-host/).

## Detailed results

### Bulk load

1,291,147 vectors ingested in 21.444 s → **60,209 vec/s**.
Loader defaults: 8 concurrent workers, batch size 1000, 3 retries with linear backoff (`attempt × 250 ms`).

### `GET /vector` — single-client latency

Queue-depth 1, 60 s. Vegeta config: `-rate=0 -max-workers=1`. File: [`get_latency_summary.txt`](results-on-host/get_latency_summary.txt).

| Metric         | Value                  |
|----------------|------------------------|
| Requests       | 654,906                |
| p50            | 86 µs                  |
| p95            | 126 µs                 |
| p99            | 192 µs                 |
| max            | 62.6 ms (1 outlier)    |

Both spec targets (p50 < 500 µs, p99 < 1 ms) met with ~5× headroom.

### `GET /vector` — throughput under concurrent load

10,000 RPS × 60 s × 32 workers. File: [`get_throughput.txt`](results-on-host/get_throughput.txt).

| Metric   | Value                  |
|----------|------------------------|
| Target   | 10,000 RPS             |
| Sustained| 10,000 RPS, 100 % 2xx  |
| p50      | 94 µs                  |
| p95      | 221 µs                 |
| p99      | 2.81 ms                |
| max      | 62.3 ms                |

### `GET /nearest` — single-client latency

Queue-depth 1, 60 s. Files: [`nearest_k1_summary.txt`](results-on-host/nearest_k1_summary.txt), [`nearest_k10_summary.txt`](results-on-host/nearest_k10_summary.txt).

| Metric       | k=1        | k=10       |
|--------------|------------|------------|
| p50          | 19.88 ms   | 19.82 ms   |
| p95          | 23.29 ms   | 23.10 ms   |
| p99          | 31.08 ms   | 27.21 ms   |
| max          | 84.88 ms   | 69.62 ms   |
| Throughput   | 49.6 RPS   | 49.9 RPS   |

k barely affects the shape — cost is dominated by the full 520 MB linear scan, not the top-K merge. **k=1 p99 misses the < 10 ms spec target by ~3×.** Profile confirms the workload is memory-bandwidth-bound (83 % of CPU time in `vecmath.Dot`, but mostly stall time waiting on DRAM).

Levers that would close this gap, each with a caveat:

- **Query-result LRU cache** — `(word, k) → results` lookup is a map hit + JSON encode, so a repeat of the same word is O(1) relative to store size. This bench always queries `king`, so a cache would "pass" the spec trivially — but that tells you about the cache, not the search. Kept out of scope because the current harness can't distinguish cache hits from real scan performance.
- **Approximate-NN index (HNSW / IVF / PQ)** — sub-linear, expected p99 well under 1 ms. **Out of scope per brief §2 "Out of Scope" which scopes search as exact.**
- **Reduced bytes per vector (`float16` / `int8` quantisation)** — halves or quarters memory bandwidth demand. Exact (within tiny rounding) for `float16`, deterministic-but-approximate for `int8`. 
- **SIMD `Dot` via `gonum.org/v1/gonum/blas/blas32`** — would pick up 3–5× per-call on amd64 (gonum ships hand-written SSE/AVX assembly in its internal `asm/f32`). Not pursued: the dev box is arm64 Apple Silicon, where gonum has no assembly and its pure-Go fallback benched marginally slower than our current scalar, so a straight swap is a local regression. A build-tag split (amd64 → gonum, arm64 → current scalar) would fix that, but the amd64 path would never run against a test we can exercise from here. Listed for completeness; held on testability.

### `GET /nearest` — saturation probe

Unbounded rate, 64 concurrent clients, 30 s. File: [`nearest_saturation.txt`](results-on-host/nearest_saturation.txt).

| Metric                         | Value    |
|--------------------------------|----------|
| Peak throughput                | 68.3 RPS |
| p50 latency (queue-wait)       | 938 ms   |
| p99 latency (queue-wait)       | 1.74 s   |
| Success ratio                  | 100 %    |

Peak RPS is the number to take here; the latency figures are queue-wait under deliberate overload, not per-query cost.

### Memory footprint

**RSS after full snapshot load: 1.30 GB** (1,332,992 KB from `ps -o rss=`).

The service calls `runtime.GC()` + `debug.FreeOSMemory()` once post-load so the ~524 MB snapshot read buffer is scavenged back to the OS immediately, giving a representative steady-state number rather than a transient `HeapIdle`-inflated one.

Approximate breakdown:

| Component                                 | Size       |
|-------------------------------------------|------------|
| Embeddings slab (1.29 M × 100 × 4 B)      | ~516 MB    |
| Inverse norms                             | ~5 MB      |
| `byLabel` map (1.29 M string→int)         | ~120–150 MB|
| `byUUID` map (1.29 M [16]byte→int)        | ~80–100 MB |
| `meta` slice + label backing              | ~50–70 MB  |
| Go runtime, stacks, residual `HeapIdle`   | ~400 MB    |
| **Total**                                 | **~1.30 GB** |

Roughly 2.5× raw-data ratio — typical for a Go service with two 1.3 M-entry hash maps.

## Environment note: native vs Docker

The headline numbers above are from the native binary on the host. We also ran the same bench against the same service running in Docker (`docker compose up`); raw outputs are in [`results-in-docker/`](results-in-docker/). The two **diverge sharply on the HTTP-heavy benches**, not because the service behaves differently, but because Docker Desktop on macOS proxies all container network traffic through its Linux VM's userspace TCP stack:

| Bench                              | Native on host                  | In Docker on macOS                                                       | Cause                                                                                |
|------------------------------------|---------------------------------|--------------------------------------------------------------------------|--------------------------------------------------------------------------------------|
| `get_throughput` (10 k RPS target) | 10 000 RPS sustained, 100 % 2xx | ~320 RPS sustained, 20 % 2xx, `bind: can't assign requested address`     | macOS ephemeral-port exhaustion + proxy backlog (numbers vary 3–5× run-to-run)       |
| `get_latency` p50                  | 86 µs                           | 297 µs                                                                   | ~200 µs of Docker Desktop proxy overhead per request                                 |
| `nearest_k1` p99                   | 31.08 ms                        | 30.67 ms                                                                 | Unaffected — query is CPU-bound inside the container, network is idle at 48 RPS     |

On Linux hosts Docker's network is genuine loopback and the delta disappears. The native-on-host numbers reflect what a real Linux deployment would see.

## Reproducing

```bash
make build
SNAPSHOT_PATH=./spsh.bin ./bin/vecstore &    # or: docker compose up
./bin/loader --url http://localhost:8888     # skip if snapshot already present
./bench/run.sh
```

Tail latency is sensitive to CPU thermal state, GC-heap churn from previous runs, and background GUI activity. For reviewer reproducibility, run immediately after a reboot with no other heavy applications.

## Raw outputs

- [`results-on-host/`](results-on-host/) — headline native-binary numbers used above.
- [`results-in-docker/`](results-in-docker/) — the same harness against `docker compose up`, kept for the environment-note comparison.

