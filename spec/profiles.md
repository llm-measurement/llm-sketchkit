# Sketch Profiles

Status: version 1.

## Scope

Profiles name bounded sketch shapes that appear in wire metadata and application
configuration. A sketch merge is permitted only when kind, profile, hash
domain, hash algorithm, and any kind-specific precision metadata required by
the merge policy are identical.

Implementations MUST reject unknown profile names. Implementations MUST NOT
substitute a nearest profile.

## v0.1 Final Constants

| Sketch kind | Profile | Final constants |
|---|---|---|
| HLL++ | `micro` | normal precision `p=12`, sparse precision `sp=16` |
| HLL++ | `small` | normal precision `p=14`, sparse precision `sp=18` |
| HLL++ | `default` | normal precision `p=15`, sparse precision `sp=20` |
| Weighted frequent-items | `micro` | map size `256` |
| Weighted frequent-items | `small` | map size `512` |
| Weighted frequent-items | `default` | map size `1024` |
| Bloom | `micro` | rated insertions `10000`, target FPR `0.001`, bit count `143776`, hash count `10` |
| Bloom | `small` | rated insertions `100000`, target FPR `0.001`, bit count `1437759`, hash count `10` |
| Bloom | `default` | rated insertions `1000000`, target FPR `0.0001`, bit count `19170117`, hash count `13` |
| MinHash | `micro` | signature length `k=64` |
| MinHash | `small` | signature length `k=128` |
| MinHash | `default` | signature length `k=128` |
| MinHash | `k256` | signature length `k=256` |

The `k256` profile is valid only for MinHash. It provides a larger signature
for callers that need lower variance than the default profile.

## HLL++ Precision Metadata

HLL++ wire metadata MUST carry both normal precision and sparse precision.
Merges are allowed only when both values match. The profile string alone is not
enough for HLL++ merge authorization.

v0.1 HLL++ implementations MUST reject normal/sparse precision pairs that do
not exactly match one of the named HLL++ profiles above. Arbitrary HLL++
precision construction is unsupported.

## Weighted Frequent-Items Metadata

Weighted frequent-items uses non-negative signed 64-bit weights in version 1.
Implementations MUST reject negative weights.

Weighted frequent-items serialization is deterministic for a fixed sketch
state: entries are encoded in hash-ascending order with a single global
`max_error`. Merge order is not a byte-stability promise. Because v0.1
Misra-Gries merge combines residual mass and then prunes back to the configured
map size, different valid merge orders MAY produce different tracked entry
sets, different `max_error` values, and different serialized bytes. All valid
merge orders MUST preserve the semantic guarantees: lower and upper bounds
bracket exact counts, untracked observed keys are bounded by `max_error`, and
`NO_FALSE_NEGATIVES` / `NO_FALSE_POSITIVES` obey the `max_error` threshold.

## Bloom Metadata

Bloom profiles define rated insertions and target false-positive rate. Encoded
Bloom sketches MUST also include the computed bit count and hash count so that
readers can validate shape compatibility without consulting this prose.

The v0.1 Bloom shapes use `ceil(-n * ln(target_fpr) / (ln 2)^2)` bits and
`round((bit_count / n) * ln 2)` hash-family positions. v0.1 implementations
MUST reject Bloom metadata whose bit count or hash count differs from the named
profile row.

The sizing formula above is the design-time shape formula. Runtime
false-positive estimates MUST be derived from the serialized bitset state as
`(set_bits / bloom_bit_count) ^ bloom_hash_count`, where `set_bits` is the
number of one bits in the canonical Bloom bitset. `inserted_count` is retained
as update-count metadata and merge evidence, but it MUST NOT be used as the
runtime estimate input.

## MinHash Metadata

MinHash profiles define signature length. The seeded hash-family derivation is
defined in `spec/hash.md`. The supported signature lengths are `micro` k=64,
`small`/`default` k=128, and `k256` k=256.
