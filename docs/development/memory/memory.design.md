# Memory L0–L4 — 实现设计（总）

> **对应需求**：[`memory.md`](./memory.md) · [`L0.md`](./L0.md)～[`L4.md`](./L4.md)  
> **开发计划**：[`memory-development.md`](./memory-development.md)  
> **规范**：[`AI-DEVELOPMENT-SPECIFICATION.md`](../../guides/AI-DEVELOPMENT-SPECIFICATION.md)

---

## 一、模块概述

五层记忆架构：**L0** 感官（上下文装配）· **L1** 工作（结构化任务态）· **L2** 情景（会话事件视图）· **L3** 语义（事实知识）· **L4** 持久（实体图谱 + Agent 进化）。

**核心设计立场**（详见 [`theory.md`](./theory.md)）：Memory = **(Raw Ledger, Derived Views, Policy)**。L1–L4 均为 Ledger 上的派生视图；L0 是每轮 Turn 的 **Context Package**（瞬时，不单独持久化内容副本）。

---

## 二、目标架构（2026-05 评审定稿）

### 2.1 单一 Ledger，多层为 View + Policy

```text
Turn Event（不可变追加）
    ↓ Policy: consolidate（MemoryWorker / AutoMemory）
Derived: L1 fields · L2 episodes · L3 facts · L4 entities
    ↓ Policy: assemble（ChatService / TeamRuntime）
L0 Context Package（每轮 ephemeral）
```

| 组件 | 职责 |
|------|------|
| **Raw Ledger** | `messages`、tool/skill 调用、memory action log、field/fact version history |
| **Derived Views** | L1–L4 表 + 向量/BM25 索引 + L0 `segments_json` |
| **Policy** | 何时读/写/遗忘/级联；决策显式记录为 Action（ADD/UPDATE/DELETE/NONE） |

**Memory-Agent**（产品设想）：后台 **Consolidator** goroutine，订阅 `runner_completion`，执行 LLM 提取、冲突检测、级联提议；**不是**第六层存储，也不单独维护第二套神经记忆库。

### 2.2 存储收敛（目标态）

| 数据 | 权威存储 | 索引/可选 |
|------|----------|-----------|
| L0 内容 | `messages` + `session_summaries` | `memory_l0_assembly_snapshots`（元数据） |
| L1–L2/L4 | SQLite（`internal/data/memory_shim_*.go`） | — |
| L3 facts | SQLite `memory_facts`（权威） | embedding 列或 `memory_fact_index`；**pgvector 仅作可选读索引** |
| 框架 Memory | **适配** L3FactReader/Writer（shim），禁止平行第三写入口 | `sqlite_adapter` → `memtrpc.NewMemoryService` |

**原则**：消除 L3 三写（`memory_facts` / pgvector `agent_memory_*` / trpc event entity 各写各的）。Runner 侧 `trpc-agent-go/memory.Service` 是框架接口真相源；Aranea L0–L4 **实现**该接口并暴露 Admin 治理面。

### 2.3 双轨关系（Runner vs 产品治理）

```go
// internal/runtime/memory_set.go
type MemorySet struct {
    TRPC     trpcmemory.Service      // Runner 注入
    Admin    biz.SessionAdminStore   // L0–L4 观测/治理 API（组合 L0/L1/L2/L3/L4 子接口）
    L2Recall biz.MemoryL2Recaller    // prompt 注入 L2 recall（usecase 层 embed + store rerank）
    L3Recall biz.MemoryL3Recaller    // prompt 注入 L3 recall
}
```

| 轨 | 包 | 用途 |
|----|-----|------|
| Runner | `internal/memory/trpc` → `L3FactReader`/`L3FactWriter`（`memory_shim_*`） | Turn 内 memory tools、跨会话 recall |
| 产品 Admin | `MemoryAdminUsecase` → `memory_shim_*` 适配器 | gRPC ListL0/L1/Facts/Entities… |
| Prompt 注入 | `MemoryL2RecallUsecase` / `MemoryL3RecallUsecase` | **唯一** query embedding 入口；Store 不再内嵌 embedder |
| L3 向量索引 | `data.NewMemoryFactIndexSync` | 单次 embed → pgvector（可选）+ `memory_facts.embedding_blob`（权威 recall） |
| 可选 pgvector | `biz.MemoryUsecase` | trpc `SearchMemories` 可选向量轨；**非** prompt 注入主路径 |

**禁止**：`internal/biz` / `internal/service` 直接 import `pkg/trpc-agent-go`。

### 2.4 统一 Policy 与 Action Log（规划）

每次记忆变更写入 `memory_action_log`（或复用 `audit_logs` + 结构化 detail）：

```json
{
  "action": "UPDATE",
  "target": "fact:xxx",
  "reason": "cascade_of:entity:person:123.location",
  "policy_version": "consolidate_v2",
  "source_event_ids": ["turn:abc"]
}
```

支持 provenance、A/B、级联 Approve/Reject（`CascadeProposal` RPC，见 Proto 待增清单）。

### 2.5 L4 级联与 bi-temporal（规划）

实体-关系边携带 **valid_from / valid_to**；检索默认 `query_time = now()`：

```text
(Person:A)-[:WORKS_AT {valid_from, valid_to}]->(Location:Beijing)
(Person:A)-[:WORKS_AT {valid_from: 2026-05}]->(Location:NYC)
```

属性变更触发 BFS → 生成 `CascadeProposal` → 用户/Critic 审核 → 批量 UPDATE 关联 facts/entities。

---

## 三、存储拓扑

**默认库**：`cmd/data/arenea.sqlite`（`configs/config.yaml` → `database.source`）

| 层 | 表（SQLite） | 说明 |
|----|--------------|------|
| L0 | `messages`, `session_summaries`, `memory_l0_assembly_snapshots` | 内容在 messages；L0 层 = 装配行为 |
| L1 | `memory_l1_tasks`, `memory_l1_fields`, `memory_l1_field_history`, `memory_l1_schemas` | |
| L2 | 复用 `messages`/trace 等 + `memory_episodes`, `memory_event_marks`, `memory_l2_index_meta` | L2 是视图，不另造聊天事实源 |
| L3 | `memory_facts`, `memory_fact_versions`, `memory_fact_conflicts`, `memory_fact_feedback`, `memory_fact_index` | |
| L4 | `memory_entities`, `memory_relations`, `memory_entity_facts`, `memory_entity_versions` | |
| 兼容 | `memory_items` | deprecated；迁移期双写 |

**可选 Postgres**：`agent_memory_<dim>` pgvector（`internal/data/pgvector`）

Schema 源：`internal/data/sql/memory_chain.sql` · 分层 SQL：`docs/sql/10_memory_l*.sql`

### 3.1 Legacy 迁移边界（旧业务系统 · 非目标态）

**Legacy** 在本模块中指 **存储收敛前** 的旧写入路径，**不属于** L0–L4 目标架构的运行时业务，仅作 **一次性/幂等数据桥接**，完成后应从热路径退场。

| Legacy 来源 | 表 / 标识 | 目标态 | 说明 |
|-------------|-----------|--------|------|
| 旧 trpc Memory 实体写路径 | `memory_entities` · `scope_type='trpc_memory'` · `entity_type='memory_fact'` | `memory_facts`（L3 权威） | 框架早期经 `sqlite_adapter` 写入 entity 形态；收敛后 **禁止新写入** |
| 旧 Admin / 兼容 API | `memory_items` | `memory_facts` | deprecated 视图层；见 [`L3.design.md`](./L3.design.md) §3.1 |
| 旧 pgvector 平行写 | `agent_memory_<dim>` 独立事实 | `memory_facts` + 索引同步 | pgvector **仅读索引**，见 §2.2 |

**代码锚点（Legacy 桥接，非产品功能）**：

- `internal/data/memory_migrate.go`（及关联 shim）— `BackfillLegacyTRPCMemoryEntities`
- 启动调用链：`internal/data/data.go` → `ensureAllSchemas`（**待拆分**，见 §十一）

**原则**：

1. Legacy 迁移 **不得** 阻塞 HTTP/gRPC 监听 indefinitely；失败须可诊断、可离线重跑。
2. 迁移写入 **不得** 长期复用运行时 `UpsertFactRow` 全路径（Policy / ActionLog 副作用与启动阶段 Store 装配不一致）。
3. SQLite 读写同表须 **先读后写、关闭游标再 UPDATE**（禁止 `rows.Next` 循环内 UPDATE 同表）。

---

## 四、Kratos 分层职责

| 层级 | 包 | 职责 |
|------|-----|------|
| Proto | `api/kratos/memory/v1/memory.proto` | 对外契约 |
| Service | `internal/service/memory.go`, `session_compress.go` | proto ↔ biz；L0 压缩 |
| Biz | `memory_admin_*.go`, `memory_l4*.go`, `memory_worker.go` | 领域；**不** import trpc |
| Agent | `trpc_build.go`, `l4_prompt.go` | Runner 装配、L4 cue 注入 |
| Data | `internal/data/memory_shim_*.go` + `memory_helpers.go` | L0–L4 表读写（原 sessionmemory 包已折叠） |
| Bridge | `internal/memory/trpc/sqlite_adapter.go` | 框架 MemoryService |
| Runtime | `internal/runtime/memory_set.go` | Wire 组装 MemorySet |

---

## 五、MemoryWorker / Consolidator

### 5.1 同步 vs 异步

| 阶段 | 时机 | 内容 |
|------|------|------|
| 同步 | Turn 内 | L0 装配；L1 tool 读写；L3/L4 recall 注入 |
| 异步 | `runner_completion` 后 | LLM 提取 fact/episode/entity；冲突检测；级联提议入队 |

### 5.2 现状 vs 目标

| 项 | 现状 | 目标 |
|----|------|------|
| 调度 | `TurnMemoryWorker` 入队 + `AutoMemoryWorker` cron | EventBus 订阅 + 可配置 worker |
| 提取 | regex 启发式 → L4 | LLM JSON schema 提取 → L2/L3/L4 |
| 巩固 | 部分 episode 归档 | L2→L3/L4 管道 + 去重 + 冲突 |
| 级联 | 元数据钩子 | BFS + CascadeProposal UI |

代码锚点：`internal/biz/memory_worker.go` · `internal/cronrunner/jobs/auto_memory.go`

**Ebbinghaus decay 扫描口径（2026-08-05，H3）**：`internal/cronrunner/jobs/memory_ebbinghaus_decay.go` per-agent 扫描改为按**产生方 agent**（`agent_id` 过滤）跨全部 scope 拉取活跃 facts（批上限 `memoryEbbinghausDecayBatchSize=500`）。旧口径 `scope_type='agent'` 只覆盖 agent 域子集，session/user 域事实的 R_t（reachability）衰减分长期不更新，fused recall 降权失真。

---

## 六、层间数据流

```text
用户消息 → messages (Ledger)
    → L0 装配 ← L1 RenderForPrompt
              ← L3 Recall(facts)
              ← L4 GraphRecall(neighbors)
Turn 结束 → MemoryWorker
    → L1 task 归档 → memory_episodes (L2)
    → 提取 facts → memory_facts (L3)
    → 提取 entities → memory_entities (L4)
L4 属性变更 → CascadeProposal → 审核 → L3/L4 批量更新
```

各层细节见 [`L0.design.md`](./L0.design.md)～[`L4.design.md`](./L4.design.md)。

---

## 七、Proto 索引

文件：`api/kratos/memory/v1/memory.proto`

**已实现 RPC（摘要）**：`ListL0Snapshots` · `ListL1Tasks/Fields` · `ListMemoryFacts` · `UpsertMemoryFact` · `ListMemoryEntities` · `GetMemoryNeighborhood` · `SpreadingActivation` · `GetAgentIdentity/Strategy` · `ListEvolutionProposals/Events` · `AppendEvolutionEvent` · `GetEvolutionMetrics` · `ListCascadeProposals` · `Approve/RejectCascadeProposal` · `PreviewCascadeApprove` · `GetCascadeSagaSteps` · `Retry/CompensateCascadeApprove` · `DebugMemoryRecall` · `CompositeSearchMemories` · `GetMemoryWorkerStatus` · `Get/UpdateMemoryPlatformSettings` · `List/Replay/AbandonMemoryDeadLetters` · `ListPIIFlaggedFacts` · `ReviewPIIFact` · `ListConflictingFacts`

