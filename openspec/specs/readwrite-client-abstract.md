# ReadWrite Client Abstract

## ReadWrite Client Abstract

### Requirement: ReadWriteClient abstraction for Ent API
The system SHALL provide a `ReadWriteClient` struct that encapsulates read/write Ent client selection with automatic transaction awareness.

#### Scenario: Read operation outside transaction
- **WHEN** `ReadWriteClient.Read(ctx)` is called and no transaction is active in context
- **THEN** it SHALL return the read-only Ent client (`readClient`)

#### Scenario: Read operation inside transaction
- **WHEN** `ReadWriteClient.Read(ctx)` is called and a transaction is active in context
- **THEN** it SHALL return the transaction's Ent client

#### Scenario: Write operation outside transaction
- **WHEN** `ReadWriteClient.Write(ctx)` is called and no transaction is active in context
- **THEN** it SHALL return the write Ent client (`entClient`)

#### Scenario: Write operation inside transaction
- **WHEN** `ReadWriteClient.Write(ctx)` is called and a transaction is active in context
- **THEN** it SHALL return the transaction's Ent client

### Requirement: ReadWriteDB abstraction for Raw SQL
The system SHALL provide a `ReadWriteDB` struct that encapsulates read/write `*sql.DB` selection with automatic transaction awareness.

#### Scenario: Raw SQL read outside transaction
- **WHEN** `ReadWriteDB.ReadDB(ctx)` is called and no transaction is active
- **THEN** it SHALL return `readDB`

#### Scenario: Raw SQL write outside transaction
- **WHEN** `ReadWriteDB.WriteDB(ctx)` is called and no transaction is active
- **THEN** it SHALL return `rawDB`

#### Scenario: Raw SQL inside transaction
- **WHEN** either method is called and a `*sql.Tx` is active in context
- **THEN** it SHALL return the transaction's `*sql.Tx` (which satisfies `execer` interface)

### Requirement: All Repos use ReadWriteClient or ReadWriteDB
Every Repo in `internal/data/` SHALL use `ReadWriteClient` or `ReadWriteDB` for database access, eliminating manual `readClient`/`txClient`/`db()` implementations.

#### Scenario: Ent Repo uses ReadWriteClient
- **WHEN** an Ent-based Repo performs a read query
- **THEN** it SHALL use `r.rw.Read(ctx)` instead of `r.readClient(ctx)` or `r.data.ReadEnt()`

#### Scenario: Raw SQL Repo uses ReadWriteDB
- **WHEN** a Raw SQL Repo performs a read query
- **THEN** it SHALL use `r.rw.ReadDB(ctx)` instead of `r.data.RawDB()`

#### Scenario: No direct Data.Ent() or Data.RawDB() in Repo methods
- **WHEN** a Repo method needs database access
- **THEN** it SHALL NOT call `r.data.Ent()`, `r.data.RawDB()`, or `r.data.ReadEnt()` directly
