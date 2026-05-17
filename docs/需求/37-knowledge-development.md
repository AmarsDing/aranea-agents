# Knowledge 知识库 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 服务已注册，工程化未完成  
> **需求**：[37 knowledge.md](./37%20knowledge.md) · **设计**：[37 knowledge.design.md](./37%20knowledge.design.md)  
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-DATA-01、EP-KN-01、EP-KN-02、EP-RT-08

---

## 1. 模块定位

RAG 知识库：文档摄取 → 分块 → 嵌入 → pgvector 检索 → `knowledge_search` 工具进入 Agent 装配链（`ToolKeyKnowledgeSearch`）。

**代码锚点**：`internal/biz/knowledge.go`、`internal/data/knowledge.go`、`internal/knowledge/*`、`internal/tools/knowledge/tool.go`、`internal/agent/trpc_build.go`（`cfg.KnowledgeSearch`）。

---

## 2. 现状评估（2026-05-17 复核）

| 项 | 状态 | 证据 |
|----|------|------|
| HTTP/gRPC Service | ✅ | `internal/server/http.go` RegisterKnowledgeService |
| Agent 工具装配 | ✅ | `buildToolsetsForAgent` + 有效工具键 |
| Postgres Repo | 🟡 | `NewKnowledgeRepoFromData` 无 PG → nil |
| 启动 Schema | ❌ | `NewData()` 未调用 `EnsureKnowledgeSchema` |
| Embedder | 🟡 | `NewKnowledgeEmbedder()` 空配置 stub |
| 摄取流水线 | 🟡 | 同步为主，无进度 observable |
| 前端 | 🟡 | features 部分页面，闭环不完整 |

---

## 3. 差距与优化

1. **P0**：EP-DATA-01 — 有 Postgres 时在 `NewData()` 调用 `EnsureKnowledgeSchema`；nil Repo 时 API 返回明确 `FailedPrecondition`，禁止 panic。
2. **P1**：EP-KN-01 — `conf.Data` / env 注入 embedder（provider、base URL、key、dim）。
3. **P2**：EP-KN-02 — 摄取改 `safego` 异步 + 进度字段 + WS/轮询给前端。
4. **P2**：EP-RT-08 — 生产 Repo 与 mem repo 测试隔离。

---

## 4. 开发阶段

- **Phase 1（1 PR）**：`data.go` + service 边界错误语义 + 单测。
- **Phase 2（1 PR）**：Embedder 配置与启动 fail-fast 文档（§6 运维）。
- **Phase 3（1–2 PR）**：异步 ingest + 前端进度条。
- **Phase 4**：多 collection 权限与 workspace 隔离（依赖 M2）。

---

## 5. 任务清单

| # | 任务 | 优先级 |
|---|------|--------|
| 1 | `EnsureKnowledgeSchema` in `NewData` when `d.Postgres()!=nil` | P0 |
| 2 | `KnowledgeService` 在 `repo==nil` 时统一错误码 | P0 |
| 3 | 配置项 + `NewKnowledgeEmbedder` 读 conf | P1 |
| 4 | Ingest worker + 进度 API | P2 |
| 5 | 更新 `37 knowledge.md` 现状对齐 + 附录 A | P1 |

---

## 6. 验收标准

- [ ] 无 Postgres 时列表/搜索 API 可预期失败，进程不 panic
- [ ] 有 Postgres + schema 时 ingest → search → agent 工具调用通
- [ ] `go test ./internal/biz/... ./internal/service/...` 覆盖 nil repo
- [ ] changelog 引用 EP-DATA-01 / EP-KN-*

---

## 7. 依赖与风险

- 依赖 LlmProvider 嵌入能力与 `data.postgres` 配置。
- 向量维度变更需迁移策略；与 Memory L3 pgvector 共用实例时注意资源争用。
