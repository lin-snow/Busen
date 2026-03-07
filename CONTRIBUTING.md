# Contributing to Busen

Thanks for your interest in contributing to `Busen`.

This project aims to stay small, explicit, and easy to reason about. Contributions that improve correctness, API clarity, documentation, and tests are especially welcome.

## Getting Started

1. Fork the repository and create a topic branch from `main`.
2. Make your change in small, reviewable commits.
3. Run the local checks before opening a pull request.
4. Update docs and examples when user-facing behavior changes.

## Development Requirements

- Go `1.25.0` or newer
- A local environment that can run standard Go tooling

## Common Commands

The repository includes a `Makefile` for the most common tasks:

```bash
make help
make fmt
make lint
make vet
make test
make test-race
make cover
make check
```

If you prefer raw Go commands, these are the expected equivalents:

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go test -coverprofile=coverage.out ./...
```

`make lint` installs `golangci-lint` `v2.3.0` into `./.bin` using the active Go toolchain and then runs it. This is intentional because the project currently targets Go `1.25.0`, and building the linter with the local toolchain avoids version skew with prebuilt binaries.

## Contribution Guidelines

- Keep changes focused. Avoid bundling unrelated refactors into the same pull request.
- Add or update tests for behavior changes and bug fixes.
- Preserve the package's typed-first API design and explicit concurrency semantics.
- Prefer clear names and straightforward control flow over clever abstractions.
- Avoid introducing new dependencies unless they provide clear, lasting value.

## Pull Request Expectations

Before submitting a pull request, please make sure:

- Formatting, lint, vet, and tests all pass locally
- New behavior is covered by tests when practical
- `README.md` and other docs are updated if the public API or workflow changes
- The pull request description explains the motivation, not only the code diff

Related project docs:

- Support: `SUPPORT.md`
- Security reporting: `SECURITY.md`
- Release flow: `RELEASING.md`
- Governance and scope: `GOVERNANCE.md`

## Commit Messages

There is no strict commit format requirement, but concise, imperative messages are preferred. Examples:

- `add async subscriber overflow tests`
- `fix topic matcher edge case`
- `document release workflow`

## Reporting Bugs

Use the bug report issue template and include:

- Go version
- Operating system
- A minimal reproduction
- Expected behavior
- Actual behavior

## Security

Please do not report security issues in public issues. Follow the process in `SECURITY.md`.
