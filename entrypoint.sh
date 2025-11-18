#!/bin/bash
set -e

# Database path from environment variable or default
DB_PATH="${DB_PATH:-/home/pipeops/app/data/loadtest.db}"

# Extract directory from database path
DB_DIR=$(dirname "$DB_PATH")

echo "==> PipeOps Load Tester Startup"
echo "==> Database path: $DB_PATH"
echo "==> Database directory: $DB_DIR"
echo "==> Current user: $(whoami) (UID: $(id -u), GID: $(id -g))"

# Create database directory if it doesn't exist
if [ ! -d "$DB_DIR" ]; then
    echo "==> Creating database directory: $DB_DIR"
    mkdir -p "$DB_DIR"
fi

# Check current ownership
echo "==> Checking directory permissions..."
ls -la "$DB_DIR" 2>/dev/null || ls -la "$(dirname "$DB_DIR")"

# Fix ownership to pipeops:pipeops (UID:GID 1000:1000)
echo "==> Ensuring correct ownership (pipeops:pipeops)..."
chown -R pipeops:pipeops "$DB_DIR"

# Fix permissions
echo "==> Setting permissions (755)..."
chmod -R 755 "$DB_DIR"

# Verify directory is now accessible by pipeops user
if [ -w "$DB_DIR" ]; then
    echo "==> Database directory is writable ✓"
else
    echo "==> ERROR: Database directory is still not writable!"
    echo "==> This should not happen - permissions were just set"
    exit 1
fi

# Start the application as pipeops user
echo "==> Starting PipeOps Load Tester as user pipeops..."
exec gosu pipeops "$@"
