# llm-sketchkit

[![CI](https://github.com/llm-measurement/llm-sketchkit/actions/workflows/ci.yml/badge.svg)](https://github.com/llm-measurement/llm-sketchkit/actions/workflows/ci.yml)

`llm-sketchkit` is a small Go and Python library for deterministic, mergeable
summaries of high-cardinality LLM data. It provides matching semantics for text
canonicalization, privacy-preserving keyed hashes, bounded sketches, and a
deterministic protobuf wire format.

Raw prompts and identifiers do not need to enter sketch state. Producers can
summarize locally and merge compatible sketches across processes or languages.

## Included Sketches

| Component | Use it for | Important property |
|---|---|---|
| HLL++ | Approximate distinct counts | Fixed memory and mergeable state |
| Weighted frequent-items | Heavy hitters and top items | Deterministic lower and upper bounds |
| Bloom filter | Set membership | No false negatives; configurable false-positive rate |
| MinHash | Approximate Jaccard similarity | Fixed-size mergeable signatures |

The Go and Python implementations share the same profiles, hash domains, test
vectors, and serialized representation.

## Requirements

- Go 1.25 or newer, with the latest security patch for that release line
- Python 3.11 or newer

## Install

Go:

```sh
go get github.com/llm-measurement/llm-sketchkit@latest
```

Python from a checkout:

```sh
git clone https://github.com/llm-measurement/llm-sketchkit.git
cd llm-sketchkit
python -m venv .venv
source .venv/bin/activate
python -m pip install --upgrade pip==26.2 setuptools==83.0.0
python -m pip install .
```

For development checks, install the optional tools:

```sh
python -m pip install -e '.[dev]'
```

## Quick Start

Generate a deployment secret and expose it to the process:

```sh
export LLM_SKETCHKIT_SECRET="$(python -c 'import secrets; print(secrets.token_hex(32))')"
```

Go HLL++ example:

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

Python HLL++ example:

```python
from llm_sketchkit import PROMPT_V1, canonicalize_text_v1, hash64, hllpp
from llm_sketchkit import secret_from_env

secret = secret_from_env("LLM_SKETCHKIT_SECRET")
sketch = hllpp.new("small", PROMPT_V1)

canonical = canonicalize_text_v1("  cafe\u0301\r\n")
sketch.add_hash(hash64(secret, PROMPT_V1, canonical))

print(f"estimated distinct prompts: {sketch.estimate():.0f}")
```

## Merge Sketches

Sketches merge only when their kind, profile, hash domain, hash algorithm, and
shape metadata match. A mismatch is an error rather than an implicit conversion.

```python
left = hllpp.new("small", PROMPT_V1)
right = hllpp.new("small", PROMPT_V1)

left.add_hash(hash64(secret, PROMPT_V1, canonicalize_text_v1("alpha")))
right.add_hash(hash64(secret, PROMPT_V1, canonicalize_text_v1("beta")))

left.merge(right)
print(f"merged distinct prompts: {left.estimate():.0f}")
```

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

See [SECURITY.md](SECURITY.md) for private vulnerability reporting.

## Wire Compatibility

Deterministic protobuf encoding is part of the compatibility surface. Go and
Python are checked against the same canonicalization, hashing, sketch, and
cross-language merge fixtures in [`vectors/`](vectors/).

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

- [`spec/`](spec/) defines canonicalization, hashing, profiles, and wire encoding.
- [`vectors/`](vectors/) contains executable conformance fixtures.
- [`reports/`](reports/) contains benchmark, accuracy, and oracle results.
- [`bench/`](bench/) contains the Go benchmark harnesses.

## Status

`llm-sketchkit` is an alpha library. The compatibility surface consists of the
specifications, the Go and Python APIs exercised by the vectors, and the checked-in
conformance fixtures.

## License

Apache-2.0.
