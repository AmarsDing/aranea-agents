# Memory 记忆 — 开发计划（总）

> **版本**：2026-06-06 | **状态**：🟢 L0–L3 + 运行时双轨已落地；🟢 L0 压缩优化阶段一已落地；🟢 L3 向量双写 + recall usecase 注入；🟢 CompositeSearch 分数修复；🟢 MemoryWorker LLM + 配置 UI；🟢 Cascade Saga + Memory Center Cascade Tab；🟢 全局衰减 + 业务化置信度模型 + 强化因子；🟢 MEM-OPT-01 L3 双轨读一致性 + MEM-OPT-03 优先级/Dead-Letter + MEM-OPT-02 L4 衰减/强化 + MEM-OPT-04 PII + MEM-OPT-05 提取协议 + MEM-OPT-06 Cascade Saga；❌ L0 压缩阶段二/三 + L4 LLM/bi-temporal + Neural Memory 未启动
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
| L3 索引一致性 | `internal/data/memory_fact_index_sync.go` + `internal/cronrunner/jobs/memory_fact_index_reconciler.go` |
| Dead-Letter | `internal/data/memory_job_deadletter.go` + `internal/service/memory_deadletter.go` + `internal/cronrunner/jobs/memory_dead_letter_replayer.go` |
| 优先级队列 | `internal/memory/trpc/auto_memory_queue.go` + `internal/biz/memory_queue_contract.go` |
| L4 衰减/强化 | `internal/biz/memory_l4.go` + `internal/data/sessionmemory/entity_lookup.go` + `internal/cronrunner/jobs/memory_l4_decay.go` + `internal/agent/l4_prompt.go` |
| entity_reinforcements | `docs/sql/10_memory_l4_reinforcements.sql` + `internal/data/memory_l4_reinforcements_patch.go` |

---

## 2. 全局现状（2026-06-06）

| 项 | 状态 |
|----|------|
| `MemorySet` / runtime 边界 | ✅ |
| L0 上下文压缩 + 快照 | ✅ |
| L0 压缩优化（阶段一：工程补强） | ✅ |
| L1 SQLite + Admin API + working_memory 工具 | ✅ |
| L1 归档 Worker + episode 归档 hook | ✅ |
| L2 episodes + 事件视图 + 多策略 Recall | ✅ |
| L2 Decay + Retention + Consolidate Worker | ✅ |
| L3 facts SQLite + Admin | ✅ |
| L3 embedding 双写（SQLite blob + pgvector 索引） | ✅ |
| L3 双轨读一致性（MEM-OPT-01） | ✅ Phase 0–3 全部落地 |
| L3 衰减 cron Job | ✅ |
| L3 冲突检测 + API | ✅ |
| L3 quality_score 5维评分 | ✅ |
| L3 PII 检测 + Review API（MEM-OPT-04） | ✅ 9 种 PII 检测器 + block/redact/review 策略 |
| L3 提取协议结构化（MEM-OPT-05） | ✅ function call schema + quality_score |
| L4 实体/关系 + prompt 注入 | ✅ |
| L4 Cascade Saga（MEM-OPT-06） | ✅ 4 步 Saga + 补偿 + Dry-Run |
| L4 Business Decay + reinforcement | ✅ |
| trpc memory.Service | ✅ |
| TurnMemoryWorker 入队 | ✅ |
| LLM 提取管道 | ✅ MVP（LLM→启发式链 + fallback 指标） |
| AutoMemoryQueue 优先级 / Dead-Letter（MEM-OPT-03） | ✅ 三优先级 + 租户配额 + Dead-Letter 持久化 + Replay RPC + 自动重放 cron + Wire Sink 连通 |
| Auto-memory upsert 失败重试 | ✅ 任一 fact 失败 fail job |
| SessionAdminStore 子接口拆分 | 🟡 L0–L4 接口已拆；typed RecallHit 渐进 |
| Memory Center 前端 | ✅ Cascade Tab + Knowledge/Session/Debug/WorkerStatus/DeadLetter/PlatformSettings 已接入；Graph Tab 需 feature flag；Store action 全覆盖（31 RPC → Store 封装）；前后端契约对齐（types.ts + api.ts wire key = proto snake_case） |
| 存储三写收敛 | ✅ facts 权威 + legacy backfill + pgvector 索引 |
| L0 压缩优化（阶段二/三） | ❌ 记忆演化 + Agent 自主压缩 |
| L4 LLM 实体抽取 | ❌ |
| L4 bi-temporal 边 | ❌ |
| Neural Memory 神经记忆系统 | ❌ 48 项任务全部未启动 |

分层现状见各 [`L*-development.md`](./README.md)。

---

## 3. 差距与优先级

