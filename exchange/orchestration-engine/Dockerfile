# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
# Shared modules (github.com/OpenDIF/opendif-core/...) are now published and
# fetched from the module proxy, so no local shared source needs to be copied.
RUN go mod download

# Copy source code (this layer is invalidated only on code changes)
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o orchestration-engine .

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


# Install wget for health checks
RUN apk --no-cache add wget

# Create config directory
RUN mkdir -p /app/config && \
    chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Environment variables with defaults
ENV SERVER_PORT=4000
ENV SERVER_HOST=0.0.0.0
ENV LOG_LEVEL=info
ENV CONFIG_FILE=/app/config.json

# Database Configuration (PostgreSQL example)
ENV DB_HOST=localhost
ENV DB_PORT=5432
ENV DB_USER=user
ENV DB_PASSWORD=password
ENV DB_NAME=orchestration_db
ENV DB_SSLMODE=disable

# Expose port
EXPOSE 4000

# Health check (adjust endpoint as needed)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${SERVER_PORT}/health || exit 1

# Run the server
CMD ["./orchestration-engine"]