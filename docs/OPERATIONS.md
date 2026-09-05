# Operational Contracts

This document states the runtime, ownership, resource, and compatibility contracts
for embedding `llm-sketchkit` in a long-running process.

## Supported Runtimes

| Runtime | Supported versions | Continuous integration |
|---|---|---|
| Go | 1.25 and 1.26, latest patch release | Tests run on both release lines |
| Python | 3.11, 3.12, 3.13, and 3.14 | Tests run on every listed version |

A runtime line is removed only in a minor release and is called out in the
changelog. Security patches to a supported runtime should be applied promptly by
the embedding application.

## Concurrency And Ownership

Sketch instances are mutable and are not internally synchronized. A caller must
confine each instance to one goroutine or thread, or protect every read and write
with the same external lock. Distinct sketch instances can be used concurrently.

Update and merge methods mutate the receiver. A merge does not mutate a distinct
source. `Clone`/`clone` returns independent state and is the supported way to
retain a pre-merge snapshot. Treat a receiver as unusable after a failed mutation
unless the method's API explicitly promises otherwise; cloning before a risky merge
provides transactional behavior at the application boundary.

The [per-method error reference](API.md) lists the unchanged-on-error cases and
the frequent-items operations that may leave partial changes. These are failure
contracts, not thread-safety guarantees.

Serialization and estimate methods do not intentionally mutate state, but they must
not run concurrently with an update or merge on the same instance. Returned MinHash
signatures and serialized byte strings are copies. Returned frequent-item records
are values rather than references into mutable state.

## Resource Ceilings

Named profiles bound the core state of one sketch. Exact constants and payload
sizes are listed in the [profile specification](../spec/profiles.md) and summarized
in the [FAQ](FAQ.md#how-much-memory-does-each-profile-use).

The surrounding application still controls total memory. Bound the number of live
windows, tenants, dimensions, and sketch instances. A bounded sketch multiplied by
an unbounded tenant map is an unbounded system.

Additional limits and failure behavior:

- Every wire parser rejects input larger than 4 MiB before protobuf decoding.
- Parsers validate the named profile and its exact shape before constructing state.
- Canonicalization is in-memory and has no library-level input limit. Bound raw text
  before calling it.
- Frequent-items weights use non-negative signed 64-bit counters. Bloom and MinHash
  update counts use unsigned 64-bit counters. Updates and merges reject overflow.
- Bloom false-positive targets apply at the rated insertion count. More insertions
  remain memory-bounded but increase the false-positive rate.
- CPU per update is bounded by the selected shape, but total processing work still
  grows with event rate and the number of sketches updated per event.

## Upgrade And Compatibility

Wire version, sketch kind, profile, hash domain, hash algorithm, and shape metadata
form the merge contract. Never merge state produced with different secrets, domains,
profiles, or window definitions even when parsing succeeds.

The [`v0.1.0` compatibility manifest](../vectors/compat/v0.1.0.json) identifies
released sparse and dense HLL++, frequent-items, Bloom, and MinHash state. Every Go
and Python test run parses those bytes, checks their digest and observable state, and
requires byte-identical serialization. Existing compatibility manifests are not
rewritten.

For an application upgrade:

1. Upgrade readers before writers when a release introduces a new wire version.
2. Load representative retained state in a canary process before broad deployment.
3. Keep the previous binary available until the canary has parsed, queried, and
   merged the retained state it will encounter.
4. Do not assume a downgrade can read state first emitted by a newer minor release.

Patch releases in a supported minor line preserve the documented wire and public API
surface. An incompatible wire change receives a new wire version and minor release.

## Support And Deprecation

The latest patch in the current stable pre-1.0 minor line receives bug and security
fixes. After a new stable minor is released, the previous stable minor receives
critical and high-severity security fixes for 90 days. Older minors and superseded
alpha releases are unsupported.

Public API removals are announced in the changelog for at least one stable minor
release when practical. A security issue may require an immediate removal or a
stricter default; that exception is documented in the release notes. Existing wire
identifiers and hash domains are never silently assigned new meanings.
