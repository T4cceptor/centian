# Stage 1: Build the binary
FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o centian \
    ./cmd/main.go


# Stage 2: Alpine image — minimal, binary only
# Tagged as: t4cceptor/centian:<version>-alpine
FROM alpine:3.21 AS alpine

RUN apk add --no-cache ca-certificates

COPY --from=builder /build/centian /usr/local/bin/centian

ENTRYPOINT ["centian"]


# Stage 3: Full image — Python + Node.js included for stdio MCP servers
# Tagged as: t4cceptor/centian:<version>  (the default)
FROM python:3.12-slim AS full

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        nodejs \
        npm \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/centian /usr/local/bin/centian

# Bundle demo processors so users only need a config file to run the demo
COPY demo/requirements.txt /tmp/demo-requirements.txt
RUN pip install --no-cache-dir -r /tmp/demo-requirements.txt \
    && rm -f /tmp/demo-requirements.txt

COPY demo/src/ /opt/centian/processors/

ENTRYPOINT ["centian"]
