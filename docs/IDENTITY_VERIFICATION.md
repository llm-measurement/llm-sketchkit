# Identity Verification

Run canonical-byte and keyed-hash comparisons against the Unicode 15 Go
reference under **both Python 3.11 and Python 3.14**. The same Go-selected
corpus is used on each runtime, including characters Python 3.11 considers
unassigned. The optional exhaustive pass checks every assigned scalar plus
normalization-sensitive contexts and seeded mixed-mark runs.

## CI Commands

The integrating change should provide a Python `[3.11, 3.14]` matrix with
Go **1.26.9** installed. Run these commands from an unsynced checkout in each
Python environment. No workflow changes are part of the identity change.

```sh
python -m pip install --require-hashes --only-binary=:all: -r requirements/dev.txt
export PYTHONPATH=python:.
export GOTOOLCHAIN=go1.26.9
python -m pytest -q
python -m scripts.differential_identity --all-assigned
python -m scripts.differential_fuzz --seed 20260903 --cases 12 --max-updates 320
python -m ruff check .
```

Once per integration, also run:

```sh
go test ./... -race
go vet ./...
python -m mypy --strict  # Python 3.11 environment
```

The differential command builds its helper in a temporary directory. It
requires the Go and x/text Unicode versions to be 15.0.0 and refuses a changed
oracle. Go 1.27 selects newer x/text tables and is not this reference. Normal
unit tests remain runnable under the supported Go versions.

## Checked Evidence

`vectors/identity/text_v1_unicode15.json` freezes canonical bytes and keyed
hashes for whitespace, controls, 29/30/31 and longer runs, decomposition,
backward composition, Hangul, explicit joiners and new Unicode 15 marks.
Both languages load it in their normal unit suites. Invalid UTF-8 still uses
the existing error types.

`vectors/validation/summary_intervals.json` distinguishes accepted zero-length
observations with configured empty sketches from rejected nonempty state. Both
languages also test every sketch kind, empty dense HLL++, and retained totals or
bits that can be hidden by an empty-looking entry list or signature.

`hllpp_estimates.json` covers both seeds for each of three profiles across
empty, sparse, promotion, forced-dense, linear-counting, bias and raw states.
`hllpp_bias_regressions.json` adds cases that fail with compensated Python
summation. All cases check exact wire SHA-256 and estimate bits, with two ULPs
allowed only in linear-counting cases. Differential runs compare wire bytes
directly as well. These are arithmetic and identity checks, not an accuracy
benchmark or a claim of exhaustive sequence coverage.

## Reproducing Vectors

From the prepared environment, generate into a new directory and compare:

```sh
fresh="$(mktemp -d)"
python -m scripts.generate_identity_vectors --output-dir "$fresh"
diff -u vectors/identity/text_v1_unicode15.json "$fresh/text_v1_unicode15.json"
diff -u vectors/identity/hllpp_estimates.json "$fresh/hllpp_estimates.json"
diff -u vectors/identity/hllpp_bias_regressions.json "$fresh/hllpp_bias_regressions.json"
```

The generator refuses to overwrite existing files. Append new vector files
for later regressions and preserve the original corpus bytes.