**仍待增 / 产品缺口 RPC**：`GetMemoryOverview` · `GetMemorySnapshot` · `Confirm/RejectMemoryFact`（非 PII） · `ResolveMemoryConflict` · `ConsolidateEpisode`（Admin 面；内部 consolidator 已存在） · 独立 Admin `SearchMemories`（现有 `CompositeSearchMemories` + 框架 Search）

完整 message 定义见下文 **附录 A §二**；新改动以 proto 源文件为准。

---

## 八、前端设计索引

Memory Center 组件与路由详见 **附录 A §九**。

关键页面：`MemoryOverview` · `KnowledgeFacts` · `SessionMemoryDrawer` · `MemorySnapshotDrawer` · `GraphExplorer`（`VITE_MEMORY_GRAPH_TAB=1`）· `EvolutionProposals`

前端规范：[`frontend-guide.md`](../../guides/frontend-guide.md)

---

## 九、与 trpc-agent-go 对齐

| 框架概念 | Aranea 落位 |
|----------|-------------|
| `session.Session` events | L0 滑动窗口事实源 |
| `memory.Service` | `sqlite_adapter` → `L3FactReader`/`L3FactWriter`（`memory_shim_*`） |
| memory tools (add/search/…) | Agent 装配于 `trpc_build.go` |
| `AddSessionToMemory` | 待与 L2/L3 写入对齐（P2） |

框架真相源：`pkg/trpc-agent-go/memory`（仅 `internal/agent`、`internal/memory/trpc` 引用）

---

## 十、关键设计原则

1. **L0 是通道，不是仓库**：价值在有效信息密度，不在堆字
2. **L2 不替代 messages**：episode 是叙事视图
3. **L3 权威在 SQLite facts**：向量是索引，不是第二事实源
4. **L4 改动须门控**：Evolution/Cascade 走 Proposal + 回滚
5. **Policy 可观测**：禁止仅靠 prompt 暗示完成记忆写入
6. **Legacy 只迁移、不扩展**：`trpc_memory` / `memory_items` 为旧系统兼容；新功能只写 `memory_facts` 与 L4 图谱表

---

## 十一、启动迁移架构（`internal/data`）

> **问题背景**（2026-05-24）：Legacy backfill 曾置于 `ensureAllSchemas` 同步热路径；SQLite 同连接「游标未关 + UPDATE 同表」导致启动死锁，HTTP 未监听。  
> **修复**：`store_legacy_backfill.go` 改为先 `scan` 入 slice 再逐条迁移。  
> **目标态**：DDL 与 Data migration 分离，与项目既有 `cmd/sqlmigrate` 体系对齐。

### 11.1 职责拆分（目标态）

```text
NewData 启动热路径（必须快、可预测）
  ├─ initSQLite          — 打开库 + PRAGMA + Ent Schema.Create / migrateDev
  ├─ ensureSchemaDDL     — 仅 DDL / patch（Ensure*Schema、列 patch）
  └─ seedInitialData     — 最小种子（admin、system_setting）

Post-start / 离线（Data migration）
  ├─ RunPendingDataMigrations — legacy trpc_memory → memory_facts 等
  └─ schema_migrations 版本表 — 每条 migration 只执行一次
```

| 类型 | 示例 | 执行时机 | 失败策略 |
|------|------|----------|----------|
| **DDL patch** | `EnsureSessionMemorySchema`、`EnsureMessageFTSSchema` | 启动同步 | 启动失败（schema 不可用） |
| **Legacy data migration** | `BackfillLegacyTRPCMemoryEntities` | 启动后 worker 或 `cmd/memory-migrate` | 日志 + 重试；**不** indefinite 阻塞 listen |
| **种子** | `ensureInitialAdminFromConfig` | 启动同步 | 启动失败 |

**现状（过渡）**：Legacy backfill 仍在 `ensureAllSchemas` 内同步调用；pending=0 时仅一次 SELECT，开销可忽略。待 `schema_migrations` 落地后移出热路径。

### 11.2 `schema_migrations` 版本表（规划）

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  applied_at TEXT NOT NULL
);
-- 例：version=20260524, name='legacy_trpc_memory_facts'
```

启动时：未 applied 的 data migration 执行并记版本；**已 applied 则 O(1) 跳过**。

### 11.3 Legacy backfill 实现约束

| 约束 | 说明 |
|------|------|
| 读写在连接上互斥 | `SELECT` 结果集 `Close` 后再 `UPDATE` / `Upsert` |
| 幂等 | `UpsertFactRow` fingerprint 去重；legacy 行 `status='migrated'` |
| 可观测 | 记录 `migrated/total` 与 `duration_ms`（flow step 或 INFO） |
| 超时 | 启动链 `context.WithTimeout`（规划，默认 30s/step） |
| 单实例 | 开发环境避免并行 `go run ./cmd/admin` 争用同一 SQLite 文件 |

### 11.4 与运维文档对齐

| 场景 | 文档 |
|------|------|
| 存量 SQL 脚本 | [`docs/merge/2026-05-21-database.md`](../../merge/2026-05-21-database.md) · `cmd/sqlmigrate` |
| Memory DDL 源 | `internal/data/sql/memory_chain.sql` |
| Legacy 专项 | 本文 §3.1 · [`L3.design.md`](./L3.design.md) §3.8 · changelog [`2026-05-24-Memory-Legacy-Backfill-Startup.md`](../../changelog/2026-05-24-Memory-Legacy-Backfill-Startup.md) |

### 11.5 演进路线（开发计划见 [`memory-development.md`](./memory-development.md) §7）

| 优先级 | 项 |
|--------|-----|
| P0 | SQLite 游标死锁修复（✅ 2026-05-24） |
| P1 | backfill 单测 + 启动 step 日志（✅ 2026-05-24） |
| P2 | `schema_migrations` + backfill 只跑一次（✅ 2026-05-24） |
| P3 | 从 `ensureAllSchemas` 拆出 `ensureSchemaDDL` / `runPendingDataMigrations`（✅ 数据迁移移入 `MemoryDataMigrationWorker`，HTTP listen 后执行） |
| P4 | `cmd/memory-migrate legacy-trpc-facts` 离线入口（✅ 2026-05-24） |

---

## 附录 A：Proto / Biz / Data / Service / Wire / Worker / 前端（原 `12-16 memory.design.md` 正文）

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/memory/v1/memory.proto`

```protobuf
service MemoryService {
  rpc ListMemories(ListMemoriesRequest) returns (ListMemoriesResponse) {
    option (google.api.http) = { get: "/v1/memories" };
  }
  rpc GetMemory(GetMemoryRequest) returns (Memory) {
    option (google.api.http) = { get: "/v1/memories/{id}" };
  }
  rpc CreateMemory(CreateMemoryRequest) returns (Memory) {
    option (google.api.http) = { post: "/v1/memories" body: "*" };
  }
  rpc DeleteMemory(DeleteMemoryRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/memories/{id}" };
  }
  rpc RecallMemories(RecallMemoriesRequest) returns (RecallMemoriesResponse) {
    option (google.api.http) = { post: "/v1/memories/recall" body: "*" };
  }
}
```

### 2.2 已新增

以下 RPC 已在 `api/kratos/memory/v1/memory.proto` 中定义：

| RPC | 路径 | 用途 | 状态 |
|-----|------|------|------|
| `ListL0Snapshots` | — | L0 快照列表 | ✅ 已定义 |
| `ListL1Tasks` | — | L1 任务列表 | ✅ 已定义 |
| `ListL1Fields` | — | L1 字段列表 | ✅ 已定义 |
| `ListMemoryFacts` | — | L3 fact 列表 | ✅ 已定义 |
| `ListMemoryEntities` | — | L4 实体列表 | ✅ 已定义 |
| `GetMemoryNeighborhood` | — | L4 邻居查询 | ✅ 已定义 |
| `GetAgentIdentity` | — | Agent 身份 | ✅ 已定义 |
| `GetAgentStrategy` | — | Agent 策略画像 | ✅ 已定义 |
| `ListEvolutionProposals` | — | 进化提议列表 | ✅ 已定义 |
| `ListEvolutionEvents` | — | 进化事件列表 | ✅ 已定义 |
| `GetEvolutionMetrics` | — | 进化指标 | ✅ 已定义 |
| `UpsertMemoryFact` | — | 写入 fact | ✅ 已定义 |
| `AppendEvolutionEvent` | — | 追加进化事件 | ✅ 已定义 |

### 2.3 仍待新增

| RPC | 路径 | 用途 |
|-----|------|------|
| `GetMemoryOverview` | `GET /v1/memories/overview` | 五层记忆总览 |
| `GetMemorySnapshot` | `GET /v1/memories/{session_id}/snapshot` | 会话记忆快照 |
| `ListCascadeProposals` | `GET /v1/memories/cascade/proposals` | 级联更新提议列表 |
| `ApproveCascadeProposal` | `POST /v1/memories/cascade/proposals/{id}/approve` | 批准级联更新 |
| `RejectCascadeProposal` | `POST /v1/memories/cascade/proposals/{id}/reject` | 拒绝级联更新 |
| `GetMemoryWorkerStatus` | `GET /v1/memories/worker/status` | MemoryWorker 运行状态 |

### 2.4 补充 Proto 定义（来自 31/38 需求）

