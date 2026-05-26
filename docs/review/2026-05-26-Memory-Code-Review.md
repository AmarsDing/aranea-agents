# Memory 模块代码层 Review（业务逻辑 / 代码质量 / 架构设计）

> **评分**：82 / 100 | **风险等级**：P2
> **审查时间**：2026-05-26
> **范围**：后端 memory 相关（共约 28 个 `internal/biz/memory*.go` + 9 个 `internal/data/memory*.go` + `internal/data/sessionmemory/`(37 文件) + 5 个 `internal/service/memory*.go` + `internal/memory/trpc/` + `internal/runtime/memory_set.go` + `internal/tools/memory/` + `internal/cronrunner/jobs/memory_*.go` + `internal/agent/*memory*` 注入层 + `internal/compress/memory_extract.go` + `api/kratos/memory/v1/memory.proto`；**不含 web/、不含 `pkg/trpc-agent-go/`**）
> **聚焦**：L0–L4 五层模型、异步 Worker、Recall/Inject、Cascade/Decay/PII/Policy、pgvector 双轨
> **真相源**：`docs/AGENT_RUNTIME_BOUNDARY.md`、`.cursor/rules/trpc-agent-framework-first.mdc`
> **历史 Review**：[memory-review.md](./memory-review.md)（MEM-R 系列，2026-05-24）

---

## 1. 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 业务逻辑正确性 | 17 | 20 | L0–L4 读/写/注入闭环完整；facts 权威 + index sync 已落地；但 Cascade Approve 后 index sync `_ =` 静默忽略、L4 confidence decay 无全局 cron、`MemoryJobQueue` 满时静默 drop |
| 架构一致性 | 22 | 25 | biz 零 trpc import；`runtime.MemorySet` + Wire 装配合规；端口/适配清晰；`wireSessionAdminStoreAdapter` 180 行纯委托重复、`MemoryUsecase.Remember()` 遗留 pgvector 直写路径未清理 |
| 代码质量 | 16 | 20 | 命名/Phase 注释良好；PolicyVersion / cascade status 字面量分裂；L4 usecase / auto_memory 多处 `_ =`；`store_more.go` 645 行、`service/memory.go` 631 行、`auto_memory.extract` ~180 行复杂度集中 |
| 测试覆盖 | 7 | 10 | Policy/PII/Consolidator/Cascade/L3 fused/Composite/auto_memory 集成测齐全；缺 Cascade Approve→pgvector 同步、Policy strict 事务阻断、`MemoryJobQueue` race/drop、LLM 提取 E2E、`service/memory.go` RPC 直测 |
| 可扩展性与抽象 | 12 | 15 | 新增 Recall/Cue/Cron/Provider 路径清晰；Heuristic/LLM 双链 Consolidator 可插拔；pgvector 多租户、真实 CE rerank 仍为 backlog |
| 文档/注释一致性 | 8 | 10 | proto 注释与实现基本对齐；L4 decay 「MVP 元数据」与仅 auto_memory 触发 `RunDecay` 的落差未文档化 |

---

## 2. 模块组成（按层归类）

### 2.1 biz（~2,400 行；28 个 `memory*.go` + 2 个 runtime policy）

| 文件 | 行数 | 职责 |
|------|------|------|
| `memory.go` | 104 | pgvector `MemoryUsecase`（Embed / Recall / UpsertFactVector）；`Remember/RememberWithUser` 遗留 |
| `memory_admin_store.go` | 101 | L0–L4 管理端口聚合接口（`SessionAdminStore` + 子接口） |
| `memory_admin_usecase.go` | 150 | Admin RPC 用例委托 + Upsert 后 index sync |
| `memory_worker.go` | 131 | `TurnMemoryWorker`：Runner 完成 / 反馈 → 入队 |
| `memory_worker_stats.go` | 63 | 全局 Worker 计数器（done/dead/fallback/backfill） |
| `memory_consolidator.go` | 216 | Heuristic / Chain / Feedback Consolidator；regex 提取 L3 |
| `memory_consolidator_llm.go` | 39 | LLM Consolidator 端口 + `DefaultMemoryConsolidator` |
| `memory_policy.go` | 112 | `MemoryPolicyEngine`：审计写入 + strict 模式 |
| `memory_platform_config.go` | 55 | `MEMORY_POLICY_STRICT` / episode backfill env 解析 |
| `memory_pii.go` | 47 | `ScanPII`：email / phone / id / card regex redaction |
| `memory_index_sync.go` | 69 | `MemoryFactIndexSyncer` / `EpisodeIndexSyncer` 端口 |
| `memory_l2_recall.go` | 55 | L2 recall 用例（embed + 委托 store rerank） |
| `memory_l3_fused_recall.go` | 177 | L3 多 scope 融合 recall + dedupe |
| `memory_composite_recall.go` | 108 | L2+L3 Composite recall 用例 |
| `memory_rerank.go` | 49 | CE rerank 词级 bigram Jaccard proxy |
| `memory_l4.go` | 48 | L4 图写入端口（`L4GraphWriter` / `L4GraphRepo`） |
| `memory_l4_usecase.go` | 248 | L4 自动写入（name / preference regex）+ 冲突门控 |
| `memory_l4_cascade.go` | 305 | Cascade 提案 BFS / Approve / Reject + fact 重命名 |
| `agent_memory_runtime_policy.go` | 229 | `MemoryRuntimePolicy` 解析（L0 注入 / 写入门控） |
| `agent_memory_maintenance.go` | 56 | Cron decay 目标 agent 列表 |
| `*_test.go`（9 个） | ~700 | Policy / PII / Cascade / L3 / Composite / Consolidator 等 |

