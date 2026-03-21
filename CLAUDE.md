# zoekt-simple

## Skills

This project has custom skills in `.claude/skills/`. You MUST use them:

- **release** — Use for ANY version bump, tagging, or release task. This includes requests like "tag a release", "cut a release", "bump the version", or "release v0.x.x". Always invoke this skill before taking any action.

## Release Process

NEVER manually tag or create releases without invoking the `release` skill first. The skill defines the exact steps and tag format. Skipping it will produce incorrect releases.

Release notes go in annotated tag messages, not in a changelog file. Use `--cleanup=verbatim` when creating tags to preserve `#` markdown headers. Always verify annotations with `git tag -n999` after creating.
