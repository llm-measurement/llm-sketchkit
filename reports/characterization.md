# Characterization Results

This file records characterization results for the v0.1 alpha
implementation. The compatibility surface is defined in `spec/`; executable
conformance vectors live in `vectors/`.

## HLL++

Reference:

- Heule, Nunkesser, and Hall, "HyperLogLog in Practice: Algorithmic
  Engineering of a State of The Art Cardinality Estimation Algorithm" (EDBT
  2013), <https://research.google/pubs/pub40671/>.
- The implementation embeds the published empirical bias-correction data for
  `p=12`, `p=14`, and `p=15`. These constants were copied from published data,
  not re-derived.

Implementation notes:

- Profiles use `micro` `p=12/sp=16`, `small` `p=14/sp=18`, and `default`
  `p=15/sp=20`, matching `spec/profiles.md`.
- Sparse mode stores sparse-precision registers in compact sorted index/rank
  slices. Dense mode stores one byte per normal-precision register.
- Promotion is deterministic at more than `m/16` sparse entries. The promotion
  boundary vectors cover the `micro` profile at 256 sparse entries and the
  first dense transition at 257 entries.
- Merges require identical kind, profile, hash domain, hash algorithm, normal
  precision, and sparse precision. Cross-precision merges return a typed error.

Accuracy was measured over 10 deterministic seeds. The enforced bound is
`3 * 1.04 / sqrt(2^p)` relative error. Linear-counting rows use the same
conservative bound while recording their estimator regime.

| Profile | p | Cardinality | Estimator regime | Seeds | Mean relative error | Max relative error | Enforced bound |
|---|---:|---:|---|---:|---:|---:|---:|
| `micro` | 12 | 10 | sparse linear-counting sp=16 | 10 | 0.00007630 | 0.00007630 | 0.04875000 |
| `micro` | 12 | 100 | sparse linear-counting sp=16 | 10 | 0.00076372 | 0.00076372 | 0.04875000 |
| `micro` | 12 | 1,000 | dense linear-counting p=12 | 10 | 0.00873478 | 0.02781277 | 0.04875000 |
| `micro` | 12 | 15,000 | dense HLL++ bias-corrected p=12 | 10 | 0.00660218 | 0.01514803 | 0.04875000 |
| `micro` | 12 | 100,000 | dense HLL++ raw p=12 | 10 | 0.01823321 | 0.03678322 | 0.04875000 |
| `micro` | 12 | 10,000,000 | dense HLL++ raw p=12 | 10 | 0.01409154 | 0.02810531 | 0.04875000 |
| `small` | 14 | 10 | sparse linear-counting sp=18 | 10 | 0.00001907 | 0.00001907 | 0.02437500 |
| `small` | 14 | 100 | sparse linear-counting sp=18 | 10 | 0.00019078 | 0.00019078 | 0.02437500 |
| `small` | 14 | 1,000 | sparse linear-counting sp=18 | 10 | 0.00152481 | 0.00310689 | 0.02437500 |
| `small` | 14 | 60,000 | dense HLL++ bias-corrected p=14 | 10 | 0.00741199 | 0.01611878 | 0.02437500 |
| `small` | 14 | 100,000 | dense HLL++ raw p=14 | 10 | 0.00942647 | 0.02330117 | 0.02437500 |
| `small` | 14 | 10,000,000 | dense HLL++ raw p=14 | 10 | 0.00530877 | 0.01289269 | 0.02437500 |

Memory and update-cost observations:

- For `p=14`, sparse register storage after 500 updates was 2,560 bytes. The
  dense register array is 16,384 bytes, a 6.40x dense/sparse ratio.
- At `p=14`, the measured sparse/dense storage crossover is near 3,200
  entries. The `m/16` promotion threshold promotes at 1,024 entries, trading
  unused sparse memory headroom for cheaper dense updates.
- Local Apple Silicon update-cost measurements showed sparse steady-state
  updates at 8.107 ns/op and dense steady-state updates at 1.496 ns/op, both
  with zero allocations.

## Weighted Frequent-Items

Implementation notes:

- The implementation uses weighted Misra-Gries with a global error offset,
  deterministic pruning, and a preallocated counter pool.
- Entry `estimate` is the deterministic upper estimate. `error` is the current
  global max error. `lower_bound = estimate - error`, clamped at zero, and
  `upper_bound = estimate`.
