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
