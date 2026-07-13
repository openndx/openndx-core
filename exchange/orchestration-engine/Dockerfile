# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files and shared dependencies
COPY exchange/orchestration-engine/go.mod exchange/orchestration-engine/go.sum ./exchange/orchestration-engine/
COPY exchange/shared/ /app/exchange/shared/

WORKDIR /app/exchange/orchestration-engine
RUN go mod download

# Copy source code (this layer is invalidated only on code changes)
COPY exchange/orchestration-engine/ ./

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
COPY --from=builder /app/exchange/orchestration-engine/orchestration-engine .

# Create config directory
RUN mkdir -p /app/config && \
    chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 4000

# Run the server
CMD ["./orchestration-engine"]
