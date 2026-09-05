# Security Policy

## Supported Versions

| Version | Security fixes |
|---|---|
| Latest `0.2.x` | Yes |
| Latest `0.1.x` | Critical and high-severity fixes through 2026-12-04 |
| Superseded alpha releases | No |

The current stable pre-1.0 minor receives fixes. After a new stable minor is
released, the previous stable minor receives critical and high-severity security
fixes for 90 days. See the [operational contracts](docs/OPERATIONS.md) for the full
runtime and deprecation policy.

## Reporting A Vulnerability

Please use GitHub's private vulnerability reporting for this repository. Do not
open a public issue for a suspected vulnerability or include secrets, exploit
details, or sensitive data in public discussions.

Include the affected version or commit, a minimal reproducer, expected and
observed behavior, and your assessment of impact. Reports involving malformed
wire input, resource exhaustion, secret exposure, cross-language disagreement,
or dependency compromise are especially useful.

Maintainers aim to acknowledge a complete report within five business days. This is
a target, not a contractual response-time promise. Confirmed issues are handled with
a GitHub security advisory and coordinated disclosure when appropriate. Do not
publish exploit details before a fix or mitigation is available to affected users.

## Security Model

`llm-sketchkit` accepts pre-hashed values and serialized sketch state. Callers are
responsible for protecting hash secrets and limiting access to sketch outputs.
Keyed hashes provide pseudonymization and domain separation; they do not provide
anonymity or differential privacy.

The parser treats serialized sketches as untrusted input, caps input size, and
validates profile-specific shapes before constructing state.
