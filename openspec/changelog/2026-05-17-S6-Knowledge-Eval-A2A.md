# Changelog — S6: Knowledge / Evaluation / A2A / CodeExecutor

**Date**: 2026-05-17  
**Sprint**: S6 (T38–T41)

---

## Summary

Sprint 6 implements the four P3 long-horizon capability modules:

- **T38** — Knowledge Base (pgvector RAG pipeline)
- **T39** — Evaluation Framework (async runner, 4 metrics)
- **T40** — Agent-to-Agent (A2A) Protocol
- **T41** — CodeExecutor Docker Sandbox

All four modules are independent and can be deployed in any order.

---

## T38 — Knowledge (M-7)

### New Files

| File | Description |
|------|-------------|
| `api/kratos/knowledge/v1/knowledge.proto` | gRPC/HTTP API (Collection, Document, Chunk, Search) |
| `internal/biz/knowledge.go` | `KnowledgeUsecase` + `KnowledgeRepo` interface |
| `internal/data/knowledge.go` | PostgreSQL + pgvector raw-SQL implementation |
| `internal/knowledge/chunker.go` | Char/token chunking strategies |
| `internal/knowledge/embedder.go` | OpenAI-compatible + Ollama embedding API client |
| `internal/knowledge/retriever.go` | Query embedding + `SearchChunks` dispatch |
| `internal/tools/knowledge/tool.go` | `knowledge_search` trpc CallableTool |
| `internal/service/knowledge.go` | Kratos service adapter + async ingest |
| `docs/guides/knowledge.md` | Feature documentation |

### Generated Files

| File | Generator |
|------|-----------|
| `api/kratos/knowledge/v1/knowledge_grpc.pb.go` | `protoc` |
| `api/kratos/knowledge/v1/knowledge_http.pb.go` | `protoc-gen-go-http` |
| `api/kratos/knowledge/v1/knowledge.pb.go` | `protoc` |
| `web/src/services/kratos/knowledge/v1/index.ts` | `protoc-gen-typescript-http` |

### Metrics

- `aranea_knowledge_ingest_documents_total`
- `aranea_knowledge_search_duration_seconds`

---

## T39 — Evaluation (M-11)

### New Files

| File | Description |
|------|-------------|
| `api/kratos/evaluation/v1/evaluation.proto` | API (Dataset, EvalCase, EvalRun, CaseResult) |
| `internal/biz/evaluation.go` | `EvalUsecase` + `EvalRepo` interface |
| `internal/data/evaluation.go` | SQLite/Postgres raw-SQL implementation |
| `internal/evaluation/runner.go` | Async evaluation runner with 4 metrics |
| `internal/service/evaluation.go` | Kratos service adapter |
| `docs/guides/evaluation.md` | Feature documentation |

### Metrics

- `aranea_eval_runs_total{status}`
- `aranea_eval_case_duration_seconds`

---

## T40 — A2A (M-9)

### New Files

| File | Description |
|------|-------------|
| `api/kratos/a2a/v1/a2a.proto` | API (AgentCard, Invoke, Discover, Audit) |
| `internal/biz/a2a.go` | `A2AUsecase` + `A2ARepo` interface |
| `internal/data/a2a.go` | SQLite/Postgres raw-SQL implementation |
| `internal/a2a/tool.go` | `call_agent` trpc CallableTool |
| `internal/service/a2a.go` | Kratos service adapter |
| `docs/guides/a2a-protocol.md` | Protocol specification + security model |

### Metrics

- `aranea_a2a_invoke_total{caller,callee,status}`
- `aranea_a2a_invoke_duration_seconds`

---

## T41 — CodeExecutor Sandbox (M-8)

### New Files

| File | Description |
|------|-------------|
| `internal/agent/codeexecutor/executor.go` | `Executor` interface + `LocalExecutor` + `DockerExecutor` |
| `docs/guides/codeexecutor.md` | Backend configuration + security controls |

### Metrics

- `aranea_codeexec_runs_total{kind,status}`
- `aranea_codeexec_duration_seconds{kind}`
- `aranea_codeexec_oom_total{kind}`

---

## Tests

| Package | New Tests |
|---------|-----------|
| `internal/biz` | `s6_coverage_test.go` — Knowledge / Evaluation / A2A usecase coverage |
| `internal/knowledge` | `chunker_test.go` — char + token chunking |
| `internal/agent/codeexecutor` | `executor_test.go` — language validation, config defaults |

---

## Backward Compatibility

- All new modules are additive; no existing API or database tables were modified.
- Schema creation calls are no-ops when tables already exist (`CREATE TABLE IF NOT EXISTS`).
- A2A is disabled by default on every agent (no behavior change for existing deployments).
- CodeExecutor `local` backend is the default; Docker is opt-in via config.

---

## Master-Plan Status Update

| Module | ID | Status |
|--------|----|--------|
| Knowledge Base | M-7 | ✅ |
| CodeExecutor Sandbox | M-8 | ✅ |
| A2A Protocol | M-9 | ✅ |
| Evaluation Framework | M-11 | ✅ |

---

## Files Changed (Summary)

**New**:
- `api/kratos/knowledge/v1/knowledge.proto` + generated
- `api/kratos/evaluation/v1/evaluation.proto` + generated
- `api/kratos/a2a/v1/a2a.proto` + generated
- `internal/biz/knowledge.go`, `evaluation.go`, `a2a.go`
- `internal/data/knowledge.go`, `evaluation.go`, `a2a.go`
- `internal/knowledge/chunker.go`, `embedder.go`, `retriever.go`
- `internal/evaluation/runner.go`
- `internal/a2a/tool.go`
- `internal/tools/knowledge/tool.go`
- `internal/service/knowledge.go`, `evaluation.go`, `a2a.go`
- `internal/agent/codeexecutor/executor.go`
- `internal/biz/s6_coverage_test.go`
- `internal/knowledge/chunker_test.go`
- `internal/agent/codeexecutor/executor_test.go`
- `docs/guides/knowledge.md`
- `docs/guides/evaluation.md`
- `docs/guides/a2a-protocol.md`
- `docs/guides/codeexecutor.md`

**Modified**: none (all S6 changes are additive).