```protobuf
service MemoryService {
  rpc GetMemoryLayers(GetMemoryLayersRequest) returns (GetMemoryLayersResponse) {
    option (google.api.http) = { get: "/v1/memories/layers" };
  }
  rpc SearchMemories(SearchMemoriesRequest) returns (SearchMemoriesResponse) {
    option (google.api.http) = { post: "/v1/memories/search" body: "*" };
  }
  rpc ListMemoryFacts(ListMemoryFactsRequest) returns (ListMemoryFactsResponse) {
    option (google.api.http) = { get: "/v1/memories/facts" };
  }
  rpc UpdateMemoryFact(UpdateMemoryFactRequest) returns (MemoryFact) {
    option (google.api.http) = { patch: "/v1/memories/facts/{id}" body: "*" };
  }
  rpc DeleteMemoryFact(DeleteMemoryFactRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/memories/facts/{id}" };
  }
  rpc ConfirmMemoryFact(ConfirmMemoryFactRequest) returns (MemoryFact) {
    option (google.api.http) = { post: "/v1/memories/facts/{id}/confirm" };
  }
  rpc RejectMemoryFact(RejectMemoryFactRequest) returns (MemoryFact) {
    option (google.api.http) = { post: "/v1/memories/facts/{id}/reject" };
  }
  rpc ListMemoryConflicts(ListMemoryConflictsRequest) returns (ListMemoryConflictsResponse) {
    option (google.api.http) = { get: "/v1/memories/conflicts" };
  }
  rpc ResolveMemoryConflict(ResolveMemoryConflictRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/memories/conflicts/{id}/resolve" body: "*" };
  }
  rpc ListMemoryEpisodes(ListMemoryEpisodesRequest) returns (ListMemoryEpisodesResponse) {
    option (google.api.http) = { get: "/v1/memories/episodes" };
  }
  rpc ConsolidateEpisode(ConsolidateEpisodeRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/memories/episodes/{id}/consolidate" };
  }
  rpc GetMemoryOverview(GetMemoryOverviewRequest) returns (MemoryOverview) {
    option (google.api.http) = { get: "/v1/memories/overview" };
  }
  rpc GetL0Snapshot(GetL0SnapshotRequest) returns (L0Snapshot) {
    option (google.api.http) = { get: "/v1/memories/sessions/{session_id}/l0-snapshot" };
  }
  rpc ListEvolutionProposals(ListEvolutionProposalsRequest) returns (ListEvolutionProposalsResponse) {
    option (google.api.http) = { get: "/v1/memories/evolution/proposals" };
  }
  rpc ApproveEvolutionProposal(ApproveEvolutionProposalRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/memories/evolution/proposals/{id}/approve" };
  }
  rpc RejectEvolutionProposal(RejectEvolutionProposalRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/memories/evolution/proposals/{id}/reject" };
  }
}

message GetMemoryLayersRequest {
  string agent_id = 1;
  string session_id = 2;
}

message GetMemoryLayersResponse {
  L0ContextLayer l0 = 1;
  L1WorkingLayer l1 = 2;
  L2EpisodicLayer l2 = 3;
  L3SemanticLayer l3 = 4;
  L4PersistentLayer l4 = 5;
}

message L0ContextLayer {
  int32 context_window_tokens = 1;
  int32 used_tokens = 2;
  float used_ratio = 3;
  string truncate_strategy = 4;
  repeated string warning_codes = 5;
  repeated SegmentInfo segments = 6;
}

message SegmentInfo {
  string name = 1;
  int32 token_estimate = 2;
  string source = 3;
  bool truncated = 4;
}

message L1WorkingLayer {
  repeated WorkingTask tasks = 1;
  int32 total_budget_tokens = 2;
  int32 used_tokens = 3;
}

message WorkingTask {
  string id = 1;
  string title = 2;
  string status = 3;
  repeated WorkingField fields = 4;
  int32 token_estimate = 5;
  string updated_at = 6;
}

message WorkingField {
  string key = 1;
  string value = 2;
  string source = 3;
  bool pinned = 4;
  int32 revision = 5;
  int32 token_estimate = 6;
}

message L2EpisodicLayer {
  repeated EpisodeInfo episodes = 1;
  int32 pending_consolidation_count = 2;
}

message EpisodeInfo {
  string id = 1;
  string title = 2;
  string kind = 3;
  string outcome = 4;
  float importance = 5;
  float confidence = 6;
  string consolidation_status = 7;
  string created_at = 8;
}

message L3SemanticLayer {
  repeated MemoryFact facts = 1;
  int32 total_count = 2;
  int32 conflict_count = 3;
  float avg_confidence = 4;
}

message L4PersistentLayer {
  repeated EntityInfo entities = 1;
  repeated RelationInfo relations = 2;
  IdentityInfo identity = 3;
  StrategyInfo strategy = 4;
  repeated EvolutionProposal proposals = 5;
}

message EntityInfo {
  string id = 1;
  string name = 2;
  string type = 3;
  float importance = 4;
  int32 relation_count = 5;
}

message RelationInfo {
  string id = 1;
  string source_entity_id = 2;
  string target_entity_id = 3;
  string relation_type = 4;
  float weight = 5;
}

message IdentityInfo {
  string persona = 1;
  repeated string values = 2;
  string tone = 3;
  repeated string domains = 4;
}

message StrategyInfo {
  float exploration = 1;
  float conciseness = 2;
  float caution = 3;
  float delegation = 4;
}

message EvolutionProposal {
  string id = 1;
  string target_field = 2;
  string current_value = 3;
  string proposed_value = 4;
  string rationale = 5;
  string risk_level = 6;
  string status = 7;
  string expires_at = 8;
  string created_at = 9;
}

message SearchMemoriesRequest {
  string agent_id = 1;
  string query = 2;
  int32 top_k = 3;
  float min_score = 4;
  repeated string scopes = 5;
  string layer = 6;
}

message SearchMemoriesResponse {
  repeated MemorySearchResult results = 1;
}

message MemorySearchResult {
  string id = 1;
  string content = 2;
  float score = 3;
  string layer = 4;
  string scope = 5;
  string source = 6;
  string created_at = 7;
}

message MemoryFact {
  string id = 1;
  string statement = 2;
  string kind = 3;
  string scope = 4;
  float confidence = 5;
  float importance = 6;
  int32 hit_count = 7;
  repeated string tags = 8;
  string source = 9;
  string status = 10;
  string created_at = 11;
  string updated_at = 12;
}

message ListMemoryFactsRequest {
  string agent_id = 1;
  string scope = 2;
  string kind = 3;
  string status = 4;
  int32 page_size = 5;
  string page_token = 6;
}

message ListMemoryFactsResponse {
  repeated MemoryFact facts = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}

message UpdateMemoryFactRequest {
  string id = 1;
  string statement = 2;
  repeated string tags = 3;
  string scope = 4;
}

message DeleteMemoryFactRequest {
  string id = 1;
}

message ConfirmMemoryFactRequest {
  string id = 1;
}

message RejectMemoryFactRequest {
  string id = 1;
  string reason = 2;
}

message ListMemoryConflictsRequest {
  string agent_id = 1;
  string status = 2;
  int32 page_size = 3;
}

message ListMemoryConflictsResponse {
  repeated MemoryConflict conflicts = 1;
}

message MemoryConflict {
  string id = 1;
  repeated string fact_ids = 2;
  string description = 3;
  string status = 4;
  string created_at = 5;
}

message ResolveMemoryConflictRequest {
  string id = 1;
  string action = 2;
  string winner_fact_id = 3;
  string merged_statement = 4;
}

message ListMemoryEpisodesRequest {
  string session_id = 1;
  string agent_id = 2;
  string consolidation_status = 3;
  int32 page_size = 4;
}

message ListMemoryEpisodesResponse {
  repeated EpisodeInfo episodes = 1;
  int32 total_count = 2;
}

message ConsolidateEpisodeRequest {
  string id = 1;
}

message GetMemoryOverviewRequest {
  string agent_id = 1;
  string scope = 2;
}

message MemoryOverview {
  float context_used_ratio = 1;
  int32 active_working_tasks = 2;
  int32 pending_episodes = 3;
  int32 active_facts = 4;
  int32 open_conflicts = 5;
  int32 pending_proposals = 6;
  float recall_hit_rate = 7;
  repeated MemorySearchResult recent_injected = 8;
}

message GetL0SnapshotRequest {
  string session_id = 1;
  int32 limit = 2;
}

message L0Snapshot {
  string id = 1;
  string session_id = 2;
  int32 context_window_tokens = 3;
  int32 prompt_token_estimate = 4;
  float used_ratio = 5;
  string segments_json = 6;
  repeated string warning_codes = 7;
  string created_at = 8;
}

message ListEvolutionProposalsRequest {
  string agent_id = 1;
  string status = 2;
  int32 page_size = 3;
}

message ListEvolutionProposalsResponse {
  repeated EvolutionProposal proposals = 1;
}

message ApproveEvolutionProposalRequest {
  string id = 1;
}

message RejectEvolutionProposalRequest {
  string id = 1;
  string reason = 2;
}
```

---


## 三、Biz 层

### 3.1 领域模型

```go
type Memory struct {
    ID          string
    SessionID   string
    AgentID     string
    Layer       string  // "L0"/"L1"/"L2"/"L3"/"L4"
    Type        string  // "sensory"/"working"/"episodic"/"semantic"/"persistent"
    Key         string
    Content     string
    Score       float64
    Metadata    map[string]interface{}
    CreatedAt   string
    UpdatedAt   string
}

type MemoryOverview struct {
    L0Enabled      bool
    L1Enabled      bool
    L2Enabled      bool
    L3Enabled      bool
    L4Enabled      bool
    TotalMemories  int64
    RecentActivity []MemoryActivity
}
```

### 3.2 Repo 接口

```go
type MemoryRepository interface {
    List(ctx, query) (MemoryListResult, error)
    GetByID(ctx, id) (Memory, error)
    Create(ctx, m Memory) (Memory, error)
    Delete(ctx, id) error
    Recall(ctx, agentID string, query string, topK int, minScore float64) ([]Memory, error)
    GetOverview(ctx, agentID string) (MemoryOverview, error)
}
```

### 3.3 Usecase

```go
type MemoryUsecase struct {
    repo MemoryRepository
}

func (uc *MemoryUsecase) List(ctx, query) (MemoryListResult, error)
func (uc *MemoryUsecase) Recall(ctx, agentID, query string, topK int, minScore float64) ([]Memory, error)
func (uc *MemoryUsecase) GetOverview(ctx, agentID string) (MemoryOverview, error)
```

### 3.4 记忆自动提取

```go
// internal/memory/extractor.go
type MemoryExtractor struct {
    llm      model.LLM
    memStore *sessionmemory.Store
    memRepo  biz.MemoryRepo
    embedder biz.EmbeddingService
}

func NewMemoryExtractor(llm model.LLM, store *sessionmemory.Store, repo biz.MemoryRepo, embedder biz.EmbeddingService) *MemoryExtractor

func (e *MemoryExtractor) ExtractAfterTurn(ctx context.Context, sessionID, agentID string, messages []Message) error {
    prompt := buildExtractionPrompt(messages)
    resp, err := e.llm.Generate(ctx, prompt)
    if err != nil {
        return err
    }
    extractions := parseExtractions(resp)
    for _, ext := range extractions {
        switch ext.Layer {
        case "L2":
            e.memStore.UpsertEventEntity(ctx, sessionmemory.EventEntityParams{
                ScopeType:  "episodic",
                ScopeID:    sessionID,
                UserID:     agentID,
                EntityType: "episode",
                Name:       ext.Title,
                Description: ext.Content,
                Importance: ext.Importance,
            })
        case "L3":
            vec, _ := e.embedder.Embed(ctx, ext.Content)
            e.memRepo.Insert(ctx, &biz.AgentMemory{
                AgentID:   agentID,
                Content:   ext.Content,
                Embedding: vec,
            })
        }
    }
    return nil
}

type ExtractionResult struct {
    Layer      string
    Title      string
    Content    string
    Importance float64
    Kind       string
}

func buildExtractionPrompt(messages []Message) string
func parseExtractions(resp string) []ExtractionResult
```

### 3.5 记忆检索增强

```go
// internal/memory/retriever.go
type MemoryRetriever struct {
    memSvc   trpcmemory.Service
    memRepo  biz.MemoryRepo
    embedder biz.EmbeddingService
}

func NewMemoryRetriever(svc trpcmemory.Service, repo biz.MemoryRepo, embedder biz.EmbeddingService) *MemoryRetriever

func (r *MemoryRetriever) Retrieve(ctx context.Context, agentID, query string, topK int, minScore float64) ([]*biz.AgentMemory, error) {
    vec, err := r.embedder.Embed(ctx, query)
    if err != nil {
        return nil, err
    }
    results, err := r.memRepo.FindSimilar(ctx, agentID, vec, topK)
    if err != nil {
        return nil, err
    }
    var filtered []*biz.AgentMemory
    for _, m := range results {
        if m.Score >= minScore {
            filtered = append(filtered, m)
        }
    }
    return filtered, nil
}

func (r *MemoryRetriever) RetrieveAndFormat(ctx context.Context, agentID, query string, topK int, minScore float64) (string, error) {
    results, err := r.Retrieve(ctx, agentID, query, topK, minScore)
    if err != nil {
        return "", err
    }
    if len(results) == 0 {
        return "", nil
    }
    var sb strings.Builder
    sb.WriteString("[Retrieved Memories]\n")
    for i, m := range results {
        sb.WriteString(fmt.Sprintf("%d. %s (score: %.2f)\n", i+1, m.Content, m.Score))
    }
    return sb.String(), nil
}
```

### 3.6 记忆管理 Usecase

```go
// internal/biz/memory_management.go
type MemoryManagementUsecase struct {
    memRepo    MemoryRepo
    embedder   EmbeddingService
    store      *sessionmemory.Store
    extractor  *MemoryExtractor
    retriever  *MemoryRetriever
    agents     AgentRepo
}

func NewMemoryManagementUsecase(
    memRepo MemoryRepo,
    embedder EmbeddingService,
    store *sessionmemory.Store,
    extractor *MemoryExtractor,
    retriever *MemoryRetriever,
    agents AgentRepo,
) *MemoryManagementUsecase

func (uc *MemoryManagementUsecase) GetLayers(ctx, agentID, sessionID string) (*MemoryLayers, error)
func (uc *MemoryManagementUsecase) Search(ctx, agentID, query string, topK int, minScore float64) ([]*MemorySearchResult, error)
func (uc *MemoryManagementUsecase) ListFacts(ctx, agentID, scope, kind, status string, page int, pageSize int) ([]*MemoryFact, int, error)
func (uc *MemoryManagementUsecase) UpdateFact(ctx, factID, statement string, tags []string, scope string) (*MemoryFact, error)
func (uc *MemoryManagementUsecase) DeleteFact(ctx, factID string) error
func (uc *MemoryManagementUsecase) ConfirmFact(ctx, factID string) (*MemoryFact, error)
func (uc *MemoryManagementUsecase) RejectFact(ctx, factID, reason string) (*MemoryFact, error)
func (uc *MemoryManagementUsecase) ListConflicts(ctx, agentID, status string) ([]*MemoryConflict, error)
func (uc *MemoryManagementUsecase) ResolveConflict(ctx, conflictID, action, winnerID, mergedStatement string) error
func (uc *MemoryManagementUsecase) ListEpisodes(ctx, sessionID, agentID, consolidationStatus string, page int, pageSize int) ([]*Episode, int, error)
func (uc *MemoryManagementUsecase) ConsolidateEpisode(ctx, episodeID string) error
func (uc *MemoryManagementUsecase) GetOverview(ctx, agentID, scope string) (*MemoryOverview, error)
func (uc *MemoryManagementUsecase) GetL0Snapshot(ctx, sessionID string, limit int) (*L0Snapshot, error)
func (uc *MemoryManagementUsecase) ListEvolutionProposals(ctx, agentID, status string) ([]*EvolutionProposal, error)
func (uc *MemoryManagementUsecase) ApproveProposal(ctx, proposalID string) error
func (uc *MemoryManagementUsecase) RejectProposal(ctx, proposalID, reason string) error
```

