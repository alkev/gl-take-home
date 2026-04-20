# Project Rules for Claude

## Git

**Never commit or push unless explicitly asked** — skip any auto-commit step a skill suggests and leave the working tree for review. Read-only inspection (`git status`, `git diff`, `git log`) is always fine.

## Markdown tables

Pad cells with spaces so pipes align vertically across header, separator, and every data row; separator dashes must match column widths. Unpadded tables (`| a | b |`) render fine but are painful to edit — don't emit them. Re-align the whole table when editing one row.

## Libraries and dependencies

**Verify any third-party library with `context7` before adding** it (`mcp__context7__resolve-library-id`, `mcp__context7__query-docs`) — confirm it's maintained, check the current API surface (training data may be stale), and decide whether the stdlib or an existing dep already suffices. Prefer stdlib; every added dependency is a future maintenance and security liability. Justify each one.

## Commands

Default to the Makefile — targets wrap the right flags (`-race`, etc.). `make` with no args prints help.

| Target                    | What it does                                                                    |
|---------------------------|---------------------------------------------------------------------------------|
| `make build`              | Compile `bin/vecstore` (service), `bin/loader`, `bin/semcheck`                  |
| `make test`               | `go test -race ./...` — unit + HTTP-contract + end-to-end                       |
| `make bench`              | Go micro-benchmarks (`testing.B`); distinct from vegeta load                    |
| `make run`                | Build + start the service on `:8888`                                            |
| `make docker`             | Build the multi-stage `vecstore:dev` image                                      |
| `make load-test`          | `bench/run.sh` — vegeta attacks against a running service (needs vegeta, curl)  |
| `make semcheck`           | 100-query semantic plausibility spot-check against a loaded service             |
| `make lint` / `make fmt`  | `golangci-lint run` / `fmt ./...` — dev-only, no-op if the linter is missing    |
| `make clean`              | Remove `bin/`                                                                   |

Drop to `go` directly only when a target doesn't fit: `go build ./...`, `go test -race -run TestFoo ./pkg`, `go mod tidy`, `go vet ./...`.

**Full end-to-end flow** (only on explicit request — needs the extracted GloVe file in the repo root):

```bash
make build
./bin/vecstore &
./bin/loader   --url http://localhost:8888
./bin/semcheck -url http://localhost:8888
./bench/run.sh
```

**Reviewers need only** `go` (+ optional `docker`, `vegeta`, `curl`). **Dev-only:** `golangci-lint`, `goimports` (invoked by `make lint` / `make fmt`, which no-op when absent). `README.md` is the reviewer entrypoint.
