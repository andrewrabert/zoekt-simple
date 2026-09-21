set default-list := true
set dotenv-load := true
set dotenv-override := true

default-env := '''
    ZOEKT_CONFIG=config.yaml
    ZOEKT_LISTEN=:8000
    ZOEKT_URL=http://localhost:8000
'''

default-config := '''
    data_dir: ./data
    indexes:
      default: {}
    mirrors:
      - git:
          urls:
            - https://github.com/andrewrabert/zoekt-simple
            - https://github.com/sourcegraph/zoekt
'''

# Ensure git submodules are initialized
[private]
ensure-submodules:
    @if [ ! -f zoekt/go.mod ]; then git submodule update --init; fi

# Write .env with defaults if missing
[private]
ensure-env:
    @[ -f .env ] || { cat <<< '{{default-env}}' > .env; echo "wrote .env"; }

# Write config.yaml with defaults if missing
[private]
ensure-config:
    #!/usr/bin/env bash
    set -euo pipefail
    config="${ZOEKT_CONFIG:-config.yaml}"
    [ -f "$config" ] || { cat <<< '{{default-config}}' > "$config"; echo "wrote $config"; }

# Build all binaries
build: ensure-submodules
    CGO_ENABLED=0 go build -o build/ ./cmd/...
    cd zoekt && CGO_ENABLED=0 go build -o ../build/ ./cmd/...

# Build the Docker image
[group: "docker"]
docker-build tag="zoekt-simple:latest":
    docker build -t {{tag}} .

# Build and run the Docker image
[group: "docker"]
docker-run tag="zoekt-simple:latest" *args="":
    just docker-build {{tag}}
    docker run --rm {{args}} {{tag}}

# Run the server
[group: "run"]
[positional-arguments]
server *args="": build ensure-env ensure-config
    #!/usr/bin/env bash
    set -euo pipefail
    export PATH="{{justfile_directory()}}/build:$PATH"
    exec zoekt-server "$@"

# Search the running server
[group: "run"]
[positional-arguments]
search *args="": build ensure-env
    ./build/zoekt-search "$@"

# Get a file from the running server
[group: "run"]
[positional-arguments]
get-file *args="": build ensure-env
    ./build/zoekt-get-file "$@"

# Run tests
[positional-arguments]
test *args="":
    go test "$@" ./...