### 3.7 补充领域模型

```go
type MemoryFact struct {
    ID          string
    AgentID     string
    Statement   string
    Kind        string  // "preference"/"fact"/"rule"/"experience"
    Scope       string  // "user"/"agent"/"team"/"workspace"/"global"
    Confidence  float64
    Importance  float64
    HitCount    int
    Tags        []string
    Source      string
    Status      string  // "active"/"archived"/"disputed"
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type MemoryConflict struct {
    ID          string
    AgentID     string
    FactIDs     []string
    Description string
    Status      string  // "open"/"resolved"
    Resolution  string
    CreatedAt   time.Time
}

type Episode struct {
    ID                 string
    SessionID          string
    AgentID            string
    Title              string
    Kind               string  // "task"/"decision"/"error_recovery"/"feedback"
    Outcome            string
    Importance         float64
    Confidence         float64
    ConsolidationStatus string  // "pending"/"consolidated"/"skipped"
    CreatedAt          time.Time
}

type EvolutionProposal struct {
    ID            string
    AgentID       string
    TargetField   string
    CurrentValue  string
    ProposedValue string
    Rationale     string
    RiskLevel     string  // "low"/"medium"/"high"
    Status        string  // "pending"/"approved"/"rejected"/"expired"
    ExpiresAt     time.Time
    CreatedAt     time.Time
}

type MemorySearchResult struct {
    ID        string
    Content   string
    Score     float64
    Layer     string
    Scope     string
    Source    string
    CreatedAt time.Time
}
```

---


## 四、Data 层

### 4.1 L0-L2/L4：sessionmemory.Store

文件：`internal/data/sessionmemory/`

```go
type Store struct {
    db *ent.Client
}

func (s *Store) GetRecentMessages(ctx, sessionID string, limit int) ([]Memory, error)
func (s *Store) GetWorkingState(ctx, sessionID string) (*WorkingState, error)
func (s *Store) GetEpisodes(ctx, agentID string, minImportance float64) ([]Memory, error)
func (s *Store) GetIdentity(ctx, agentID string) (*IdentityState, error)
```

### 4.2 L3：向量双写（SQLite 权威 + pgvector 可选）

| 写入路径 | 实现 | 说明 |
|----------|------|------|
| 事实权威 | `sessionmemory.UpsertFactRow` | SQLite `memory_facts` |
| 召回向量（运行时） | `sessionmemory.UpsertFactEmbedding` | `embedding_blob`；`RecallL3Facts` 读此列 |
| 可选读索引 | `data.NewMemoryFactIndexSync` | 包装 `MemoryUsecase`：一次 embed → pgvector + SQLite blob |
| Episode 对称 | `data.NewMemoryEpisodeIndexSync` | L2 `memory_episodes.embedding_blob` |

Wire：`provideFactIndexSync` 注入 AutoMemoryWorker、`MemoryAdminUsecase`、`L4CascadeUsecase`、trpc adapter。

**Recall 分数**：`RecallL2EpisodesScored` / `RecallL3FactsScored` 返回完整 breakdown；`CompositeSearchMemories` 按 `Scores.Total` 排序。

**Auto-memory 失败策略**：任一 `UpsertFactRow` 失败 → job 返回 error → 指数退避重试；episode 仅在全部 fact 成功且 `added > 0` 时写入。

**Admin 端口拆分**（渐进）：`SessionAdminStore` = `L0AdminReader` + `L1AdminReader` + `L2RecallStore` + `L3FactAdminStore` + `L4GraphAdminStore`；typed `RecallHit` 在 biz 层引入，JSON `[][]byte` 逐步替换。

文件：`internal/data/pgvector/`（可选 Postgres）

```go
type VectorStore struct {
    db *sql.DB
}

func (v *VectorStore) Search(ctx, agentID string, embedding []float64, topK int, minScore float64) ([]Memory, error)
func (v *VectorStore) Insert(ctx, m Memory, embedding []float64) error
```

### 4.3 Ent Schema

- `user_embedding_setting.go` — 嵌入模型配置

### 4.4 记忆提取持久化

```go
// internal/data/memory_extraction.go
type memoryExtractionRepo struct {
    store *sessionmemory.Store
}

func NewMemoryExtractionRepo(store *sessionmemory.Store) *memoryExtractionRepo

func (r *memoryExtractionRepo) SaveExtraction(ctx context.Context, params sessionmemory.EventEntityParams) error {
    return r.store.UpsertEventEntity(ctx, params)
}

func (r *memoryExtractionRepo) ListExtractions(ctx context.Context, scopeType, scopeID, entityType string, limit int) ([][]byte, error) {
    rows, _, err := r.store.ListEntityRows(ctx, scopeType, scopeID, "", "", entityType, "", limit, 0)
    return rows, err
}
```

### 4.5 L0 快照查询

```go
// internal/data/sessionmemory/l0_snapshot.go
func (st *Store) GetLatestL0Snapshot(ctx context.Context, sessionID string) (*L0SnapshotRow, error) {
    var row L0SnapshotRow
    err := queryOne(ctx, st.client, sqlL0Select+` WHERE session_id = ? ORDER BY created_at DESC LIMIT 1`,
        []any{sessionID},
        &row.ID, &row.SessionID, &row.RunID, &row.TurnID, &row.SpanID,
        &row.AgentID, &row.TeamID, &row.Provider, &row.Model,
        &row.ContextWindowTokens, &row.BudgetTokens, &row.RecentWindowTurns,
        &row.RecentWindowTokens, &row.SummaryTokenEstimate,
        &row.L1FieldCount, &row.L1TokenEstimate,
        &row.L3ChunkCount, &row.L3TokenEstimate,
        &row.L4PathCount, &row.L4TokenEstimate,
        &row.PromptTokenEstimate, &row.PromptTokenActual,
        &row.UsedRatio, &row.TruncateStrategy,
        &row.TruncatedMessageCount, &row.SummarizedTurnFrom, &row.SummarizedTurnTo,
        &row.SegmentsJSON, &row.WarningCodesJSON, &row.MetadataJSON, &row.CreatedAt,
    )
    return &row, err
}
```

### 4.6 冲突检测

```go
// internal/data/memory_conflict.go
type memoryConflictRepo struct {
    data *Data
}

func NewMemoryConflictRepo(d *Data) *memoryConflictRepo

func (r *memoryConflictRepo) FindConflicts(ctx context.Context, agentID, status string) ([]*biz.MemoryConflict, error) {
    rows, err := r.data.Ent().QueryContext(ctx,
        `SELECT id, agent_id, fact_ids, description, status, resolution, created_at
         FROM memory_conflicts WHERE agent_id = ? AND status = ? ORDER BY created_at DESC`,
        agentID, status)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*biz.MemoryConflict
    for rows.Next() {
        var c biz.MemoryConflict
        var factIDsJSON string
        if err := rows.Scan(&c.ID, &c.AgentID, &factIDsJSON, &c.Description, &c.Status, &c.Resolution, &c.CreatedAt); err != nil {
            continue
        }
        json.Unmarshal([]byte(factIDsJSON), &c.FactIDs)
        out = append(out, &c)
    }
    return out, nil
}
```

### 4.7 Ent Schema 新增

```go
// internal/data/ent/schema/memory_fact.go
type MemoryFact struct {
    ent.Schema
}

func (MemoryFact) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("agent_id").NotEmpty(),
        field.Text("statement").NotEmpty(),
        field.String("kind").Default("fact"),
        field.String("scope").Default("agent"),
        field.Float("confidence").Default(1.0),
        field.Float("importance").Default(0.5),
        field.Int("hit_count").Default(0),
        field.JSON("tags", []string{}).Optional(),
        field.String("source").Default("auto"),
        field.String("status").Default("active"),
        field.String("created_at").Default(time.NowString),
        field.String("updated_at").Default(time.NowString),
    }
}

// internal/data/ent/schema/memory_conflict.go
type MemoryConflict struct {
    ent.Schema
}

func (MemoryConflict) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("agent_id").NotEmpty(),
        field.JSON("fact_ids", []string{}),
        field.Text("description"),
        field.String("status").Default("open"),
        field.Text("resolution").Default(""),
        field.String("created_at").Default(time.NowString),
    }
}

// internal/data/ent/schema/evolution_proposal.go
type EvolutionProposal struct {
    ent.Schema
}

func (EvolutionProposal) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("agent_id").NotEmpty(),
        field.String("target_field").NotEmpty(),
        field.Text("current_value"),
        field.Text("proposed_value"),
        field.Text("rationale"),
        field.String("risk_level").Default("low"),
        field.String("status").Default("pending"),
        field.String("expires_at").Default(""),
        field.String("created_at").Default(time.NowString),
    }
}
```

---


## 五、运行时层

### 5.1 Runner MemoryService（实现真相）

```go
// internal/memory/trpc/sqlite_adapter.go
func NewSQLiteMemoryService(store *sessionmemory.Store) trpcmemory.Service

// internal/biz/memory_runtime_set.go
type RuntimeSet struct {
    TRPC  trpcmemory.Service   // Runner load_memory / preload_memory
    Admin SessionAdminStore    // MemoryAdminUsecase / memory/v1 API
}
```

Wire：`PersistenceSet.Memory` → `agent.BuildTRPCLLMAgent`；**禁止**在 `internal/service` import `pkg/trpc-agent-go`。

### 5.2 记忆注入

```go
// internal/agent/trpc_build.go
func WithMemoryInjection(ctx, ag, deps) llmagent.Option
```

根据 `settings` 决定注入哪些层：
- `l0_inject_l1` → 注入 L1 工作记忆
- `l0_inject_l3` → 注入 L3 语义检索
- `l0_inject_l4` → 注入 L4 身份/策略

---


## 六、Service 层

```go
func (s *MemoryService) ListMemories(ctx, req) (*ListMemoriesResponse, error)
func (s *MemoryService) RecallMemories(ctx, req) (*RecallMemoriesResponse, error)
func (s *MemoryService) GetMemoryOverview(ctx, req) (*MemoryOverviewResponse, error)

func (s *MemoryService) GetMemoryLayers(ctx context.Context, req *GetMemoryLayersRequest) (*GetMemoryLayersResponse, error) {
    layers, err := s.uc.GetLayers(ctx, req.AgentId, req.SessionId)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoLayers(layers), nil
}

func (s *MemoryService) SearchMemories(ctx context.Context, req *SearchMemoriesRequest) (*SearchMemoriesResponse, error) {
    results, err := s.uc.Search(ctx, req.AgentId, req.Query, int(req.TopK), req.MinScore)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    resp := &SearchMemoriesResponse{}
    for _, r := range results {
        resp.Results = append(resp.Results, toProtoSearchResult(r))
    }
    return resp, nil
}

func (s *MemoryService) ListMemoryFacts(ctx context.Context, req *ListMemoryFactsRequest) (*ListMemoryFactsResponse, error)
func (s *MemoryService) UpdateMemoryFact(ctx context.Context, req *UpdateMemoryFactRequest) (*MemoryFact, error)
func (s *MemoryService) DeleteMemoryFact(ctx context.Context, req *DeleteMemoryFactRequest) (*emptypb.Empty, error)
func (s *MemoryService) ConfirmMemoryFact(ctx context.Context, req *ConfirmMemoryFactRequest) (*MemoryFact, error)
func (s *MemoryService) RejectMemoryFact(ctx context.Context, req *RejectMemoryFactRequest) (*MemoryFact, error)
func (s *MemoryService) ListMemoryConflicts(ctx context.Context, req *ListMemoryConflictsRequest) (*ListMemoryConflictsResponse, error)
func (s *MemoryService) ResolveMemoryConflict(ctx context.Context, req *ResolveMemoryConflictRequest) (*emptypb.Empty, error)
func (s *MemoryService) ListMemoryEpisodes(ctx context.Context, req *ListMemoryEpisodesRequest) (*ListMemoryEpisodesResponse, error)
func (s *MemoryService) ConsolidateEpisode(ctx context.Context, req *ConsolidateEpisodeRequest) (*emptypb.Empty, error)
func (s *MemoryService) GetL0Snapshot(ctx context.Context, req *GetL0SnapshotRequest) (*L0Snapshot, error)
func (s *MemoryService) ListEvolutionProposals(ctx context.Context, req *ListEvolutionProposalsRequest) (*ListEvolutionProposalsResponse, error)
func (s *MemoryService) ApproveEvolutionProposal(ctx context.Context, req *ApproveEvolutionProposalRequest) (*emptypb.Empty, error)
func (s *MemoryService) RejectEvolutionProposal(ctx context.Context, req *RejectEvolutionProposalRequest) (*emptypb.Empty, error)
```

