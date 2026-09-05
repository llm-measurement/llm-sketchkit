# Combine Summaries From Separate Systems

Two teams can keep their collectors and raw telemetry separate, then exchange
bounded summary files for an agreed scope. No hashing secret is needed on the
machine combining compatible summaries.

This example uses the summary API in the source checkout. It is not included in
the `0.1.0` PyPI release. From the repository root:

```sh
python -m pip install -e .
python examples/summary-exchange/combine.py \
  --expected platform data \
  --window-start 120000000000 \
  -- exports/platform/*.json exports/data/*.json
```

Replace the window start with a value from your files. Output includes window
counters, distinct estimates, tracked heavy items with bounds, contributing
epochs, missing producers, and partial observation intervals. Files are read
locally; nothing is uploaded. The `--` separates the expected producer list from
file paths. Large input sets are rejected rather than silently truncated.

Before combining, operators must agree on scope, accounting rules, hashing key
and version, and window duration. Each producer must own a disjoint stream of
observations. Repeated *summary files* are handled; repeated *underlying spans*
across collectors are not. Producer declarations are not authentication.

## Go

```go
left, err := summary.Parse(leftBytes)
if err != nil { return err }
right, err := summary.Parse(rightBytes)
if err != nil { return err }
combined, err := summary.Combine(
    []summary.Envelope{left, right}, []string{"platform", "data"},
)
```

Import `github.com/llm-measurement/llm-sketchkit/go/sketchkit/summary`.
`combined.Sketches` contains complete new state that existing sketch parsers can
read. `combined.Missing` and `combined.Partial` must remain visible to callers.

## Compare Windows

`summary.Compatible` in Go or `summary.compatible` in Python checks whether two
envelopes have the same measurement contract, allowing different window starts.
Combine producers separately for each window, then compare the resulting counts
and estimates. Keep sketch uncertainty and source coverage beside each result.
Subtracting two truncated top-k lists does not discover every heavy mover.

See the [format contract](../../spec/summary.md) for replay, restart, privacy,
resource limits, and failure behavior. The shared vectors exercise all four
sketch kinds in Go and Python, including their additive update counters.
