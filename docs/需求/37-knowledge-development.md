# Knowledge 知识库 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[37 knowledge.md](./37%20knowledge.md) · **设计**：[37 knowledge.design.md](./37%20knowledge.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Knowledge 知识库：管理 Agent 的知识来源，支持文档上传、分块、向量化、检索和注入。

**代码锚点**：
- `api/kratos/knowledge/v1/` — Knowledge CRUD RPC
- `internal/service/knowledge.go` — KnowledgeService
- `internal/biz/knowledge.go` — KnowledgeUsecase
- `internal/data/knowledge.go` — KnowledgeRepo
- `internal/knowledge/chunker.go` — 文档分块
- `internal/knowledge/embedder.go` — 向量化
- `internal/agent/trpc_build.go` — Knowledge 注入

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Knowledge CRUD | ✅ | Create/Update/Delete/Get/List |
| 文档上传 | ✅ | 文件上传 + 解析 |
| 文档分块 | ✅ | `chunker.go`（Markdown/Text 分块） |
| 向量化 | ✅ | `embedder.go`（OpenAI Embedding） |
| 向量检索 | ✅ | SQLite 向量搜索 |
| Knowledge 注入 | ✅ | `BuildTRPCLLMAgent` 中 `WithKnowledge` |
| 前端管理 | ✅ | Knowledge 设置页 |

---

## 3. 差距与优化

1. **P2**：文档解析仅支持 Markdown/Text，不支持 PDF/Word/HTML 等常见格式。
2. **P2**：向量化仅支持 OpenAI Embedding，不支持本地 Embedding 模型。
3. **P3**：知识库无增量更新，文档变更后需全量重新向量化。
4. **P3**：检索结果无 re-ranking，相关性可能不佳。

---

## 4. 开发阶段

- **Phase 1**：支持 PDF/Word/HTML 文档解析
- **Phase 2**：支持本地 Embedding 模型
- **Phase 3**：增量更新 + re-ranking

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | PDF/Word/HTML 解析器集成 | P2 | — |
| 2 | 本地 Embedding 模型支持（如 sentence-transformers） | P2 | — |
| 3 | 增量向量化（仅处理变更文档） | P3 | — |
| 4 | 检索结果 re-ranking | P3 | — |

---

## 6. 验收标准

- [ ] 可上传 PDF/Word/HTML 文档并正确解析
- [ ] 可使用本地 Embedding 模型
- [ ] 文档变更后仅增量更新向量

---

## 7. 依赖与风险

- PDF 解析需引入第三方库（如 unidoc/unioffice）
- 本地 Embedding 模型需 GPU 或大量 CPU 资源
