#!/bin/bash
set -e

# Database path from environment variable or default
DB_PATH="${DB_PATH:-/home/pipeops/app/data/loadtest.db}"

# Extract directory from database path
DB_DIR=$(dirname "$DB_PATH")

echo "==> PipeOps Load Tester Startup"
echo "==> Database path: $DB_PATH"
echo "==> Database directory: $DB_DIR"

# Create database directory if it doesn't exist
if [ ! -d "$DB_DIR" ]; then
    echo "==> Creating database directory: $DB_DIR"
    mkdir -p "$DB_DIR"
fi

# Check if directory is writable
if [ ! -w "$DB_DIR" ]; then
    echo "==> WARNING: Database directory is not writable: $DB_DIR"
    echo "==> Current user: $(whoami) (UID: $(id -u), GID: $(id -g))"
    echo "==> Directory permissions:"
    ls -la "$DB_DIR" 2>/dev/null || ls -la "$(dirname "$DB_DIR")"

    # Try to fix permissions if we have permission
    if [ -w "$(dirname "$DB_DIR")" ]; then
        echo "==> Attempting to fix permissions..."
        chmod 755 "$DB_DIR" 2>/dev/null || true
    fi
fi

# Verify directory is now accessible
if [ -w "$DB_DIR" ]; then
    echo "==> Database directory is writable ✓"
else
    echo "==> ERROR: Database directory is still not writable!"
    echo "==> Please ensure the volume mount has correct permissions"
    echo "==> Run: docker exec <container> chown -R pipeops:pipeops $DB_DIR"
    exit 1
fi

# Start the application
echo "==> Starting PipeOps Load Tester..."
exec "$@"
