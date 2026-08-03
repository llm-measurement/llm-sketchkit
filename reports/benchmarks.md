# Benchmark Results

These results characterize the v0.1 alpha implementation. Commands use
`GOMAXPROCS=1` and `-benchmem` so per-core update cost and steady-state
allocation behavior are visible.

## Performance Targets

| Path | Target |
|---|---:|
| HMAC-SHA256-64, 64-byte canonical input | `>=1,000,000 ops/s/core` |
| HMAC-SHA256-64, 1 KiB canonical input | `>=200,000 ops/s/core` |
| HLL++ `AddHash(uint64)` | `<=500 ns/op`, `0 allocs/op` steady state |
| Weighted frequent-items `AddHash(uint64, weight)` | `<=500 ns/op`, `0 allocs/op` steady state |

Bloom and MinHash are characterized for accuracy and wire compatibility in
v0.1 alpha. No hot-path update target is included for them.

## Local Apple Silicon Results

Recorded 2026-07-03 on Darwin arm64, Apple M4 Max, Go `go1.26.1`,
`GOMAXPROCS=1`.

Hash command:

```sh
GOMAXPROCS=1 go test ./bench/hash -run '^$' -bench='BenchmarkHMACSHA25664_(64B|1KB)$' -benchmem -count=5
```

64-byte HMAC-SHA256-64 input:

| Run | ns/op | MB/s | B/op | allocs/op | ops/s |
|---:|---:|---:|---:|---:|---:|
| 1 | 278.6 | 229.70 | 529 | 8 | 3,589,375 |
| 2 | 275.7 | 232.10 | 529 | 8 | 3,627,131 |
| 3 | 276.5 | 231.49 | 529 | 8 | 3,616,637 |
| 4 | 275.0 | 232.76 | 529 | 8 | 3,636,364 |
| 5 | 283.9 | 225.41 | 529 | 8 | 3,522,367 |

1 KiB HMAC-SHA256-64 input:

| Run | ns/op | MB/s | B/op | allocs/op | ops/s |
|---:|---:|---:|---:|---:|---:|
| 1 | 581.3 | 1761.57 | 529 | 8 | 1,720,282 |
| 2 | 611.9 | 1673.53 | 529 | 8 | 1,634,254 |
| 3 | 587.3 | 1743.60 | 529 | 8 | 1,702,707 |
| 4 | 584.9 | 1750.76 | 529 | 8 | 1,709,694 |
| 5 | 583.2 | 1755.69 | 529 | 8 | 1,714,678 |

Sketch command:

```sh
GOMAXPROCS=1 go test ./bench/sketch -run '^$' -bench='Benchmark(HLLPPAddHash|FrequentItemsAddHash)_' -benchmem -count=5
```

HLL++ `AddHash(uint64)`:

| Benchmark | Run 1 ns/op | Run 2 ns/op | Run 3 ns/op | Run 4 ns/op | Run 5 ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|
| Amortized from empty | 1.379 | 1.328 | 1.394 | 1.391 | 1.385 | 0 | 0 |
| Sparse steady-state | 7.963 | 8.107 | 7.973 | 7.907 | 7.863 | 0 | 0 |
| Dense steady-state | 1.348 | 1.393 | 1.460 | 1.496 | 1.369 | 0 | 0 |

Weighted frequent-items `AddHash(uint64, weight)`:

| Benchmark | Run 1 ns/op | Run 2 ns/op | Run 3 ns/op | Run 4 ns/op | Run 5 ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|
| Amortized skewed | 91.87 | 91.00 | 84.14 | 83.90 | 83.65 | 0 | 0 |
| Tracked steady-state | 29.15 | 29.20 | 29.13 | 29.16 | 28.88 | 0 | 0 |
| Drop steady-state | 73.36 | 72.94 | 73.15 | 73.83 | 74.23 | 0 | 0 |

## Current Linux Runner Results

