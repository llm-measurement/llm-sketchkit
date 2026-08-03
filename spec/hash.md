# Hashing Specification

Status: version 1.

## Scope

This specification defines the stable 64-bit hash values used as sketch input.
All sketches operate on already-hashed `uint64` values. Producers MUST apply:

1. canonicalization
2. domain-separated keyed hashing
3. sketch update

Implementations MUST NOT combine, reorder, or duplicate these operations.

## Domains

A hash domain is a lowercase ASCII string registered in this file. Domain
separation is mandatory. The same canonical bytes hashed under different
domains MUST produce different digests except with normal cryptographic
collision probability.

Registered version 1 domains:

| Domain | Use |
|---|---|
| `prompt:v1` | Prompt or prompt-signature content |
| `user:v1` | End-user or account identity keys |
| `tool:v1` | Tool names, tool call keys, or tool argument signatures |
| `retrieval-doc:v1` | Retrieval document identifiers |
| `session:v1` | Session, conversation, trace, or request grouping keys |
| `mcp-session:v1` | MCP session identifiers |
| `mcp-method:v1` | MCP request or notification method names |
| `tool-error:v1` | Ordered `(tool name, error type)` signatures |

For `tool-error:v1`, canonicalize each value with `text_v1`, join the two byte
strings with one NUL byte, and hash the result.

Reusing a registered domain for a different semantic field is a compatibility
break. Implementations MUST reject unregistered domains.

## `hmac_sha256_64`

`hmac_sha256_64` is the only supported version 1 hash algorithm. The digest is:

```text
HMAC-SHA256(secret, domain || 0x00 || canonical_bytes)
```

The 64-bit sketch hash is the first eight bytes of the HMAC digest interpreted
as an unsigned big-endian integer. Hex encodings in vectors and diagnostic
surfaces MUST be lowercase, fixed-width, 16-character big-endian encodings of
those eight bytes.

The `0x00` separator is part of the input and MUST be emitted even when
`canonical_bytes` is empty.

## Secret Semantics

The secret is an opaque byte string supplied by the caller. Implementations MUST
load it from protected secret material and MUST NOT log, serialize, export, or
include it in errors. Test vectors MAY include example secrets explicitly.

Mergeability requires identical secret bytes across producers. Secret rotation
intentionally resets comparability and cross-producer mergeability.
Implementations MUST treat mismatched hash metadata as a merge error rather than
attempting conversion.

## Algorithm Registry

| Algorithm | Status | Notes |
|---|---|---|
| `hmac_sha256_64` | Supported | HMAC-SHA256 truncated to 64 bits |
| `siphash_64` | Reserved and rejected | Not part of the version 1 compatibility surface |

## Sketch-Local Hash Families

Bloom and MinHash need deterministic hash-family values derived from the
already-keyed 64-bit sketch hash. This family is not a replacement for
`hmac_sha256_64`, is not secret, and MUST NOT be applied to raw canonical bytes.

All arithmetic in this section is unsigned 64-bit arithmetic modulo 2^64. Right
shifts are logical shifts. Implementations in languages with unbounded integer
arithmetic MUST mask intermediate state to 64 bits after each multiply/xor
operation that changes `x`. `mix64(x)` is:

```text
x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
x = (x ^ (x >> 27)) * 0x94d049bb133111eb
return x ^ (x >> 31)
```

The family base seeds are the first eight bytes of SHA-256 over the ASCII tag,
interpreted as unsigned big-endian integers:

| Family | Tag | Base seed |
|---|---|---:|
| Bloom | `llm-sketchkit:bloom:v1` | `0x83984a98fd448a39` |
| MinHash | `llm-sketchkit:minhash:v1` | `0xea59f2718f8069a6` |

For zero-based family index `i` and keyed sketch input `h`, the family value is:

```text
seed_i = mix64(base_seed ^ uint64(i))
family_i(h) = mix64(h ^ seed_i)
```

Bloom position `i` is `family_i(h) mod bloom_bit_count`.

MinHash signature position `i` is updated as:

```text
signature[i] = min(signature[i], family_i(h))
```

Empty MinHash signatures initialize every position to `0xffffffffffffffff`.
Implementations in every language MUST reproduce these operations byte for byte,
including overflow behavior.

## Worked Example

The example in `vectors/hash/example_text_v1_hmac_sha256_64.json` uses:

| Field | Value |
|---|---|
| secret UTF-8 | `example-vector-secret` |
| domain | `prompt:v1` |
| canonical bytes UTF-8 | `Hello, sketchkit!` |
| message bytes | `prompt:v1 || 00 || Hello, sketchkit!` |
| digest64 hex | `6709352f3df45c33` |
| digest64 uint | `7424523937716132915` |
