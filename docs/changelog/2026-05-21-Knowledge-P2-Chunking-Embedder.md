# Knowledge P2：高级分块 + 文档解析 + EmbedBatch

**日期**：2026-05-21  
**模块**：Knowledge (37)

## 摘要

- 集成 trpc `chunking/*`：`markdown` / `json` / `recursive` 策略（`chunk_strategy` proto 字段）。
- 集成 trpc `document/reader/*` + HTML 文本提取：PDF/DOCX/HTML 入库。
- `EmbedBatch`：OpenAI/Gemini/HuggingFace TEI 批量 embedding；新增 `gemini` / `huggingface` provider。
- 文档三件套与 `README-development.md` 对齐。

## 代码

| 文件 | 变更 |
|------|------|
| `internal/knowledge/chunk_strategy.go` | trpc 分块桥接 |
| `internal/knowledge/document_extract.go` | 二进制文档 → 文本 |
| `internal/knowledge/readers_import.go` | reader 侧载 |
| `internal/knowledge/html_text.go` | HTML 可见文本 |
| `internal/knowledge/embedder.go` | Gemini/HF + 批量 API |
| `internal/knowledge/ingest.go` | 策略 + EmbedBatch |
| `api/kratos/knowledge/v1/knowledge.proto` | `chunk_strategy` |
| `web/src/features/knowledge/types.ts` / `api.ts` | 前端字段 |

## 验证

```bash
go test ./internal/knowledge/... -count=1
make api
```
