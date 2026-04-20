# GL-AI-simple-vecstore — Assignment Brief

## 1. Context & Motivation

Our AI application stack requires a fast, self-contained knowledge retrieval service. Modern AI applications — retrieval-augmented generation (RAG), semantic search, recommendation, and question answering — depend on finding knowledge that is *semantically relevant* to a query, not just keyword-matched. The bottleneck today is retrieval latency: we need to search across hundreds of thousands of embeddings and return semantically relevant results in under 10ms.

We are bringing you in to design and deliver this service from the ground up. You have freedom over language, framework, and internal design. What is fixed is the API contract, the acceptance criteria, and the deliverables. We expect the judgment and experience of a senior engineer to be reflected in every decision you make.

## 2. The Assignment

You will build a production-grade, high-performance **in-memory knowledge retrieval service** exposed as a REST/JSON API. The service stores dense vector embeddings and serves semantic search queries at low latency.

### In Scope

- In-memory vector store with insert and retrieval
- Semantic search: given a query embedding, return the most semantically similar embeddings in the store
- Pairwise cosine similarity
- REST/JSON API suitable for integration into any AI application stack
- Initialization and validation against the GloVe Wikipedia + Gigaword 100d embedding dataset

### Out of Scope

- Distributed or sharded deployments (single-node only)
- Authentication and authorization (delegate to the API gateway layer)
- Full-text or metadata filtering
- Approximate semantic search

## 3. Reference Dataset

You will initialize and validate the system using **GloVe Wikipedia + Gigaword 100d** — 1.2 million pre-trained English word embeddings at 100 dimensions, produced by the Stanford NLP Group from a 2024 corpus of Wikipedia and Gigaword 5 (11.9 billion tokens).

| Property      | Value                                                 |
|---------------|-------------------------------------------------------|
| Project page  | nlp.stanford.edu/projects/glove                       |
| Archive       | `glove.2024.wikigiga.100d.zip`                        |
| Vectors       | 1,200,000                                             |
| Dimension     | 100                                                   |
| Corpus        | 2024 Wikipedia + Gigaword 5 (11.9B tokens, uncased)   |
| Download size | 560 MB                                                |
| Format        | Space-delimited text; each line: `word f1 f2 ... f100`|

This dataset is a realistic knowledge base: each vector encodes the semantic meaning of an English word as learned from large-scale text. At 1.2 million vectors it exercises the system at meaningful production scale, and its semantic structure allows correctness verification — the most similar words to `king` should be thematically related words.

The word token on each line should be stored alongside the embedding as a human-readable label. `VECTOR_DIMENSION` must be set to `100`.

## 4. Configuration

The service must be configurable entirely via environment variables.

| Variable           | Default | Description                                |
|--------------------|---------|--------------------------------------------|
| `PORT`             | `8888`  | HTTP listening port                        |
| `VECTOR_DIMENSION` | `100`   | Fixed dimension for all stored embeddings  |
| `LOG_LEVEL`        | `info`  | `debug`, `info`, `warn`, `error`           |

## 6. API Contract

The following API is fixed. All request and response bodies are `application/json`. Error handling and error response design are left to the solution.

Base URL: `http://host:PORT`

### 6.1 Insert Embeddings

`POST /vectors`

Store one or more embeddings. UUIDs are auto-generated and returned.

**Request:**

```json
{
  "embeddings": [
    { "label": "king", "data": [0.418, 0.245, -0.131, "..."] },
    { "label": "queen", "data": [0.382, 0.271, -0.112, "..."] }
  ]
}
```

**Response 201 Created:**

```json
{
  "inserted": 2,
  "embeddings": [
    { "uuid": "550e8400-e29b-41d4-a716-446655440000", "label": "king", "dimension": 100, "data": [0.418, 0.245, -0.131, "..."] },
    { "uuid": "661f9511-f3ac-52e5-b827-557766551111", "label": "queen", "dimension": 100, "data": [0.382, 0.271, -0.112, "..."] }
  ]
}
```

### 6.2 Retrieve Embedding

`GET /vector?word={word}` or `GET /vector?uuid={uuid}`

Exactly one of `word` or `uuid` must be provided.

**Response 200 OK:**

```json
{
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "label": "king",
  "dimension": 100,
  "data": [0.418, 0.245, -0.131, "..."]
}
```

### 6.3 Semantic Similarity

`GET /compare/{metric}?uuid1={uuid1}&uuid2={uuid2}`

Compute a pairwise similarity or distance between two stored embeddings.

