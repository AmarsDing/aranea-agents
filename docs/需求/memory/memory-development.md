# Memory 记忆 — 开发计划（总）

> **版本**：2026-05-24 | **状态**：🟢 L0–L3 + 运行时双轨已落地；🟢 L3 向量双写 + recall usecase 注入；🟢 CompositeSearch 分数修复；🟢 MemoryWorker LLM + 配置 UI；🟢 Cascade 后端 + Memory Center Tab；🟡 全局衰减  
> **需求**：[`memory.md`](./memory.md) · [`L0.md`](./L0.md)～[`L4.md`](./L4.md)  
> **设计**：[`memory.design.md`](./memory.design.md)  
> **进度真相**：[execution-plan.md](../../guides/execution-plan.md) · [0-system-development.md](../0-system-development.md) §8.6  
> **运行时边界**：[AGENT_RUNTIME_BOUNDARY.md](../../AGENT_RUNTIME_BOUNDARY.md)

---

## 1. 模块定位

Agent 记忆：**五层产品模型（L0–L4）** + **trpc-agent-go `memory.Service` 适配** + **Aranea 管理/观测 API**。

### 1.1 架构分层

| 层级 | 记忆相关包 |
|------|------------|
| `api/kratos/memory/v1` | 对外契约 |
| `internal/service` | `memory.go`、`session_compress.go`（**不** import trpc） |
| `internal/biz` | `MemoryAdminUsecase`、`L4GraphUsecase`、`MemoryUsecase`(pgvector)、`memory_worker.go` |
| `internal/agent` | `trpc_build.go`、`l4_prompt.go` |
| `internal/memory/trpc` | `sqlite_adapter.go` |
| `internal/data/sessionmemory` | L0–L4 表；Admin + 框架 adapter **共用** |
| `internal/runtime` | `memory_set.go` |

### 1.2 主从关系（已定稿）

- **Runner**：`MemorySet.TRPC` = `NewSQLiteMemoryService(sessionmemory.Store)`
- **治理**：`MemorySet.Admin` = `SessionAdminStore`（L0–L4 子接口组合）
- **Prompt 注入**：`MemorySet.L2Recall` / `L3Recall` = `MemoryL2RecallUsecase` / `MemoryL3RecallUsecase`
- **L3 索引**：`data.NewMemoryFactIndexSync` → pgvector（可选）+ `memory_facts.embedding_blob`
- **可选 pgvector**：`MemoryUsecase`；trpc Search 可选轨，**非** prompt 主路径

### 1.3 全局代码锚点

| 能力 | 路径 |
|------|------|
| L0 压缩 | `internal/service/session_compress.go` |
| Admin API | `internal/service/memory.go` |
| 存储 | `internal/data/sessionmemory` + `memory_chain.sql` |
| 框架 Memory | `internal/memory/trpc/sqlite_adapter.go` |
| L4 图 | `internal/biz/memory_l4_usecase.go` |
| L4 注入 | `internal/agent/l4_prompt.go` |
| Turn 后调度 | `internal/biz/memory_worker.go` |
| 周期 AutoMemory | `internal/cronrunner/jobs/auto_memory.go` |

---

## 2. 全局现状（2026-05-24）

| 项 | 状态 |
|----|------|
| `MemorySet` / runtime 边界 | ✅ |
| L0 上下文压缩 + 快照 | ✅ |
| L1 SQLite + Admin API | ✅ |
| L2 episodes + 事件视图 | ✅ |
| L3 facts SQLite + Admin | ✅ |
| L3 embedding 双写（SQLite blob + pgvector 索引） | ✅ |
| L3 pgvector（可选 Search 轨） | 🟡 |
| L4 实体/关系 + prompt 注入 | ✅ |
| L4 启发式写入 / 衰减元数据 | 🟡 MVP |
| trpc memory.Service | ✅ |
| TurnMemoryWorker 入队 | 🟡 MVP |
| LLM 提取管道 | 🟡 MVP（LLM→启发式链 + fallback 指标） |
| 级联 BFS + 审核 UI | ✅ RPC + 门控 + Memory Center Cascade Tab |
| L3 rerank / 统一 decay | 🟡 rerank + scored recall ✅；decay cron ✅ |
| Auto-memory upsert 失败重试 | ✅ 任一 fact 失败 fail job |
| SessionAdminStore 子接口拆分 | 🟡 L0–L4 接口已拆；typed RecallHit 渐进 |
| Memory Center 前端 | 🟡（Cascade Tab ✅；其余 Tab 已接入） |
| 存储三写收敛 | ✅ facts 权威 + legacy backfill（旧 trpc 路径，见 §7）+ pgvector 索引 |

