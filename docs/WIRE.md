# Wire Compatibility

Deterministic protobuf encoding is part of the compatibility surface. Go and
Python are checked against the same canonicalization, hashing, sketch, and
cross-language merge fixtures in
[`vectors/`](https://github.com/llm-measurement/llm-sketchkit/tree/main/vectors/).
Released state is also pinned by digest in
[`vectors/compat/`](https://github.com/llm-measurement/llm-sketchkit/tree/main/vectors/compat/)
and loaded by both implementations on every test run.

Run all local checks:

```sh
go test ./... -race
python -m pytest -q
ruff check .
mypy --strict
```

The optional Apache DataSketches comparison checks weighted frequent-items
query behavior against an independent implementation:

```sh
python -m pip install -e '.[oracle]'
python scripts/datasketches_oracle.py --check
```

## Status

See the [security policy](../SECURITY.md#supported-versions) for supported release lines. Patch releases preserve the
documented wire formats, named hash domains, and Go and Python APIs exercised by
the checked-in conformance vectors. Additive or incompatible changes to that
surface receive a new minor version and are called out in the changelog before
`1.0`. Supported runtimes, deprecation notice, and security backports follow the
[operational policy](https://github.com/llm-measurement/llm-sketchkit/blob/main/docs/OPERATIONS.md#support-and-deprecation).
