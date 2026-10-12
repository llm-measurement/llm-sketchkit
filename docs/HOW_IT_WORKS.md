# How It Works

Keep per-user answers without putting user identities in metric labels.

Canonicalize and keyed-hash inputs with the library's HMAC-SHA256 helpers before
adding them to a sketch. HyperLogLog++ estimates distinct counts; weighted
frequent-items ranks heavy keys with deterministic error bounds; Bloom filters
test membership; MinHash estimates set similarity. Go and Python share one
protobuf wire format and conformance vectors, so compatible summaries produced
in either language can be read and merged in the other.

The [summary exchange API](https://github.com/llm-measurement/llm-sketchkit/blob/main/examples/summary-exchange/README.md)
combines compatible window snapshots, handles replay and restart epochs, and
reports missing producers. It is available in both languages from `0.2.0`.

| Component | Use it for | Important property |
|---|---|---|
| HLL++ | Approximate distinct counts | Bounded, mergeable state |
| Weighted frequent-items | Heavy hitters and top items | Deterministic lower and upper bounds |
| Bloom filter | Set membership | No false negatives; configurable false-positive rate |
| MinHash | Approximate Jaccard similarity | Bounded, mergeable signatures |

The Go and Python implementations share the same profiles, hash domains, test
vectors, and serialized representation.

![Python notebook showing synthetic truth inside token-volume bounds from Go summaries](https://raw.githubusercontent.com/llm-measurement/llm-sketchkit/main/docs/images/token-bounds.png)

Actual output from the [Go-to-Python notebook](https://github.com/llm-measurement/llm-sketchkit/blob/main/examples/go-to-python/README.md).
Go produces the summaries; Python reads the same wire bytes, merges service shards,
and plots the results. The green marks are known synthetic counts used to check the
bounds. Each row is normalized to its own upper estimate.
[Watch the 90-second walkthrough](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/media/README.md).

## Metric Overflow

The [collector integration](https://github.com/llm-measurement/otelcol-genai-sketches/blob/main/docs/METRICS.md)
uses `slice_value="__overflow__"` for excess configured slice values; sketchkit
itself emits neither that label nor metrics. The OpenTelemetry SDK's
[`otel.metric.overflow=true`](https://opentelemetry.io/docs/specs/otel/metrics/sdk/#overflow-attribute)
instead marks a single-attribute aggregate at the SDK cardinality limit. Both
retain aggregate measurements while losing excess label detail, with different
grouping rules and no automatic translation between markers.
[Cardinality Guardian](https://github.com/YElayyat/otel-cardinality-processor)
is a related generic metric-label processor; keyed sketch summaries address
distinct counts and rankings separately from metric-label controls.

## Go To Python Notebook

The [runnable notebook](https://github.com/llm-measurement/llm-sketchkit/blob/main/examples/go-to-python/go-to-python.ipynb) produces
per-service HLL++ and weighted frequent-items summaries in Go, then loads,
validates, merges, and plots them in Python. It checks that serialization round trips
return the same bytes,
explicit merge rejection for incompatible profiles, distinct-count estimates,
and deterministic bounds around token-heavy pseudonymous keys.

The tutorial checks estimates against known synthetic counts and emits keyed
summaries. See the
[example guide](https://github.com/llm-measurement/llm-sketchkit/blob/main/examples/go-to-python/README.md)
for setup and security details.

## Evidence At A Glance

Measurements use deterministic workloads and report the least favorable of five
Linux benchmark runs where applicable.

- HMAC-SHA256-64 sustained at least **1.65 million 64-byte inputs/s/core** and
  **850,340 1 KiB inputs/s/core** on an Intel Xeon Platinum 8573C with Go 1.26.5.
- HLL++ and weighted frequent-items updates took at most **10.58 ns/op** and
  **145.6 ns/op**, respectively, with **0 allocations/op** in the measured paths.
- The HLL++ `small` profile's maximum observed relative error was **2.3301%**
  across the characterization grid, within its **2.4375%** enforced bound.
- Bloom profile false-positive rates were at or below their configured targets
  in the measured trials, with zero false negatives among inserted hashes.
- MinHash mean absolute error fell from **0.02845** at `k=128` to **0.02009** at
  `k=256`, closely following the expected inverse-square-root relationship.
- Both weighted frequent-items oracle workloads retained **100% true top-20
  recall** and valid no-false-positive query results in both implementations.

See the [visual scorecard](https://github.com/llm-measurement/llm-sketchkit/blob/main/reports/scorecard.md),
[raw measurement records](https://github.com/llm-measurement/llm-sketchkit/blob/main/reports/README.md),
and [general-purpose library comparison](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/DATASKETCHES.md)
for methods, measured workloads, and reproduction commands.