| Metric path                  | Description                 |
|------------------------------|-----------------------------|
| `/compare/cosine_similarity` | Cosine similarity ∈ [-1, 1] |

**Response 200 OK:**

```json
{
  "metric": "cosine_similarity",
  "uuid1": "...",
  "uuid2": "...",
  "result": 0.9231
}
```

### 6.4 Semantic Search

`GET /nearest?word={word}&k={k}`

The core operation. Given a query word, return the `k` most semantically similar words in the store, ordered from most to least similar.

| Parameter | Type    | Default | Description                 |
|-----------|---------|---------|-----------------------------|
| `word`    | string  | —       | Query word                  |
| `k`       | integer | `1`     | Number of results to return |

**Response 200 OK:**

```json
{
  "query": "king",
  "results": [
    {
      "uuid": "...",
      "label": "queen",
      "dimension": 100,
      "data": [...],
      "distance": 0.043
    }
  ]
}
```

Results are ordered from most to least similar.

## 7. Acceptance Criteria

### 7.1 Correctness

| Criterion             | Requirement                                                                                                               |
|-----------------------|---------------------------------------------------------------------------------------------------------------------------|
| UUID uniqueness       | No two stored embeddings share a UUID                                                                                     |
| Dimension enforcement | Requests with wrong dimension are rejected with 400                                                                       |
| Read-your-writes      | A successfully inserted embedding is immediately visible to `GET` and `/nearest`                                          |
| Search correctness    | `/nearest` returns the exact top-k most similar results, not approximate results                                          |
| Semantic correctness  | For a sample of 100 word queries, the top-1 result is a semantically plausible word (manual spot-check; pass/fail)        |

### 7.2 Reliability

| Criterion        | Requirement                                                                       |
|------------------|-----------------------------------------------------------------------------------|
| Bad input safety | Malformed JSON, oversized payloads, and invalid values produce 400, never a crash |

## 8. Deliverables

The following must all be present in the repository at the time of delivery review.

| Artifact            | Description                                                                                                                        |
|---------------------|------------------------------------------------------------------------------------------------------------------------------------|
| Service source code | Full implementation of the API and in-memory store                                                                                 |
| Data model          | Definition of the embedding record structure, field types, constraints, and any invariants the API relies on                       |
| Loader script/tool  | Downloads the GloVe Wikipedia + Gigaword 100d dataset, parses it, and bulk-inserts all 1.2 million embeddings into a running instance |

### Bonus

| Artifact             | Description                                                                                                                                                                                     |
|----------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Dockerfile`         | Multi-stage build; slim runtime image; non-root user; HEALTHCHECK on `/health`                                                                                                                  |
| `docker-compose.yml` | Local dev setup with sensible defaults                                                                                                                                                          |
| Benchmark report     | Results for all performance benchmarks below, with hardware and OS specifications                                                                                                               |
| Persistence          | Snapshot endpoint (`POST /snapshot`), periodic auto-snapshot, and restore from snapshot on startup. Snapshots must be written atomically. Configuration via `SNAPSHOT_PATH` and `SNAPSHOT_INTERVAL` environment variables. |

### Performance Benchmarks

All benchmarks are run against a fully loaded GloVe Wikipedia + Gigaword 100d store (1,200,000 vectors, `VECTOR_DIMENSION=100`). You must document the hardware and OS the benchmarks were run on.

| Benchmark           | Target                                                        |
|---------------------|---------------------------------------------------------------|
| Bulk load           | Report vectors/second loading all 1.2M embeddings             |
| GET latency         | p50 < 0.5ms, p99 < 1ms under no concurrency                   |
| GET throughput      | ≥ 10,000 requests/second sustained with 32 concurrent clients |
| Search latency k=1  | p99 < 10ms                                                    |
| Search latency k=10 | Report p50 and p99                                            |
| Memory footprint    | Report RSS after full load                                    |

## 9. Technical Standards

These standards apply to all code delivered under this engagement.

- **Logging:** structured JSON to stdout; every request logged with method, path, status, and latency
- **Error responses:** all errors return `{ "error": "...", "code": <http_status> }`
- **HTTP status codes:** follow the table below; no deviations

| Scenario                         | Code |
|----------------------------------|------|
| Successful read / retrieve       | 200  |
| Successful insert                | 201  |
| Bad request / validation error   | 400  |
| Resource not found               | 404  |
| Method not allowed               | 405  |
| Internal server error            | 500  |
| Store empty (retrieval on empty) | 503  |