分层现状见各 [`L*-development.md`](./README.md)。

---

## 3. 差距与优先级

| 优先级 | 项 | 说明 |
|--------|-----|------|
| **P1** | 存储收敛 | L3 单一写路径；pgvector 降为索引 |
| **P1** | Policy Action Log | 统一 memory action 审计 | 🟡 Upsert/Delete/Clear/Entity/Cascade |
| **P2** | MemoryWorker LLM | 替代 regex 提取 | ✅ MVP + Agent 设置 UI |
| **P2** | L4 级联 + bi-temporal | CascadeProposal + valid_from/to | ✅ 后端 + Cascade Tab |
| **P2** | L4 治理 UI | 冲突、Evolution 审核台 | 🟡 Cascade ✅ / Evolution 图谱 Tab 需 `VITE_MEMORY_GRAPH_TAB` |
| **P3** | L3 rerank、PII、全局衰减 | Phase 4–5 |

---

## 4. 开发阶段

### Phase 1：L4 基础 — 🟡 MVP

- ✅ Schema / Repo / `L4MemoryCue` / 启发式写入
- ❌ GraphRAG、Proposal 审核台、EvolutionScanner 闭环

### Phase 2：MemoryWorker — 🟢 MVP

- ✅ Turn 完成后入队 + AutoMemory cron
- ✅ LLM 提取（`MemoryLLMExtractor` + 启发式 fallback）
- ✅ `memory_worker_provider/model` + `l0_compress_*`（Agent 设置 · 记忆 Tab）
- ✅ AutoMemory 直写 `memory_facts`（含 session/message provenance）
- ✅ L2 episode 写入（巩固完成后）

### Phase 3：级联与 Policy — 🟡

- ✅ bi-temporal 边、BFS、CascadeProposal RPC
- ✅ Approve 同步 L3 facts 更名 + L4 实体
- ✅ Memory Center Cascade Tab
- 🟡 Action Log（Upsert/Delete/Clear/Entity/Cascade；turn_id 经 source_message_id）

### Phase 4–5：L3 增强

- ❌ rerank、Cross-Encoder、Composite Search
- ❌ 全局衰减 Job、PII 管道

---

## 5. 跨层任务清单

| # | 任务 | 层 | 状态 |
|---|------|-----|------|
| T1 | `MemorySet` 迁至 runtime | 总 | ✅ |
| T2 | L0 `SessionCompressor` | L0 | ✅ |
| T3 | L1 表 + List API | L1 | ✅ |
| T4 | L2 episode 归档 | L2 | ✅ |
| T5 | L3 facts + conflicts 元数据 | L3 | 🟡 |
| T6 | L4 图 + 注入 | L4 | ✅ |
| T7 | TurnMemoryWorker | 总 | ✅ |
| T8 | LLM 提取管道 | L2/L3/L4 | ✅ MVP |
| T9 | CascadeProposal | L4 | ✅ |
| T10 | Memory Center Tab | 总 | 🟡（+ Cascade Tab） |
| T11 | pgvector 与 facts 收敛 | L3 | ✅ |
| T12 | legacy trpc_memory backfill | L3 | ✅（2026-05-24 修复 SQLite 死锁；见 §7） |
| T13 | Agent 设置 · Worker 模型 UI | 总 | ✅ |

---

## 6. 依赖与风险

