# Security And Privacy

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

See [SECURITY.md](https://github.com/llm-measurement/llm-sketchkit/blob/main/SECURITY.md)
for private vulnerability reporting and
[Operational Contracts](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/OPERATIONS.md)
for concurrency, ownership, resource, upgrade, and support guarantees.
The [API reference](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/API.md)
lists which failed mutations leave a sketch unchanged.

## Feedback

Questions, integration reports, or feedback: [open an issue](https://github.com/llm-measurement/llm-sketchkit/issues).
See [Contributing](https://github.com/llm-measurement/llm-sketchkit/blob/main/CONTRIBUTING.md) for checks and signed, signed-off commits.
Use the private reporting instructions in [SECURITY.md](https://github.com/llm-measurement/llm-sketchkit/blob/main/SECURITY.md)
for vulnerabilities; do not include secrets or raw customer identifiers.
