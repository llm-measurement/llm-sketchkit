# Model Usage v1 Integration

Model Usage v1 is an optional extension to Summary Exchange v1. It records
per-model observed attempts, reported token coverage, quality diagnostics and
jointly complete token splits. It does not record raw telemetry or prices.
Shared conformance data lives under `vectors/model_usage/`: 112 cases,
65 exact wire fixtures, expected outputs and synthetic contract preimages.
The public data manifest binds those files by size and SHA-256.

## Go

Import `github.com/llm-measurement/llm-sketchkit/go/sketchkit/modelusage`.

- `Block`: contract/version/histogram header and `map[string]Model`.
- `Model`: one retained row, with `Fields`, `Complete Cohort`, `[4]Bin`,
  `Exclusions`, `Provenance`, requests, and cache-partition violations.
- `Cohort`: requests, four component totals, `InputMin` and `InputMax` pointers.
  Empty extrema are nil. Totals are not multiplied by requests.
- `ContractID(accountingID, mapping, Routing)`: exact configuration hash.
  `Routing` contains algorithm, max-label bytes, max slices, selected slice and
  ordered `[]Slice`. Each slice has name, ordered keys and resource policy.
  Resolve producer defaults first; empty resource policy expands to its keys.
- `New(contractID)`: enabled empty block. The unchanged base accounting ID is
  an input to `ContractID`, never a result of it.
- `ClassifyModel(observed, mapping, classification)`: approved-map lookup.
  Classification comes from the existing source/router: empty, `__overflow__`,
  `__missing__` or `__too_long__`. Overflow takes precedence; no raw pass-through.
- `AddAttempt(previous Model, Attempt) (Model, error)`: checked, nonmutating
  staging of one accepted attempt. `Attempt` has four `Observation` components
  and separate input/output origins. Observation has `Value *uint64`, `Invalid`,
  `Conflict` and `SubsetViolation`. Nil means missing. Empty origin means unknown.
- `Add(a, b Model)`: checked disjoint row sum. `Add(Model{}, row)` deep-clones
  the row, including all extrema pointers. A zero Model is an accumulator only,
  not a serializable row. Errors return a zero row and leave inputs unchanged.
- `Block.CheckNames(maxSlices)` and `Block.Validate(counters, instantaneous)`:
  budget and wire/conservation checks. Validation with nil counters checks the
  block alone; envelope validation always supplies the base counters.
- `Merge(a, b *Block)`: checked disjoint-owner sum, with newly owned output.
  Never use this to add cumulative snapshots. `summary.Combine` handles latest
  selection, identical replay, conflicting sequence, regression, and replacement.
- `summary.Envelope.ModelUsage` and `summary.Result.ModelUsage` are `*Block`.
  Nil omits the wire member; an enabled empty window uses `New` and `models:{}`.
- `summary.HasModelUsage(data)` is bounded structural capability detection.
  `summary.Parse` performs full validation. `modelusage.Error.Reason` is static
  and product-neutral. Existing base errors retain their prior text.

Collector owns acceptance, token alias precedence, deduplication, existing model
classification, LRU decisions and processing-time windows. Explicit unavailable
provenance must already have suppressed the base input/output observation. Supply
the existing quality flags; the helper also checks joint cache partitioning.
Stage row and base updates before committing either. Check model and byte budgets
before mutation; never drop an enabled block to fit. Retain rows on global windows,
not on evictable live metric slices. Reaching a retained-row budget does not authorize
deleting prior rows or relabeling their history as overflow.

## Python

`llm_sketchkit.model_usage` uses the same wire-shaped dictionaries, with
`Observation(value=None, invalid=False, conflict=False, subset_violation=False)`.
`collect(block, model_id, fields, input_origin, output_origin, max_models)` stages
one accepted attempt and returns independent state. `fields` has exactly input,
output, cache_read and cache_write observations. Origins default to unknown.

`contract_id`, `classify_model`, `empty`, `validate`, `compatible`,
`check_regression` and `merge` have the corresponding Go roles. `summary.Envelope`
and `summary.Result` append an optional `model_usage` argument defaulting to None.
`summary.has_model_usage` performs bounded structural preflight. Extension errors
inherit `summary.SummaryError` and carry a static `reason` attribute.

## Compatibility And Scope

Disabled wire bytes and base accounting are unchanged. Both summary combiners
reject mixed presence and incompatible contracts; model-set differences alone
are allowed. Operators still must check their expected contract ID. Hashes are
compatibility declarations, not authentication. No identity migration is implied.

The 0.2.x backport preserves its dependency declarations, normalization sources,
and identity/hash vectors. It does not copy newer normalization or unrelated base
summary behavior. Existing readers reject the additional member; upgrade readers
before enabling producers. No release/version number is assigned here.

Pricing and CLI diagnostics appear only in conformance tests, not a production
price engine. There is no key/team intersection. Consumer and collector
integration are separate from this library. Shared-library tests do not claim
collector OTLP execution; that is a separate integration check.
