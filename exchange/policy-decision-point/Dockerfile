# Multi-stage Dockerfile for Policy Decision Point
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Build arguments
ARG SERVICE_PATH=policy-decision-point
ARG BUILD_VERSION=latest
ARG BUILD_TIME
ARG GIT_COMMIT

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
# Shared modules (github.com/OpenDIF/opendif-core/...) are now published and
# fetched from the module proxy, so no local shared source needs to be copied.
RUN go mod download

# Copy source code (this layer is invalidated only on code changes)
COPY . .

# Build the application with build info
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.Version=${BUILD_VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}" \
    -o /app/service_binary .

# Final stage
FROM alpine:3.17

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user with specific UID for WSO2 Choreo compliance
RUN adduser -D -s /bin/sh -u 10001 appuser

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/service_binary /app/

# Change ownership to appuser
RUN chown -R appuser:appuser /app

# Switch to non-root user with specific UID
USER 10001

# Expose port
EXPOSE 8082

# Set environment variables
ENV CONFIG_DIR=/app/config
ENV ENVIRONMENT=production

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8082/health || exit 1

# Run the application with environment-based flags
CMD ["./service_binary", "-env=${ENVIRONMENT:-production}", "-port=8082"]