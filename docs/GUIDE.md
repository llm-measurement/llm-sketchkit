# Usage Guide

## When This Fits

Use `llm-sketchkit` when your application needs recurring usage answers from
measurements it can keep, combine and share. Rank token consumers as events
arrive, combine distinct-user counts across days and services, and pass summaries
to another team for analysis while keeping raw identities at the source.

Use it with hosted model APIs, self-hosted models, or a mixture of both. Your
pipeline supplies the identities and reported usage; the library processes that
data locally without calling a model provider. See the
[deployment FAQ](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/FAQ.md#can-i-use-this-with-self-hosted-models-or-a-mix-of-providers)
for input and comparison requirements.

### Questions It Answers

- **Tracking tokenmaxxing?** Whether you run a usage leaderboard or set per-team
  AI budgets, rank the heaviest users by token volume, with guaranteed
  bounds, from keyed IDs rather than a raw per-employee table. Then see how
  concentrated the usage is. See [Token-Volume Heavy Hitters](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/GUIDE.md#token-volume-heavy-hitters)
  for a runnable example.
- How many distinct users were active in this window?
- Which users, sessions or prompts account for the most reported tokens?
- How much do two sets of prompts, tools or documents overlap?
- Can Python read and merge the measurements produced by my Go services?
- Can several workers combine their measurements while keeping raw identities local?

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
| GenAI spans already flow through an OpenTelemetry Collector | [OpenTelemetry Collector connector](https://github.com/llm-measurement/otelcol-genai-sketches) | It produces usage metrics, distinct counts and keyed rankings while keeping your existing trace backend. |
| You have compatible summary exports and want to compare two windows across operators | [fleetdiff](https://github.com/llm-measurement/fleetdiff) | A local, read-only command reports usage changes, distinct activity, tracked-item bounds, and missing coverage. |
| A custom Go or Python streaming service processes events | `llm-sketchkit` directly | Measure events locally, then serialize or merge compatible summaries for each window. |
| A batch or warehouse job reads stored events | `llm-sketchkit` directly | Create summaries per partition or window and merge them before publishing results. |
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
It shows how one team's reported token usage can fall while the combined
total rises, using synthetic data. No account, upload, or model API key is needed.

## Canonicalizing Prompts In Python

Normalize text before hashing so equivalent prompt spellings share an identity.
This example trims whitespace, normalizes line endings and composes the accent.

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
