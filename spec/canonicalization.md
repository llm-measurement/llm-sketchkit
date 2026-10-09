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
2. Apply stream-safe NFC as defined below.
3. Replace every CRLF (`\r\n`) and lone CR (`\r`) with LF (`\n`).
4. Trim leading and trailing Unicode White_Space code points.
5. Apply stream-safe NFC again.
6. Encode as UTF-8 bytes.

The empty string is valid. After trimming, an all-whitespace input canonicalizes
to the empty byte string.

### Exact White_Space Set

Trimming MUST use precisely these 25 Unicode 15.0 `White_Space` code points:

```text
U+0009..U+000D U+0020 U+0085 U+00A0 U+1680 U+2000..U+200A
U+2028 U+2029 U+202F U+205F U+3000
```

In particular, U+001C through U+001F, U+180E, U+200B and U+FEFF are preserved.
Interior whitespace is preserved except for the newline conversion in step 3.

### Stream-Safe NFC

`text_v1` follows the existing Go `golang.org/x/text/unicode/norm.NFC`
behavior. The Unicode 15.0 reference is `golang.org/x/text v0.42.0` built with
Go 1.26.9. It incorporates the [UAX #15 Stream-Safe Text Process](https://www.unicode.org/reports/tr15/#Stream_Safe_Text_Format),
with Go's additional treatment of backward-combining starters:

1. For each original scalar, count leading and trailing non-starters in its
   full compatibility decomposition (NFKD). Count a decomposed scalar when its
   canonical combining class is nonzero **or** it can combine backwards,
   including modern Hangul Jamo V and T. This counting does not replace the
   original text with NFKD text.
2. Maintain the count since the last starter. If adding the next scalar's
   leading count would exceed 30, insert U+034F COMBINING GRAPHEME JOINER
   **before that original scalar**, and reset the count to zero.
3. Add its leading count if nonzero; otherwise set the count to its trailing
   count. The reference's nonzero leading and trailing counts are equal.
4. Apply NFC to the resulting text. Preserve both existing and inserted U+034F.

For example, U+0344 contributes two non-starters, U+00A8 contributes a trailing
non-starter despite having combining class zero, and U+AC01 contributes two
trailing Jamo. A test of `combining(character) != 0` alone is insufficient.
Insertion occurs before normalization reorders or composes those characters.

### Unicode Identity Boundary

The cross-language identity guarantee covers strings consisting entirely of
Unicode scalar values assigned in Unicode **15.0.0**, including private-use
values. Surrogates are invalid UTF-8. Unassigned values, noncharacters, and
characters assigned after Unicode 15.0 are outside that guarantee; this is a
compatibility boundary, not an additional rejection rule. Their behavior can
depend on the runtime's Unicode database.

Python uses its standard library normalization data, with the ten combining
class additions needed for Python 3.11's Unicode 14 database. These additions
have no canonical decomposition or composition mappings. Canonical ordering
and composition blocking must nevertheless account for their classes.
The differential verification corpus is selected from the Go Unicode 15
assigned set, not from whichever set the Python runtime happens to recognize.
Run it against Python 3.11 and 3.14 as described in
[identity verification](../docs/IDENTITY_VERIFICATION.md).

The profile name, public API and wire format remain unchanged. See the
[minor-release migration note](../docs/IDENTITY_MIGRATION.md) before mixing
retained Python-produced identities with newly produced identities.

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