### 2.2 data（~900 行 memory 适配 + ~4,500 行 sessionmemory 实现）

| 文件 | 行数 | 职责 |
|------|------|------|
| `memory.go` | 135 | pgvector `MemoryRepo`（按 user 维度分表） |
| `memory_fact_index_sync.go` | 67 | facts → pgvector + SQLite `embedding_blob` 双写 |
| `memory_episode_sync.go` | 45 | episode → `embedding_blob` |
| `memory_l4.go` | 133 | `L4GraphRepo` / `L4GraphWriter` 适配 |
| `memory_cascade.go` | 81 | `CascadeGraphStore` 适配 |
| `memory_composite_adapter.go` | 41 | Composite search 适配 |
| `memory_l3_scored_adapter.go` | 50 | Scored L3 hits → `biz.RecallHit` |
| `memory_migrate.go` | 73 | legacy trpc_memory → facts 一次性迁移 |
| `memory_chain_schema.go` | 64 | schema 版本常量 |
| **`sessionmemory/`**（37 文件） | ~4,500 | SQLite L0–L4 真相存储、recall rerank、policy audit、cascade |
| 热点 | | `store_more.go`(645) / `store_writes.go`(327) / `store_l2_recall.go`(333) / `store_l3_recall.go`(253) |

### 2.3 service（~900 行）

| 文件 | 行数 | 职责 |
|------|------|------|
| `memory.go` | 631 | Kratos `MemoryService`：L0–L4 列表 / 写入 / Cascade / Evolution RPC |
| `memory_recall.go` | 111 | DebugRecall / CompositeSearch / WorkerStatus RPC |
| `memory_platform.go` | 41 | Platform settings（policy_strict / backfill toggle） |
| `memory_llm_extractor.go` | 155 | LLM 提取实现（OpenAI-compat + compress prompt） |
| `memory_decode.go` | 163 | proto ↔ JSON row 解码 helpers |

### 2.4 runtime / tools / compress / cron

| 层 | 文件 | 行数 | 职责 |
|----|------|------|------|
| runtime | `memory_set.go` | 21 | `MemorySet{TRPC, Admin, L2/L3/CompositeRecall}` 框架装配 DTO |
| tools | `tools/memory/tools.go` | 23 | 5 个标准 memory tool（不含 clear） |
| compress | `memory_extract.go` | 89 | LLM 提取 prompt + JSON 解析 |
| cron | `auto_memory.go` | 444 | 队列 drain + LLM→启发式提取 + L2/L3/L4 写入 |
| cron | `memory_l2_decay.go` | 116 | L2 importance 衰减 + retention purge |
| cron | `memory_l3_decay.go` | 110 | L3 fact importance 衰减 |
| cron | `memory_episode_backfill.go` | 78 | 缺失 embedding 的 episode 回填 |
| cron | `memory_data_migration.go` | 63 | legacy trpc_memory 一次性迁移 |

### 2.5 framework binding（`internal/memory/trpc/`）

| 文件 | 行数 | 职责 |
|------|------|------|
| `sqlite_adapter.go` | 378 | `trpcmemory.Service` → SQLite facts；Search 优先 pgvector |
| `auto_memory_queue.go` | 119 | 有界 channel 队列 + 30 s debounce + dropped 计数 |
| `settings_loader.go` | 64 | Agent settings → memory tool search limits |
| `strutil.go` | 7 | `TruncateString` 薄包装（几乎无独立价值） |

### 2.6 proto / schema / 注入层

