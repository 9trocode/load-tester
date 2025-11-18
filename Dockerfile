# Multi-stage Dockerfile for PipeOps Load Tester
# Uses Debian-based images to avoid musl libc SQLite compatibility issues

# Stage 1: Build the Go application
FROM golang:1.23-bookworm AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    libc6-dev \
    libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o load-tester .

# Stage 2: Create minimal runtime image
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    libsqlite3-0 \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN groupadd -g 1000 pipeops && \
    useradd -u 1000 -g pipeops -m -s /bin/bash pipeops

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
ENV DB_PATH=/home/pipeops/app/data/loadtest.db

# Run the application
CMD ["./load-tester"]
