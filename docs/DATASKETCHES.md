# What llm-sketchkit Adds To General-Purpose Sketch Libraries

Apache DataSketches provides a broad collection of general-purpose sketch
algorithms. `llm-sketchkit` addresses a narrower integration problem: producing
bounded, pseudonymous summaries of high-cardinality LLM and agent telemetry that
behave consistently across services, languages, and merge boundaries.

The contribution is the complete measurement contract around the sketch method,
not a claim that the underlying algorithms were invented here. Without that
contract, every telemetry pipeline must independently decide how values are
canonicalized and keyed, what state may be merged, which memory and error profiles
are compatible, and how summaries move between runtimes.

## The Additional Contract

`llm-sketchkit` defines one contract across Go and pure Python:

- text canonicalization before hashing;
- domain-separated, keyed HMAC-SHA256-64 inputs;
- named profiles with fixed shape and error parameters;
- explicit merge-compatibility metadata; and
- a deterministic protobuf representation checked byte-for-byte by shared
  fixtures.

A backend is interchangeable only if it satisfies that complete contract.

The companion
[`otelcol-genai-sketches`](https://github.com/llm-measurement/otelcol-genai-sketches)
collector applies these rules to GenAI spans with bounded windows, explicit missing
token accounting, cardinality controls, and restricted metric and top-k surfaces.

## Choosing Between Them

Use `llm-sketchkit` when the application needs the complete contract above, matching
Go and pure-Python behavior, or direct compatibility with components built on that
contract. Use DataSketches when its algorithms, APIs, binary formats, and packaging
already fit and the additional LLM-telemetry integration rules are unnecessary.

`llm-sketchkit` uses the DataSketches Python frequent-items implementation as an
independent behavioral oracle. This tests shared frequent-items guarantees without
making DataSketches a runtime dependency or claiming binary compatibility.

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

## Scope Of The Comparison

The comparison is evidence about weighted frequent-items query behavior on two
specified workloads. It does not establish wire compatibility, identical error
magnitudes, or equivalence between the HLL families. The shared vectors and
characterization report remain the authority for this project's own semantics.