| 文件 | 角色 |
|------|------|
| `api/kratos/memory/v1/memory.proto` | 19 个 RPC：L0–L4 管理、Cascade、Recall debug、Composite、Worker status、Platform settings |
| `internal/data/sql/memory_chain.sql` | 完整 SQLite schema（`memory_facts` / `memory_episodes` / L4 图 / cascade / action_log） |
| `internal/agent/memory_inject.go` | BeforeModel hook：L1/L2/L3/L4/Composite cue 组装（**范围外但读写必经**） |
| `internal/agent/{l1,l2,l3,l4,composite}_prompt.go` | 各层 cue 格式化 |

---

## 3. 业务逻辑分析

### 3.1 Memory L0–L4 五层模型现状

| 层 | 名称 | 存储 | 写路径 | 读 / Recall | 注入 |
|----|------|------|--------|-------------|------|
| **L0** | Sensory | trpc Session 上下文窗口 | Runner 压缩 / 摘要（框架） | Session 历史 | 框架原生 |
| **L0 快照** | Assembly 审计 | SQLite `memory_l0_assembly_snapshots` | L0 组装投影 | Admin `ListL0Snapshots` | — |
| **L1** | Working | SQLite L1 tasks / fields | L1 任务运行时 | `ListL1Tasks/Fields` | `L1MemoryCue`（pin fields） |
| **L2** | Episodic | SQLite `memory_episodes` | `AutoMemoryWorker` batch upsert episode | `RecallL2Episodes`（keyword / vector / importance / decay / CE） | `L2MemoryCue` 或 Composite |
| **L3** | Semantic | SQLite `memory_facts`（权威） + 可选 pgvector + `embedding_blob` | auto_memory / trpc AddMemory / Admin UpsertFact / feedback | `RecallL3Facts` / fused multi-scope | `L3MemoryCue` 或 Composite |
| **L4** | Persistent graph | SQLite entities / relations | `L4GraphUsecase.WriteFromUserText`（regex） | BFS neighborhood / identity / strategy | `L4MemoryCue` |

**注入总线**（`memory_inject.go`，简版流程）：

1. 解析 `ResolveMemoryRuntimePolicy(ag.Settings)`；master off → 无注入。
2. `RecallKeywordFromMessages` 取 query（Intent hints 优先）。
3. Composite 模式（`RecallL2 && InjectL3` 同时开）走 fused block；否则 L2 / L3 分开注入。
4. L4 独立注入 graph / persona 片段。
5. `prepend SystemMessage(cue)` 到 model 请求。

### 3.2 横切能力

| 能力 | 实现位置 | 要点 |
|------|----------|------|
| **PII 检测** | `biz.ScanPII` → `store_writes.upsertFactRowOn` | 写 fact 时设 `pii_flag` + `redacted_statement`；**不阻断写入** |
| **Cascade 级联** | `L4CascadeUsecase` + `store_cascade*.go` | 名称冲突 → BFS affected → `pending` 提案；Approve 重命名 entity + 整词 fact 替换 + index sync |
| **Decay 衰减** | L2 cron（0.95/24h）、L3 cron（0.97/按 agent interval）、L4 `RunDecay`（0.92/30d） | L2/L3 有 24 h cron；**L4 仅在 auto_memory 写图后调用 `RunDecay`，无独立 cron** |
| **Worker 异步** | `TurnMemoryWorker` → `MemoryJobQueue` → `AutoMemoryWorker` | 10 s ticker drain；LLM → Heuristic chain；3 次退避（30 s / 2 m / 10 m） |
| **Composite Search** | `sessionmemory.CompositeSearchMemories` + biz/service RPC | L2+L3 分数融合；prompt 与 debug RPC 共用 store |
| **Policy strict** | `MemoryPolicyEngine` + `store_policy.recordPolicyOnTx` | env `MEMORY_POLICY_STRICT` > DB；strict 时 audit 失败阻断 batch 事务 |

### 3.3 L3/L4 写入收敛（facts 权威 vs pgvector）

| 轨 | 角色 | 现状 |
|----|------|------|
| **SQLite `memory_facts`** | 权威行存储 | 所有写路径最终 `UpsertFactRow` |
| **pgvector** | 可选读索引 | `NewMemoryFactIndexSync` 在写后 `SyncFactIndexFromRow` |
| **SQLite `embedding_blob`** | 本地向量索引 | 与 pgvector 同步双写 |
| **trpc `SearchMemories`** | 工具 / API 读 | **优先 pgvector**，失败 / 空才回退 SQLite keyword list |

> **结论**：写路径已收敛到 facts；**读路径仍双轨** —— pgvector stale 时 Search 可能返回旧向量结果（MEM-R03 在 Cascade 路径仍有 `_ =` 风险，见 §6 MEM-Q-01）。

