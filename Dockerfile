# Multi-stage Dockerfile for PipeOps Load Tester
# Uses latest Go version with CGO support for SQLite

# Stage 1: Build the Go application
FROM golang:1.23-alpine AS builder

# Install build dependencies for CGO and SQLite
RUN apk add --no-cache \
    gcc \
    musl-dev \
    sqlite-dev \
    git

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application with CGO enabled
# -ldflags "-s -w" removes debug info and reduces binary size
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -a -installsuffix cgo -o load-tester .

# Stage 2: Create minimal runtime image
FROM alpine:latest

# Install runtime dependencies and security updates
RUN apk --no-cache add \
    ca-certificates \
    sqlite-libs \
    tzdata \
    && apk upgrade --no-cache

# Create non-root user for security
RUN addgroup -g 1000 pipeops && \
    adduser -D -u 1000 -G pipeops pipeops

# Set working directory
WORKDIR /home/pipeops/app

# Copy binary from builder stage
COPY --from=builder --chown=pipeops:pipeops /app/load-tester .

# Copy static files
COPY --from=builder --chown=pipeops:pipeops /app/static ./static

# Create directory for database with proper permissions
RUN mkdir -p /home/pipeops/app/data && \
    chown -R pipeops:pipeops /home/pipeops

# Switch to non-root user
USER pipeops

# Expose port
EXPOSE 8080

# Add health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080 || exit 1

# Set environment variables
ENV PORT=8080

# Run the application
CMD ["./load-tester"]
