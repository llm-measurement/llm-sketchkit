# llm-sketchkit

[![CI](https://github.com/llm-measurement/llm-sketchkit/actions/workflows/ci.yml/badge.svg)](https://github.com/llm-measurement/llm-sketchkit/actions/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/llm-measurement/llm-sketchkit/badge)](https://scorecard.dev/viewer/?uri=github.com/llm-measurement/llm-sketchkit)

`llm-sketchkit` is a Go and Python library for measuring LLM usage. It counts
distinct users and ranks the heaviest users, sessions or keys by tokens, and keeps
the results in small summary files you can store, combine and share.

Use it to answer **how many distinct users your LLM app has, and who uses the most
tokens, across services, days and tools:**

- **Fast:** [0.026 ms or less per range query](https://llm-measurement.github.io/summaries.html), measured as a batch average.
- **Cheap to keep:** [305 days in 2.1 MB vs 1.25 GB of source records](https://llm-measurement.github.io/summaries.html).
- **Right when combined:** [Two tools with 46 and 26 users add up to 72; combined correctly, it's 52.](https://llm-measurement.github.io/summaries.html)
- **Portable:** [Go and Python, same bytes, shared conformance vectors](https://github.com/llm-measurement/llm-sketchkit/blob/main/vectors/sketches/README.md).
- **Private by design:** [Keyed hashing before counting, no raw IDs in the files](https://llm-measurement.github.io/summaries.html); keyed and pseudonymous, not anonymous.

| | |
| --- | --- |
| Languages | Go and Python, one wire format, checked by shared conformance vectors |
| Used by | [otelcol-genai-sketches](https://github.com/llm-measurement/otelcol-genai-sketches) and [fleetdiff](https://github.com/llm-measurement/fleetdiff) |
| <a id="license"></a>License | [Apache-2.0](https://github.com/llm-measurement/llm-sketchkit/blob/main/LICENSE) |

![Python notebook showing synthetic truth inside token-volume bounds from Go summaries](https://raw.githubusercontent.com/llm-measurement/llm-sketchkit/main/docs/images/token-bounds.png)
<a id="go-to-python-notebook"></a>
[Go-to-Python notebook and walkthrough](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/HOW_IT_WORKS.md#go-to-python-notebook).

## Use It To

- **Tracking tokenmaxxing?** Whether you run a usage leaderboard or set per-team AI budgets, rank the heaviest users by token volume, with guaranteed bounds, from keyed IDs rather than a raw per-employee table. Then see how concentrated the usage is. [Runnable example](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/GUIDE.md#token-volume-heavy-hitters).
- Count distinct users across days, workers and services without counting the same user twice.
- Compare overlap between sets of prompts, tools or documents.
- Share and merge measurements across Go and Python while keeping raw identities at the source.

<a id="requirements"></a>

## Install

Python 3.11-3.14; Go 1.26 or 1.27 with the latest security patch for `main` (published `0.2.x` releases also support Go 1.25).

```sh
python -m pip install llm-sketchkit
# Go: inside a module (run "go mod init example" in a new directory first).
go get github.com/llm-measurement/llm-sketchkit/go/sketchkit/...@latest
```

### Python

Generate a process secret, then count users and rank token consumption:

```sh
export LLM_SKETCHKIT_SECRET="$(python -c 'import secrets; print(secrets.token_hex(32))')"
python - <<'PY'
from llm_sketchkit import USER_V1, canonicalize_text_v1, frequentitems, hash64, hllpp, secret_from_env

secret = secret_from_env("LLM_SKETCHKIT_SECRET")
users = hllpp.new("small", USER_V1)
heaviest = frequentitems.Sketch("small", USER_V1)
for user_id, tokens in [("ana", 1_200), ("ben", 300), ("ana", 2_400), ("cy", 900)]:
    digest = hash64(secret, USER_V1, canonicalize_text_v1(user_id))
    users.add_hash(digest)
    heaviest.add_hash(digest, tokens)
top = max(heaviest.frequent_items(frequentitems.NO_FALSE_NEGATIVES), key=lambda i: i.lower_bound)
print(f"distinct users: {users.estimate():.0f}; heaviest user: {top.lower_bound:,} tokens")
PY
```

Output: `distinct users: 3; heaviest user: 3,600 tokens`.

### Go

With the same secret, put this in `main.go` and run `go run .`. These user IDs are already canonical ASCII.

```go
package main

import (
	"fmt"
	"log"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
)

func main() {
	secret, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_SECRET")
	if err != nil {
		log.Fatal(err)
	}
	users, err := hllpp.New(hllpp.ProfileSmall, sketchhash.UserV1, sketchhash.HMACSHA25664)
	if err != nil {
		log.Fatal(err)
	}
	for _, userID := range []string{"ana", "ben", "ana", "cy"} {
		digest, err := sketchhash.Hash64(secret, sketchhash.UserV1, []byte(userID))
		if err != nil {
			log.Fatal(err)
		}
		users.AddHash(digest)
	}
	fmt.Printf("distinct users: %.0f\n", users.Estimate())
}
```

Output: `distinct users: 3`. See [Go canonicalization and the token-ranking gateway example](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/GO.md).

## Choosing An Integration

| Your pipeline | Use | Why |
|---|---|---|
| GenAI spans already flow through an OpenTelemetry Collector | [OpenTelemetry Collector connector](https://github.com/llm-measurement/otelcol-genai-sketches) | It produces usage metrics, distinct counts and keyed rankings while keeping your existing trace backend. |
| You have compatible summary exports and want to compare two windows across operators | [fleetdiff](https://github.com/llm-measurement/fleetdiff) | A local, read-only command reports usage changes, distinct activity, tracked-item bounds, and missing coverage. |
| A custom Go or Python streaming service processes events | `llm-sketchkit` directly | Measure events locally, then serialize or merge compatible summaries for each window. |
| A batch or warehouse job reads stored events | `llm-sketchkit` directly | Create summaries per partition or window and merge them before publishing results. |
| You need to store or visualize finished metrics | Your existing backend integration | Send aggregated results to ClickHouse, Datadog, Prometheus, or a similar system. |

## Limits

Distinct counts and similarity estimates have statistical error; ranking bounds are deterministic. The opening measurements describe one machine and one lab's released Claude Code and Codex records. Query times are batch averages over daily states already in memory; the whole-period distinct estimate rounds to 52.
The [summary measurements](https://llm-measurement.github.io/summaries.html) and [whole-period merge check](https://github.com/llm-measurement/llm-sketchkit/blob/main/reports/summary-union.md) record the inputs, methods and commands. Library microbenchmarks have their own machine and workload details in [the report](https://github.com/llm-measurement/llm-sketchkit/blob/main/reports/benchmarks.md).

Reported tokens describe usage rather than billing. For concurrent access, follow the [ownership rules](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/OPERATIONS.md#concurrency-and-ownership).

## Documentation

- <a id="included-sketches"></a><a id="how-it-works"></a><a id="evidence-at-a-glance"></a> [How it works](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/HOW_IT_WORKS.md) and [evidence at a glance](https://llm-measurement.github.io/summaries.html).
- <a id="when-this-fits"></a><a id="questions-it-answers"></a><a id="token-volume-heavy-hitters"></a><a id="merge-sketches"></a> [Usage guide: questions, token-heavy keys and merging](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/GUIDE.md).
- <a id="go-gateway-distinct-users-and-top-keys-by-tokens"></a> [Go and the gateway example](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/GO.md), [Go-to-Python notebook](https://github.com/llm-measurement/llm-sketchkit/blob/main/examples/go-to-python/README.md) and [summary exchange](https://github.com/llm-measurement/llm-sketchkit/blob/main/examples/summary-exchange/README.md).
- <a id="security-and-privacy"></a><a id="wire-compatibility"></a><a id="reference"></a> [Security and privacy](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/SECURITY_PRIVACY.md), [wire compatibility](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/WIRE.md) and [API reference](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/API.md).
- <a id="development"></a> [Development and contributing](https://github.com/llm-measurement/llm-sketchkit/blob/main/CONTRIBUTING.md), [FAQ](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/FAQ.md) and [all documentation](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/README.md).

<a id="status"></a>

**Status:** Pre-1.0; `0.2.x` is the supported release line. See the [changelog](https://github.com/llm-measurement/llm-sketchkit/blob/main/CHANGELOG.md) and [compatibility policy](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/WIRE.md#status).
<a id="feedback"></a>

**Feedback:** [Open an issue](https://github.com/llm-measurement/llm-sketchkit/issues), [contribute](https://github.com/llm-measurement/llm-sketchkit/blob/main/CONTRIBUTING.md), or [report a vulnerability privately](https://github.com/llm-measurement/llm-sketchkit/blob/main/SECURITY.md).
