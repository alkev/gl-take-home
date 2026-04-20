# Benchmark results

**Hardware:** _TBD — fill in after running_
**OS:** _TBD_
**Go version:** _TBD_
**Dataset:** GloVe Wikipedia + Gigaword 100d, 1,200,000 vectors

## Summary

| Benchmark              | Target         | Achieved    |
|------------------------|----------------|-------------|
| Bulk load              | report vec/s   | (pending)   |
| GET latency p50        | < 0.5 ms       | (pending)   |
| GET latency p99        | < 1 ms         | (pending)   |
| GET throughput @ 32    | ≥ 10,000 RPS   | (pending)   |
| Search latency k=1 p99 | < 10 ms        | (pending)   |
| Search latency k=10    | report p50/p99 | (pending)   |
| Memory footprint (RSS) | report         | (pending)   |

## Reproducing

```bash
make build
./bin/vecstore &                        # or: docker compose up
./bin/loader --url http://localhost:8888
./bench/run.sh
```

## Raw outputs

See `bench/results/*.txt`. HDR plots can be regenerated from `bench/raw/*.bin`:

```bash
vegeta report -type=hdrplot < bench/raw/nearest_k1.bin > bench/nearest_k1_hdr.html
```
