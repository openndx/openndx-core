# Multi-stage Dockerfile for Consent Engine
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Build arguments
ARG BUILD_VERSION=latest
ARG BUILD_TIME
ARG GIT_COMMIT

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files and shared dependencies
COPY exchange/consent-engine/go.mod exchange/consent-engine/go.sum ./exchange/consent-engine/
COPY exchange/shared/ /app/exchange/shared/

WORKDIR /app/exchange/consent-engine
RUN go mod download

# Copy source code (this layer is invalidated only on code changes)
COPY exchange/consent-engine/ ./

# Build the application with build info from consent-engine directory
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.Version=${BUILD_VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}" \
    -o /app/ce .

# Final stage
FROM alpine:3.17

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user with specific UID for WSO2 Choreo compliance
RUN adduser -D -s /bin/sh -u 10001 appuser

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/ce /app/

# Change ownership to appuser
RUN chown -R appuser:appuser /app

# Switch to non-root user with specific UID
USER 10001

# Expose port
EXPOSE 8081

# Set environment variables
ENV ENVIRONMENT=production

# Run the application (env and port are read from the ENVIRONMENT and PORT variables)
CMD ["./ce"]