| 优先级 | 项 | 说明 |
|--------|-----|------|
| **P1** | Policy Action Log | 统一 memory action 审计 | 🟡 Upsert/Delete/Clear/Entity/Cascade |
| **P2** | L0 压缩优化阶段二 | 记忆操作语义化（ADD/UPDATE/DELETE/MERGE/NOOP）+ 时间维度 + 动态链接 |
| **P2** | L0 压缩优化阶段三 | Agent 自主压缩（CompactContext/RecallDetail 工具）+ 代码骨架提取 |
| **P2** | L4 LLM 实体/关系抽取 | 替代 regex 启发式 |
| **P2** | L4 bi-temporal 边 | valid_from/valid_to 时间维度 |
| **P2** | L4 治理 UI | Evolution 审核 UI 闭环 + Graph Tab 生产就绪 |
| **P2** | L3 Conflict UI | 冲突检测前端展示 + 仲裁 |
| **P2** | L2 ListEvents 跨表视图 | UNION ALL 统一视图 |
| **P3** | L3 rerank / pgvector HNSW | Cross-Encoder 重排序 + 向量近似索引 |
| **P3** | L1 field history UI | 字段版本历史展示 + 回滚 |
| **P3** | Neural Memory 神经记忆系统 | 48 项任务，依赖 L0-L4 基础设施 |

---

## 4. 开发阶段

### Phase 1：L4 基础 — ✅

- ✅ Schema / Repo / `L4MemoryCue` / 启发式写入
- ✅ Cascade Saga + 补偿回滚 + Dry-Run
- ✅ Business Decay + reinforcement + 归档
- ✅ Name Conflict 检测 + 中文 regex
- ❌ GraphRAG、EvolutionScanner 闭环

### Phase 2：MemoryWorker — ✅ MVP

- ✅ Turn 完成后入队 + AutoMemory cron
- ✅ LLM 提取（`MemoryLLMExtractor` + 启发式 fallback）
- ✅ `memory_worker_provider/model` + `l0_compress_*`（Agent 设置 · 记忆 Tab）
- ✅ AutoMemory 直写 `memory_facts`（含 session/message provenance）
- ✅ L2 episode 写入（巩固完成后）
- ✅ AutoMemoryQueue 优先级 + Dead-Letter + 自动重放

### Phase 3：级联与 Policy — 🟡

- ✅ Cascade Saga（4 步 + 补偿 + Dry-Run）
- ✅ Approve 同步 L3 facts 更名 + L4 实体
- ✅ Memory Center Cascade Tab
- ✅ PII 检测 + Review API（MEM-OPT-04）
- ✅ 提取协议结构化（MEM-OPT-05）
- ✅ Action Log（Upsert/Delete/Clear/Entity/Cascade；turn_id 列已添加）

### Phase 4：L0 压缩优化 — 🟡 阶段一完成

- ✅ 阶段一：工程补强（工具结果持久化 + 三层代价递进 + 9 章节摘要 + LLM 缓存 + 手动压缩）
- ❌ 阶段二：记忆演化（操作语义化 + 时间维度 + 动态链接）
- ❌ 阶段三：Agent 自主压缩（CompactContext/RecallDetail + 代码骨架）

### Phase 5：L3/L4 增强 — ❌

- ❌ L3 Conflict UI、rerank、pgvector HNSW
- ❌ L4 LLM 实体抽取、bi-temporal、Evolution 审核 UI
- ❌ Neural Memory 神经记忆系统

---

## 5. 跨层任务清单

| # | 任务 | 层 | 状态 |
|---|------|-----|------|
| T1 | `MemorySet` 迁至 runtime | 总 | ✅ |
| T2 | L0 `SessionCompressor` | L0 | ✅ |
| T3 | L1 表 + List API + working_memory 工具 | L1 | ✅ |
| T4 | L2 episode 归档 + Decay + Consolidate | L2 | ✅ |
| T5 | L3 facts + conflicts + decay + quality_score | L3 | ✅ |
| T6 | L4 图 + 注入 + Cascade Saga + Decay | L4 | ✅ |
| T7 | TurnMemoryWorker | 总 | ✅ |
| T8 | LLM 提取管道 | L2/L3/L4 | ✅ MVP |
| T9 | CascadeProposal Saga | L4 | ✅ |
| T10 | Memory Center Tab | 总 | 🟡（+ Cascade Tab ✅；Graph Tab 需 feature flag） |
| T11 | pgvector 与 facts 收敛 | L3 | ✅ |
| T12 | legacy trpc_memory backfill | L3 | ✅（2026-05-24 修复 SQLite 死锁；见 §7） |
| T13 | Agent 设置 · Worker 模型 UI | 总 | ✅ |
| T14 | L0 压缩优化阶段一（工程补强） | L0 | ✅ |
| T15 | PII 检测 + Review API（MEM-OPT-04） | L3 | ✅ |
| T16 | 提取协议结构化（MEM-OPT-05） | L3 | ✅ |
| T17 | L0 压缩优化阶段二（记忆演化） | L0 | ❌ |
| T18 | L0 压缩优化阶段三（Agent 自主压缩） | L0 | ❌ |
| T19 | L4 LLM 实体/关系抽取 | L4 | ❌ |
| T20 | L4 bi-temporal 边 | L4 | ❌ |
| T21 | Neural Memory 神经记忆系统 | 总 | ❌ |

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

