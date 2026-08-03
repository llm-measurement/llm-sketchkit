# Changelog

All notable changes to `llm-sketchkit` are recorded here. Versions follow
[Semantic Versioning](https://semver.org/) with alpha prereleases while the API
and wire-compatibility surface are being established.

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

[0.1.0-alpha.3]: https://github.com/llm-measurement/llm-sketchkit/compare/v0.1.0-alpha.2...v0.1.0-alpha.3
[0.1.0-alpha.2]: https://github.com/llm-measurement/llm-sketchkit/releases/tag/v0.1.0-alpha.2
