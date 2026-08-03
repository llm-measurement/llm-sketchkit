# Sketch Vectors

Sketch vectors describe operations and expected sketch state. Each file MUST
validate against `vectors/schemas/sketch_vector.schema.json`.

The vector suite covers HLL++, weighted frequent-items, Bloom, MinHash, merge
behavior, cross-language fixtures, and byte-stable serialization.

HLL++ vectors:

- `hllpp_sparse_updates.json` covers sparse register updates.
- `hllpp_promotion_boundary_sparse.json` stops exactly at the micro profile
  sparse-promotion threshold.
- `hllpp_promotion_boundary_dense.json` adds one more update and must promote
  to dense representation.
- `hllpp_merge_sparse.json` covers sparse plus sparse merge behavior.
- `hllpp_merge_sparse_dense.json` covers sparse plus dense merge behavior.

Weighted frequent-items vectors:

- `frequent_items_weighted_updates.json` covers weighted updates and bounds.
- `frequent_items_merge.json` covers source-partitioned merge behavior.
- `frequent_items_query_modes.json` covers deterministic query-mode output
  ordering.

Bloom and MinHash vectors:

- `bloom_insertions.json` covers seeded Bloom positions, membership, and
  false-positive estimate.
- `bloom_merge.json` covers source-partitioned Bloom OR merge behavior.
- `minhash_signature.json` covers seeded MinHash signature generation.
- `minhash_merge.json` covers element-wise MinHash merge and source Jaccard
  estimation.

Cross-language vectors:

- `cross_language_hllpp_dense.json` covers Go/Python HLL++ merge parity.
- `cross_language_bloom.json` covers Go/Python Bloom merge parity.
- `cross_language_frequent_items.json` covers Go/Python frequent-items merge
  semantics.
- `cross_language_minhash.json` covers Go/Python MinHash merge parity.

Worked example:

```json
{
  "schema_version": 1,
  "name": "empty HLL++ small sparse",
  "metadata": {
    "kind": "HLLPP",
    "wire_version": 1,
    "profile": "small",
    "hash_domain": "prompt:v1",
    "hash_algo": "hmac_sha256_64",
    "representation_mode": "HLLPP_SPARSE",
    "hllpp_normal_precision": 14,
    "hllpp_sparse_precision": 18
  },
  "operations": [],
  "expected": {
    "estimate": 0,
    "body": {"sparse_registers": []}
  }
}
```
