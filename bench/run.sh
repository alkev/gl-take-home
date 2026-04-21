#!/usr/bin/env bash
# bench/run.sh — run vegeta benchmarks against a running vecstore.
#
# Requires: vegeta, curl (all documented in README.md)
set -euo pipefail

for tool in vegeta curl; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "missing required tool: $tool (see README.md § Requirements)" >&2
        exit 1
    fi
done

BASE_URL=${BASE_URL:-http://localhost:8888}
OUT=bench/results
RAW=bench/raw
mkdir -p "$OUT" "$RAW"

# Sanity-check the sample word is loaded.
if ! curl -sf -o /dev/null "$BASE_URL/vector?word=king"; then
    echo "could not find sample word 'king' — load GloVe first" >&2
    exit 1
fi

# Warm up once so every benchmark below starts from the same steady state:
# OS page cache populated with embedding rows, Go GC calibrated, connection
# pool filled, JIT/branch-predictor state hot. Results discarded.
echo "[bench] warmup (results discarded)"
{
    echo "GET $BASE_URL/vector?word=king"
    echo "GET $BASE_URL/nearest?word=king&k=10"
} | vegeta attack -rate=500 -duration=3s -workers=4 >/dev/null

# GET throughput: 10k RPS for 60s, 32 concurrent workers.
echo "[bench] GET throughput 10k RPS / 60s / 32 workers"
echo "GET $BASE_URL/vector?word=king" | \
    vegeta attack -rate=10000 -duration=60s -workers=32 | \
    tee "$RAW/get_throughput.bin" | \
    vegeta report -type=text > "$OUT/get_throughput.txt"

# GET latency: single-client, queue-depth 1. `-rate=0 -max-workers=1` makes
# vegeta send, wait, send — so throughput = 1 / mean_latency and we're
# measuring pure per-query latency. Self-enforcing: stays queue-depth 1 even
# if the server slows down (a fixed -rate with extra workers would silently
# slide into measuring queue dynamics). Buckets extended past 5ms so
# GC-pause / scheduler-jitter tail is visible, not clipped.
echo "[bench] GET latency single-client (queue-depth 1, 60s)"
echo "GET $BASE_URL/vector?word=king" | \
    vegeta attack -rate=0 -max-workers=1 -duration=60s | \
    tee "$RAW/get_latency.bin" | \
    vegeta report -type='hist[0,100us,250us,500us,1ms,2ms,5ms,10ms,50ms]' > "$OUT/get_latency.txt"
vegeta report -type=text < "$RAW/get_latency.bin" > "$OUT/get_latency_summary.txt"

# Search latency k=1: single-client, queue-depth 1. `-rate=0 -max-workers=1`
# makes vegeta send, wait, send — so throughput = 1 / mean_latency and we're
# measuring pure per-query latency, not queue dynamics.
echo "[bench] Nearest k=1 single-client latency (queue-depth 1, 60s)"
echo "GET $BASE_URL/nearest?word=king&k=1" | \
    vegeta attack -rate=0 -max-workers=1 -duration=60s | \
    tee "$RAW/nearest_k1.bin" | \
    vegeta report -type='hist[0,1ms,2ms,5ms,10ms,20ms,50ms,100ms,500ms]' > "$OUT/nearest_k1.txt"
vegeta report -type=text < "$RAW/nearest_k1.bin" > "$OUT/nearest_k1_summary.txt"

# Search latency k=10: same single-client shape, larger k.
echo "[bench] Nearest k=10 single-client latency (queue-depth 1, 60s)"
echo "GET $BASE_URL/nearest?word=king&k=10" | \
    vegeta attack -rate=0 -max-workers=1 -duration=60s | \
    tee "$RAW/nearest_k10.bin" | \
    vegeta report -type='hist[0,1ms,2ms,5ms,10ms,20ms,50ms,100ms,500ms]' > "$OUT/nearest_k10.txt"
vegeta report -type=text < "$RAW/nearest_k10.bin" > "$OUT/nearest_k10_summary.txt"

# Search saturation: uncapped rate, bounded client concurrency. Reports
# peak sustained RPS plus the latency distribution at that peak (which
# will be high by design — saturation is not a latency number).
echo "[bench] Nearest k=1 saturation probe (unlimited rate, 64 clients, 30s)"
echo "GET $BASE_URL/nearest?word=king&k=1" | \
    vegeta attack -rate=0 -max-workers=64 -duration=30s | \
    tee "$RAW/nearest_saturation.bin" | \
    vegeta report -type=text > "$OUT/nearest_saturation.txt"

