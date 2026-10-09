# Identity Migration For The Next Minor Release

Python `text_v1` will match current-main Go Unicode 15 behavior in the planned
0.3.0 minor release. The Python correction does not change Go's canonicalizer.
The separate `x/text` dependency update on main does change some Go identities
relative to 0.2.2, as described below. Both changes stay separate from the
output-preserving 0.2.3 hardening backport.

## Affected Python Inputs

- U+001C through U+001F at the edges previously disappeared under Python's
  broader `str.strip()` rule. They are now preserved.
- Runs exceeding 30 non-starters now receive U+034F COMBINING GRAPHEME JOINER.
  Counts include compatibility-decomposition tails and backward-combining
  starters, so fewer than 31 original characters can reach the limit.
- Python 3.11 now handles the Unicode 15 combining classes for U+10EFD..U+10EFF,
  U+11F41..U+11F42, U+1E08F and U+1E4EC..U+1E4EF. Ordering, composition blocking
  and stream-safe insertion involving those marks can change.

The [canonicalization specification](../spec/canonicalization.md) defines the
exact assigned-Unicode-15 boundary and whitespace set. Ordinary ASCII input and
the pre-existing conformance vector files are unchanged.

## Go Dependency Changes Since 0.2.2

The 0.2.x line uses `golang.org/x/text v0.41.0`. Main uses v0.42.0, whose
normalization corrections change some canonical bytes and hashes. For example,
`U+11F41 U+0301` previously normalized to the unrelated Greek character U+1F45;
the newer dependency preserves `U+11F41 U+0301`. This is a dependency correction,
not a new Go canonicalization rule introduced by the Python parity work.

Go producers upgrading from 0.2.x should treat potentially affected windows as
an identity migration too. The same keep-separate or rebuild procedure below
applies. The example identifies a confirmed affected input, not an exhaustive
list of everything corrected upstream. Main before and after the Python parity
work produces identical Go output on the shared identity corpus.

Check the resolved `golang.org/x/text` version in each Go build. Other module
dependencies can select a newer version than sketchkit's minimum, so the
sketchkit version alone does not identify the normalization behavior.

## Retained Summaries

The same minor release rejects zero-length observation intervals carrying
nonzero counters or any sketch payload. Empty intervals may carry zero counters
and an empty sketch map. Epochs with tied start times sort by end time and then
epoch identifier. Producers should record a positive observed interval whenever
they include measurements.

Protobuf groups are rejected at every known message level before decoding. The
schema defines no group fields, and existing canonical encodings contain none.
Recreate noncanonical external encodings with a conforming writer.

Affected old hashes will not match newly computed hashes of the same original
inputs. The wire format and `text_v1` profile name are unchanged, so
merge compatibility checks cannot identify a mixture of old and new identities.
There is no automatic repair from retained keyed hashes or sketch state.

1. Identify Python producers and Go producers upgrading from 0.2.x whose
   retained windows may contain affected inputs. Record producer and dependency
   versions outside the unchanged wire contract.
2. Upgrade affected producers together at a new observation-window boundary.
   Keep affected old and new windows separate in comparisons and merges.
3. When original inputs and the original hash key are legitimately available,
   rebuild affected retained state with the upgraded canonicalizer. Otherwise
   retain it as a separate historical series. Never relabel it as rebuilt.
4. Run the shared identity checks before resuming cross-language comparisons.
   Go-only pipelines already using the same v0.42.0 dependency need no identity
   rebuild for the Python-only correction.

## HLL++ Estimates

Python 3.12+ may report slightly different last bits after switching bias sums
to explicit left-to-right addition. Go and Python 3.11 already use that order.
Stored HLL++ registers, sketch bytes and hash identities are unchanged by this
estimate-only correction. Existing state can be read directly; recomputing its
estimate is sufficient. Statistical accuracy is unchanged.

## Limits

No new public API, profile, wire format or key-rotation mechanism is introduced.
Characters outside the assigned-Unicode-15 boundary remain runtime-dependent.
Vector tests require exact bias/raw estimates and allow two ULPs for the
platform logarithm in linear counting. The interval and protobuf acceptance
changes in this release are described under Retained Summaries.
