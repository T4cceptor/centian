# Stage 1: Build the frontend
FROM node:22-alpine AS web-builder

WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build


# Stage 2: Build the binary
FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /src/web/dist/. /build/internal/ui/dist/

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o centian \
    ./cmd/main.go


# Stage 3: Alpine image — minimal, binary only
# Tagged as: t4cceptor/centian:<version>-alpine
FROM alpine:3.21 AS alpine

RUN apk add --no-cache ca-certificates

COPY --from=builder /build/centian /usr/local/bin/centian

ENTRYPOINT ["centian", "start"]


# Stage 4: Full image — Python + Node.js included for stdio MCP servers
# Tagged as: t4cceptor/centian:<version>  (the default)
FROM python:3.12-slim AS full

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        nodejs \
        npm \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/centian /usr/local/bin/centian

ENTRYPOINT ["centian", "start"]
