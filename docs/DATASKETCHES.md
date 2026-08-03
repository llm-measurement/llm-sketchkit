# Why This Is Not A DataSketches Wrapper

Apache DataSketches is mature, carefully engineered prior art. `llm-sketchkit`
uses its Python frequent-items implementation as an independent behavioral
oracle. It does not use DataSketches as a runtime dependency because this
library's compatibility contract extends beyond the sketch algorithm itself.

## The Compatibility Contract

`llm-sketchkit` defines one contract across Go and pure Python:

- text canonicalization before hashing;
- domain-separated, keyed HMAC-SHA256-64 inputs;
- named profiles with fixed shape and error parameters;
- explicit merge-compatibility metadata; and
- a deterministic protobuf representation checked byte-for-byte by shared
  fixtures.

A backend is interchangeable only if it satisfies that complete contract.

## Where The Implementations Differ

DataSketches HLL and this project's HLL++ follow different algorithm families,
so equal inputs are not expected to produce equal internal state or estimates.
DataSketches also has its own binary formats, while this project encodes sketch
kind, profile, hash domain, hash algorithm, and representation metadata in its
specified protobuf format. Its Python distribution uses native code; this
project intentionally provides a readable pure-Python implementation alongside
Go.

The weighted frequent-items API does share familiar concepts with DataSketches:
estimates, lower and upper bounds, and no-false-negative and no-false-positive
query modes. Those common semantics make an independent comparison useful even
though capacities, error signals, and bytes differ.

## What The Oracle Checks

The optional oracle runs two deterministic weighted workloads through both
implementations. It checks that:

- no-false-negative results retain the exact true top-20 items;
- no-false-positive results contain only items whose exact count exceeds the
  workload threshold; and
- partitioned merges preserve those properties across the tested orders.

Both implementations achieved full top-20 recall and valid
no-false-positive results for both workloads. The fixture, generated output,
and detailed intervals are public:

- [Oracle report](../reports/datasketches_oracle.md)
- [Machine-readable results](../reports/datasketches_oracle.json)
- [Deterministic workload fixture](../vectors/oracles/datasketches_frequent_items.json)
- [Oracle runner](../scripts/datasketches_oracle.py)

Run it from a checkout:

```sh
python -m pip install -e '.[oracle]'
python scripts/datasketches_oracle.py --check
```

## Honest Limits

The comparison is evidence about weighted frequent-items query behavior on two
specified workloads. It does not establish wire compatibility, identical error
magnitudes, or equivalence between the HLL families. The shared vectors and
characterization report remain the authority for this project's own semantics.
