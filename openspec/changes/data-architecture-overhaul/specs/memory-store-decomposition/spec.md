## ADDED Requirements

### Requirement: Store decomposition into independent Repos
The system SHALL decompose `sessionmemory.Store` (96 methods) into 6 independent Repo structs: `L0SnapshotRepo` (4 methods), `L1WorkingMemoryRepo` (8 methods), `L2EpisodeRepo` (12 methods), `L3FactRepo` (16 methods), `L4EntityRepo` (12 methods), `CascadeRepo` (14 methods). Each Repo SHALL hold `*Data` (not `*ent.Client`).

#### Scenario: Each Repo independently implements biz interfaces
- **WHEN** a biz layer usecase needs L3 fact operations
- **THEN** it SHALL depend on `biz.L3FactReader` / `biz.L3FactWriter` interfaces, implemented by `L3FactRepo`, without depending on other memory layer repos

#### Scenario: No Store.Client() backdoor
- **WHEN** any code needs to execute raw SQL against memory tables
- **THEN** it SHALL use `Data.ExecInTx` / `Data.ClientFromCtx` / `ReadWriteDB`, NOT `Store.Client()`

### Requirement: Wire adapter relocation
All data-layer adapters currently in `cmd/admin/wire_memory.go` SHALL be relocated to `internal/data/`. The `wireSessionAdminStoreAdapter` and `wireL3FactWriterAdapter` SHALL become `internal/data/memory_admin_adapter.go` and `internal/data/memory_l3_fact_writer_adapter.go`.

#### Scenario: Adapter in data layer
- **WHEN** Wire assembles the dependency graph
- **THEN** all data-layer adapters SHALL be in `internal/data/` package, not in `cmd/admin/`

### Requirement: Eliminate Store satisfying biz interfaces directly
`*sessionmemory.Store` SHALL NOT directly implement any biz interface. All biz interface satisfaction SHALL go through explicit adapter structs in `internal/data/`.

#### Scenario: SessionL2RecallStore via adapter
- **WHEN** `biz.MemoryL2RecallUsecase` needs `SessionL2RecallStore`
- **THEN** it SHALL receive an explicit `l2RecallAdapter` struct, NOT `*sessionmemory.Store`

### Requirement: Store method parameters use data-layer DTOs
Store method parameters that currently accept `biz.L0AssemblySnapshotInsert`, `biz.L1TaskInsert`, `biz.L1FieldInsert`, `biz.L1ArchiveEpisodeInsert`, `biz.ReinforcementSignal`, `biz.L4DecayConfig` SHALL be replaced with data-layer DTOs. Conversion SHALL happen in the adapter layer.

#### Scenario: L1 task insert with data DTO
- **WHEN** `L1WorkingMemoryRepo.StartL1Task` is called
- **THEN** it SHALL accept a `data.L1TaskInsert` DTO, and the adapter SHALL convert from `biz.L1TaskInsert` to `data.L1TaskInsert`

### Requirement: Shim migration phase
During migration, each new Repo SHALL delegate to the existing Store methods (shim pattern). This allows incremental migration without breaking existing functionality.

#### Scenario: L3FactRepo delegates to Store
- **WHEN** `L3FactRepo.UpsertFactRow` is called during shim phase
- **THEN** it SHALL delegate to `Store.UpsertFactRow` internally

#### Scenario: Store removal after full migration
- **WHEN** all Store methods have been migrated to independent Repos
- **THEN** the `sessionmemory.Store` struct SHALL be deleted
