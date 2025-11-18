# Database Migrations

This directory contains SQL migration scripts for the PipeOps Load Tester database.

## Migration History

### 001_add_http_fields.sql
- **Date**: 2024
- **Description**: Adds support for custom HTTP methods, request bodies, and headers
- **Changes**:
  - Added `method` column (TEXT, default: 'GET')
  - Added `body` column (TEXT, optional)
  - Added `headers` column (TEXT, stores JSON string)

## Running Migrations

The migrations in this directory have already been applied to the database. They serve as documentation of schema changes.

For new deployments, the schema is automatically created by the `InitDB()` function in `database.go`.

For existing databases that need to be migrated, run the SQL commands manually:

```bash
sqlite3 loadtest.db < migrations/001_add_http_fields.sql
```

## Adding New Migrations

1. Create a new `.sql` file with naming convention: `XXX_description.sql`
2. Include migration metadata in comments (date, description, changes)
3. Test the migration on a copy of the database first
4. Update this README with the migration details
5. Apply the migration to production database

## Schema Verification

To verify the current schema:

```bash
sqlite3 loadtest.db "PRAGMA table_info(test_runs);"
```

To view the full table definition:

```bash
sqlite3 loadtest.db "SELECT sql FROM sqlite_master WHERE type='table' AND name='test_runs';"
```
