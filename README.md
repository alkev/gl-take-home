# vecstore

In-memory vector store with cosine-similarity search, exposed as a REST/JSON API. Built for the GL-AI-simple-vecstore take-home. See `docs/assignment.md` for the original brief.

**Benchmark results:** [`bench/RESULTS.md`](bench/RESULTS.md) — headline numbers (Apple M1 Pro, 1.29 M vectors): `GET /vector` p99 192 µs, throughput 10 k RPS sustained, `/nearest` p99 31 ms, RSS 1.30 GB.

## Requirements

- **Go 1.23+** — only hard requirement. macOS: `brew install go`. Other platforms: https://go.dev/dl/.
- **Docker** *(optional)* — for the containerized build (`make docker`).
- **vegeta, curl** *(optional)* — for reproducing the load benchmark (`make load-test`). macOS: `brew install vegeta` (`curl` ships with the OS). Linux: `apt install curl`; vegeta binaries at https://github.com/tsenart/vegeta/releases.
- **golangci-lint** *(optional, dev-only)* — only needed if you want to run `make lint` / `make fmt`; both targets no-op gracefully if the binary is missing.

## Quickstart

```bash
make build           # compiles ./bin/vecstore, ./bin/loader, ./bin/semcheck
make test            # unit + HTTP contract + end-to-end tests with -race
./bin/vecstore &     # starts the service on :8888
```

Populate with GloVe and query:

```bash
curl http://localhost:8888/health
./bin/loader
curl 'http://localhost:8888/nearest?word=king&k=5'
```

Sanity-check with the 100-query semantic harness against the loaded service:

```bash
./bin/semcheck -url http://localhost:8888
```

## Configuration

All via environment variables. Full list and defaults in `docs/vecstore-design.md` §11; most common ones:

| Variable            | Default | Purpose                                    |
|---------------------|---------|--------------------------------------------|
| `PORT`              | `8888`  | HTTP listening port                        |
| `VECTOR_DIMENSION`  | `100`   | Fixed dimension for stored embeddings      |
| `LOG_LEVEL`         | `info`  | `debug` / `info` / `warn` / `error`        |
| `SNAPSHOT_PATH`     | *unset* | If set, enables persistence                |
| `SNAPSHOT_INTERVAL` | `300s`  | Between auto-snapshots                     |

## Docker

```bash
make docker                  # build the image (multi-stage, distroless runtime)
docker compose up            # run with a named volume for snapshot persistence
```

## Benchmarks

```bash
make bench                   # Go micro-benchmarks
make load-test               # vegeta attacks; requires a running service on :8888
```

Results on the reference machine: `bench/RESULTS.md`.

## API

Fixed per the assignment brief. Summary:

| Method + path                                 | Purpose                                  |
|-----------------------------------------------|------------------------------------------|
| `POST /vectors`                               | Bulk insert embeddings; returns UUIDs    |
| `GET  /vector?word={}` / `?uuid={}`           | Retrieve a single embedding              |
| `GET  /compare/cosine_similarity?uuid1&uuid2` | Pairwise cosine similarity               |
| `GET  /nearest?word={}&k={}`                  | Top-k most similar embeddings            |
| `POST /snapshot`                              | Trigger a synchronous snapshot           |
| `GET  /health`                                | Service liveness                         |

Full contract in `docs/assignment.md` §6.

## Documentation

| File                         | What's in it                                                    |
|------------------------------|-----------------------------------------------------------------|
| `docs/assignment.md`         | Original take-home brief (converted from the provided PDF)     |
| `docs/assumptions.md`        | All assumptions grouped by topic, with rationale                |
| `docs/vecstore-design.md`    | Full design doc: architecture, data model, search, persistence  |
| `docs/optimizations-*.md`    | Session-by-session performance changes with before/after        |
| `bench/RESULTS.md`           | Benchmark report with hardware/OS specifications                |
