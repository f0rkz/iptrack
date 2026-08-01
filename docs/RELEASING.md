# Releasing iptrack

iptrack uses Conventional Commits and semantic-release. A successful CI run on `main` starts the release workflow. If qualifying commits exist, semantic-release calculates a version, creates a `vX.Y.Z` tag and draft GitHub release, publishes the application image, and lets GoReleaser attach signed Terraform provider artifacts before finalizing the release.

## Version rules

| Commit | Release |
|---|---|
| `fix: correct address allocation` | Patch |
| `feat: add VLAN inventory` | Minor |
| `feat!: change the API` or a `BREAKING CHANGE:` footer | Major |
| `docs:`, `test:`, `ci:`, `chore:` | No release by default |

Use squash merge for pull requests and give the squash commit a Conventional Commit title. The first `feat:` or `fix:` commit on `main` creates the initial release.

## Required repository setup

Create an RSA GPG signing key dedicated to provider releases. Add these GitHub Actions secrets:

- `GPG_PRIVATE_KEY`: ASCII-armored private key from `gpg --armor --export-secret-keys KEY_ID`
- `PASSPHRASE`: the private-key passphrase

Add the corresponding public key to the Terraform Registry account with `gpg --armor --export KEY_ID`. HashiCorp requires signed provider releases and supports RSA or DSA keys, not the default ECC key type.

After the first workflow publishes `ghcr.io/f0rkz/iptrack`, set the package visibility to public in GitHub Packages. The workflow uses the repository `GITHUB_TOKEN`; no registry password is needed.

Recommended repository settings:

- Require the `Go tests`, `Container build`, and `Release configuration` checks before merging to `main`.
- Require pull requests and disallow force pushes on `main`.
- Enable immutable GitHub releases after validating the first release.

## Published outputs

Application images are published for Linux AMD64 and ARM64 with these tags:

- `ghcr.io/f0rkz/iptrack:X.Y.Z`
- `ghcr.io/f0rkz/iptrack:X.Y`
- `ghcr.io/f0rkz/iptrack:X`
- `ghcr.io/f0rkz/iptrack:latest`

Images include OCI source/version labels, SBOM metadata, and build provenance.

The GitHub release receives ZIP archives for the Terraform provider on Linux, macOS, Windows, and FreeBSD, plus a manifest, SHA-256 checksums, and a detached GPG signature.

## Terraform Registry limitation

The public Terraform Registry only discovers repositories named `terraform-provider-{NAME}`. This monorepo builds correctly named artifacts, but `f0rkz/iptrack` cannot itself be registered as `f0rkz/iptrack`.

Before public Registry publication, move or mirror `terraform-provider-iptrack/` and the provider release configuration into a public `f0rkz/terraform-provider-iptrack` repository. The Terraform source address remains `registry.terraform.io/f0rkz/iptrack`. Keep provider versioning independent after the split so application-only changes do not publish provider releases.

## Recovering a partial release

If semantic-release creates a tag/draft but a later image or artifact step fails, rerun the Release workflow manually and provide the existing version without the `v` prefix. For example, enter `1.2.3` for tag `v1.2.3`. The workflow checks out that tag and resumes publishing without calculating a new version.
