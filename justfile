[private]
default:
    @just -l

# Ensure git submodules are initialized
[private]
ensure-submodules:
    @if [ ! -f zoekt/go.mod ]; then git submodule update --init; fi

# Generate upstream wrapper packages and build overlay
generate: ensure-submodules
    go run ./internal/upstream/generate.go

# Build all binaries (generates upstream wrappers first)
build: generate
    CGO_ENABLED=0 go build -overlay=overlay.json -o build/ ./cmd/...
    cd zoekt && CGO_ENABLED=0 go build -o ../build/ ./cmd/...

# Build the Docker image
docker-build tag="zoekt-simple:latest":
    docker build -t {{tag}} .

# Build and run the Docker image
docker-run tag="zoekt-simple:latest" *args="":
    just docker-build {{tag}}
    docker run --rm {{args}} {{tag}}

# Run tests (requires overlay for upstream imports)
test *args="": generate
    go test -overlay=overlay.json -vet=off {{args}} ./internal/... ./cmd/...