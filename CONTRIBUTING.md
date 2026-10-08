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

## Signed and signed-off commits

Every pull-request commit needs both:

- A cryptographic signature that GitHub verifies, to establish commit provenance.
- A `Signed-off-by: Name <email>` trailer matching the commit author's name and
  email. This records your certification under the
  [Developer Certificate of Origin](https://developercertificate.org/) that you
  have the right to contribute the work under this project's Apache-2.0 license.

A signature and a sign-off serve different purposes; neither replaces the other.
The `commit-signatures` and `dco` checks name every failing commit. A passing final
commit does not cover earlier commits. The DCO check exempts only GitHub's own
verified branch-update merge commits; ordinary merges and bots still need sign-offs.

### Set up SSH signing

Use Git 2.34 or later. Use an existing signing key or create one with a passphrase;
do not overwrite an existing key:

```sh
ssh-keygen -t ed25519 -C "your-verified-email@example.com" -f "$HOME/.ssh/id_ed25519_signing"
eval "$(ssh-agent -s)"
ssh-add "$HOME/.ssh/id_ed25519_signing"
```

Add the contents of `~/.ssh/id_ed25519_signing.pub` to GitHub under **Settings >
SSH and GPG keys > New SSH key**, choosing **Signing key**. Upload only the public
`.pub` file; a key registered only for authentication is not enough.

From this repository, configure your author identity and signing. Use an email
verified on GitHub; your GitHub-provided no-reply email also works.

```sh
git config --local user.name "Your Name"
git config --local user.email "your-verified-email@example.com"
git config --local gpg.format ssh
git config --local user.signingkey "$HOME/.ssh/id_ed25519_signing.pub"
git config --local commit.gpgsign true
git commit -s -S -m "Describe the change"
```

`-s` adds the DCO trailer; `-S` creates the cryptographic signature. GPG signatures
verified by GitHub are also accepted. Read the DCO before signing off.

### Fix your own commits

For the latest commit, use `git commit --amend -s -S --no-edit`. For a branch
containing only your own contributions, fetch the current base and re-sign it:

```sh
git fetch origin
git rebase --force-rebase --signoff --gpg-sign origin/main
git push --force-with-lease
```

The rebase command rewrites commits even when the branch is already up to date.
These commands change commit IDs. Coordinate before rewriting a shared branch.
Use the name and email matching your commit author identity, and ask other authors
to fix their own commits; do not add their certification for them. After pushing,
confirm **Verified** on each commit and passing `commit-signatures` and `dco`
checks. Both checks reject incomplete API results, including PRs over 250 commits.

See GitHub's [SSH signing setup](https://docs.github.com/en/authentication/managing-commit-signature-verification/telling-git-about-your-signing-key#telling-git-about-your-ssh-key),
[adding an SSH signing key](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/adding-a-new-ssh-key-to-your-github-account),
and [signing commits](https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-commits).
