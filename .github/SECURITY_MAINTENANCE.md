# Actions Permissions

In **Settings > Actions > General > Workflow permissions**, keep the default
token read-only and turn off **Allow GitHub Actions to create and approve pull
requests**. This is a manual repository setting, not something a workflow changes.

No llm-sketchkit workflow creates or approves pull requests. Release publishing
uses job-scoped permissions for releases, attestations and PyPI trusted publishing;
it does not need this setting. Dependabot opens its PRs through its own GitHub App,
not the workflow token. The signature and DCO checks read PR metadata without
checking out PR code.

After changing the setting, confirm the next PR's checks run normally. Keep
publishing permissions scoped to the release or Scorecard job that needs them.
No personal access token or additional GitHub App is needed.

See GitHub's [workflow permission settings](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/enabling-features-for-your-repository/managing-github-actions-settings-for-a-repository#setting-the-permissions-of-the-github_token).
