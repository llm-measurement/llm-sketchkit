# Combine Summaries From Separate Systems

Two teams can keep their collectors and raw telemetry separate, then exchange
bounded summary files for an agreed scope. No hashing secret is needed on the
machine combining compatible summaries.

This example uses the summary API available from `0.2.0`. It generates its own
four synthetic requests; no collector, account, or model API key is required.
From the repository root, with Python 3.11 or later:

```sh
python -m venv .venv
source .venv/bin/activate
python -m pip install -e .
export LLM_SKETCHKIT_SECRET="$(python -c 'import secrets; print(secrets.token_hex(32))')"
python examples/summary-exchange/generate.py
unset LLM_SKETCHKIT_SECRET
python examples/summary-exchange/combine.py \
  --expected platform data \
  --window-start 120000000000 \
  -- examples/summary-exchange/generated/*.json
```

The generator writes two summary envelopes to the ignored `generated/` directory.
The combiner runs without the secret. Expected counters are `requests: 4` and
`tokens: 14720`, with `missing: []`, `partial: []`, and two source records. The
distinct estimate rounds to 3; the token estimates are 8900, 3600, and 2220 with
equal lower and upper bounds for this tiny input. Hashes and unrounded HLL++
estimates depend on the generated secret. These are example outputs, not accuracy
or throughput claims. Re-running the generator replaces only its two named files;
do not mix generated files from runs with different secrets under one key ID.

For real exports, replace the file paths and window start with your own values.
Output includes window
counters, distinct estimates, tracked heavy items with bounds, contributing
epochs, missing producers, and partial observation intervals. Files are read
locally; nothing is uploaded. The `--` separates the expected producer list from
file paths. Large input sets are rejected rather than silently truncated.

Before combining, operators must agree on scope, accounting rules, hashing key
and version, and window duration. Each producer must own a disjoint stream of
observations. Repeated *summary files* are handled; repeated *underlying spans*
across collectors are not. Producer declarations are not authentication.

## Go

This complete program reads the same generated inputs. Put it in a separate
directory as `main.go`, run `go mod init example`, then
`go get github.com/llm-measurement/llm-sketchkit/go/sketchkit/...@latest`.
From the repository root, run `go run /path/to/main.go`:

```go
package main

import (
    "fmt"
    "log"
    "os"

    "github.com/llm-measurement/llm-sketchkit/go/sketchkit/summary"
)

func main() {
    var inputs []summary.Envelope
    for _, name := range []string{"platform", "data"} {
        data, err := os.ReadFile("examples/summary-exchange/generated/" + name + ".json")
        if err != nil { log.Fatal(err) }
        doc, err := summary.Parse(data)
        if err != nil { log.Fatal(err) }
        inputs = append(inputs, doc)
    }
    combined, err := summary.Combine(inputs, []string{"platform", "data"})
    if err != nil { log.Fatal(err) }
    fmt.Printf("requests=%d tokens=%d missing=%v partial=%v\n",
        combined.Counters["requests"], combined.Counters["tokens"],
        combined.Missing, combined.Partial)
}
```

Expected output: `requests=4 tokens=14720 missing=[] partial=[]`.
`combined.Sketches` contains complete new state that existing sketch parsers can
read. `combined.Missing` and `combined.Partial` must remain visible to callers.

## Compare Windows

Use [fleetdiff](https://github.com/llm-measurement/fleetdiff) for a local, read-only
comparison without writing your own report code. Put each window's exports from
all expected producers in a separate directory, then run the installed command:

```sh
fleetdiff compare --before ./before --after ./after --expected platform,data
```

It reports counter changes, distinct estimates, tracked-item change bounds, and
missing coverage. See the [two-operator trial](https://github.com/llm-measurement/fleetdiff/blob/main/docs/TWO_OPERATOR_TRIAL.md)
for setup and sharing requirements. It does not recover every unknown heavy mover
or prove why a change happened.

To build a custom comparison with the library instead:
`summary.Compatible` in Go or `summary.compatible` in Python checks whether two
envelopes have the same measurement contract, allowing different window starts.
Combine producers separately for each window, then compare the resulting counts
and estimates. Keep sketch uncertainty and source coverage beside each result.
Subtracting two truncated top-k lists does not discover every heavy mover.

See the [format contract](../../spec/summary.md) for replay, restart, privacy,
resource limits, and failure behavior. The shared vectors exercise all four
sketch kinds in Go and Python, including their additive update counters.
