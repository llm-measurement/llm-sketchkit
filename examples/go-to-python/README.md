# Go-To-Python Notebook

This example produces mergeable HLL++ and weighted frequent-items summaries in
Go, then loads, validates, merges, and plots them in a Python notebook.

The producer generates two service shards in each of two windows. Raw synthetic
user IDs are canonicalized and keyed-hashed inside the Go process. The emitted
files contain sketch state, a manifest, and bounded synthetic validation
aggregates; they do not contain the raw IDs.

## Run It

From the repository root:

```sh
python -m venv .venv
source .venv/bin/activate
python -m pip install --upgrade pip
python -m pip install llm-sketchkit pandas matplotlib jupyterlab
jupyter lab examples/go-to-python/go-to-python.ipynb
```

Run the notebook from top to bottom. It invokes the Go producer itself. If
`LLM_SKETCHKIT_SECRET` is unset, the notebook creates an ephemeral secret for
that run without displaying it.

To preserve pseudonymous key comparability across notebook restarts, provide a
deployment secret explicitly:

```sh
export LLM_SKETCHKIT_SECRET="$(python -c 'import secrets; print(secrets.token_hex(32))')"
jupyter lab examples/go-to-python/go-to-python.ipynb
```

Generated files are written under `examples/go-to-python/generated/` and are
ignored by Git.

## What It Demonstrates

- Go and Python parse the same canonical wire representation.
- Re-serializing an unchanged Go-produced sketch in Python is byte-identical.
- Incompatible profile merges fail explicitly.
- Compatible service shards merge by window.
- HLL++ distinct estimates can be compared with a documented profile-level
  characterization bound.
- Weighted frequent-items returns deterministic lower and upper bounds for
  token-heavy pseudonymous keys.

The exact counts and token weights in `synthetic-validation.json` exist only to
check the tutorial's synthetic workload. Production consumers generally do not
have an exact side channel.

## Security Boundary

Keyed hashes are pseudonymous, not anonymous. Repeated values remain linkable
while the same secret and domain are in use, and anyone with the secret can test
a candidate value. Protect both the secret and serialized summaries. Secret
rotation intentionally breaks comparability and mergeability with older state.

The OpenTelemetry connector currently exports Prometheus metrics and bounded
structured top-item records. It does not export these serialized sketch files;
this example is specifically the library-to-notebook workflow.