---


## 七、Wire 注入

已有：
```
data.ProviderSet → NewSessionMemoryStore
biz.ProviderSet → (MemoryUsecase 待创建)
service.ProviderSet → NewMemoryService
```

待新增：
```
data.ProviderSet → NewMemoryWorker, NewMemoryExtractionRepo, NewMemoryConflictRepo
biz.ProviderSet → NewMemoryManagementUsecase
memory.ProviderSet → NewMemoryExtractor, NewMemoryRetriever
service.ProviderSet → NewMemoryService
```

---


## 八、MemoryWorker 设计（EP-MEM-01 / EP-MEM-02）

> 来源：`_deprecated/需求/随心记.md`「记忆管家 Memory-Agent」需求，经可行性分析后调整为后台 goroutine 方案。

### 8.1 定位

MemoryWorker 是后台运行的记忆管理 goroutine（非独立进程），可配置开启和关闭。核心目标：

1. **认知一致性**：当某个记忆块变动时，自动复盘关联记忆块是否需要更新
2. **异步提取**：Turn 完成后自动从对话中提取 fact / episode / entity
3. **冲突检测**：新 fact 与现有 fact 矛盾时标记冲突
4. **级联更新**：实体属性变更时，沿图谱关系传播更新

### 8.2 架构

```
┌─────────────────────────────────────────────────────────┐
│                    Aranea 主进程                          │
│                                                          │
│  ┌──────────────┐    EventBus     ┌──────────────────┐  │
│  │ Chat/Runner  │ ──── event ───→ │ MemoryWorker     │  │
│  │ (对话运行时)  │   turn.completed│ (safego.Go)      │  │
│  └──────────────┘                 │                  │  │
│                                   │ 1. 异步提取       │  │
│  ┌──────────────┐                 │ 2. 冲突检测       │  │
│  │ CronRunner   │ ──── tick ────→ │ 3. 级联更新检查   │  │
│  │ (定时任务)    │   30min ticker  │ 4. 巩固管道       │  │
│  └──────────────┘                 │ 5. Proposal 管理  │  │
│                                   └──────────────────┘  │
│                                          │               │
│                                    ┌─────┴──────┐       │
│                                    │ L3 Facts   │       │
│                                    │ L4 Graph   │       │
│                                    └────────────┘       │
└─────────────────────────────────────────────────────────┘
```

### 8.3 Biz 层

```go
type MemoryWorker struct {
    extractor   *MemoryExtractor
    retriever   *MemoryRetriever
    graphRepo   MemoryGraphRepo
    factRepo    MemoryFactRepo
    proposalRepo EvolutionProposalRepo
    eventBus    *event.Bus
    settings    MemoryWorkerSettings
}

func NewMemoryWorker(
    extractor *MemoryExtractor,
    retriever *MemoryRetriever,
    graphRepo MemoryGraphRepo,
    factRepo MemoryFactRepo,
    proposalRepo EvolutionProposalRepo,
    eventBus *event.Bus,
    settings MemoryWorkerSettings,
) *MemoryWorker

func (w *MemoryWorker) Start(ctx context.Context)
func (w *MemoryWorker) Stop()
func (w *MemoryWorker) OnTurnCompleted(ctx context.Context, event TurnCompletedEvent)
func (w *MemoryWorker) RunConsolidation(ctx context.Context)
func (w *MemoryWorker) RunCascadeCheck(ctx context.Context, entityID string, changedAttribute string)
func (w *MemoryWorker) GetStatus() MemoryWorkerStatus
```

### 8.4 级联更新流程

```
实体属性变更（如 work_location: 北京 → 纽约）
        ↓
MemoryWorker.RunCascadeCheck(entityID, "work_location")
        ↓
1. 从 L4 图谱 BFS 查找关联实体（≤ max_hops 跳）
   - 交通方式 ← depends_on ← work_location
   - 天气偏好 ← depends_on ← work_location
   - 时区设置 ← depends_on ← work_location
        ↓
2. 对每个关联实体，检查是否需要更新
   - 调用 LLM 判断：属性变更是否影响该关联实体
   - 生成 CascadeCheckProposal
        ↓
3. Proposal 进入审核队列
   - proposal 模式：等待用户/Critic 确认
   - auto 模式：自动应用（高风险，默认关闭）
        ↓
4. 审核通过后应用更新
   - 更新关联 L3 fact（superseded_by 指向新 fact）
   - 更新关联 L4 实体属性
   - 记录 EvolutionEvent（含变更前后值，可回滚）
```

### 8.5 Data 层

```go
type memoryGraphRepo struct {
    db *ent.Client
}

func (r *memoryGraphRepo) CreateEntity(ctx, entity *MemoryEntity) error
func (r *memoryGraphRepo) GetEntity(ctx, id string) (*MemoryEntity, error)
func (r *memoryGraphRepo) UpdateEntityAttribute(ctx, id, attribute, value string) error
func (r *memoryGraphRepo) BFSNeighbors(ctx, entityID string, maxHops int) ([]*MemoryEntity, []*MemoryRelation, error)
func (r *memoryGraphRepo) CreateRelation(ctx, relation *MemoryRelation) error
func (r *memoryGraphRepo) GetRelationsByEntity(ctx, entityID string) ([]*MemoryRelation, error)
```

### 8.6 配置

通过 `agent_runtime_settings` 扩展：

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_extract_mode TEXT NOT NULL DEFAULT 'auto';
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_cascade_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_cascade_max_hops INTEGER NOT NULL DEFAULT 2;
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_cascade_mode TEXT NOT NULL DEFAULT 'proposal';
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_batch_size INTEGER NOT NULL DEFAULT 10;
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_consolidation_interval_minutes INTEGER NOT NULL DEFAULT 30;
```

### 8.7 前端补充

MemoryWorker 相关前端组件：

| 组件 | 说明 |
|------|------|
| `MemoryWorkerStatusCard.vue` | MemoryWorker 运行状态卡片（运行中/暂停/错误） |
| `MemoryCascadeProposalCard.vue` | 级联更新提议卡片（实体变更→关联影响→diff） |
| `MemoryCascadeReviewDialog.vue` | 级联更新审核对话框（批量批准/拒绝） |

TypeScript 类型补充：

```typescript
export interface MemoryWorkerStatus {
  enabled: boolean
  running: boolean
  extractMode: 'auto' | 'manual' | 'hybrid'
  cascadeEnabled: boolean
  cascadeMaxHops: number
  cascadeMode: 'proposal' | 'auto'
  lastExtractionAt: string
  lastConsolidationAt: string
  pendingCascadeProposals: number
  extractionCount24h: number
  cascadeCount24h: number
}

export interface CascadeProposal {
  id: string
  agentId: string
  triggerEntityId: string
  triggerEntityName: string
  triggerAttribute: string
  oldValue: string
  newValue: string
  affectedEntities: CascadeAffectedEntity[]
  status: 'pending' | 'approved' | 'rejected' | 'expired' | 'applied'
  riskLevel: 'low' | 'medium' | 'high'
  rationale: string
  createdAt: string
  expiresAt: string
}

export interface CascadeAffectedEntity {
  entityId: string
  entityName: string
  entityType: string
  relationType: string
  hops: number
  suggestedUpdate: string
  currentFactIds: string[]
}
```

API 补充：

```typescript
export async function getMemoryWorkerStatus(agentId: string): Promise<MemoryWorkerStatus>
export async function listCascadeProposals(agentId: string, status?: string): Promise<CascadeProposal[]>
export async function approveCascadeProposal(id: string): Promise<void>
export async function rejectCascadeProposal(id: string, reason?: string): Promise<void>
```

路由补充：

```typescript
{ path: '/memory/worker', component: MemoryWorkerPage, name: 'MemoryWorker' },
```

记忆中心总览页补充——在 `MemoryCenterPage` 健康卡片区域新增：

| KPI | 组件 | 颜色语义 |
|-----|------|----------|
| Worker Status | `QBadge` | 运行中=绿 / 暂停=灰 / 错误=红 |
| Cascade Proposals | `QBadge` | 0=绿 / >0=黄 |
| Extraction 24h | `QBadge` | 蓝色 |

在待处理事项区域新增：

- 待审核级联更新提议
- MemoryWorker 错误/重试

---


## 九、前端实现设计

> UX 需求（页面目标、交互规范、验收标准）见 [`memory.md`](./memory.md)，本节聚焦实现层：组件文件结构、组件设计、TypeScript 类型、API 函数、路由配置。

### 9.1 文件结构

```
web/src/features/memory/
├── api.ts
├── types.ts
├── composables/
│   ├── useMemoryOverview.ts
│   ├── useMemorySearch.ts
│   ├── useMemoryFacts.ts
│   └── useMemoryLayers.ts
└── components/
    ├── MemoryCenterPage.vue         ← 记忆中心主页
    ├── MemoryOverviewCard.vue       ← 总览卡片
    ├── MemoryHealthDashboard.vue    ← 健康仪表盘
    ├── MemoryKnowledgePage.vue      ← 知识库（L3 facts）
    ├── MemoryFactTable.vue          ← Fact 列表
    ├── MemoryFactDetailDrawer.vue   ← Fact 详情抽屉
    ├── MemoryConflictCard.vue       ← 冲突卡片
    ├── MemoryConflictDialog.vue     ← 冲突解决对话框
    ├── MemorySessionsPage.vue       ← 会话记忆（L0/L1/L2）
    ├── MemoryContextPanel.vue       ← L0 上下文窗口
    ├── MemoryWorkingPanel.vue       ← L1 工作记忆
    ├── MemoryTimelinePanel.vue      ← L2 事件时间线
    ├── MemoryEpisodeTable.vue       ← Episode 列表
    ├── MemoryGraphPage.vue          ← 知识图谱（L4 graph）
    ├── MemoryEvolutionPage.vue      ← Agent 进化（L4 evolution）
    ├── MemoryProposalCard.vue       ← 进化提议卡片
    ├── MemoryEvolutionLog.vue       ← 进化日志
    ├── MemoryDebugPage.vue          ← 调试工具
    ├── MemoryPromptPreview.vue      ← Prompt 预览
    ├── MemoryRecallTester.vue       ← Recall 测试器
    ├── MemoryLayerConfig.vue        ← 各层记忆配置面板
    └── MemorySettingsTab.vue        ← Agent 记忆设置 Tab