### 3.4 L3MemoryCue / L2MemoryCue 注入流程

```
Runner BeforeModel (priority=5)
  → buildRuntimeMemoryCue
    → policy gates
    → L2MemoryCue:        MemoryL2Recaller.RecallEpisodes  → JSON rows → markdown list
    → L3MemoryCue:        MemoryL3Recaller.RecallFactsFused → multi-scope merge → statements
    → CompositeMemoryCue: 当 RecallL2 && InjectL3 同时开
  → prepend SystemMessage(cue)
```

关键门控：`l0_inject_l3` / `l2_recall_enabled` 映射到 `InjectL3` / `RecallL2`（MEM-R01/01b 已闭合）。

### 3.5 MemoryPolicyEngine 策略点

| 触发点 | PolicyVersion | strict 行为 |
|--------|---------------|-------------|
| `UpsertFactsAndEpisodeBatch`（auto_memory） | `biz.PolicyVersionConsolidateV1` | tx 内 audit 失败 → rollback |
| 单条 `UpsertFactRow` | `"consolidate_v1"` **字面量** | best-effort / strict 阻断 |
| L2 / L3 decay cron | `l2_decay_v1` / `l3_decay_v1` | best-effort |
| Cascade insert / approve | `cascade_v1` | best-effort |
| L4 entity upsert（data 层） | `"consolidate_v1"` **字面量** | 直接 `InsertActionLog`，**绕过 Engine** |

### 3.6 关键 Cron Job 及调度

| Worker | 默认周期 | 触发 | 禁用 env |
|--------|----------|------|----------|
| `AutoMemoryWorker` | **10 s** drain | `cmd/admin/main.go` goroutine | queue nil 则不创建 |
| `MemoryL2DecayWorker` | **24 h** | 启动即 runOnce + ticker | `MEMORY_L2_DECAY_DISABLED` |
| `MemoryL3DecayWorker` | **24 h**（per-agent 可配 `L3DecayIntervalHours`） | 同上 | `MEMORY_L3_DECAY_DISABLED` |
| `MemoryEpisodeBackfillWorker` | **6 h** | 同上 | env / DB `episode_backfill_disabled` |
| `MemoryDataMigrationWorker` | 一次性（AfterStart） | 30 s timeout | `MEMORY_DATA_MIGRATION_DISABLED` |
| Queue debounce | **30 s** / session | 入队时 | — |

---

## 4. 代码质量评估

### 4.1 复杂度热点

| 文件 | 行数 | 复杂度问题 |
|------|------|----------|
| `internal/data/sessionmemory/store_more.go` | **645** | Admin 列表 / 进化 / metrics 聚合堆叠 |
| `internal/service/memory.go` | **631** | 19 个 RPC handler 同文件 |
| `internal/memory/trpc/sqlite_adapter.go` | **378** | trpc Service 全实现 |
| `internal/cronrunner/jobs/auto_memory.go` | **444** | `extract()` ~180 行：policy / LLM / L2 / L3 / L4 / index 九职责 |
| `internal/biz/memory_l4_cascade.go` | **305** | Approve 事务链 + JSON 解析 |
| `internal/data/sessionmemory/store_l2_recall.go` | **333** | rerank 算法内联 |
| `cmd/admin/wire_memory.go` | **227** | `wireSessionAdminStoreAdapter` 纯委托 |

单方法 ≥ 100 行集中：`AutoMemoryWorker.extract`（~180）、`Approve` + `touchAffectedEntities` 合计 ~140。

### 4.2 命名与一致性

- **优点**：端口命名清晰（`SessionL3RecallStore` / `MemoryL3Recaller` / `MemoryFactIndexSyncer`）；Policy 常量集中在 `memory_policy.go`。
- **分裂问题**：

| 域 | 分裂示例 |
|----|----------|
| PolicyVersion | `biz.PolicyVersionConsolidateV1` vs `"consolidate_v1"` 字面量（`store_writes.go` / `store_facts_ops.go` / `data/memory_l4.go`） |
| Cascade status | `"pending"` / `"applied"` / `"rejected"` 散落，无 biz 常量 |
| Fact status | `"active"` 默认；entity `"pending"` → `"active"` |
| Episode 状态 | `consolidation_status='consolidated'` vs `embedding_status='pending'`（不同语义易混） |
| L2 recall policy audit | `"l2_decay_v1"` 字面量 vs `biz.PolicyVersionL2DecayV1` |

### 4.3 死代码 / 测试-生产偏离（**新发现**）

