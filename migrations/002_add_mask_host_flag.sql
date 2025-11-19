-- Migration: Add mask_host column to test_runs
-- Date: 2024-10
-- Description: Track whether the UI should mask the target host for each test.

ALTER TABLE test_runs ADD COLUMN mask_host INTEGER NOT NULL DEFAULT 1;
