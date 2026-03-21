# Ensure git submodules are initialized
[private]
ensure-submodules:
    @if [ ! -f zoekt/go.mod ]; then git submodule update --init; fi

# Build all binaries
build: ensure-submodules
    CGO_ENABLED=0 go build -o build/ ./cmd/...
    cd zoekt && CGO_ENABLED=0 go build -o ../build/ ./cmd/...

# Build the Docker image
docker-build tag="zoekt-simple:latest":
    docker build -t {{tag}} .

# Build and run the Docker image
docker-run tag="zoekt-simple:latest" *args="":
    just docker-build {{tag}}
    docker run --rm {{args}} {{tag}}

# Run tests
test *args="":
    go test {{args}} ./...