```

### 9.2 核心组件设计

**MemoryCenterPage.vue**：记忆中心主页

| 区域 | 组件 | 说明 |
|------|------|------|
| 顶部选择器 | `QSelect` + `QBtnToggle` | Agent 选择 + scope 切换 |
| 健康卡片 | `MemoryHealthDashboard` | 7 个 KPI 指标 |
| 记忆流向图 | `QCard` + SVG | L0←L1/L2/L3/L4 流程图 |
| 最近影响 | `QList` | 最近 10 条注入 prompt 的记忆 |
| 待处理事项 | `QList` | 冲突/PII/巩固失败/proposal |

**MemoryHealthDashboard.vue**：健康仪表盘

| KPI | 组件 | 颜色语义 |
|-----|------|----------|
| Context Used | `QCircularProgress` | <70% 绿 / 70-90% 橙 / >90% 红 |
| Active Working Tasks | `QBadge` | 蓝色 |
| Pending Episodes | `QBadge` | 黄色 |
| Active Facts | `QBadge` | 蓝色 |
| Open Conflicts | `QBadge` | 红色 |
| Pending Proposals | `QBadge` | 黄色 |
| Recall Hit Rate | `QCircularProgress` | 绿/黄/红 |

**MemoryKnowledgePage.vue**：知识库页面

| 区域 | 组件 | 说明 |
|------|------|------|
| 顶栏 | `QChip` + `QBtn` | scope 切换 + 新增 Fact |
| 统计 | `QCard` | 总数/活跃/归档/冲突/平均置信度 |
| 筛选 | `QSelect` + `QInput` | kind/tags/status/scope/关键字 |
| 表格 | `MemoryFactTable` | statement/kind/scope/confidence/hit_count |
| 详情 | `MemoryFactDetailDrawer` | 5 个 Tab：内容/证据/使用/反馈/版本 |
| 批量操作 | `QBtnGroup` | 归档/删除/重建 embedding/导出 |

**MemoryFactDetailDrawer.vue**：Fact 详情抽屉

| Tab | 组件 | 说明 |
|-----|------|------|
| 内容 | `QForm` | statement/details/tags/kind/scope |
| 证据 | `QList` | 来源 episode/session/message |
| 使用 | `QTable` | 最近召回记录、hit_count |
| 反馈 | `QTimeline` | confirm/reject/refine 时间线 |
| 版本 | `QCard` + diff | fact_versions、回滚按钮 |

**MemoryContextPanel.vue**：L0 上下文窗口

| 区域 | 组件 | 说明 |
|------|------|------|
| Token 仪表 | `QCircularProgress` | used ratio / max ratio |
| Prompt 分段瀑布 | `QExpansionItem` 列表 | system/skill/L1/L2/L3/L4/summary/history |
| 装配快照 | `QSelect` | 最近 20 次快照选择 |
| 操作 | `QBtn` | 重新预览/复制 prompt/调整设置 |

**MemoryTimelinePanel.vue**：L2 事件时间线

| 区域 | 组件 | 说明 |
|------|------|------|
| KPI | `QBadge` 行 | 消息数/模型调用/工具调用/失败数/tokens/成本 |
| 筛选 | `QSelect` + `QInput` | 类型/actor/状态/关键字 |
| 时间线 | `QTimeline` | turn 聚合卡片 |
| 标记菜单 | `QBtnDropdown` | 标星/巩固/复盘/好范例/坏范例/遗忘 |

**MemoryGraphPage.vue**：知识图谱页面

| 区域 | 组件 | 说明 |
|------|------|------|
| Sidebar | `QDrawer` | scope/entity_type/keyword 筛选 |
| 主图 | `@vue-flow/core` 或 D3.js | 力导图；节点大小=importance |
| 右侧详情 | `QDrawer` | entity 属性/relations/facts/versions |
| 降级视图 | `QTable` | 实体表格/邻居列表/关系表格 |

**MemoryEvolutionPage.vue**：Agent 进化页面

| 区域 | 组件 | 说明 |
|------|------|------|
| Identity 面板 | `QCard` + `QForm` | persona/values/tone/domains |
| Strategy 面板 | `QSlider` 行 | exploration/conciseness/caution/delegation |
| Proposal 审核 | `MemoryProposalCard` 列表 | 当前值 vs 建议值 diff + 操作按钮 |
| Evolution Log | `QTimeline` | 所有 EvolutionEvent |

**MemoryProposalCard.vue**：进化提议卡片

| 区域 | 组件 | 说明 |
|------|------|------|
| 目标 | `QLabel` | target_field |
| Diff | `QCard` | 当前值 vs 建议值（红绿 diff） |
| 理由 | `QExpansionItem` | rationale + 证据 |
| 风险 | `QBadge` | low 绿 / medium 黄 / high 红 |
| 操作 | `QBtn` | 批准/拒绝/延后 |

**MemoryLayerConfig.vue**：各层记忆配置面板

| 层 | 控件 | 绑定字段 | 说明 |
|---|------|----------|------|
| L0 | `QToggle` + `QInput` | `l0_inject_l1` + `l0_recent_window_turns` | 启用注入 + 窗口轮数 |
| L0 | `QInput` + `QSelect` | `l0_recent_window_tokens` + `l0_truncate_strategy` | 窗口 tokens + 裁剪策略 |
| L0 | `QSlider` | `l0_summary_threshold` | 摘要触发阈值 |
| L0 | `QToggle` × 3 | `l0_inject_l1`/`l0_inject_l3`/`l0_inject_l4` | 注入各层开关 |
| L1 | `QToggle` | `l1_enabled` | 启用工作记忆 |
| L1 | `QInput` | `l1_budget_tokens` + `l1_field_max_tokens` | 预算 + 单字段上限 |
| L2 | `QToggle` + `QSlider` | `l2_episode_enabled` + `l2_episode_min_importance` | 启用 + 重要性阈值 |
| L2 | `QToggle` + `QSelect` | `l2_index_enabled` + `l2_index_embedding_model` | 启用索引 + 嵌入模型 |
| L3 | `QToggle` + `QInput` | `memory_enabled` + `memory_max_results` | 启用 + topK |
| L3 | `QSlider` | `memory_min_score` | 最低分数 |
| L4 | `QToggle` | `evolution_self_evolve` | 启用自我演化 |

**MemorySettingsTab.vue**：Agent 记忆设置 Tab

| 区域 | 组件 | 说明 |
|------|------|------|
| 总开关 | `QToggle` | 记忆总开关 |
| 模式选择 | `QBtnToggle` | 轻量/标准/深度 |
| 预计影响 | `QBadge` | 低/中/高 |
| 隐私级别 | `QSelect` | 严格/标准/宽松 |
| 各层配置 | `MemoryLayerConfig` | L0-L4 各层详细配置 |
| 配置防呆 | `QBanner` | 自动禁用冲突配置 + 风险提示 |

### 9.3 TypeScript 类型定义

```typescript
// web/src/features/memory/types.ts
export interface MemoryOverview {
  contextUsedRatio: number
  activeWorkingTasks: number
  pendingEpisodes: number
  activeFacts: number
  openConflicts: number
  pendingProposals: number
  recallHitRate: number
  recentInjected: MemorySearchResult[]
}

export interface MemoryFact {
  id: string
  statement: string
  kind: 'preference' | 'fact' | 'rule' | 'experience'
  scope: 'user' | 'agent' | 'team' | 'workspace' | 'global'
  confidence: number
  importance: number
  hitCount: number
  tags: string[]
  source: string
  status: 'active' | 'archived' | 'disputed'
  createdAt: string
  updatedAt: string
}

export interface MemoryConflict {
  id: string
  factIds: string[]
  description: string
  status: 'open' | 'resolved'
  createdAt: string
}

export interface Episode {
  id: string
  sessionId: string
  agentId: string
  title: string
  kind: 'task' | 'decision' | 'error_recovery' | 'feedback'
  outcome: string
  importance: number
  confidence: number
  consolidationStatus: 'pending' | 'consolidated' | 'skipped'
  createdAt: string
}

export interface EvolutionProposal {
  id: string
  agentId: string
  targetField: string
  currentValue: string
  proposedValue: string
  rationale: string
  riskLevel: 'low' | 'medium' | 'high'
  status: 'pending' | 'approved' | 'rejected' | 'expired'
  expiresAt: string
  createdAt: string
}

export interface MemorySearchResult {
  id: string
  content: string
  score: number
  layer: string
  scope: string
  source: string
  createdAt: string
}

export interface L0Snapshot {
  id: string
  sessionId: string
  contextWindowTokens: number
  promptTokenEstimate: number
  usedRatio: number
  segmentsJson: string
  warningCodes: string[]
  createdAt: string
}

export interface FactQueryParams {
  scope?: string
  kind?: string
  status?: string
  pageSize?: number
  pageToken?: string
}

export interface UpdateFactRequest {
  statement?: string
  tags?: string[]
  scope?: string
}

export interface ResolveConflictRequest {
  action: 'keep_a' | 'keep_b' | 'merge' | 'disputed' | 'split_scope'
  winnerFactId?: string
  mergedStatement?: string
}
```

### 9.4 TypeScript API 定义

```typescript
// web/src/features/memory/api.ts
import type {
  MemoryOverview, MemoryFact, MemoryConflict, Episode,
  EvolutionProposal, L0Snapshot, MemorySearchResult,
  GetMemoryLayersResponse
} from './types'

