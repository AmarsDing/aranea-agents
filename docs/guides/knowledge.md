# Knowledge Base Guide

> Sprint 6 — T38 | M-7

The Knowledge module adds a pgvector-backed RAG (retrieval-augmented generation) pipeline. Agents can upload documents, automatically index them into vector embeddings, and then perform semantic searches at query time via the `knowledge_search` tool.

---

## Architecture

```
                  ┌─────────────────────┐
                  │  KnowledgeService   │  ← Kratos HTTP/gRPC
                  └────────┬────────────┘
                           │ biz.KnowledgeUsecase
              ┌────────────┴────────────┐
              │                         │
     chunker.go                   knowledge.go (pgvector)
     embedder.go                  ├── knowledge_collections
     retriever.go                 ├── knowledge_documents
              │                   └── knowledge_chunks (vector)
              └────────────────────────►
```

### Components

| Component | Path | Purpose |
|-----------|------|---------|
| Proto | `api/kratos/knowledge/v1/knowledge.proto` | HTTP + gRPC API |
| Biz | `internal/biz/knowledge.go` | Domain logic + `KnowledgeRepo` interface |
| Data | `internal/data/knowledge.go` | PostgreSQL + pgvector raw SQL |
| Chunker | `internal/knowledge/chunker.go` | Text splitting (char/token strategies) |
| Embedder | `internal/knowledge/embedder.go` | OpenAI-compatible + Ollama embedding APIs |
| Retriever | `internal/knowledge/retriever.go` | Embeds query → calls `SearchChunks` |
| Tool | `internal/tools/knowledge/tool.go` | `knowledge_search` trpc tool for agents |
| Service | `internal/service/knowledge.go` | Kratos service adapter |

---

## Database Schema

Tables are created by `data.EnsureKnowledgeSchema(ctx, db, dim)`:

```sql
knowledge_collections   (id, name, embedding_model, dim, status, ...)
knowledge_documents     (id, collection_id, source, status, chunk_count, ...)
knowledge_chunks        (id, doc_id, collection_id, content, embedding vector(N), metadata jsonb, ...)
```

Index: `ivfflat` on `knowledge_chunks.embedding` for cosine similarity.

**Requirements**: PostgreSQL with the `pgvector` extension.

---

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/knowledge/collections` | Create a collection |
| GET | `/v1/knowledge/collections` | List all collections |
| GET | `/v1/knowledge/collections/{id}` | Get one collection |
| DELETE | `/v1/knowledge/collections/{id}` | Delete collection + all data |
| POST | `/v1/knowledge/documents` | Ingest a document (async indexing) |
| GET | `/v1/knowledge/documents` | List documents |
| DELETE | `/v1/knowledge/documents/{id}` | Delete document + chunks |
| POST | `/v1/knowledge/search` | Semantic search |

### Ingestion Flow

1. Client calls `POST /v1/knowledge/documents` with `content_base64` (document payload).
2. Service creates a document record with `status=pending` and returns immediately.
3. A background goroutine (`safego.Go`) chunks the text, embeds each chunk, and inserts them into `knowledge_chunks`.
4. On completion, the document status is updated to `indexed`; on failure it becomes `error`.

---

## Chunking Strategies

| Strategy | Key | Description |
|----------|-----|-------------|
| By character | `char` | Default. Splits into windows of N runes with overlap. |
| By token | `token` | Whitespace-tokenized words; proxy for true token count. |

Parameters:
- `chunk_size` — window size (default 512)
- `chunk_overlap` — overlap between consecutive chunks (default 64)

---

## Embedding Providers

The `Embedder` supports two backends selected via `provider`:

| Provider | Endpoint | Notes |
|----------|----------|-------|
| `openai` (default) | `POST /v1/embeddings` | Compatible with any OpenAI-API server |
| `ollama` | `POST /api/embeddings` | Local Ollama instance |

---

## Agent Tool: `knowledge_search`

Attach a `Retriever` to the agent context:

```go
ctx = knowledgetool.WithRetriever(ctx, retriever)
```

Then register `knowledgetool.NewSearchTool()` with the trpc runner.

The model can then call:

```json
{ "collection_id": "abc123", "query": "What is the refund policy?", "top_k": 5 }
```

---

## Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `aranea_knowledge_ingest_documents_total` | Counter | Documents successfully indexed |
| `aranea_knowledge_search_duration_seconds` | Histogram | Search latency |

---

## Limitations

- pgvector is required; the repo falls back gracefully when `db == nil` (schema-only call).
- Embedding dimension is fixed per collection; changing it requires recreating the collection.
- Document content must be text-decodable (PDF/image extraction is out-of-scope for S6).