| 符号 | 文件 | 性质 |
|------|------|------|
| `MemoryUsecase.Remember` / `RememberWithUser` | `internal/biz/memory.go` | **零生产调用**；pgvector 直插遗留入口，仅由旧 trpc memory tool 使用 |
| `internal/memory/strutil.go` | 整文件 | 仅 re-export `pkg/strutil.TruncateBytes`，无独立价值 |
| `PolicyStrictMode()` | `memory_platform_config.go` | 标记 legacy；生产用 `ResolvePolicyStrict` |
| `wireSessionAdminStoreAdapter` | `cmd/admin/wire_memory.go` | 与 `SessionAdminStore` 1:1 委托 180 行，可用 embed 或 code gen 压缩 |

### 4.4 错误处理风格

| 模式 | 占比 | 代表 |
|------|------|------|
| FlowLog / SessionSysLog warn + 继续 | 高 | auto_memory `extract_fail`、decay cron |
| `slog.Warn` | 中 | cascade `touchAffected`、`syncFactIndexBestEffort` |
| `return err` | 关键路径 | `UpsertFactsAndEpisodeBatch`、LLM extract |
| **`_ =` 静默忽略** | 风险点 | L4 upsert、cascade index sync、episode sync |

### 4.5 并发安全

| 组件 | 模型 | 风险 |
|------|------|------|
| `MemoryJobQueue` | channel + `sync.Map` debounce + atomic counters | 满队列 **非阻塞 drop**；`recent` map 无 TTL GC（仅覆盖） |
| `MemoryWorkerStatsGlobal` | atomic | 安全 |
| `memoryRepo.store` map | `sync.RWMutex` | pgvector 按 dim 缓存 store，安全 |
| `AutoMemoryWorker.drain` | 单 goroutine ticker | 无并行 drain；`safego` 包裹 runOnce |

> **Race 风险**：`MemoryJobQueue` debounce / drop 无 `-race` 测试；高并发 enqueue 可能丢 job 且无 per-session 告警。

### 4.6 测试覆盖

| 维度 | 评价 |
|------|------|
| Heuristic / Chain Consolidator | ★★★★ |
| PII / Policy strict（biz） | ★★★★ |
| L3 fused / Composite recall | ★★★★ |
| Cascade Approve / Reject 单元 | ★★★ |
| auto_memory queue → fact → episode | ★★★★ integration |
| Consolidate batch + policy tx | ★★★★ |
| L2 / L3 recall rerank（store 层） | ★★★★ |
| LLM extractor | ★★ 仅 service 层单测 |
| **Cascade Approve → pgvector sync** | ✗ 缺 |
| **Policy strict 阻断 consolidate batch** | ✗ 缺 |
| **`MemoryJobQueue` 满 / debounce / race** | ✗ 缺 |
| **`service/memory.go` RPC handlers** | ✗ 缺直接单测 |
| **L4 name conflict → cascade proposal E2E** | ✗ 缺 |

---

## 5. 架构与设计评估

### 5.1 依赖方向（红线核查）

| 红线 | 状态 |
|------|------|
| `internal/biz` 不 import `pkg/trpc-agent-go` / `trpc.group/*` | ✅ 仅 `doc.go` 注释提及 |
| Runner / MemoryService 装配在 service / runtime / wire | ✅ `cmd/admin/wire_memory.go` + `runtime.MemorySet` |
| biz 不依赖 `internal/memory/trpc` | ✅ 通过 `AutoMemoryEnqueuer` 端口反向调用 |

### 5.2 端口 / 适配

| biz 端口 | 实现 |
|----------|------|
| `SessionAdminStore` | `sessionmemory.Store` / wire adapter |
| `MemoryL2Recaller` / `MemoryL3Recaller` / `MemoryCompositeRecaller` | biz usecase + data adapter |
| `MemoryFactIndexSyncer` | `data.NewMemoryFactIndexSync` |
| `EpisodeIndexSyncer` | `data.NewMemoryEpisodeIndexSync` |
| `L4GraphWriter` / `L4GraphRepo` | `data/memory_l4.go` |
| `CascadeGraphStore` | `data/memory_cascade.go` |
| `MemoryConsolidator` | Heuristic + service `MemoryLLMExtractor` chain |
| `AutoMemoryEnqueuer` | `memtrpc.NewAutoMemoryEnqueuer` |

### 5.3 可扩展点

