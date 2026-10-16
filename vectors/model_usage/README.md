# Model Usage v1 Conformance Data

The optional `model_usage` member adds bounded per-model accounting to Summary
Exchange v1. Its own version is 1. Absence preserves legacy bytes; JSON null is
invalid. Model IDs are approved public IDs or four fixed reserved buckets.
Missing usage is unknown, not zero.

Each model records reported component totals and coverage, overlapping quality
flags, provenance, exclusion counts, and a jointly observed complete cohort.
Four fixed input bins end at 100000, 200000, 272000, and the integer maximum,
inclusively. Each bin retains reported input coverage and complete component
sums with observed extrema. Complete cohort totals are sums, not averages.

Snapshots replace earlier snapshots of the same producer/epoch/window; they
are never added together. Combining disjoint owners requires matching base and
extension contracts and matching extension presence. Model-set differences are
allowed. Historical rows, additive scalars and per-bin history cannot regress.

## Dataset

- `fixtures/`: 65 exact wire files, including deliberately invalid envelopes.
- `cases/`: 112 parse, merge, collection, limits, compatibility and arithmetic
  cases. Successful merge cases require permutations, replay and input immutability.
- `expected/`: exact selected-source, coverage and aggregate projections.
- `contract.json`, `contract-unicode.json`: synthetic configuration preimages
  and hashes. UTF-8 is hashed without Unicode normalization.
- `manifest.json`: the 82 data payloads, byte lengths and SHA-256 digests.

The pricing cases are exact rational arithmetic examples using synthetic rates.
They are not a production price-file schema or a provider-price claim. Compatibility
diagnostics show consumer mappings of product-neutral library reasons; released
readers retain their historical generic errors.

See `spec/model_usage.md` for integration APIs and ownership boundaries.
