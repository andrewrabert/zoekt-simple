# zoekt-simple

## Build

This project uses [just](https://github.com/casey/just) as a command runner. Run `just -l` to see available recipes. Key recipes: `build`, `test`, `docker-build`, `docker-run`.

**NEVER run `go build` or `go test` directly.** Always use `just build` and `just test`. Running `go build` without `-o` dumps binaries in the repo root.

## Skills

This project has custom skills in `.claude/skills/`. You MUST use them:

- **release** — Use for ANY version bump, tagging, or release task. This includes requests like "tag a release", "cut a release", "bump the version", or "release v0.x.x". Always invoke this skill before taking any action.

## Release Process

NEVER manually tag or create releases without invoking the `release` skill first. The skill defines the exact steps and tag format. Skipping it will produce incorrect releases.

Release notes go in annotated tag messages, not in a changelog file. Use `--cleanup=verbatim` when creating tags to preserve `#` markdown headers. Always verify annotations with `git tag -n999` after creating.