export async function getMemoryOverview(agentId: string, scope?: string): Promise<MemoryOverview>
export async function getMemoryLayers(agentId: string, sessionId?: string): Promise<GetMemoryLayersResponse>
export async function searchMemories(agentId: string, query: string, topK?: number, minScore?: number): Promise<MemorySearchResult[]>
export async function listMemoryFacts(agentId: string, params: FactQueryParams): Promise<{ facts: MemoryFact[]; totalCount: number }>
export async function updateMemoryFact(id: string, req: UpdateFactRequest): Promise<MemoryFact>
export async function deleteMemoryFact(id: string): Promise<void>
export async function confirmMemoryFact(id: string): Promise<MemoryFact>
export async function rejectMemoryFact(id: string, reason?: string): Promise<MemoryFact>
export async function listMemoryConflicts(agentId: string, status?: string): Promise<MemoryConflict[]>
export async function resolveMemoryConflict(id: string, req: ResolveConflictRequest): Promise<void>
export async function listMemoryEpisodes(sessionId: string, params?: EpisodeQueryParams): Promise<{ episodes: Episode[]; totalCount: number }>
export async function consolidateEpisode(id: string): Promise<void>
export async function getL0Snapshot(sessionId: string, limit?: number): Promise<L0Snapshot>
export async function listEvolutionProposals(agentId: string, status?: string): Promise<EvolutionProposal[]>
export async function approveEvolutionProposal(id: string): Promise<void>
export async function rejectEvolutionProposal(id: string, reason?: string): Promise<void>
```

### 9.5 路由配置

```typescript
const memoryRoutes = [
  { path: '/memory', component: MemoryCenterPage, name: 'MemoryCenter' },
  { path: '/memory/knowledge', component: MemoryKnowledgePage, name: 'MemoryKnowledge' },
  { path: '/memory/sessions', component: MemorySessionsPage, name: 'MemorySessions' },
  { path: '/memory/graph', component: MemoryGraphPage, name: 'MemoryGraph' },
  { path: '/memory/evolution', component: MemoryEvolutionPage, name: 'MemoryEvolution' },
  { path: '/memory/debug', component: MemoryDebugPage, name: 'MemoryDebug' },
  { path: '/memory/worker', component: MemoryWorkerPage, name: 'MemoryWorker' },
]
```

---

## 十、记忆中心重设计（2026-07-25）：层级全景 + 跨层关联图谱

> 需求见 `memory.md` §21。本节为架构、API 契约与前端组件设计。高保真原型：`.superpowers/brainstorm/vc-20260725051916/content/redesign-overview.html` / `redesign-graph.html`。

### 10.1 信息架构

记忆中心 Tab 从现状（总览 / 知识库 / 会话 / 图谱与进化 / 调试 / Worker）重组为 4 个：

| 新 Tab | 来源 | 说明 |
|--------|------|------|
| 层级全景 | 新增（替代总览） | 五层卡片管道 + 需要关注 + 最近记忆动态 |
| 关联图谱 | 升级 MemoryGraphExplorer | 跨层统一图（实体/事实/情景三类节点） |
| 记忆浏览 | 整合 MemoryKnowledgePanel + 新增 L2 情景时间线 | facts 表格与 episodes 时间线，支持层级过滤与图谱跳转定位 |
| 治理 | 归并 cascade / evolution / settings / worker 面板 | 审批、演进、平台设置、Worker 状态集中 |

页面 Hero 区保留 Agent 选择器，新增全记忆搜索框与「记忆健康」总分徽标。

### 10.2 后端 API 契约

仅新增 2 个聚合端点，无新表、无 Schema 变更，数据全部来自现有 repo。

#### ① `GET /v1/memory/layer-overview?agent_id=&session_id=`

单次请求返回五层统计 + 行动项 + 动态事件流（proto：`GetMemoryLayerOverviewResponse`，`headline_json` 为 JSON 字符串，前端解析后展示）：

```json
{
  "layers": [
    {
      "layer": "L0", "item_count": 12, "today_added": 2, "recall_hits": 0,
      "health": "ok",
      "headline_json": "{\"context_usage_pct\":68,\"compress_status\":\"normal\"}"
    },
    { "layer": "L1", "item_count": 3, "today_added": 1, "recall_hits": 0,
      "health": "ok", "headline_json": "{\"active_tasks\":3,\"field_count\":24}" },
    { "layer": "L2", "item_count": 45, "today_added": 6, "recall_hits": 3,
      "health": "ok", "headline_json": "{}" },
    { "layer": "L3", "item_count": 128, "today_added": 2, "recall_hits": 11,
      "health": "warn", "headline_json": "{\"conflict_open\":2}" },
    { "layer": "L4", "item_count": 67, "today_added": 3, "recall_hits": 0,
      "health": "ok", "headline_json": "{\"relation_count\":143,\"strategy_count\":4}" }
  ],
  "action_items": [
    { "kind": "fact_conflict", "count": 2, "target_tab": "browse" },
    { "kind": "evolution_pending", "count": 1, "target_tab": "governance" },
    { "kind": "context_risk", "count": 1, "target_tab": "panorama" }
  ],
  "activity_feed": [
    { "ts": "2026-07-25T10:32:00+08:00", "kind": "fact_extracted",
      "layer_from": "L2", "layer_to": "L3", "summary": "从「季度复盘讨论」提炼出 2 条事实" }
  ]
}
```

**数据来源**（全部现有能力）：

| 字段 | 来源 |
|------|------|
| L0 item_count / context_usage | `L0SnapshotRepo.GetLatestSessionSnapshot` / `ListSessionSnapshots` |
| L1 active_tasks / field_count | 现有 L1 tasks/fields API |
| L2 item_count / today_added | `episodeRepo.ListLatestByScope` + `created_at` 聚合 |
| L3 item_count / conflict_open / recall_hits | facts 表 count（按产生方 `agent_id` 跨全部 scope 聚合，2026-08-05 起；即刻事实写 session scope、巩固事实写 user scope，scope='agent' 计数会漏算）+ `conflict_count>0` + `use_count` 聚合 |
| L4 item_count / relation_count | `memoryGraphRepo.GraphStats`（已有） |
| action_items | 上述冲突数 + cascade 待审批 + evolution pending + 上下文阈值 |
| activity_feed | 归并最近 facts / episodes / entities（按 `created_at` 倒序，limit 20） |

`recall_hits` 口径：召回次数（全量 `use_count` 求和）。`hit_count` 全库无递增代码（死计数器，恒 0），2026-07-30 起改为聚合 `use_count`（召回链路真实递增），前端标签同步改为「召回次数」；仅 L3 有召回追踪，其他层不展示该指标。三段计数（recalled/injected/cited）见 P1 规划。

#### ② `GET /v1/memory/graph/unified?agent_id=&focus=&hops=2&min_weight=&layers=L2,L3,L4`

跨层统一图，返回节点与边（proto：`GetUnifiedMemoryGraphResponse`；`meta_json` 为 JSON 字符串；统计字段平铺，无 `stats` 嵌套；空图时 `empty_reason` 给出原因）：

```json
{
  "focus": "ent_001",
  "nodes": [
    { "id": "ent_001", "layer": "L4", "kind": "entity", "label": "用户画像",
      "weight": 23, "meta_json": "{\"entity_type\":\"person\",\"confidence\":0.92}" },
    { "id": "fact_101", "layer": "L3", "kind": "fact", "label": "偏好简洁回复",
      "weight": 11, "meta_json": "{\"statement\":\"用户偏好简洁回复（完整原文）\",\"confidence\":0.88,\"hit_count\":34}" },
    { "id": "ep_201", "layer": "L2", "kind": "episode", "label": "季度复盘讨论",
      "weight": 1, "meta_json": "{\"happened_at\":\"...\",\"summary\":\"...\"}" }
  ],
  "edges": [
    { "source": "ent_001", "target": "ent_002", "type": "entity_relation",
      "label": "提到 12 次", "weight": 0.8, "polarity": "SUPPORTS" },
    { "source": "ent_001", "target": "fact_101", "type": "entity_fact", "weight": 1.0 },
    { "source": "fact_101", "target": "fact_102", "type": "fact_link", "weight": 0.6 },
    { "source": "fact_101", "target": "ep_201", "type": "fact_source", "weight": 1.0 },
    { "source": "fact_103", "target": "fact_104", "type": "fact_conflict",
      "polarity": "INHIBIT", "weight": 0.9 }
  ],
  "node_count": 24,
  "edge_count": 41,
  "filtered_edge_count": 17,
  "empty_reason": ""
}
```

> **fact 节点 `meta_json.statement`**：节点 `label` 被截断为 40 字符（省略号结尾），前端「在记忆浏览中打开」跳转依赖 `meta_json` 中的完整 `statement` 原文做知识库搜索（2026-07-26 修复，见 `TestUnifiedMemoryGraph_FactMetaCarriesFullStatement`）。
>
> **`empty_reason` 取值**：`""`（正常）/ `no_memory_data`（无任何记忆数据）/ `focus_not_found`（指定 focus 不在图内）。
>
> **L3 fact 节点查询口径（2026-08-05，H1）**：统一图 fact 节点按**产生方 agent**（`memory_facts.agent_id` 列）跨全部 scope 聚合扫描（`biz/memory_center.go`，上限 `unifiedGraphScanLimit`），与全景卡 / 浏览 Tab 的 F1 口径一致。旧实现按 `scope_type='agent'` 过滤，即刻事实（session 域）与巩固事实（user 域）全部漏算，图谱中 fact 节点近乎恒空。

**边的来源映射**（已对照代码核实，2026-07-25）：

| 边类型 | 数据来源 |
|--------|---------|
| `entity_relation`（含 INHIBIT 红边） | `memory_relations` 表，两端点均命中 `memory_entities`（`l4GraphRepo` / `NeighborhoodJSON` 同源） |
| `entity_fact` | `memory_relations` 中一端命中 `memory_entities`、另一端命中 `memory_facts` 的关系（schema 不限制端点类型；无数据时自然不渲染） |
| `fact_link` | ① `memory_relations` 中两端均为 fact ID 的 `EVOLVED_FROM` 关系（`link_evolution.go` `applyEvolvedFromSideEffects` 写入）；② `memory_facts.links_json`（A-MEM 链接） |
| `fact_source` | `memory_facts.source_episode_id`（`episode_consolidator` 写入，已核实） |
| `fact_conflict` | facts `conflict_count>0` 的冲突对（`ListConflictingFacts`）+ INHIBIT 关系 |

> 注：此前草稿提到的 `fact_entities` 关联表**不存在**，实体↔事实关联以 `memory_relations` 端点解析为准。
>
> **EVOLVED_FROM relation 写入 scope（2026-08-05，H4）**：`link_evolution.go` `applyEvolvedFromSideEffects` 写 `memory_relations` 时，fact 带 `agent_id` 则统一落到 `agent` scope（key = 产生方 agent）；无 `agent_id` 的遗留行回退 fact 自身 scope。旧行为继承 fact 的 session/user scope，而统一图关系读取按 `scope_type='agent'` 过滤，导致 EVOLVED_FROM 边（`fact_link` ①）在图谱中不可见。
>
> **冲突 facts 查询口径（2026-08-05，H2）**：`GET /v1/memory/l3/facts/conflicts`（`ListConflictingFacts`）新增可选 `agent_id` 参数，按**产生方 agent** 跨全部 scope 过滤 `conflict_count>0` 的活跃 facts，与 scope_type/scope_id AND 组合；scope 查询保持向后兼容。`scope_type` 与 `agent_id` 同时为空返回 400（防全表扫描）。旧实现仅支持 scope 过滤，而冲突 facts 实际分布在 session/user 域（F1 同一根因），治理面按 agent 浏览时漏算。

**默认 focus 策略**：未指定时由 repo 查询关系数最多的活跃实体（`memory_relations` 按端点聚合计数 Top 1）；空图时返回空 nodes 并附 `empty_reason`。

**BFS 扩展**：从 focus 出发按跳数扩展，在组装好的跨层邻接表上做内存 BFS（邻接来源见边来源映射）；`min_weight` 过滤 `memory_relations.weight` 弱边。

### 10.3 前端组件架构

```
web/src/features/memory/
├── panorama/                          ← P1 新增
│   ├── MemoryPanoramaTab.vue          ← 层级全景容器
│   ├── LayerFlowCards.vue             ← 五层卡 + 层间箭头 + 双向流向条
│   ├── LayerCard.vue                  ← 单层卡（props：layer/stats/health/headline）
│   ├── MemoryActionItems.vue          ← 需要关注
│   ├── MemoryActivityFeed.vue         ← 最近记忆动态
│   └── composables/useLayerOverview.ts
├── graph/                             ← P2 升级 MemoryGraphExplorer
│   ├── UnifiedMemoryGraph.vue         ← 跨层统一图（Vue Flow 渲染）
│   ├── GraphFilterRail.vue            ← 左侧：层级开关/边图例/权重阈值
│   ├── GraphNodeDetailDrawer.vue      ← 右侧：选中节点详情 + 连接列表
│   ├── memoryGraphLayout.ts           ← dagre 分层布局（L4→L3→L2 自上而下）
│   └── composables/useUnifiedMemoryGraph.ts
├── browse/                            ← P3 整合
│   ├── MemoryBrowseTab.vue            ← facts 表格 + episodes 时间线 + 层级过滤
│   └── MemoryEpisodeTimeline.vue      ← L2 情景时间线（新增）
└── （现有面板归并到治理 Tab：MemoryCascadePanel / 演进 / MemoryPlatformSettingsPanel / Worker 状态）
```

**渲染选型**：Vue Flow（已在依赖）+ dagre 分层布局（已在依赖），零新增依赖。层级自上而下排布（L4 → L3 → L2），与五层模型心智一致；缩放/拖拽/选中/连线交互由 Vue Flow 提供。力导向布局（d3-force）列为 P4 可选增强。

**数据流**：遵循项目数据流铁律——composable 调 service（`web/src/services/kratos/memory/v1`），组件不直接调 API；`memoryEndpoints.ts` 增补 `layerOverview` / `unifiedMemoryGraph` 两个端点。

### 10.4 UX 规范

| 项 | 规范 |
|----|------|
| 层级色码（全站统一） | L0 `#90a4ae` / L1 `#7986cb` / L2 `#4db6ac` / L3 `#ba68c8` / L4 `#ff8a65` |
| 图谱节点形状 | 圆形=L4 实体、圆角方块=L3 事实、三角=L2 情景 |
| 边样式 | 实体关系=实线（颜色随源层级）；事实来源=虚线；冲突/INHIBIT=红色虚线 `#ef5350` |
| 层级卡内容 | 图标+名称 / 一句话人话说明 / 核心大数字 / 今日新增 / 召回次数（仅 L3） / 健康徽标 |
| 流向条 | 卡片排上方「沉淀 ⬇（今日新增）」、下方「召回 ⬆（召回次数）」 |
| 图谱默认视图 | focus + 2 跳 + 权重阈值 ≥0.35；底部状态栏显示节点/边/已过滤数 |
| i18n | 层级名、健康状态、边类型全部走语言包，枚举映射中文 |

### 10.5 落地 Phase

| Phase | 范围 | 出口标准 |
|-------|------|---------|
| P1 | layer-overview API + 层级全景 Tab（FR-R1~R4） | 首页 3 秒看懂各层记忆量与健康；点击层级卡可钻取 |
| P2 | unified graph API + 关联图谱 Tab（FR-R5~R8） | 跨层三类节点同图；冲突红边可见；默认 2 跳不毛线球 |
| P3 | 记忆浏览 Tab（含 L2 时间线）+ 治理 Tab 归并（FR-R9~R10）✅ 2026-07-26 | L2 前端可浏览；旧 Tab 能力不丢失 |
| P4 | 扩散激活回放动画（FR-R11；力导向切换经 2026-07-26 评审移出本期，见 §10.6.5）✅ 2026-07-26 | 激活路径逐层点亮可演示 |

### 10.6 P3/P4 细化设计（2026-07-26 三视角评审）

