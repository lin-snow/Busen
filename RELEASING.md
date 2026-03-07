# Releasing Busen

## Release Principles

- Releases are tag-driven
- GitHub Releases are generated from version tags
- Release notes should explain user-visible changes and migration concerns

## Before Releasing

Run the local checks:

```bash
make check
make release-check
```

Verify that:

- Tests pass
- Public API changes are documented
- `README.md` and governance docs are up to date when needed
- The version tag follows the `vX.Y.Z` format
- The release workflow assumptions in `.github/workflows/release.yml` still match the repository setup

## Create a Release

1. Make sure the target commit is already on the default branch.
2. Create an annotated tag:

```bash
git tag -a v0.1.0 -m "release v0.1.0"
```

3. Push the tag:

```bash
git push origin v0.1.0
```

4. Wait for the GitHub release workflow to run.
5. Review the generated GitHub Release notes and edit them if needed.

The GitHub release workflow will re-run lint, formatting, vet, unit tests, and race tests before creating the release.

## Pre-release Tags

Pre-release tags such as `v0.2.0-rc.1` may be used when appropriate. The release workflow should still be validated against the same checks before tagging.

## If the Workflow Fails

- Inspect the failed workflow logs
- Fix the issue on the default branch
- Create a new tag if the existing release tag should not be reused

Avoid editing a published release tag unless you have a clear reason and understand the downstream impact for users and automation.
