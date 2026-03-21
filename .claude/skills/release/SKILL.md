---
name: release
description: Use for ANY release, version, or tagging task. Triggers include "cut a release", "tag a release", "tag a new release", "tag v0.x.x", "prepare a release", "bump the version", "release v0.x.x", "new release", "create a release", or any mention of version bumps, git tags for versions, or releasing. This skill MUST be invoked before taking any action.
---

# Release Process

Follow these steps in order when preparing a new release.

## 1. Determine the Version

Check the git tags for the current version. The new version must follow [Semantic Versioning](https://semver.org/):

- **patch** (0.1.0 -> 0.1.1): bug fixes, dependency updates, no API changes
- **minor** (0.1.0 -> 0.2.0): new features, backwards-compatible API additions
- **major** (0.1.0 -> 1.0.0): breaking changes

If the user hasn't specified which bump, recommend one based on the changes since the last tag and confirm before proceeding.

## 2. Tag

Create an annotated tag with release notes as the message. Inspect `git log <last-tag>..HEAD` and write user-facing release notes in the tag message.

### Tag Message Format

```
v<version>

## Added
- Description of new feature

## Changed
- Description of change

## Fixed
- Description of fix
```

Use only the categories that have entries. Order: Added, Changed, Deprecated, Removed, Fixed, Security. Write in imperative mood ("Add feature" not "Added feature"). Focus on user-facing changes, skip internal-only changes. Prefix breaking changes with **BREAKING:**.

### Creating the Tag

Pipe the message via `printf` to avoid a trailing newline (heredocs add one, and `--cleanup=verbatim` preserves it):

```sh
printf '...' | git tag -a v<version> --cleanup=verbatim -F -
```

The `--cleanup=verbatim` flag prevents git from stripping lines starting with `#` (which it otherwise treats as comments).

After creating the tag, always verify the annotation with `git tag -n999 v<version>` to confirm nothing was stripped.

## 3. Push

Push the tag. The build workflow (`.github/workflows/build.yml`) triggers on `v*` tags and creates a GitHub Release with binary archives. The docker workflow (`.github/workflows/docker.yml`) builds and pushes multi-arch Docker images tagged with the version.

```sh
git push --tags
```

Always confirm with the user before pushing.
