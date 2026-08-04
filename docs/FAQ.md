# Frequently Asked Questions

These answers cover using `llm-sketchkit` to summarize high-volume,
high-cardinality LLM telemetry with bounded state.

## How Can I Count Distinct LLM Prompts Without Storing Prompt Text?

Canonicalize the prompt, hash the canonical bytes with a registered domain and
a high-entropy secret, and add the resulting 64-bit hash to an HLL++ sketch.
The sketch retains registers rather than the prompt text or a per-prompt map.
Its dense memory shape is fixed by the selected profile rather than growing
linearly with the number of distinct prompts.

The keyed hash is pseudonymous, not anonymous. Someone with the secret can test
candidate prompts, and repeated prompts remain linkable while the same secret
and domain are in use. Secret handling and rotation requirements are described
in the [security guidance](../README.md#security-and-privacy) and
[hash specification](../spec/hash.md).

## How Can I Find The Largest Token Consumers Without An Unbounded Per-Key Map?

Use weighted frequent-items. Hash the identity being measured, such as a model,
user, prompt template, or tool, and use its token count as the update weight.
The sketch retains at most the selected profile's map size and returns an
estimate with deterministic lower and upper bounds for each tracked key.

The no-false-negative query mode favors recall; the no-false-positive mode
returns only keys whose lower bound clears the sketch's error threshold. The
[profile specification](../spec/profiles.md#weighted-frequent-items-metadata)
defines the guarantees and merge behavior.

## Can Sketches Produced In Go Be Read And Merged In Python?

Yes. Both implementations use the same canonicalization profiles, hash
domains, sketch shapes, and deterministic protobuf representation. Shared
fixtures exercise Go-origin state, Python parsing and merging, and expected
serialized output.

Merges are deliberately strict: sketch kind, profile, hash domain, hash
algorithm, and shape metadata must match. Incompatible state produces an error
instead of an implicit conversion. The executable examples are in
[`vectors/`](../vectors/).

## How Much Memory Does Each Profile Use?

Profiles fix the core state shape rather than allowing unbounded growth:

| Sketch | `micro` | `small` | `default` |
|---|---:|---:|---:|
| HLL++ dense register payload | 4 KiB | 16 KiB | 32 KiB |
| Weighted frequent-items capacity | 256 entries | 512 entries | 1,024 entries |
| Bloom bitset | about 17.6 KiB | about 175.5 KiB | about 2.29 MiB |
| MinHash signature payload | 512 B | 1 KiB | 1 KiB |

The optional MinHash `k256` profile uses a 2 KiB signature payload. HLL++ starts
in a sparse representation and promotes deterministically to its dense shape.
These figures describe core arrays, bitsets, or entry capacities; object,
allocator, map, and serialization overhead depends on the language and runtime.
Exact profile constants are in [`spec/profiles.md`](../spec/profiles.md).

## What Error Guarantees Do The Estimates Provide?

The guarantee depends on the sketch:

- HLL++ distinct-count error is characterized against a conservative
  three-sigma relative-error bound for each precision.
- Weighted frequent-items returns deterministic per-item lower and upper
  bounds and exposes its current maximum error.
- Bloom filters have no false negatives for inserted hashes and a configured
  false-positive target at their rated capacity.
- MinHash similarity error is statistical and decreases as signature length
  increases.

The [characterization report](../reports/characterization.md) records workloads,
seeds, observed errors, theoretical bounds, and important limitations. The
[scorecard](../reports/scorecard.md) provides a shorter visual summary.

## Does Keyed Hashing Make Sketch Output Anonymous?

No. It keeps raw values out of sketch state and prevents testing candidates
without the secret, but it does not provide anonymity or differential privacy.
Anyone with the secret can recompute candidate hashes. Repeated values are also
linkable while the same secret and domain remain in use, and frequent-items
intentionally exposes the keyed hashes of tracked heavy items.

Protect the secret, separate domains, rotate when the trust boundary changes,
and control access to serialized sketches. Rotation intentionally breaks
comparison with older state.

## When Should I Use Apache DataSketches Instead?

Use DataSketches when its algorithms, APIs, binary formats, and native Python
distribution fit the application and this project's cross-language contract is
unnecessary. It is a mature library with a broader algorithm surface.

Use `llm-sketchkit` when Go and pure Python must agree on canonicalization,
domain-separated keyed hashing, named profiles, strict merge authorization, and
the checked-in protobuf representation. The detailed tradeoffs and independent
frequent-items comparison are in [Why This Is Not A DataSketches
Wrapper](DATASKETCHES.md).

## Is This A Trace Collector Or Sampling System?

No. `llm-sketchkit` does not receive telemetry, choose which attributes to
measure, sample traces, export metrics, store results, or provide a dashboard.
It supplies bounded data structures and compatibility rules that another
component can embed in its own processing path.

This separation is intentional: applications retain control over input limits,
attribute selection, secret management, windowing, export policy, and access to
the resulting aggregate state.
