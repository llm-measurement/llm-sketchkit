# Identity Vectors

These files append coverage without modifying the earlier hash or sketch vectors.
They are loaded by Go and Python unit tests and by
`python -m scripts.differential_identity --all-assigned`.

- `text_v1_unicode15.json`: schema version 1, Unicode version, public fixture
  secret, and named cases with input strings, canonical UTF-8 hex and 64-bit
  HMAC digest hex. JSON escaping preserves the exact input scalars.
- `hllpp_estimates.json` and `hllpp_bias_regressions.json`: schema version 1,
  `splitmix64` generator, and named cases with profile, update count, initial
  seed, optional forced dense conversion, expected big-endian binary64 estimate
  bits, wire SHA-256 and allowed ULP distance. `force_dense` is always present.

For SplitMix64, add `0x9e3779b97f4a7c15` to the state before each output,
multiply `(z ^ (z >> 30))` by `0xbf58476d1ce4e5b9`, then multiply
`(z ^ (z >> 27))` by `0x94d049bb133111eb`, and emit `z ^ (z >> 31)`.
All arithmetic wraps modulo 2^64. Insert those hashes in order through the
public HLL++ API, then force dense mode only when requested.

Bias and raw estimate cases require exact bits. Linear-counting cases allow
two ULPs for platform logarithms. This tolerance does not apply to wire bytes
or canonical identities. Fixture secrets are public test data.

See [verification commands and reproduction](../../docs/IDENTITY_VERIFICATION.md)
and the [migration note](../../docs/IDENTITY_MIGRATION.md).
