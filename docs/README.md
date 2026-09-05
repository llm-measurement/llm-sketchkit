# Documentation

Start with the repository [README](../README.md) for installation, a working
example, merge behavior, and security guidance.

Adoption guidance:

- [Frequently asked questions](FAQ.md)
- [Why this is not a DataSketches wrapper](DATASKETCHES.md)
- [Operational contracts](OPERATIONS.md)
- [Per-method mutation and error behavior](API.md)
- [Supply-chain controls and artifact verification](SUPPLY_CHAIN.md)

Runnable example:

- [Produce summaries in Go, then merge and plot them in Python](../examples/go-to-python/README.md)

Normative behavior:

- [Canonicalization](../spec/canonicalization.md)
- [Hashing and domain separation](../spec/hash.md)
- [Sketch profiles](../spec/profiles.md)
- [Protobuf wire schema](../spec/sketches.proto)

Verification material:

- [Conformance vectors](../vectors/sketches/README.md)
- [Release compatibility fixtures](../vectors/compat/README.md)
- [Evidence scorecard](../reports/scorecard.md)
- [Benchmark and characterization reports](../reports/README.md)
