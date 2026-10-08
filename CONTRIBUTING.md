# Contributing

Bug reports, examples and focused improvements are welcome. Open an
[issue](https://github.com/llm-measurement/llm-sketchkit/issues/new/choose) for a
behavior change. Include synthetic input and the result you expected. Report
vulnerabilities through [Security](SECURITY.md), not a public issue.

Preserve Go/Python compatibility and add conformance cases for behavior changes.
Keep changes focused and retain existing license headers. With the development
dependencies installed, run:

```sh
go test ./... -race
python -m pytest -q
ruff check .
```

## Signed commits

Every commit in a pull request must have a signature GitHub verifies. The
`commit-signatures` check lists every unverified commit, including bot and merge
commits. A signed final commit does not cover earlier unsigned commits. A
`Signed-off-by` line (`git commit -s`) is not a cryptographic signature.

For SSH signing, use Git 2.34 or later. Use an existing signing key or create one
with a passphrase; do not overwrite an existing key:

```sh
ssh-keygen -t ed25519 -C "your-verified-email@example.com" -f "$HOME/.ssh/id_ed25519_signing"
eval "$(ssh-agent -s)"
ssh-add "$HOME/.ssh/id_ed25519_signing"
```

Add the contents of `~/.ssh/id_ed25519_signing.pub` to GitHub under **Settings >
SSH and GPG keys > New SSH key**, choosing **Signing key**. Upload only the public
`.pub` file; a key registered only for authentication is not enough.

From this repository, configure signing and an email verified on your GitHub
account (your GitHub-provided no-reply email also works):

```sh
git config --local user.email "your-verified-email@example.com"
git config --local gpg.format ssh
git config --local user.signingkey "$HOME/.ssh/id_ed25519_signing.pub"
git config --local commit.gpgsign true
git commit -S -m "Describe the change"
```

After pushing, confirm **Verified** on every PR commit and a passing
`commit-signatures` check. Existing unsigned commits need to be signed again by
their contributor; adding another signed commit does not fix them. Rewriting a
shared branch requires coordination and changes commit IDs. GPG signatures
verified by GitHub also satisfy the check.

Split PRs with more than 250 commits: GitHub's PR-commit API caps the list there,
and the check rejects incomplete lists rather than approving unexamined commits.

See GitHub's [SSH signing setup](https://docs.github.com/en/authentication/managing-commit-signature-verification/telling-git-about-your-signing-key#telling-git-about-your-ssh-key),
[adding an SSH signing key](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/adding-a-new-ssh-key-to-your-github-account),
and [signing commits](https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-commits).