- `NO_FALSE_NEGATIVES` and `NO_FALSE_POSITIVES` use `max_error` as the
  implicit threshold. `NO_FALSE_NEGATIVES` returns tracked items whose upper
  bound exceeds that threshold. `NO_FALSE_POSITIVES` returns tracked items
  whose lower bound exceeds it.
- Serialization is deterministic for a fixed sketch state. Merge order is a
  semantic guarantee over estimates, bounds, and query-mode outputs, not a
  byte-stability promise for weighted frequent-items state.

Merge and workload evidence:

| Workload | Profile | Updates | Keys | Result |
|---|---|---:|---:|---|
| Random weighted streams | `small` | varied | varied | Exact true counts were bracketed between lower and upper bounds; sampled untracked keys had true count no greater than `max_error`. |
| Adversarial 5-partition merge | `small` | synthetic skew | varied | Part max errors summed to 5; merged `max_error` was 5; `NO_FALSE_NEGATIVES` returned all 20 true heavy keys; `NO_FALSE_POSITIVES` returned only keys above threshold. |
| Zipf(1.1) single sketch | `small` | 1,000,000 | 100,000 | `max_error=1,019`; true 20th item count 5,011; theoretical weighted-MG error bound about 1,949; measured error ratio 0.52x. |
| Zipf(1.1) 5 key partitions, all 120 merge orders | `small` | 1,000,000 | 100,000 | Part max errors ranged 142-144; merged `max_error` ranged 730-736; global weighted-MG bound was 1,950; all orders preserved bounds and query-mode guarantees. |

Query-mode costs observed on the Zipf(1.1) single-sketch workload:

| Mode | Candidates | Guarantee observed |
|---|---:|---|
| `NO_FALSE_NEGATIVES` | 427 | Included all true top-20 keys. |
| `NO_FALSE_POSITIVES` | 44 | Returned only keys whose exact count was above `max_error`. |

Update-cost measurements are recorded in `reports/benchmarks.md`. The slowest
local weighted frequent-items run was 91.87 ns/op with `0 B/op` and
`0 allocs/op`; the slowest Linux runner run was 176.4 ns/op with `0 B/op` and
`0 allocs/op`.

### DataSketches Oracle Comparison

The optional Apache DataSketches Python oracle checks weighted frequent-items
query semantics against deterministic fixtures. It is not a runtime dependency,
wire-format authority, or HLL++ semantic authority; DataSketches HLL uses a
different algorithm family than this project's HLL++ implementation.

| Workload | Total weight | Distinct keys | Threshold | Sketchkit NFN recall | DataSketches NFN recall | Sketchkit NFP valid | DataSketches NFP valid |
|---|---:|---:|---:|---:|---:|---|---|
| `zipf_1_1_weighted_small` | 233,931 | 14,301 | 1,243 | 1.00 | 1.00 | yes | yes |
| `tail_churn_weighted_small` | 128,728 | 24,032 | 3,272 | 1.00 | 1.00 | yes | yes |

The fixture is checked in at `vectors/oracles/datasketches_frequent_items.json`;
the generated report is `reports/datasketches_oracle.md`, with detailed top-item
intervals and partitioned merge rows in `reports/datasketches_oracle.json`.

## Bloom Filters

Bloom filters use a deterministic sketch-local hash family over already-keyed
64-bit sketch hashes. Each profile stores a fixed bitset and estimates false
positive rate from serialized state as `(set_bits / bit_count)^hash_count`.

| Profile | Rated insertions | Bit count | Hash count | Queries | False positives | Empirical FPR | Target FPR | 1.5x target | Runtime estimate |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `micro` | 10,000 | 143,776 | 10 | 200,000 | 199 | 0.00099500 | 0.00100000 | 0.00150000 | 0.00101064 |
| `small` | 100,000 | 1,437,759 | 10 | 200,000 | 181 | 0.00090500 | 0.00100000 | 0.00150000 | 0.00099649 |
| `default` | 1,000,000 | 19,170,117 | 13 | 1,000,000 | 94 | 0.00009400 | 0.00010000 | 0.00015000 | 0.00010017 |

All three profile cells also checked every inserted hash for zero false
negatives. Merge vectors verify that Bloom merge is bitset OR over the union.

## MinHash

