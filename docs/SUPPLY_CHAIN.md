# Supply-Chain Controls

Source review, dependency scanning, and signed publication records let you inspect
how an artifact was built and what it contains. This guide lists the controls and
shows how to verify a Python release.

## Source And Dependency Checks

- GitHub Actions are referenced by immutable commit SHA, with the release name in a
  comment for review.
- Pull requests run GitHub's dependency review and reject newly introduced known
  vulnerabilities rated moderate or higher.
- Go code runs `govulncheck`; the Python environment runs `pip-audit`.
- Dependabot checks Go modules, Python packages, and GitHub Actions each week.
- GitHub CodeQL, secret scanning, push protection, and private vulnerability
  reporting are enabled as repository controls.
- Branch rules require pull requests, signed commits, linear history, resolved review
  threads, and successful checks before `main` moves.

Forks and downstream copies do not inherit GitHub repository settings. Their owners
must enable equivalent controls in their own environment.

## Release Artifacts

Python releases use PyPI Trusted Publishing with GitHub OIDC. The release job does
not use a long-lived PyPI upload token. PyPI publish attestations are generated for
the wheel and source distribution.

The release workflow also generates an SPDX JSON software bill of materials with a
pinned Syft version and writes SHA-256 checksums. These files and the Python
distributions are attached to GitHub releases built after this control was added.
Use the SBOM for dependency review and vulnerability scanning.

### Publishing A Release

Run `release-distributions` by `workflow_dispatch` at the exact existing release
tag: the dispatch ref and `release_tag` input must name the same tag, whose source
commit contains this workflow and belongs to protected `main`. A published release
event does not start this workflow.

Both `publish_release` and `publish_pypi` default to `false`. The default run builds
once, assembles the wheel, source distribution, SPDX SBOM and `SHA256SUMS`, then
uploads all four files to a draft. Authenticated paginated release listings locate
the draft; subsequent checks and publication use its validated release ID, not a
draft-by-tag lookup. Names, sizes, SHA-256 digests, tag, source and draft state must
match before publication.

For an authorized end-to-end publication, select `publish_release=true` at dispatch
and, when publishing to PyPI, `publish_pypi=true`. The same run publishes the checked
draft, reads it back by ID, then sends the original wheel and source distribution
from the same artifact ID and checksum handoff through the existing `pypi`
environment. PyPI-only publication is refused. No job uploads GitHub assets after
publication, and no publication event rebuilds the distributions.

A draft-only run is not later promoted by another rebuilding dispatch. Existing
drafts or published releases and workflow reruns fail closed. Stop on a partial
failure; preserve the original artifact ID, checksums and release ID for a
separately authorized recovery, or fix forward with a new version. Do not replace
assets, retag, or skip a conflicting PyPI version.

## Verify The Published Python Artifact

The `0.1.0` wheel and source distribution both expose PyPI provenance tied to this
repository and `.github/workflows/release.yml`. Verify the wheel with the official
`pypi-attestations` client:

```sh
python -m pip install pypi-attestations
pypi-attestations verify pypi \
  --repository https://github.com/llm-measurement/llm-sketchkit \
  https://files.pythonhosted.org/packages/21/28/9e77bf6d5326d8df07b127980edfd26073dc1a44a46d1ddaf8711d43de4b/llm_sketchkit-0.1.0-py3-none-any.whl
```

Compare a downloaded artifact with the checksum published on PyPI or, for later
releases, the `SHA256SUMS` file attached to the GitHub release. Verification checks
the artifact and publisher identity represented by the attestation. Review the
dependencies and apply local software policy separately.