- **Wire**：`MemoryUsecase` 依赖 `EmbeddingService` + Postgres 配置；需与 `provideMemoryService` 一并注入
- **双轨误接**：实现者将 pgvector 直接挂 Runner → 须读 [`memory.design.md`](./memory.design.md) §二.2
- **启动管道**：Legacy backfill 在 `ensureAllSchemas` 同步执行；SQLite 同表游标+UPDATE 曾导致启动死锁（已修复）；目标态拆分见 [`memory.design.md`](./memory.design.md) §十一
- **文档**：以本 development 与各层 `*-development.md` 为准

---

## 7. Legacy 迁移与启动管道（2026-05-24）

> **定位**：`trpc_memory` / `memory_items` 属 **旧业务系统** 兼容层，非 L3 目标写路径。详见 [`memory.design.md`](./memory.design.md) §3.1、§十一 · [`L3.design.md`](./L3.design.md) §3.8。

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| T14 | Legacy backfill SQLite 游标死锁修复 | P0 | ✅ |
| T15 | `BackfillLegacyTRPCMemoryEntities` 单测 + 启动 step 日志 | P1 | ✅ |
| T16 | `schema_migrations` 版本表（Ent schema）；backfill 只跑一次 | P2 | ✅ |
| T17 | 拆分 `ensureSchemaDDL` / `RunPendingDataMigrations`；移出 wire 热路径 | P3 | ✅（`MemoryDataMigrationWorker` · kratos `AfterStart`） |
| T18 | `cmd/memory-migrate legacy-trpc-facts` 离线 CLI | P4 | ✅ |

**代码锚点**：

| 路径 | 说明 |
|------|------|
| `internal/data/sessionmemory/store_legacy_backfill.go` | Legacy entity → `memory_facts` |
| `internal/data/schema_migrations.go` | Ent `SchemaMigration` 读写 · version gate |
| `internal/cronrunner/jobs/memory_data_migration.go` | HTTP listen 后一次性数据迁移 worker |
| `internal/data/memory_migrate.go` | `RunLegacyTRPCMemoryMigration` |
| `internal/data/data.go` | `ensureSchemaDDL` / `runPendingDataMigrations` / `[startup]` 日志 |
| `cmd/memory-migrate` | 离线 `legacy-trpc-facts --dry-run|--apply` |

**验收（T14–T18）**：

- [x] 含 pending legacy 行的库可在数秒内完成 `NewData`，HTTP `:8000` 可 listen
- [x] 迁移幂等：二次启动不重复插入 fact
- [x] `schema_migrations` 记录 version `20260524` 后跳过 backfill
- [x] 启动输出 `[startup] <step> done in …` 与 legacy 迁移摘要
- [x] `go run ./cmd/memory-migrate legacy-trpc-facts --dry-run|--apply`

---

## 8. MemoryWorker 设计要点

**产品设想**（§8.1–8.4 见 [`memory.design.md`](./memory.design.md) 附录 A §八）：Turn 完成后异步 LLM 提取、巩固、级联提议。

**实现现状**（2026-05-24）：`TurnMemoryWorker` 入队 → `AutoMemoryWorker` cron → `ChainConsolidator`（LLM → 启发式）→ `UpsertFactRow`；模型解析顺序：**`memory_worker_*` → `l0_compress_*` → session/agent 聊天模型**。配置入口：**Agent 设置 → 记忆 →「巩固 Worker 模型」**（同页另有 L0 摘要 Provider/Model）。

| 字段 | DB 列 | 用途 |
|------|--------|------|
| `memory_worker_provider` / `memory_worker_model` | `agent_runtime_settings` | AutoMemory LLM 提取 |
| `l0_compress_provider` / `l0_compress_model` | 同上 | L0 会话摘要；Worker 未配时的 fallback |

---

## 9. 验收标准（模块级）

- [x] L0–L3 Admin 读 API 可用
- [x] Runner 注入 memory.Service（有 Store 时）
- [x] MemoryWorker LLM 提取端到端（独立 worker 模型 + Agent 设置 UI）
- [x] 级联 Proposal 审核闭环（后端 RPC + L4 冲突门控 + Cascade Tab）
- [x] L3 单一写路径 + pgvector 索引同步 + legacy backfill
- [ ] Memory Center 全 Tab 非占位（Evolution 图谱 Tab 仍 gated）

