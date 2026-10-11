# Go

## Requirements

- Go 1.26 or 1.27, with the latest security patch, for builds from `main`.
  The published `0.2.x` releases also support Go 1.25.
- Python 3.11, 3.12, 3.13, or 3.14

## Install And Count Distinct Prompts

Generate a process secret before running either example:

```sh
export LLM_SKETCHKIT_SECRET="$(python -c 'import secrets; print(secrets.token_hex(32))')"
```

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

## Go Gateway: Distinct Users And Top Keys By Tokens

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
