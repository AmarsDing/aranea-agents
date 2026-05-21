# Knowledge 文档同步 + 摄取流水线优化

**日期**：2026-05-21  
**模块**：Knowledge (37)

## 摘要

- 三份 Knowledge 文档与代码对齐（Reranker、Embedder Admin API、WS 入库进度、EP-DATA-01/KN-01/KN-02 状态）。
- 提取 `internal/knowledge/ingest.go`（`BuildIndexedChunks`），Service 层单一职责。
- 修复文档 `metadata_json` 未写入 Chunk 的业务逻辑缺口。
- Reranker 配置错误时 SysLog 警告，避免静默降级。

## 代码

| 文件 | 变更 |
|------|------|
| `internal/knowledge/ingest.go` | 新增分块+向量化流水线 |
| `internal/knowledge/ingest_test.go` | metadata 校验与写入单测 |
| `internal/service/knowledge.go` | 使用 `BuildIndexedChunks`；校验 metadata_json |
| `internal/service/knowledge_retriever.go` | reranker 配置错误日志 |

## 文档

- `docs/需求/37 knowledge.md`
- `docs/需求/37 knowledge.design.md`
- `docs/需求/37-knowledge-development.md`
- `docs/需求/README-development.md`

## 验证

```bash
go test ./internal/knowledge/... ./internal/service/... -count=1 -run "Knowledge|Ingest|Rerank|Normalize"
```