- **新 memory provider**：实现 `trpcmemory.Service` + Wire 替换 `NewSQLiteMemoryService`。
- **新 Cue 层**：`agent/memory_inject.buildRuntimeMemoryCue` 加 part + policy 字段。
- **新 Cron**：`cmd/admin/main.go` 注册 + Wire provider（参照 L2 / L3 decay）。
- **新 Consolidator**：实现 `MemoryConsolidator` 接口，注入 `AutoMemoryWorker`。

### 5.4 设计气味

| 气味 | 位置 | 说明 |
|------|------|------|
| **God function** | `AutoMemoryWorker.extract` | ~180 行，policy/LLM/L2/L3/L4/index 九职责 |
| **God file** | `store_more.go` / `service/memory.go` | 单文件 600+ 行 |
| **Stringly-typed status / policy version** | cascade / facts / policy audit | 见 §4.2 |
| **Magic constants** | 队列 256 / 30 s、decay 0.95/0.97/0.92、recall 权重、CE proxy | 散落，未走配置中心 |
| **隐藏限流** | `MemoryJobQueue` drop + debounce 无 metrics 暴露 | dropped/debounced 计数存在但未接 RPC |
| **双轨读不一致** | `SearchMemories` pgvector 优先 | index 延迟时返回 stale |
| **Regex 协议脆弱** | Heuristic + L4 共用 name/preference 正则 | LLM 输出格式漂移即失效 |
| **未完成收敛** | `MemoryUsecase.Remember()` 遗留、wire adapter 手工委托 | refactor 在途但未清理 |

---

## 6. 问题清单（按优先级）

### P1 — 当前迭代应处理

| ID | 问题 | 影响 | 建议 |
|----|------|------|------|
| **MEM-Q-01** | Cascade `Approve` 中 `_ = indexSync.SyncFactIndexFromRow` 静默忽略 | Approve 后 pgvector 可能仍指向旧 statement，导致 `SearchMemories` 返回过时记忆 | 改为检查 error + FlowLog warn；strict 模式下阻断或重试 |
| **MEM-Q-02** | `MemoryJobQueue` 满时 silent drop（每 100 次 warn 一次） | 高负载 session 记忆永不提取且前端无感知 | 暴露 `dropped`/`debounced` 到 `WorkerStatus` RPC；考虑 per-session 优先级或 dead-letter |
| **MEM-Q-03** | L4 `RunDecay` 仅 auto_memory 写图后触发，无全局 cron | 长期无新 L4 写入的 agent 实体 confidence 不衰减 | 新增 `MemoryL4DecayWorker` 或并入 L3 cron |
| **MEM-Q-04** | `SearchMemories` pgvector 优先于 SQLite | pgvector stale 时 tool search 返回过时记忆 | pgvector 命中后校验 `fact_id` 仍 active；或 Search 默认走 SQLite scored recall |
| **MEM-Q-05** | L4 entity upsert（`data/memory_l4.go`）直接 `InsertActionLog` 绕过 `MemoryPolicyEngine` | strict 模式下 L4 audit 失败不阻断、版本号字面量 | 统一走 `recordPolicyBestEffort` / `recordPolicyOnTx` |

### P2 — 下一迭代

| ID | 问题 | 建议 |
|----|------|------|
| **MEM-Q-06** | PolicyVersion 字面量分裂（`"consolidate_v1"` vs 常量） | 全库统一 `biz.PolicyVersion*` |
| **MEM-Q-07** | Cascade status 无常量（pending / applied / rejected） | 提取 `biz.CascadeStatus*` |
| **MEM-Q-08** | L4 `WriteFromUserText` 大量 `_ =` UpsertEntity/Relation | 聚合 error；至少 FlowLog warn |
| **MEM-Q-09** | `AutoMemoryWorker.extract` ~180 行 | 拆为 `extractFacts` / `writeEpisode` / `writeL4` / `syncIndexes` 四个 helper |
| **MEM-Q-10** | `GetEvolutionMetrics` 忽略 `req.GetRange()` | 实现 range 过滤或 proto 标注 deprecated |
| **MEM-Q-11** | `MemoryUsecase.Remember()` 零调用遗留 | 删除或标 Deprecated + 移除 `MemoryRepo.Insert` 路径 |
| **MEM-Q-12** | Heuristic / L4 regex 重复且脆弱 | 共享 extractor 或统一走 LLM chain |
| **MEM-Q-13** | pgvector 多租户隔离缺集成测 | 补 agent+user 并发 upsert/search 测（memory-review backlog） |

### P3 — 优化建议

