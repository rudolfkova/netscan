FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/netscan ./cmd

FROM debian:bookworm-slim

# ping and mtr need raw ICMP. CAP_NET_RAW on a non-root user is unreliable in
# Docker, so the process stays root. Compose adds only NET_RAW, not privileged.
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates iputils-ping mtr-tiny && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/netscan /usr/local/bin/netscan

ENTRYPOINT ["/usr/local/bin/netscan"]
