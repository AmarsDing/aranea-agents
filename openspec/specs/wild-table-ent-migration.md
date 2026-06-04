# Wild Table Ent Migration

## Wild Table Ent Migration

### Requirement: Batch 1 wild tables into Ent Schema
The system SHALL create Ent Schema definitions for the following 6 high-frequency tables: `session_runs`, `session_participants`, `session_run_checkpoints`, `channel_inbound_receipts`, `channel_turn_jobs`, `channel_runtime_lease`. These tables SHALL be managed by Ent's `Schema.Create` for new installations and DDL migration for existing installations.

**Status**: ✅ Implemented. All 6 Ent schemas created. Repos partially migrated (Ent API where possible, Raw SQL for ON CONFLICT/UPSERT).

#### Scenario: New installation creates tables via Ent
- **WHEN** a fresh database is initialized
- **THEN** Ent `Schema.Create` SHALL create these 6 tables with correct columns and indexes

#### Scenario: Existing installation migrates via DDL registry
- **WHEN** an existing database is upgraded
- **THEN** the DDL migration registry SHALL detect missing columns and add them via ALTER TABLE

### Requirement: Batch 2 memory tables into Ent Schema
The system SHALL create Ent Schema definitions for the following 6 memory tables: `memory_facts`, `memory_entities`, `memory_relations`, `memory_episodes`, `memory_l1_tasks`, `memory_l1_fields`. Complex queries (vector search, cascade, JSON aggregation) MAY remain as Raw SQL.

#### Scenario: Memory table schema defined in Ent
- **WHEN** a new column is added to `memory_facts`
- **THEN** the Ent Schema SHALL be the single source of truth for the column definition

#### Scenario: Complex queries remain Raw SQL
- **WHEN** a vector similarity search is needed
- **THEN** the Repo MAY use Raw SQL via `ReadWriteDB`, but the table structure SHALL be defined in Ent Schema

### Requirement: memory_chain.sql deduplication
The system SHALL remove table definitions from `memory_chain.sql` that overlap with Ent Schema definitions (23 tables). `memory_chain.sql` SHALL only contain the 34 Memory-specific tables not managed by Ent.

**Status**: ✅ Implemented. 24 overlapping table definitions removed. `memory_chain.sql` now contains only 34 Memory-specific tables.

#### Scenario: Overlapping table removed from SQL file
- **WHEN** a table is defined in both Ent Schema and `memory_chain.sql`
- **THEN** the `memory_chain.sql` definition SHALL be removed, and Ent Schema SHALL be the single source of truth

### Requirement: DDL migration system SQL file support
The `ddl_migration_registry` SHALL support registering SQL file paths (embedded via `go:embed`) in addition to Go functions. This reduces inline SQL strings in Go code.

**Status**: ✅ Implemented. `embed.FS` with `//go:embed sql/migrations/*.sql`. SQL files in `internal/data/sql/migrations/`.

#### Scenario: Migration from SQL file
- **WHEN** a DDL migration is registered with a `SQL` field pointing to an embedded SQL file
- **THEN** the migration system SHALL read and execute the SQL file contents

### Requirement: Zero wild tables target
The long-term target SHALL be 0 wild tables (all 34 pure-wild tables managed by Ent Schema). Batch 3 (remaining ~28 tables after Batch 1 and 2) SHALL be migrated incrementally after Batch 1 and 2 are stable.

#### Scenario: Wild table count tracking
- **WHEN** a new table is added to the system
- **THEN** it MUST be defined in Ent Schema first, with no raw SQL CREATE TABLE allowed
