# Changelog

All notable changes to `llm-sketchkit` are recorded here. Versions follow
[Semantic Versioning](https://semver.org/). Within each supported minor line,
patch releases preserve the documented wire formats, named hash domains, and
public Go and Python APIs exercised by the conformance vectors.

## [0.2.0] - 2026-09-05

- Added a versioned summary envelope and matching Go/Python APIs for combining
  measurements across independently operated producers without sharing raw input.
- Added snapshot replay handling, restart epochs, compatibility checks, and
  missing-producer and partial-window reporting, with shared canonical fixtures.
- Added a local summary-file example and an API contract for failure atomicity.
- Kept existing sketch algorithms and protobuf encodings unchanged. The optional
  JSON envelope wraps existing state and requires disjoint observation streams;
  it does not authenticate producers or deduplicate overlapping events.

- Replaced stale alpha wording in the FAQ with the current supported-release status.
- Added test coverage for every supported Go and Python runtime.
- Added continuous and scheduled fuzzing for untrusted wire input,
  canonicalization, merge behavior, and generated Go/Python differential cases.
- Added operational contracts for thread safety, mutation ownership, resource
  ceilings, upgrades, support lifetime, deprecation, and security backports.
- Added automated dependency updates and review, release SBOMs and checksums, and
  documented PyPI attestation verification.
- Added versioned `v0.1.0` state fixtures loaded by both implementations to protect
  patch and minor upgrade compatibility.

No sketch semantics, profiles, hash domains, or wire encodings changed.

## [0.1.0] - 2026-08-30

- Promoted the documented compatibility surface to a supported `0.1.x` release
  line.
- Made the published Python package the primary installation path.
- Added a runnable Go-to-Python notebook with cross-language wire validation,
  compatible merges, explicit mismatch rejection, and bounded plots.
- Generalized release checks for final and prerelease version tags.
- Made source distributions self-contained for their shipped conformance tests
  and refreshed the patched packaging-tool pin.

No sketch semantics, profiles, hash domains, or wire encodings changed.

## [0.1.0-alpha.4] - 2026-08-04

- Added task-oriented adoption guidance and answers to common integration,
  capacity, privacy, and compatibility questions.
- Added Apache-2.0 SPDX identifiers to source files and an automated check that
  keeps new source files consistent.

No sketch semantics, profiles, hash domains, or wire encodings changed in this
release.

## [0.1.0-alpha.3] - 2026-08-02

- Added a reproducible visual scorecard for performance, accuracy, and oracle
  results.
- Added a public explanation of the implementation boundary with Apache
  DataSketches.
- Refreshed Linux measurements on Go 1.26.5 and retained the complete five-run
  samples.
- Added clean wheel and source-distribution build, validation, and installation
  checks.
- Moved the generated protobuf module inside the `llm_sketchkit` package to
  avoid publishing a generic top-level namespace.
- Added a trusted-publishing workflow that remains disabled while the repository
  is private.
- Updated GitHub Actions to current, commit-pinned releases.

No sketch semantics, profiles, hash domains, or wire encodings changed in this
release.

## [0.1.0-alpha.2] - 2026-08-02

- Published deterministic Go and pure-Python implementations of HLL++, weighted
  frequent-items, Bloom filters, and MinHash.
- Published canonicalization, domain-separated HMAC-SHA256-64 hashing, named
  profiles, deterministic protobuf encoding, and shared conformance vectors.
- Added bounded parsing of untrusted serialized state and hardened secret
  loading.
- Added accuracy characterization, Go microbenchmarks, and an Apache
  DataSketches frequent-items oracle.

[0.2.0]: https://github.com/llm-measurement/llm-sketchkit/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/llm-measurement/llm-sketchkit/compare/v0.1.0-alpha.4...v0.1.0
[0.1.0-alpha.4]: https://github.com/llm-measurement/llm-sketchkit/compare/v0.1.0-alpha.3...v0.1.0-alpha.4
[0.1.0-alpha.3]: https://github.com/llm-measurement/llm-sketchkit/compare/v0.1.0-alpha.2...v0.1.0-alpha.3
[0.1.0-alpha.2]: https://github.com/llm-measurement/llm-sketchkit/releases/tag/v0.1.0-alpha.2
