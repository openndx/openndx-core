# Multi-stage Dockerfile for Portal Backend
# Build from this directory: docker build -t portal-backend .
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Build arguments
ARG BUILD_VERSION=latest
ARG BUILD_TIME
ARG GIT_COMMIT

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files and source code
COPY go.mod go.sum ./
COPY . ./

# Download dependencies
RUN go mod download

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
EXPOSE 3000

# Run the application
CMD ["./service_binary"]