| ID | 问题 | 建议 |
|----|------|------|
| **MEM-Q-14** | decay / recall 魔法常量硬编码（0.95/0.97/0.92/256/30s/CE 权重） | 迁入 agent settings 或 env |
| **MEM-Q-15** | CE rerank 为 lexical proxy | 文档标注；接真实模型服务 |
| **MEM-Q-16** | `wireSessionAdminStoreAdapter` 180 行委托 | embed `Store` 或 codegen 压缩 |
| **MEM-Q-17** | `memory_l4_usecase.go` 字段缩进不齐 / import 块分裂 | `gofmt -s` / `goimports` |
| **MEM-Q-18** | `MemoryJobQueue.Stats()` 未暴露到 RPC | 扩展 `MemoryWorkerStatus` proto |
| **MEM-Q-19** | `internal/memory/strutil.go` 仅 re-export | 直接调用 `pkg/strutil`，删除文件 |

---

## 7. 回归风险点（已修复但建议加测）

| 修复 ID（历史） | 现状 | 测试 | 建议加测 |
|----------------|------|------|----------|
| **MEM-R01**（L3 运行时注入） | ✅ `L3MemoryCue` + BeforeModel | `l3_prompt_test.go` | E2E：settings on → system message 含 L3 |
| **MEM-R01b**（L2 注入） | ✅ `L2MemoryCue` | 间接 | Composite + L2-only 开关回归 |
| **MEM-R02**（L3 已有则 skip L4） | ✅ `msgIDsWithL3` | integration 间接 | 显式断言同一 user msg 不写 L4 |
| **MEM-R03**（Cascade Approve index sync） | ⚠️ 代码存在但 `_ =` | cascade 单测**无 index 断言** | Approve → pgvector statement 更新断言 |
| **MEM-R04**（queue → extract 集成） | ✅ `auto_memory_integration_test.go` | OK | 加 LLM fallback 路径 |
| **MEM-R05**（Action Log turn_id） | ✅ `composeTurnRef` in batch | `consolidate_batch_test` | strict 模式 turn_id 落库 |
| **MEM-R06**（L2 recall / decay） | ✅ `store_l2_recall` + cron | store 单测 | cron 24 h 逻辑 mock time |
| Policy Engine 集中 | ✅ | `policy_test` | L4 entity write 也应走 Engine |
| AutoMemory 队列 Wire | ✅ 显式 `MemoryJobQueue` | drain test | 满队列 drop 行为 |

---

## 8. 关键代码引用

### 8.1 注入总线（BeforeModel）

```39:62:internal/agent/memory_inject.go
func newMemoryInjectBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled || !policy.AnyInject() {
		return nil
	}
	// ...
	return callbacks.NewBeforeModelHook(5, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		// ...
		cue := buildRuntimeMemoryCue(ctx, deps, ag, args.Request.Messages)
		// ...
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
```

### 8.2 L3 fused recall 多 scope 合并

```140:176:internal/biz/memory_l3_fused_recall.go
	var merged []RecallHit
	for _, target := range L3ScopeTargets(q.Runtime, q.Scopes) {
		hits, err := uc.scored.RecallL3Hits(ctx, target.ScopeType, target.ScopeID, strings.TrimSpace(q.Runtime.UserID), query, qvec, perScope)
		// ...
		merged = append(merged, hits...)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Scores.Total > merged[j].Scores.Total
	})
```

### 8.3 `MemoryJobQueue` silent drop（MEM-Q-02）

```51:75:internal/memory/trpc/auto_memory_queue.go
func (q *MemoryJobQueue) Enqueue(r AutoMemoryJobRequest) {
	if q == nil {
		return
	}
	if r.EnqueuedAt.IsZero() {
		r.EnqueuedAt = time.Now()
	}
	if sid := strings.TrimSpace(r.SessionID); sid != "" {
		if t, ok := q.recent.Load(sid); ok {
			if time.Since(t.(time.Time)) < q.debounce {
				q.debounced.Add(1)
				return
			}
		}
		q.recent.Store(sid, time.Now())
	}
	select {
	case q.ch <- r:
	default:
		n := q.dropped.Add(1)
		if n%100 == 1 {
			slog.Warn("auto-memory queue full, job dropped", "dropped", n, "session_id", r.SessionID)
		}
	}
}
```

### 8.4 Cascade Approve 静默 index sync（MEM-Q-01）

```207:218:internal/biz/memory_l4_cascade.go
	if oldName != "" && newName != "" {
		updatedRows, _, err := uc.store.ReplaceNameInAgentFacts(ctx, agentID, oldName, newName)
		if err != nil {
			return nil, err
		}
		if uc.indexSync != nil {
			for _, raw := range updatedRows {
				_ = uc.indexSync.SyncFactIndexFromRow(ctx, raw)
			}
		}
	}
	return uc.store.UpdateCascadeProposalStatus(ctx, id, "applied", reviewer, "")
```