> 评审结论：系统视角（后端增量最小化）、业务视角（五层可见性补齐 + 审批集中化）、用户视角（7 Tab → 4 Tab，跳转矩阵一致）。

#### 10.6.1 P3 后端：新增 1 个 RPC（治理归并零后端变更）

`MemoryService` 追加：

```proto
rpc ListMemoryEpisodes(ListMemoryEpisodesRequest) returns (ListMemoryEpisodesResponse) {
  option (google.api.http) = {get: "/v1/memory/episodes"};
}

message ListMemoryEpisodesRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
  string session_id = 2;   // 可选：按会话过滤，MVP 前端传空
  int32 limit = 3;
  int32 offset = 4;
}
message MemoryEpisode {
  string id = 1; string session_id = 2; string agent_id = 3;
  string episode_kind = 4; string title = 5; string outcome_summary = 6;
  double importance = 7; string consolidation_status = 8;  // pending | consolidated
  int32 consolidated_l3_count = 9;
  string ended_at = 10; string created_at = 11;
}
message ListMemoryEpisodesResponse {
  repeated MemoryEpisode items = 1; int32 total = 2;
}
```

**数据通路**：service handler → `authorizeMemoryScope`（与现有 memory admin 一致）→ biz `L2EpisodeAdminReader.ListEpisodeRowsAdmin`（Task 2 已建，走 `RWDB().ReadDB` 读连接）→ `scanEpisodeRowJSON` 字段直映 proto。**无新表、无 Schema 变更、无新 repo**。

#### 10.6.2 P3 前端：Tab 归并映射（7 → 4）

| 旧 Tab | 去向 | 说明 |
|--------|------|------|
| panorama | 保留 | 不动 |
| graph | 保留 | 不动 |
| knowledge（MemoryKnowledgePanel） | **browse**（L3 段） | 组件原样迁入 |
| sessions（MemorySessionsPanel：L0 快照 + L1 任务） | **browse**（L0/L1 段） | 组件原样迁入 |
| —（新增 MemoryEpisodeTimeline） | **browse**（L2 段） | 新组件，§10.6.3 |
| cascade（MemoryCascadePanel） | **governance** | 原样迁入 |
| evolution（MemoryEvolutionPanel） | **governance** | 原样迁入 |
| evolution（MemoryGraphExplorer） | **退役** | SVG 静态图被 P2 unified graph 取代；其静态激活高亮由 P4 回放取代 |
| settings（PlatformSettings/WorkerStatus/DeadLetter/RecallTester/SettingsStatus 5 面板） | **governance** | 原样迁入 |

**browse Tab 内部结构**：层级 chips（全部 / L0 / L1 / L2 / L3）控制显示对应分段；`?layer` 状态由 `MemoryBrowseTab` 持有，钻取跳转时由父页写入。

**L3 facts 查询口径（2026-08-05，F1）**：`GET /v1/memory/l3/facts` 新增可选 `agent_id` 参数，按**产生方 agent**（`memory_facts.agent_id` 列）跨全部 scope 过滤，与 scope_type/scope_id 命名空间过滤 AND 组合。记忆中心浏览 Tab 始终带上当前选中 agent，使 L3 列表与全景卡计数口径一致（全景卡同样按 agent_id 跨 scope 计数）；切换 agent 时 facts 列表自动刷新。

**跳转矩阵（全部更新为终态命名）**：

| 触发 | 旧目标 | 新目标 |
|------|--------|--------|
| 层级卡钻取 L0/L1/L2/L3 | sessions / knowledge | `browse` + `layer=Lx` |
| 层级卡钻取 L4 | evolution | `graph` |
| 图谱节点「在记忆浏览中打开」fact | knowledge + statement 搜索 | `browse` + `layer=L3` + statement 搜索 |
| 图谱节点 episode | sessions | `browse` + `layer=L2` |
| 图谱节点 entity | evolution | `graph`（聚焦该实体） |
| 需要关注 action_items `target_tab` | browse→knowledge / governance→evolution | `browse` / `governance` 直达 |

**清理项（R4 全局搜索）**：`MemoryGraphExplorer.vue` 删除及其全部引用；`useMemoryCenterPage.loadEvolution` 中仅服务 GraphExplorer 的 `entities` 加载移除；i18n `memory.tabs.*` 旧 key 清理。

#### 10.6.3 P3 新组件：`browse/MemoryEpisodeTimeline.vue`

- 数据源：`api.ts` 新增 `getMemoryEpisodes(agentID, sessionID='', limit=20, offset=0)`
- 卡片流（按 `created_at` 倒序 + 分页加载更多）：标题 / 相对时间 / `outcome_summary`（2 行截断）/ importance 星阶 / 状态徽标（`consolidated`=已提炼 + l3_count 条事实；`pending`=提炼中，pulse 动画）
- 空态：引导文案「情景记忆在对话归档后自动生成」
- 点击卡片：展开完整摘要（不做跨页跳转，MVP）

#### 10.6.4 P4：扩散激活回放（复用现有 RPC，零后端变更）

- **入口**：`GraphNodeDetailDrawer` 实体节点（kind=entity）详情底部加「回放扩散激活」按钮；回放以该实体为 `center_id`
- **流程**：点击 → `load(centerId)` 将 unified graph 聚焦该实体（ hops 不变）→ 调 `SpreadingActivation(centerId, hops=3, topK=20)` → 按 `hop_count` 分组，每 600ms 点亮一跳 → 节点按 activation 归一化强度发光（box-shadow + 尺寸微放大），`activation_path` 经过的边同步高亮 → 动画结束保留终态高亮，切换 focus / 再点按钮清除
- **排行列表**：回放期间图谱底部状态栏切换为 Top-K 激活排行（节点名 + activation 值 + hop 数徽标）；不在当前子图内的激活节点列出但标记「图外」
- **实现**：`graph/composables/useActivationReplay.ts`（service 调用 + 分组 + 定时器驱动 `activeHops`/`playing`/`replay()`/`stop()`）；`UnifiedGraphNode` 增 `activation` data 字段驱动样式；定时器在组件 unmount 时清理
- **测试**：composable 单测（mock service + vi.useFakeTimers 验证逐跳推进）；节点激活样式映射单测

#### 10.6.5 评审范围收敛说明

| 项 | 结论 | 理由 |
|----|------|------|
| 力导向布局（d3-force） | **移出本期**，列后续可选 | `web/package.json` 无 d3 依赖，违反零新依赖原则；dagre 分层已满足层级心智 |
| Hero 区全记忆搜索框 + 健康总分徽标（§10.1 提及） | 列后续候选 | P1~P4 出口标准均未含；需独立设计搜索口径（跨层复合检索 vs facts 关键词） |
| L2 时间线 session 过滤 | MVP 不做 | `ListEpisodeRowsAdmin` 已支持 sessionID 参数，前端后续按需开启 |

---

## 子模块：会话记忆分类治理与复用增强设计（2026-07-29）

> 需求见 [`memory.md`](./memory.md) §22；任务进度见 [`memory-development.md`](./memory-development.md) Phase 6。

### A. 提取分类透传（FR-M1）

**现状链路**：`compress.MemoryExtractSystemPromptV2` + function schema 已让 LLM 输出 `subject_type / scope / confidence`（`internal/compress/memory_extract.go`），但 `convertFactsToProposals`（`internal/service/memory_llm_extractor.go`）丢弃这三字段，`auto_memory.go` 落库写死 `FactKind="fact" / ScopeType="agent" / Confidence=0.85`。

**改动点**：

1. `internal/compress/memory_extract.go`：`subject_type` 枚举追加 `constraint`（负向约束/工作要求，如"别用某工具""排查要从全局出发"），prompt 规则补充 constraint 语义说明。
2. `biz.MemoryProposal` 增加 `SubjectType / Scope / Confidence` 三字段；`HeuristicConsolidator` 默认 `preference/user`；`FeedbackConsolidator` 默认 `preference/user`。
3. `convertFactsToProposals` 透传三字段。
4. `auto_memory.go` 落库映射：

| subject_type | fact_kind |
|--------------|-----------|
| person | profile |
| preference | preference |
| constraint | constraint |
| event | event |
| concept | knowledge |
| other / 空 | fact |

`scope=user → ScopeType="user", ScopeID=userID`；`scope=agent → ScopeType="agent", ScopeID=appName`；`confidence` 直接透传（默认 0.7 由解析层兜底）。

### B. 冲突判决与自动 supersede（FR-M2）

**判决器**：biz 新增纯函数 `DecideMemoryConflict(kind, neighbors) → MemoryConflictDecision`（可单测，无 IO，`memory_conflict.go`）。

- **近邻来源**：新增 `MemoryConflictNeighborSearcher`（data 层 pgvector `SearchByAgent`，agent+user 分区，score 钳制 [0,1]）；`MemoryConflictDetector` 包装 embed → 近邻搜索 → `GetFactRowsByIDs` 活性校验与 kind 富化（剔除 superseded/deleted 行）→ 纯函数判决。全链路基础设施失败降级为无动作，绝不阻塞写入。AutoMemoryWorker / memory_remember 均注入 detector。
- **阈值与动作**（仅对 `preference / constraint / profile` 类 proposal 执行；knowledge/event/fact 类可叠加不判决）：

| 近邻最高分 | 动作 |
|-----------|------|
| ≥ 0.92 且同 kind | **supersede**：新值正常写入，旧值 `status='superseded', superseded_by=新id, updated_at=now` |
| 0.80 – 0.92 | 旧值 `conflict_count+1`，候选记入新值 `metadata_json.conflict_candidates`，留待记忆管家 dream_cycle 治理 |
| < 0.80 | 无动作 |

- **supersede 语义**：标记不删除，可回滚；召回查询与常驻注入均过滤 `status='active'`（既有召回已过滤 active，无需改动）。
- **Data 层**：`memory_shim_l3.go` 新增 `SupersedeFact(ctx, oldID, newID)`，冲突标记复用既有 `BatchIncrementConflictCounts(ctx, ids)`，均走 `RWDB().WriteDB(ctx)` 事务感知路径。supersede 在写后应用（需新事实 ID），标记批量执行。

### C. 常驻偏好/约束注入（FR-M3）

- **Data 层**：`memory_shim_l3.go` 新增 `ListActivePreferenceFacts(ctx, agentID, userID, kinds, limit)`，查询 `scope∈{(user,userID),(agent,agentID)} + fact_kind∈kinds + status='active'`，按 `importance DESC, updated_at DESC` 排序。
- **Biz 层**：窄接口 `MemoryPreferenceLister`（≤2 方法，Stability:evolving）。
- **Agent 层**：`internal/agent/composite_prompt.go` 新增 `PinnedPreferenceCue(...)`，输出块：

```
## 用户偏好与工作要求（始终生效）
- [PREFERENCE] ...
- [CONSTRAINT] ...
```

- **装配**：与 `CompositeMemoryCue` 同处拼接，置于召回块之前；常量 `pinnedMax=10`、单条 ≤200 字符（rune 截断）。
- **兼容**：存量 `fact_kind='fact'` 不进常驻块，渐进生效。

### D. memory_remember 显式记忆工具（FR-M4）

- **位置**：`internal/tools/memoryremember/remember.go`（function tool，对齐 skills_butler 工具模式）。
- **输入**：`statement`（必填）、`kind`（preference|constraint，默认 preference）。`agentID / userID` 由装配闭包注入，**禁止 LLM 填写**（防越权写他人记忆）。
- **写入**：复用 FR-M2 判决器做冲突治理 → `MemoryConsolidationWriter.UpsertFactsAndEpisodeBatch`；字段 `fact_kind=kind, scope_type=user, source_kind='explicit', importance=0.8, confidence=0.95, status='active'`。
- **装配**：`ChatOrchestrator` custom tools 追加（全部 chat agent 可用）；`prompts/IDENTITY.md` 增加使用指导（用户明确说"记住/以后都……/不要再……"时调用）。

### E. 技能管家对齐（联动 20-skill）

- `prompts/skills/skills.md`：移除未实现的 `retire_skill`；工作流对齐实际 8 工具（analyze_skill_usage / recommend_skills / evolve_skill / optimize_skill / analyze_skill_health / analyze_tool_weights / analyze_orchestration / optimize_orchestration，装配于 `cli_admin_tools.go:225`）。
- `recommend_skills` 已实现（pending proposals + 调用统计健康度），本期补单测。

