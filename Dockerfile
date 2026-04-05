# Stage 1: Build all Go binaries
FROM golang:1.26-alpine AS builder
ENV CGO_ENABLED=0
WORKDIR /app
COPY go.mod go.sum ./
COPY zoekt/ zoekt/
RUN go mod download
COPY internal/ internal/
COPY cmd/ cmd/
# Generate upstream wrapper packages and build overlay, then build with it.
RUN go run ./internal/upstream/generate.go && \
    go install -overlay=overlay.json ./cmd/...
WORKDIR /app/zoekt
RUN go install ./cmd/...

# Stage 2: Final image
FROM alpine:3.23

RUN apk add --no-cache \
    git ca-certificates jansson ctags tini \
 && ln -s /usr/bin/ctags /usr/bin/universal-ctags

COPY --from=builder /go/bin/ /usr/local/bin/

ENV ZOEKT_CONFIG=/config.yaml
ENV ZOEKT_LISTEN=:8000
EXPOSE 8000

ENTRYPOINT ["tini", "--"]
CMD ["zoekt-unified", "serve"]