### 8.5 MEM-R02：L3 已有则 skip L4

```289:297:internal/cronrunner/jobs/auto_memory.go
	if memoryPolicy.WriteL4Graph && w.l4 != nil && agentID != "" {
		for _, msg := range msgs {
			if msg.Role != "user" {
				continue
			}
			if _, skip := msgIDsWithL3[msg.ID]; skip {
				continue
			}
```

### 8.6 Policy strict 事务阻断

```52:73:internal/data/sessionmemory/store_policy.go
func (st *Store) recordPolicyOnTx(ctx context.Context, db sqlRunner, in MemoryActionLogInsert) error {
	// ...
	err := st.insertMemoryActionLogOn(ctx, db, in)
	if err == nil {
		return nil
	}
	if st.policy != nil && st.policy.StrictEnabled(ctx) {
		return err
	}
	return nil
}
```

### 8.7 PII 写路径

```65:69:internal/data/sessionmemory/store_writes.go
	if pii := biz.ScanPII(stmt); pii.PIIFlag {
		in.PIIFlag = true
		if rs := strings.TrimSpace(pii.RedactedStatement); rs != "" {
			in.RedactedStatement = rs
		}
	}
```

### 8.8 `SearchMemories` pgvector 优先（MEM-Q-04）

```143:174:internal/memory/trpc/sqlite_adapter.go
	if q != "" && s.vector != nil {
		hits, err := s.vector.RecallWithUser(ctx, uk.AppName, uk.UserID, q, int(topK))
		if err == nil && len(hits) > 0 {
			// ... build entries from pgvector ...
			if len(out) > 0 {
				return out, nil
			}
		}
	}
```

### 8.9 runtime 装配点（框架边界）

```9:17:internal/runtime/memory_set.go
type MemorySet struct {
	TRPC            trpcmemory.Service
	Admin           biz.SessionAdminStore
	L2Recall        biz.MemoryL2Recaller
	L3Recall        biz.MemoryL3Recaller
	CompositeRecall biz.MemoryCompositeRecaller
}
```

### 8.10 PolicyVersion 字面量分裂（MEM-Q-06）

```38:43:internal/data/sessionmemory/store_writes.go
	if err := st.recordPolicyBestEffort(ctx, MemoryActionLogInsert{
		Action:        "UPSERT",
		TargetKind:    "fact",
		TargetID:      factIDFromRow(raw, in.ID),
		Reason:        strings.TrimSpace(in.SourceKind),
		PolicyVersion: "consolidate_v1",
```

对比 `store_consolidate_batch.go` 使用 `biz.PolicyVersionConsolidateV1`。

---

## 9. 验证命令

```bash
# biz + data memory 单测
go test ./internal/biz/... -run 'Memory|Cascade|Consolidator|Policy|PII|Recall' -count=1

# sessionmemory recall / consolidate
go test ./internal/data/sessionmemory/... -count=1

# auto_memory 集成
go test ./internal/cronrunner/jobs/... -run AutoMemory -count=1 -v

# service / agent 注入
go test ./internal/service/... ./internal/agent/... -run 'Memory|Recall|Composite|L3' -count=1

# race（建议 CI 开启）
go test ./internal/memory/trpc/... ./internal/cronrunner/jobs/... -race -count=1

# 架构红线
make runtime-boundary

# 全量
go build ./...
```

---

## 10. 总结

- **五层模型落地完整**：SQLite `sessionmemory` 为产品真相源；L2 / L3 recall 含 keyword / vector / importance / decay / CE rerank；L3 / L2 / Composite 运行时注入（MEM-R01 / 01b）已闭合。
- **架构边界干净**：biz 无 trpc import；`runtime.MemorySet` + Wire 符合 `trpc-agent-framework-first` 规则；端口化程度高，可测性优秀。
- **主要技术债集中在四处**：
  1. **静默失败链**：Cascade index sync、L4 upsert、`MemoryJobQueue` drop 均以 `_ =` / `每 100 次 warn` 处理；
  2. **L4 decay 无独立 cron**，与 L2 / L3 已有 24 h worker 形成落差；
  3. **字符串常量分裂**（PolicyVersion / cascade status），易导致 audit 过滤漏判一类；
  4. **读路径双轨**（`SearchMemories` pgvector 优先）在 index 延迟时返回 stale。
- **建议本迭代**：解决 MEM-Q-01..05（index sync 错误处理、队列可观测、L4 decay cron、Search 一致性、L4 entity 走 Engine）；下一迭代统一常量 + 拆 `extract()` + 补 Cascade / strict 集成测。