Recorded 2026-08-02 on GitHub Actions `ubuntu-24.04`, image
`20260720.247.2`, Intel Xeon Platinum 8573C, 2 online vCPUs, Go
`go1.26.5 linux/amd64`, `GOMAXPROCS=1`. The complete workflow run is
[30782282374](https://github.com/llm-measurement/llm-sketchkit/actions/runs/30782282374).

Commands:

```sh
GOMAXPROCS=1 go test ./bench/hash -run '^$' -bench='BenchmarkHMACSHA25664_(64B|1KB)$' -benchmem -count=5
GOMAXPROCS=1 go test ./bench/sketch -run '^$' -bench='Benchmark(HLLPPAddHash|FrequentItemsAddHash)_' -benchmem -count=5
```

64-byte HMAC-SHA256-64 input:

| Run | ns/op | MB/s | B/op | allocs/op | ops/s |
|---:|---:|---:|---:|---:|---:|
| 1 | 580.9 | 110.18 | 529 | 8 | 1,721,467 |
| 2 | 575.0 | 111.30 | 529 | 8 | 1,739,130 |
| 3 | 605.1 | 105.77 | 529 | 8 | 1,652,619 |
| 4 | 586.8 | 109.07 | 529 | 8 | 1,704,158 |
| 5 | 597.4 | 107.14 | 529 | 8 | 1,673,920 |

1 KiB HMAC-SHA256-64 input:

| Run | ns/op | MB/s | B/op | allocs/op | ops/s |
|---:|---:|---:|---:|---:|---:|
| 1 | 1,142 | 896.92 | 529 | 8 | 875,657 |
| 2 | 1,162 | 881.21 | 529 | 8 | 860,585 |
| 3 | 1,150 | 890.43 | 529 | 8 | 869,565 |
| 4 | 1,141 | 897.39 | 529 | 8 | 876,424 |
| 5 | 1,176 | 870.49 | 529 | 8 | 850,340 |

HLL++ `AddHash(uint64)`:

| Benchmark | Run 1 ns/op | Run 2 ns/op | Run 3 ns/op | Run 4 ns/op | Run 5 ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|
| Amortized from empty | 2.543 | 2.532 | 2.579 | 2.568 | 2.584 | 0 | 0 |
| Sparse steady-state | 10.37 | 10.58 | 10.42 | 10.44 | 10.57 | 0 | 0 |
| Dense steady-state | 2.568 | 2.573 | 2.509 | 2.560 | 2.830 | 0 | 0 |

Weighted frequent-items `AddHash(uint64, weight)`:

| Benchmark | Run 1 ns/op | Run 2 ns/op | Run 3 ns/op | Run 4 ns/op | Run 5 ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|
| Amortized skewed | 145.6 | 143.8 | 142.9 | 144.4 | 141.1 | 0 | 0 |
| Tracked steady-state | 53.38 | 53.67 | 56.09 | 53.30 | 54.63 | 0 | 0 |
| Drop steady-state | 126.2 | 126.6 | 126.6 | 129.0 | 130.9 | 0 | 0 |

## Earlier Linux Runner Results

Recorded 2026-07-04 on GitHub Actions `ubuntu-24.04`, image
`20260628.225.1`, Linux `6.17.0-1018-azure`, AMD EPYC 7763 64-Core Processor,
2 online vCPUs, Go `go1.25.11 linux/amd64`, `GOMAXPROCS=1`.

Hash commands:

```sh
GOMAXPROCS=1 go test ./bench/hash -run '^$' -bench='BenchmarkHMACSHA25664_(64B|1KB)$' -benchmem -count=5
GOMAXPROCS=1 go test ./bench/sketch -run '^$' -bench='Benchmark(HLLPPAddHash|FrequentItemsAddHash)_' -benchmem -count=5
```

64-byte HMAC-SHA256-64 input:

| Run | ns/op | MB/s | B/op | allocs/op | ops/s |
|---:|---:|---:|---:|---:|---:|
| 1 | 732.3 | 87.40 | 529 | 8 | 1,365,560 |
| 2 | 681.4 | 93.92 | 529 | 8 | 1,467,567 |
| 3 | 684.3 | 93.52 | 529 | 8 | 1,461,347 |
| 4 | 694.0 | 92.22 | 529 | 8 | 1,440,922 |
| 5 | 684.0 | 93.56 | 529 | 8 | 1,461,988 |

1 KiB HMAC-SHA256-64 input:

| Run | ns/op | MB/s | B/op | allocs/op | ops/s |
|---:|---:|---:|---:|---:|---:|
| 1 | 1290 | 793.79 | 529 | 8 | 775,194 |
| 2 | 1294 | 791.37 | 529 | 8 | 772,798 |
| 3 | 1287 | 795.91 | 529 | 8 | 776,690 |
| 4 | 1287 | 795.76 | 529 | 8 | 776,690 |
| 5 | 1285 | 796.95 | 529 | 8 | 778,210 |

HLL++ `AddHash(uint64)`:

| Benchmark | Run 1 ns/op | Run 2 ns/op | Run 3 ns/op | Run 4 ns/op | Run 5 ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|
| Amortized from empty | 4.690 | 4.690 | 4.914 | 4.711 | 4.698 | 0 | 0 |
| Sparse steady-state | 14.51 | 14.25 | 14.52 | 14.41 | 14.42 | 0 | 0 |
| Dense steady-state | 4.695 | 4.701 | 4.691 | 4.706 | 4.707 | 0 | 0 |

Weighted frequent-items `AddHash(uint64, weight)`:

| Benchmark | Run 1 ns/op | Run 2 ns/op | Run 3 ns/op | Run 4 ns/op | Run 5 ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|
| Amortized skewed | 175.4 | 173.8 | 175.2 | 176.4 | 175.6 | 0 | 0 |
| Tracked steady-state | 59.00 | 58.91 | 58.48 | 58.83 | 58.61 | 0 | 0 |
| Drop steady-state | 157.4 | 158.7 | 156.6 | 156.4 | 157.2 | 0 | 0 |

## Summary

On the current Linux run, the slowest hash result was `1,652,619 ops/s/core`
for 64-byte inputs and `850,340 ops/s/core` for 1 KiB inputs. The slowest
sketch update result was `10.58 ns/op` for HLL++ and `145.6 ns/op` for
weighted frequent-items, both with `0 B/op` and `0 allocs/op`. All four paths
met their stated performance targets in every run.
