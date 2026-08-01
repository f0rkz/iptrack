# Contributing

Pull requests should pass `make test` and the Terraform provider tests. Use Conventional Commit titles so semantic-release can determine version changes:

- `feat:` for a backward-compatible feature
- `fix:` for a bug fix
- `feat!:` or a `BREAKING CHANGE:` footer for an incompatible change
- `docs:`, `test:`, `ci:`, and `chore:` for changes that should not release by themselves

Prefer a focused pull request with a squash-merge title that describes its user-visible outcome.
