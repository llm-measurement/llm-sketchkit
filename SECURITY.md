# Security Policy

## Reporting A Vulnerability

Please use GitHub's private vulnerability reporting for this repository. Do not
open a public issue for a suspected vulnerability or include secrets, exploit
details, or sensitive data in public discussions.

Include the affected version or commit, a minimal reproducer, expected and
observed behavior, and your assessment of impact. Reports involving malformed
wire input, resource exhaustion, secret exposure, cross-language disagreement,
or dependency compromise are especially useful.

## Security Model

`llm-sketchkit` accepts pre-hashed values and serialized sketch state. Callers are
responsible for protecting hash secrets and limiting access to sketch outputs.
Keyed hashes provide pseudonymization and domain separation; they do not provide
anonymity or differential privacy.

The parser treats serialized sketches as untrusted input, caps input size, and
validates profile-specific shapes before constructing state.
