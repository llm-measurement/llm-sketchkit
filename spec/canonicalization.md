# Canonicalization Specification

Status: version 1.

## Scope

Canonicalization converts field values into deterministic UTF-8 byte strings
before keyed hashing. Applications MUST call the shared sketchkit implementation
instead of reimplementing these rules.

Canonicalization failures MUST be explicit errors. Implementations MUST NOT
silently replace invalid UTF-8, invent missing values, or hash non-text values
through text profiles without an explicit conversion rule.

## Profile

`text_v1` is the only supported canonicalization profile. Profile names are
wire-visible. Implementations MUST reject every other profile name.

| Profile | NFC | Newlines | Trim | Case fold | Whitespace fold |
|---|---:|---:|---:|---:|---:|
| `text_v1` | yes | yes | yes | no | no |

## `text_v1` Pipeline

For a Unicode string input, implementations MUST apply these operations in order:

1. Decode as UTF-8. Invalid UTF-8 is an error.
2. Normalize to Unicode NFC.
3. Replace every CRLF (`\r\n`) and lone CR (`\r`) with LF (`\n`).
4. Trim leading and trailing Unicode White_Space code points.
5. Normalize to NFC again.
6. Encode as UTF-8 bytes.

The empty string is valid. After trimming, an all-whitespace input canonicalizes
to the empty byte string.

## Input Type

Version 1 defines text canonicalization only. Numeric, boolean, bytes, and JSON
values have no canonical representation and MUST NOT be passed through `text_v1`
without an application-defined conversion to text.

## Worked Example

Input:

```text
  Hello, sketchkit!\r\n
```

Profile: `text_v1`

Canonical UTF-8 bytes:

```text
Hello, sketchkit!
```
