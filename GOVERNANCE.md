# Governance

## Project Goals

`Busen` is maintained as a small, typed-first Go library with explicit behavior and a conservative surface area.

Project decisions should favor:

- API clarity over feature breadth
- Correctness over convenience
- Explicit concurrency semantics over hidden behavior
- Small, maintainable changes over framework-style expansion

## Maintainers

Maintainers are responsible for:

- Reviewing and merging pull requests
- Triaging issues
- Setting release timing
- Protecting project quality and scope
- Enforcing the Code of Conduct

The repository owner is the initial maintainer. Additional maintainers may be added over time by consensus of the existing maintainer group.

## Decision Making

The default decision model is maintainer consensus. In practice:

- Straightforward fixes and documentation changes may be merged after normal review
- API changes, concurrency behavior changes, and backward-incompatible changes require explicit maintainer agreement
- If consensus cannot be reached in a reasonable time, the repository owner makes the final decision

## Contributions

External contributions are welcome, but acceptance is based on project fit rather than effort alone. A contribution may be declined when it:

- Expands the scope beyond the core library goals
- Adds maintenance burden disproportionate to its value
- Introduces implicit behavior or unclear guarantees

## Releases

Releases are cut by maintainers. The release process is documented in `RELEASING.md`.
