-- Migration: Add HTTP method, body, and headers columns to test_runs table
-- Date: 2024
-- Description: Adds support for custom HTTP methods, request bodies, and headers in load tests

-- Add method column (defaults to GET for backward compatibility)
ALTER TABLE test_runs ADD COLUMN method TEXT DEFAULT 'GET';

-- Add body column for request payloads (optional)
ALTER TABLE test_runs ADD COLUMN body TEXT;

-- Add headers column for custom HTTP headers (stored as JSON string)
ALTER TABLE test_runs ADD COLUMN headers TEXT;

-- Note: These columns have already been applied to the database
-- This file serves as documentation of the schema change
