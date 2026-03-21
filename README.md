# zoekt-simple

An opinionated [Zoekt](https://github.com/sourcegraph/zoekt) deployment. One config file, one container.

- **YAML config** -- one file configures everything, with env var expansion and credential helpers
- **MCP server** -- built-in [Model Context Protocol](https://modelcontextprotocol.io/) endpoint for AI-powered code search
- **On-demand reindex** -- REST API to trigger reindexing without waiting for the next mirror cycle
- **CLI tools** -- `zoekt-search` (ripgrep-style output) and `zoekt-get-file`
- **Multi-host mirroring** -- GitHub, GitLab, Gitea, Gerrit, Bitbucket Server, Gitiles, and CGit

## Quick Start

```sh
docker run -d \
  -p 8000:8000 \
  -v zoekt_data:/data \
  -v ./config.yaml:/config.yaml:ro \
  ghcr.io/andrewrabert/zoekt-simple:latest
```

See `config.yaml.example` for all configuration options.

## Configuration

All configuration lives in a single YAML file. Set `ZOEKT_CONFIG` or pass `-config <path>`. See [`config.yaml.example`](config.yaml.example) for all options including server settings, indexer tuning, MCP instructions, and mirror entries for all supported code hosts.

## MCP Tools

**search** -- Search code across indexed repositories. Returns JSON with `total_matches`, `returned`, `truncated`, and `results`. Supports `output_mode` of `lines`, `files`, or `repos`.

**get_file** -- Retrieve file contents from the index by hostname, repo, and path.

## CLI Tools

```sh
export ZOEKT_URL=http://localhost:8000

# Search (ripgrep-style output)
zoekt-search 'lang:go func main'
zoekt-search -C 3 -l 'TODO'

# Get file
zoekt-get-file github.com/myorg/myrepo src/main.go
```

## REST API

```sh
# Trigger reindex
curl -X POST http://localhost:8000/api/reindex \
  -H 'Content-Type: application/json' \
  -d '{"repo": "github.com/myorg/myrepo"}'

# Check status
curl http://localhost:8000/api/reindex/{task-id}
```

## Building

Requires [just](https://github.com/casey/just).

```sh
just build            # build all binaries to build/
just test             # run tests
just docker-build     # build the Docker image
```
