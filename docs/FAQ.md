# Frequently Asked Questions

These answers cover using `llm-sketchkit` to obtain continuous, bounded answers
about high-cardinality agent traffic without exporting or indexing every underlying
value.

## How Does This Fit Into An Agent Observability Stack?

Trace and evaluation systems explain individual agent runs. `llm-sketchkit`
provides bounded measurement primitives for summaries across many runs, workers,
and windows, including distinct populations, heavy-item concentration, approximate
membership, and set change.

The library can be embedded in an agent observability pipeline when raw prompts or
identifiers should not enter aggregate state, or when exporting and indexing every
value would make operational cost or query latency unpredictable. Use its summaries
alongside trace explorers, evaluation systems, and controls.

## Can I Use This With Self-Hosted Models Or A Mix Of Providers?

Yes. The library works on data your application supplies, whether the model runs
behind a hosted API, on your own infrastructure, or across both. It does not call a
model provider or require a hosted analysis service. Your pipeline extracts the
identities and reported usage, applies keyed hashing, and updates the sketches.

Separate teams can combine compatible measurements using the
[summary exchange API](../examples/summary-exchange/README.md). Producers must agree
on scope, keys, accounting rules, and windows, and observe disjoint request streams.
Normalize source fields before creating summaries. The API checks compatibility;
operators authorize identity linkage and assign each request to one producer.

Keep model and usage definitions explicit when comparing deployments. Combine
token volume with provider pricing or serving metrics when studying costs.

## When Should I Use Sketches Or Exact Records?

Use sketches for continuous aggregate answers with predictable resource use and
mergeable state. Use exact tables or maps when values are safe to retain and exact
queries remain fast and affordable.

Sketches are therefore best understood as an **always-on bounded evidence plane**,
alongside the selected raw traces or events you retain for diagnosis, audit, or
replay.

Memory savings are only one property. Depending on the selected sketch and the system
embedding it, sketches can provide:

- bounded cardinality;
- bounded export volume per summary or window;
- explicit uncertainty;
- mergeability across processes, languages, or locations;
- privacy-conscious aggregation that keeps raw values out of sketch state;
- predictable processing cost; and
- query latency and operational responsiveness that do not depend on scanning the
  complete event history.

With the current release, an embedding pipeline can directly investigate:

- Which keyed agents, prompts, tools, or other identities account for most of a
  reported capacity unit? Weighted frequent-items reports estimates with deterministic
  lower and upper bounds.
- How many distinct identities or workflows were active? HLL++ provides a statistical
  distinct-count estimate with bounded state.
- Is reported usage concentrated in a small set of keyed values? Weighted
  frequent-items exposes the heavy portion without retaining every key.
- Did a population change materially between observations? MinHash compares set
  similarity using fixed-size signatures.

For "What changed between these windows?", use MinHash for set similarity or
[fleetdiff](https://github.com/llm-measurement/fleetdiff) for tracked contributors
and token-volume changes. To evaluate a policy change, pair those measurements
with the application's policy and outcome records.

## How Can I Count Distinct LLM Prompts Without Storing Prompt Text?

Canonicalize the prompt, hash the canonical bytes with a registered domain and
a high-entropy secret, and add the resulting 64-bit hash to an HLL++ sketch.
The sketch retains registers rather than the prompt text or a per-prompt map.
Its dense memory shape is fixed by the selected profile rather than growing
linearly with the number of distinct prompts.

See the [security guidance](../README.md#security-and-privacy) for secret handling,
linkability, and rotation, and the [hash specification](../spec/hash.md) for the
wire contract.

## How Can I Find The Largest Token Consumers Without Indexing Every Key?

Use weighted frequent-items. Hash a user or API/virtual key ID with `user:v1`,
a prompt template with `prompt:v1`, or a tool with `tool:v1`, and use its reported
token count as the update weight. There is no registered model hash domain;
keep model names as a bounded application dimension rather than inventing one.
The sketch retains at most the selected profile's map size and returns an
estimate with deterministic lower and upper bounds for each tracked key.

The no-false-negative query mode favors recall; the no-false-positive mode
returns only keys whose lower bound clears the sketch's error threshold. The
[profile specification](../spec/profiles.md#weighted-frequent-items-metadata)
defines the guarantees and merge behavior.

Results sort by estimate descending, then unsigned hash ascending for ties.
`user:v1` covers both end users and API/virtual keys for now, but one sketch must
use one agreed identity meaning. See the [registered domains](../spec/hash.md#domains).

## Can This Help Investigate "Tokenmaxxing" Or "Token-Maxing"?

It can detect concentration in reported token volume. Use a prompt template, tool,
user, or another identity with an appropriate registered hash domain as the keyed
item and its reported token count as the update weight. Merge compatible summaries
across workers or windows, then inspect estimates with their deterministic lower and
upper bounds.

This supports token-accounting investigations for both API spending and self-hosted
workloads. Without a per-token invoice, long responses and repeated calls can still
occupy shared serving capacity. Compare token concentration with queueing and
latency measurements from your serving system.

Count missing usage separately. Applications can use the measurements to trigger
alerts, investigate high-share sessions, or inform their own budget controls.

For GenAI spans already flowing through OpenTelemetry, the
[`otelcol-genai-sketches`](https://github.com/llm-measurement/otelcol-genai-sketches)
distribution supplies collection, bounded metrics, and structured top-k surfaces.
The [README example](../README.md#token-volume-heavy-hitters) shows direct library
use in a custom pipeline.

## Can Sketches Produced In Go Be Read And Merged In Python?

Yes. Both implementations use the same canonicalization profiles, hash
domains, sketch shapes, and deterministic protobuf representation. Shared
fixtures exercise Go-origin state, Python parsing and merging, and expected
serialized output.

Merge validation is strict: sketch kind, profile, hash domain, hash
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

<a id="does-keyed-hashing-make-sketch-output-anonymous"></a>

## What Privacy Does Keyed Hashing Provide?

Keyed hashing keeps raw values out of sketch state and requires the secret to
recompute candidate hashes. Domain separation keeps different entity classes
separate. Use access controls and rotate keys when the trust boundary changes;
the [security guidance](../README.md#security-and-privacy) explains pseudonymity,
linkability, and the absence of differential privacy.

## How Does This Differ From A General-Purpose Sketch Library?

`llm-sketchkit` packages the decisions a high-cardinality LLM telemetry pipeline
otherwise has to make around each sketch: text canonicalization, domain-separated
keyed hashing, bounded profiles, strict merge compatibility, deterministic wire
semantics, and matching Go and pure-Python behavior. The sketch algorithm is one
part of that contract rather than the complete integration.

Use it when producers and analysis systems must produce compatible, pseudonymous
summaries without defining those rules independently in every service. Apache
DataSketches offers a broader general-purpose algorithm catalog. Choose based on
the algorithms and interoperability contract your pipeline needs.

The [general-purpose library comparison](DATASKETCHES.md) explains the boundary and
records the independent frequent-items oracle results.

## Is This A Trace Collector Or Sampling System?

`llm-sketchkit` is an embeddable measurement library. For trace collection and
ready-made metrics, use the
[OpenTelemetry connector](https://github.com/llm-measurement/otelcol-genai-sketches).

This separation is intentional: applications retain control over input limits,
attribute selection, secret management, windowing, export policy, and access to
the resulting aggregate state.
