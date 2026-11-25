# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Lanka Software Foundation

# Build stage
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Build arguments
ARG BUILD_VERSION=latest
ARG BUILD_TIME
ARG GIT_COMMIT

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code (this layer is invalidated only on code changes)
COPY . .

# Build the application with build info
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-X main.Version=${BUILD_VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}" \
    -o /app/orchestration-engine ./cmd/oe

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/orchestration-engine .

# Create config directory
RUN mkdir -p /app/config && \
    chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 4000

# The image has no config file or schema: mount them here, or point CONFIG_PATH
# elsewhere. schemaPath is resolved against the config file's directory.
ENV CONFIG_PATH=/app/config/config.yaml

# Run the server
CMD ["./orchestration-engine"]