MinHash uses a deterministic sketch-local hash family over already-keyed
64-bit sketch hashes. Signatures are initialized to all
`0xffffffffffffffff`, updated by element-wise minima, and merged by
element-wise minima. Jaccard similarity is estimated as matching signature
positions divided by signature length.

For a MinHash signature of length `k`, the binomial standard error of the
Jaccard estimate is `sqrt(J * (1-J) / k)` and is maximized at
`1 / (2 * sqrt(k))`.

| Profile | k | Pairs | Mean abs error | Mean bound | p95 abs error | p95 bound |
|---|---:|---:|---:|---:|---:|---:|
| `micro` | 64 | 1,000 | 0.04067578 | 0.05700000 | 0.10156250 | 0.12700000 |
| `small` | 128 | 1,000 | 0.02844922 | 0.04000000 | 0.07421875 | 0.09000000 |
| `default` | 128 | 1,000 | 0.02844922 | 0.04000000 | 0.07421875 | 0.09000000 |
| `k256` | 256 | 1,000 | 0.02009375 | 0.03000000 | 0.05078125 | 0.07000000 |

Merge vectors verify that MinHash merge produces the exact signature of the
union.

## Cross-Language Conformance

The pure-Python implementation provides the same conceptual API shape as Go:
profile constructors, `add_hash`, `merge`, estimates/query methods,
deterministic `marshal_binary`, and typed parse errors. Runtime dependencies
are `protobuf` plus the Python standard library; there are no native
extensions.

Go and Python both load the checked-in hash and sketch vectors. The same
fixtures cover canonicalization, HMAC-SHA256-64, HLL++ sparse/promotion/merge,
weighted frequent-items updates/query modes/merge, Bloom insert/merge/FPR, and
MinHash signatures/merge/Jaccard. Cross-language fixtures include Go-origin
serialized inputs and merged outputs for HLL++, Bloom, frequent-items, and
MinHash.

## Finalized profile constants

| Sketch kind | Profile | Final constants | Characterization evidence |
|---|---|---|---|
| HLL++ | `micro` | `p=12`, `sp=16`, sparse promotion after 256 sparse registers | 3-sigma relative-error bound 0.04875000; vectors cover sparse and promotion behavior. |
| HLL++ | `small` | `p=14`, `sp=18`, sparse promotion after 1,024 sparse registers | 3-sigma relative-error bound 0.02437500; p=14 accuracy grid and memory/CPU promotion tradeoff recorded. |
| HLL++ | `default` | `p=15`, `sp=20`, sparse promotion after 2,048 sparse registers | Uses the same HLL++ estimator with published p=15 bias constants; default trades memory for lower RSE than `small`. |
| Weighted frequent-items | `micro` | bounded map size 256 | Vectors and random weighted bound checks. |
| Weighted frequent-items | `small` | bounded map size 512 | Zipf(1.1) n=1,000,000: `max_error=1,019`, 427 `NO_FALSE_NEGATIVES` candidates for true top-20, 44 `NO_FALSE_POSITIVES` candidates, 91.87 ns/op worst local amortized update. |
| Weighted frequent-items | `default` | bounded map size 1,024 | Same deterministic weighted Misra-Gries strategy with lower theoretical error `W / 1025` than `small`. |
| Bloom | `micro` | rated insertions 10,000, target FPR 0.001, bit count 143,776, hash count 10 | Empirical FPR 0.00099500; runtime estimate uses fill ratio. |
| Bloom | `small` | rated insertions 100,000, target FPR 0.001, bit count 1,437,759, hash count 10 | Empirical FPR 0.00090500; runtime estimate uses fill ratio. |
| Bloom | `default` | rated insertions 1,000,000, target FPR 0.0001, bit count 19,170,117, hash count 13 | Empirical FPR 0.00009400; runtime estimate uses fill ratio. |
| MinHash | `micro` | signature length `k=64` | Mean absolute error 0.04067578, p95 0.10156250 under random-pair characterization. |
| MinHash | `small` | signature length `k=128` | Mean absolute error 0.02844922, p95 0.07421875. |
| MinHash | `default` | signature length `k=128` | Same shape and measured accuracy as `small`. |
| MinHash | `k256` | signature length `k=256` | Mean absolute error 0.02009375, p95 0.05078125; optional higher-accuracy profile. |
