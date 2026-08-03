# Hash Vectors

Hash vectors validate canonicalization, domain separation, and `Hash64`.

Each file MUST validate against `vectors/schemas/hash_vector.schema.json`.
Test-vector secrets are intentionally embedded so implementations can reproduce
the digest exactly. Production secrets MUST never be serialized or logged.

Worked example:

```json
{
  "schema_version": 1,
  "name": "text_v1 prompt example",
  "canonicalization": "text_v1",
  "input": {"encoding": "utf8", "value": "  Hello, sketchkit!\\r\\n"},
  "canonical_bytes_hex": "48656c6c6f2c20736b657463686b697421",
  "domain": "prompt:v1",
  "hash_algo": "hmac_sha256_64",
  "secret": {"utf8": "example-vector-secret"},
  "expected": {
    "digest64_hex": "6709352f3df45c33",
    "digest64_uint": 7424523937716132915
  }
}
```
