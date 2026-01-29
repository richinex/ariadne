# Multi-stage Dockerfile for Ariadne GitHub Action
# Stage 1: Build the Go binary with CGO enabled for SQLite

FROM golang:1.24-alpine AS builder

# Install build dependencies for CGO and SQLite
RUN apk add --no-cache \
    gcc \
    musl-dev \
    sqlite-dev

# Set working directory
WORKDIR /build

# Copy go module files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build optimized binary with CGO enabled
# -ldflags="-s -w" strips debug info and symbol tables
# -trimpath removes file system paths from binary
# -buildvcs=false prevents embedding VCS info for deterministic builds
ENV CGO_ENABLED=1
RUN go build \
    -ldflags="-s -w" \
    -trimpath \
    -buildvcs=false \
    -o ariadne \
    ./cmd/ariadne

# Stage 2: Runtime image with minimal dependencies
FROM alpine:3.21

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    sqlite-libs \
    bash

# Copy binary from builder
COPY --from=builder /build/ariadne /usr/local/bin/ariadne

# Copy entrypoint script
COPY --chmod=755 entrypoint.sh /entrypoint.sh

# Set entrypoint
ENTRYPOINT ["/entrypoint.sh"]
