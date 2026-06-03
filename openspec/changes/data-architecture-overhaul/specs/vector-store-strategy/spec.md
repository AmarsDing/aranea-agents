## ADDED Requirements

### Requirement: VectorStore interface
The system SHALL define a `VectorStore` interface in `internal/data/vector/` with three methods: `Upsert`, `Search`, `Delete`. The interface SHALL abstract away the underlying vector storage engine.

#### Scenario: Upsert embedding
- **WHEN** a fact or episode embedding is computed
- **THEN** the system SHALL call `VectorStore.Upsert(ctx, id, embedding, model, dim)` to store the vector

#### Scenario: Search similar vectors
- **WHEN** a recall query needs similar vectors
- **THEN** the system SHALL call `VectorStore.Search(ctx, embedding, limit, minScore)` and receive ranked results

#### Scenario: Delete embedding
- **WHEN** a fact is deleted
- **THEN** the system SHALL call `VectorStore.Delete(ctx, id)` to remove the vector

### Requirement: SQLite vector store implementation
The system SHALL provide a `SQLiteVectorStore` that stores embeddings as JSON in SQLite columns and computes cosine similarity in Go.

#### Scenario: SQLite vector search
- **WHEN** `SQLiteVectorStore.Search` is called
- **THEN** it SHALL load candidate vectors from SQLite and compute cosine similarity in Go, returning top-K results

### Requirement: Postgres pgvector store implementation
The system SHALL provide a `PgVectorStore` that uses pgvector extension for vector storage and search.

#### Scenario: pgvector search with IVFFlat
- **WHEN** `PgVectorStore.Search` is called
- **THEN** it SHALL execute a pgvector similarity search query using the configured index method

### Requirement: VectorStore selection by configuration
The system SHALL select the VectorStore implementation based on configuration. When Postgres is configured and available, `PgVectorStore` SHALL be used. Otherwise, `SQLiteVectorStore` SHALL be the fallback.

#### Scenario: Postgres available
- **WHEN** Postgres connection is configured and pgvector extension is available
- **THEN** the system SHALL use `PgVectorStore`

#### Scenario: Postgres unavailable
- **WHEN** Postgres is not configured or pgvector is not available
- **THEN** the system SHALL use `SQLiteVectorStore`

### Requirement: Eliminate dual-write of embeddings
The `memory_facts` table SHALL NOT store `embedding_blob` / `embedding_norm` columns. Instead, it SHALL store `embedding_ref TEXT` referencing the vector ID in VectorStore.

#### Scenario: Fact creation stores vector reference
- **WHEN** a fact with embedding is upserted
- **THEN** the system SHALL store the embedding in VectorStore and save only the `embedding_ref` in `memory_facts`
