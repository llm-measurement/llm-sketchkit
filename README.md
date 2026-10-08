# llm-sketchkit

[![CI](https://github.com/llm-measurement/llm-sketchkit/actions/workflows/ci.yml/badge.svg)](https://github.com/llm-measurement/llm-sketchkit/actions/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/llm-measurement/llm-sketchkit/badge)](https://scorecard.dev/viewer/?uri=github.com/llm-measurement/llm-sketchkit)

`llm-sketchkit` finds top users or API keys by reported tokens and estimates
distinct users with fixed memory bounds per sketch, instead of per-user metric
labels. Embed the Go or Python library in your gateway to summarize each window
locally and merge compatible summaries across workers or services.

Both implementations share text canonicalization, keyed pseudonymous hashing,
mergeable sketches, and a deterministic protobuf wire format.

Canonicalize and keyed-hash inputs in your process before adding them to a sketch.
Only the hashes enter sketch state; compatible producers can merge that state
across processes or languages.

![Python notebook showing synthetic truth inside token-volume bounds from Go summaries](https://raw.githubusercontent.com/llm-measurement/llm-sketchkit/main/docs/images/token-bounds.png)

Actual output from the [Go-to-Python notebook](https://github.com/llm-measurement/llm-sketchkit/blob/main/examples/go-to-python/README.md).
Go produces the summaries; Python reads the same wire bytes, merges service shards,
and plots the results. The green marks are known synthetic counts used to check the
bounds. Each row is normalized to its own upper estimate.
[Watch the 90-second walkthrough](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/media/README.md).

## When This Fits

Use `llm-sketchkit` inside telemetry producers and processing components when
exporting and indexing every key would create uncontrolled cardinality, make
operational queries slow or unpredictable, or retain values that should not enter
aggregate state.

Agent fleets and multi-agent systems can produce more identities and events than
an observability backend should continuously index. The library provides bounded,
mergeable summaries across workers and windows.

Use it with hosted model APIs, self-hosted models, or a mixture of both. Your
pipeline supplies the identities and reported usage; the library processes that
data locally without calling a model provider. See the
[deployment FAQ](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/FAQ.md#can-i-use-this-with-self-hosted-models-or-a-mix-of-providers)
for input and comparison requirements.

Questions it can help answer include:

- **How many distinct prompts, users, sessions, tools, or documents are active
  without keeping one counter per value?** HLL++ provides a statistical estimate
  with bounded state.
- **Your token budget is climbing and FinOps wants to know which configured
  identities account for the reported volume. How certain is the answer?** Weighted
  frequent-items identifies token-heavy or request-heavy keys with deterministic
  lower and upper bounds.
- **Have we already observed this request or document without maintaining an exact
  set of every value?** Bloom filters provide bounded approximate membership checks.
- **Did the prompt, tool, or retrieval-document population change materially after
  a deployment or model change?** MinHash compares large sets using bounded
  similarity signatures.
- **Can Go services summarize locally while Python analysis jobs read and merge the
  same state?** Shared profiles, fixtures, and wire semantics keep the two
  implementations compatible.
- **Can separately operated agent systems combine measurements without pooling
  their raw telemetry?** The [summary exchange API](https://github.com/llm-measurement/llm-sketchkit/blob/main/examples/summary-exchange/README.md)
  combines compatible window snapshots, handles replay and restart epochs, and
  reports missing producers. Available in Go and Python from `0.2.0`.

For investigations described as "tokenmaxxing" (also written "token-maxing"),
reported token counts can be used as weights in the frequent-items sketch to identify
which keyed values account for the most token volume. Use these concentration
measurements to focus an investigation or inform a separate budget policy.
See [Token-Volume Heavy Hitters](https://github.com/llm-measurement/llm-sketchkit#token-volume-heavy-hitters) for a runnable example.

Token accounting matters with self-hosted models too: there may be no per-token
invoice, but long responses and repeated calls can occupy shared serving capacity.
Pair reported token volume with serving metrics to understand capacity use.

The library embeds in your processing code. The integrations below connect it to
collectors, investigations, and existing backends; the
[FAQ](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/FAQ.md)
covers accuracy, privacy, and interoperability.

## Choosing An Integration

| Your pipeline | Use | Why |
|---|---|---|
| GenAI spans already flow through an OpenTelemetry Collector | [OpenTelemetry Collector connector](https://github.com/llm-measurement/otelcol-genai-sketches) | It applies keyed hashing, bounded windows, cardinality controls, and trace-to-metrics conversion at the collector boundary. |
| You have compatible summary exports and want to compare two windows across operators | [fleetdiff](https://github.com/llm-measurement/fleetdiff) | A local, read-only command reports usage changes, distinct activity, tracked-item bounds, and missing coverage. |
| A custom Go or Python streaming service processes events | `llm-sketchkit` directly | Update sketches inside each bounded window, then serialize or merge compatible summaries. |
| A batch or warehouse job reads stored events | `llm-sketchkit` directly | Build bounded summaries per partition or window and merge them before publishing results. |
| You need to store or visualize finished metrics | Your existing backend integration | Send aggregated results to ClickHouse, Datadog, Prometheus, or a similar system. |

Use the collector path when the source spans already flow through OpenTelemetry. Use
the library when you own the event-processing code or need matching Go and Python
summaries outside an OpenTelemetry pipeline. In either case, compatible producers
must agree on profile, hash domain, hash algorithm, secret, and window boundaries.

For measurements spanning independently operated systems, the
[summary envelope](https://github.com/llm-measurement/llm-sketchkit/blob/main/spec/summary.md) also carries scope, accounting rules, producer
identity, and key version. No hashing secret is needed to combine compatible state.
Use authenticated exchange, authorize identity linkage, and assign each event to
one producer.

For a complete comparison example, try [fleetdiff's two-operator demo](https://github.com/llm-measurement/fleetdiff#try-it-in-a-minute).
It shows how one team's reported token usage can fall while the combined fleet
total rises, using synthetic data. No account, upload, or model API key is needed.

## Included Sketches

| Component | Use it for | Important property |
|---|---|---|
| HLL++ | Approximate distinct counts | Bounded, mergeable state |
| Weighted frequent-items | Heavy hitters and top items | Deterministic lower and upper bounds |
| Bloom filter | Set membership | No false negatives; configurable false-positive rate |
| MinHash | Approximate Jaccard similarity | Bounded, mergeable signatures |

The Go and Python implementations share the same profiles, hash domains, test
vectors, and serialized representation.

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

## Requirements

- Go 1.26 or 1.27, with the latest security patch, for builds from `main`.
  The published `0.2.x` releases also support Go 1.25.
- Python 3.11, 3.12, 3.13, or 3.14

## Install

### Python

```sh
python -m pip install llm-sketchkit
```

Generate a process secret:

```sh
export LLM_SKETCHKIT_SECRET="$(python -c 'import secrets; print(secrets.token_hex(32))')"
```

Run a distinct-count example:

```sh
python - <<'PY'
from llm_sketchkit import PROMPT_V1, canonicalize_text_v1, hash64, hllpp
from llm_sketchkit import secret_from_env

secret = secret_from_env("LLM_SKETCHKIT_SECRET")
sketch = hllpp.new("small", PROMPT_V1)

canonical = canonicalize_text_v1("  cafe\u0301\r\n")
sketch.add_hash(hash64(secret, PROMPT_V1, canonical))

print(f"estimated distinct prompts: {sketch.estimate():.0f}")
PY
```

Expected output:

```text
estimated distinct prompts: 1
```

### Go

From an existing Go module, add the packages used by the examples (including
canonicalization's dependencies). In a new directory, run `go mod init example`
first. This is a library, so use `go get`, not `go install`:

```sh
go get github.com/llm-measurement/llm-sketchkit/go/sketchkit/...@latest
```

Use the same `LLM_SKETCHKIT_SECRET`. Put this program in `main.go`, then run
`go run .`:

```go
package main

import (
	"fmt"
	"log"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
)

func main() {
	secret, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_SECRET")
	if err != nil {
		log.Fatal(err)
	}

	sketch, err := hllpp.New(
		hllpp.ProfileSmall,
		sketchhash.PromptV1,
		sketchhash.HMACSHA25664,
	)
	if err != nil {
		log.Fatal(err)
	}

	canonical, err := canon.CanonicalizeString(canon.TextV1, "  cafe\u0301\r\n")
	if err != nil {
		log.Fatal(err)
	}
	digest, err := sketchhash.Hash64(secret, sketchhash.PromptV1, canonical)
	if err != nil {
		log.Fatal(err)
	}

	sketch.AddHash(digest)
	fmt.Printf("estimated distinct prompts: %.0f\n", sketch.Estimate())
}
```

Expected output:

```text
estimated distinct prompts: 1
```

For concurrent use, give each worker its own sketch or protect every read and
write with the same mutex. See
[concurrency and ownership](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/OPERATIONS.md#concurrency-and-ownership).

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

## Token-Volume Heavy Hitters

If by "tokenmaxxing" (also written "token-maxing") you mean unexpected or runaway
token consumption, use a bounded frequent-items sketch to find where reported volume
is concentrated. Hash the value being investigated, such as a prompt template, tool,
or user, with the registered domain for that entity class, and use the reported token
count as its weight. This example measures prompt templates with `prompt:v1`:

```python
from llm_sketchkit import PROMPT_V1, canonicalize_text_v1, frequentitems
from llm_sketchkit import hash64, secret_from_env

secret = secret_from_env("LLM_SKETCHKIT_SECRET")
sketch = frequentitems.Sketch("small", PROMPT_V1)

events = [
    ("support/refund", 1_240),
    ("research/synthesis", 8_900),
    ("support/refund", 980),
    ("coding/review", 3_600),
]

names = {}  # This tiny demo already knows the authorized template names.
for prompt_template, reported_tokens in events:
    canonical = canonicalize_text_v1(prompt_template)
    digest = hash64(secret, PROMPT_V1, canonical)
    names[digest] = prompt_template
    sketch.add_hash(digest, reported_tokens)

for item in sketch.frequent_items(frequentitems.NO_FALSE_NEGATIVES)[:10]:
    print(
        f"{names[item.hash]} estimate={item.estimate} "
        f"bounds=[{item.lower_bound}, {item.upper_bound}]"
    )
```

Expected output:

```text
research/synthesis estimate=8900 bounds=[8900, 8900]
coding/review estimate=3600 bounds=[3600, 3600]
support/refund estimate=2220 bounds=[2220, 2220]
```

`FrequentItems` in Go and `frequent_items` in Python sort by estimate descending,
then unsigned hash ascending for ties. Taking the first ten limits the returned
list. Ranks remain approximate; the no-false-negative guarantee applies to the
full query result. This small example retains every key, so its bounds coincide.

The sketch retains at most the selected profile's bounded map size. Returned hashes
are pseudonymous and remain linkable while the same secret and domain are in use.
Use the lower and upper bounds to decide whether an item is meaningfully heavy.

Count missing usage separately and weight sketches with reported counts. For an OTLP
pipeline with ready-made metrics, bounded slices, missing-usage accounting, and
token-weighted top-k snapshots, use
[`otelcol-genai-sketches`](https://github.com/llm-measurement/otelcol-genai-sketches).

### Go Gateway: Distinct Users And Top Keys By Tokens

The [complete runnable example](https://github.com/llm-measurement/llm-sketchkit/blob/main/examples/gateway/main.go)
composes HLL++ and weighted frequent-items with `sync.Mutex`. From a repository
checkout, with `LLM_SKETCHKIT_SECRET` set as above:

```sh
go run ./examples/gateway
go test -race ./examples/gateway
```

Its update path creates one pair of sketches per window. Multiple workers share
the lock; reads use it too:

```go
distinct, err := hllpp.New(hllpp.ProfileSmall, sketchhash.UserV1, sketchhash.HMACSHA25664)
if err != nil {
    return err
}
top, err := frequentitems.New(frequentitems.ProfileSmall, sketchhash.UserV1, sketchhash.HMACSHA25664)
if err != nil {
    return err
}
var mu sync.Mutex
observe := func(e event) error {
    digest, err := hashKey(secret, e.key)
    if err != nil {
        return err
    }
    mu.Lock()
    defer mu.Unlock()
    if err := top.AddHash(digest, e.tokens); err != nil {
        return err
    }
    distinct.AddHash(digest)
    return nil
}
```

This excerpt uses the imports, `event`, and canonicalizing `hashKey` helper in the
linked program. It waits for workers, queries the top ten, then re-hashes keys
from an existing authorized catalog to name matching hashes. Only those ten
matches are retained; unknown keys stay hashes. Use non-secret key IDs from that
catalog, keeping both memory use and access to names controlled.

After taking the locked snapshot and limiting `items` to ten, the lookup is:

```go
names := make(map[uint64]string, len(items))
for _, key := range knownKeys {
    digest, err := hashKey(secret, key)
    if err != nil {
        return err
    }
    for _, item := range items {
        if item.Hash == digest {
            names[digest] = key
        }
    }
}
```

Expected output for its four synthetic requests:

```text
distinct keys: 3
research tokens=8900 bounds=[8900,8900]
coding tokens=3600 bounds=[3600,3600]
support tokens=2220 bounds=[2220,2220]
```

`user:v1` currently covers end users and API or virtual key identities. Choose one
identity meaning per sketch so the distinct count has a consistent definition.
Use stable, non-secret key IDs, and agree on the same mapping across producers.
See the [registered hash domains](https://github.com/llm-measurement/llm-sketchkit/blob/main/spec/hash.md#domains).

Create fresh sketches at each window boundary, bound the number of live windows
and workers, and discard a window if an update fails. Zero-token requests count
toward distinct keys; missing usage gets a separate counter. Expose distinct
estimates as gauges and top-k through an access-controlled log or JSON endpoint,
keeping identities out of metric labels. See the [Go-to-Python producer](https://github.com/llm-measurement/llm-sketchkit/blob/main/examples/go-to-python/producer/main.go)
for serialization and the [ownership contract](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/OPERATIONS.md#concurrency-and-ownership)
for worker-local sketches and merging.

## Merge Sketches

Sketches merge only when their kind, profile, hash domain, hash algorithm, and
shape metadata match. A mismatch is an error rather than an implicit conversion.

```python
from llm_sketchkit import PROMPT_V1, canonicalize_text_v1, hash64, hllpp
from llm_sketchkit import secret_from_env

secret = secret_from_env("LLM_SKETCHKIT_SECRET")
left = hllpp.new("small", PROMPT_V1)
right = hllpp.new("small", PROMPT_V1)

left.add_hash(hash64(secret, PROMPT_V1, canonicalize_text_v1("alpha")))
right.add_hash(hash64(secret, PROMPT_V1, canonicalize_text_v1("beta")))

left.merge(right)
print(f"merged distinct prompts: {left.estimate():.0f}")
```

Expected output: `merged distinct prompts: 2`. Both producers must use the same
secret and window definition; bare sketches cannot detect mismatched secrets.

## Security And Privacy

- Hash inputs with a registered domain and a high-entropy secret before adding
  them to a sketch. The built-in secret loaders require at least 16 bytes and
  reject known placeholder values.
- Keyed hashes are pseudonymous, not anonymous. Anyone with the secret can test
  candidate values, and repeated hashes remain linkable while the same secret
  and domain are in use.
- Never log, serialize, or commit the hash secret. Rotate it when the trust
  boundary changes; rotation intentionally breaks comparison with older state.
- Bound raw input size before canonicalization. Canonicalization operates on
  in-memory text and intentionally leaves application-specific limits to callers.
- Sketches reveal bounded aggregate information and may reveal membership or
  recurrence. They do not provide differential privacy.
- Treat serialized sketches as untrusted input at process boundaries. The parse
  APIs cap input size and reject invalid profiles, domains, shapes, counters, and
  register values.

See [SECURITY.md](https://github.com/llm-measurement/llm-sketchkit/blob/main/SECURITY.md)
for private vulnerability reporting and
[Operational Contracts](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/OPERATIONS.md)
for concurrency, ownership, resource, upgrade, and support guarantees.
The [API reference](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/API.md)
lists which failed mutations leave a sketch unchanged.

## Development

```sh
git clone https://github.com/llm-measurement/llm-sketchkit.git
cd llm-sketchkit
python -m venv .venv
source .venv/bin/activate
python -m pip install --upgrade pip==26.2.1 setuptools==84.0.0
python -m pip install -e '.[dev]'
```

## Wire Compatibility

Deterministic protobuf encoding is part of the compatibility surface. Go and
Python are checked against the same canonicalization, hashing, sketch, and
cross-language merge fixtures in
[`vectors/`](https://github.com/llm-measurement/llm-sketchkit/tree/main/vectors/).
Released state is also pinned by digest in
[`vectors/compat/`](https://github.com/llm-measurement/llm-sketchkit/tree/main/vectors/compat/)
and loaded by both implementations on every test run.

Run all local checks:

```sh
go test ./... -race
python -m pytest -q
ruff check .
mypy --strict
```

The optional Apache DataSketches comparison checks weighted frequent-items
query behavior against an independent implementation:

```sh
python -m pip install -e '.[oracle]'
python scripts/datasketches_oracle.py --check
```

## Reference

- [`spec/`](https://github.com/llm-measurement/llm-sketchkit/tree/main/spec/) defines canonicalization, hashing, profiles, and wire encoding.
- [`vectors/`](https://github.com/llm-measurement/llm-sketchkit/tree/main/vectors/) contains executable conformance fixtures.
- [`reports/`](https://github.com/llm-measurement/llm-sketchkit/tree/main/reports/) contains benchmark, accuracy, and oracle results.
- [`bench/`](https://github.com/llm-measurement/llm-sketchkit/tree/main/bench/) contains the Go benchmark harnesses.
- [`docs/FAQ.md`](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/FAQ.md) answers common adoption questions.
- [`docs/OPERATIONS.md`](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/OPERATIONS.md) defines runtime, concurrency, resource, upgrade, and support contracts.
- [`docs/SUPPLY_CHAIN.md`](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/SUPPLY_CHAIN.md) documents dependency controls, SBOMs, checksums, and PyPI attestation verification.
- [`CHANGELOG.md`](https://github.com/llm-measurement/llm-sketchkit/blob/main/CHANGELOG.md) records release-level changes.

## Limits

Reported tokens are not an invoice. HLL++ and MinHash error is statistical;
frequent-items supplies deterministic count bounds. Sketch instances are not
thread-safe: follow the [ownership rules](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/OPERATIONS.md#concurrency-and-ownership).
Each benchmark series describes one recorded machine and workload, with dates
and hardware in [the report](https://github.com/llm-measurement/llm-sketchkit/blob/main/reports/benchmarks.md).

## Status

`llm-sketchkit 0.2.x` is the current supported release line. Patch releases preserve the
documented wire formats, named hash domains, and Go and Python APIs exercised by
the checked-in conformance vectors. Additive or incompatible changes to that
surface receive a new minor version and are called out in the changelog before
`1.0`. Supported runtimes, deprecation notice, and security backports follow the
[operational policy](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/OPERATIONS.md#support-and-deprecation).

## License

Apache-2.0.

## Feedback

Questions, integration reports, or feedback: [open an issue](https://github.com/llm-measurement/llm-sketchkit/issues).
See [Contributing](https://github.com/llm-measurement/llm-sketchkit/blob/main/CONTRIBUTING.md) for checks and signed, signed-off commits.
Use the private reporting instructions in [SECURITY.md](https://github.com/llm-measurement/llm-sketchkit/blob/main/SECURITY.md)
for vulnerabilities; do not include secrets or raw customer identifiers.
