# Memory 业务逻辑优化方案（2026-05-26）

> **关联**：[`memory.md`](./memory.md) · [`memory.design.md`](./memory.design.md) · [`memory-development.md`](./memory-development.md) · 代码 Review [2026-05-26-Memory-Code-Review](../../review/2026-05-26-Memory-Code-Review.md)
> **作者范围**：本文聚焦 **业务逻辑** 层面（用户/Agent 实际感受得到的能力差异）的优化。仅涉及代码结构、命名、格式的"代码质量"问题已收敛在 review 文档 P3，本文不重复。
> **状态**：🟢 Sprint A 已落地（MEM-OPT-01 Phase 0–3 + MEM-OPT-03 优先级/Dead-Letter/Replay） · 🟢 Sprint B 已落地（MEM-OPT-02 Worker + 强化因子） · 🟢 Sprint C 已落地（Dead-Letter 前端 + PII block 模式） · 🟢 Sprint D 已落地（MEM-OPT-06 Saga + Dry-Run + 前端 Cascade Tab 升级 + L4 置信度/衰减 UI + PII 策略配置 UI）

---

## 0. 背景

完成 L0–L4 五层模型与 Cascade / Decay / Worker / Recall 基础设施落地后（详见 `memory-development.md`），从业务可观测角度仍存在 **6 项业务正确性 / 用户体验** 缺陷。本方案给出可执行设计，按优先级分级：

| 编号 | 主题 | 业务问题（用户视角） | 优先级 |
|------|------|--------------------|--------|
| MEM-OPT-01 | **L3 双轨读一致性策略** | Cascade Approve / Fact 更新后，Agent 可能在数分钟内仍引用旧统计或旧人名 → 信任损失 | **P1** |
| MEM-OPT-02 | **L4 实体衰减全局调度 + 强化因子** | 长期不活跃 entity 永不衰减 → 图谱越久越脏 → recall 引入噪声 | **P1** |
| MEM-OPT-03 | **AutoMemoryQueue 优先级 / 容量隔离 / Dead-Letter** | 高负载 session 永久失忆且运维不可见 → "为什么 Agent 突然忘了我？" 无解 | **P1** |
| MEM-OPT-04 | **PII 分级处理（redact / block / review）** | 当前只 redact 不阻断，合规场景无法拒写或人工审核 | **P2** |
| MEM-OPT-05 | **提取协议结构化（function call schema）** | LLM 输出格式漂移即失效，heuristic regex 误伤 → 错记/漏记 | **P2** |
| MEM-OPT-06 | **Cascade Saga 化 + Dry-Run 预览** | Approve 黑盒一键执行；中间步骤失败用户感知差，无回滚 | **P2** |

---

## 1. MEM-OPT-01：L3 双轨读一致性策略

### 1.1 现状与业务问题

| 路径 | 现状 |
|------|------|
| **写** | `memory_facts`（权威）→ pgvector + `embedding_blob`（best-effort）；Cascade Approve 中索引 sync 用 `_ =` 静默 |
| **读** | `trpc.SearchMemories` 优先 pgvector，命中即返回；recall_l3 走 SQLite scored；Cue 注入用 `RecallL3Facts`（SQLite）但 tool search 走 pgvector |

**业务后果**（用户视角）：
1. 用户在 Memory Center 把 entity `"张总"` 改名为 `"张某某"` → Cascade Approve 成功 → 1 分钟后 Agent 主动谈论该人时**仍说"张总"**（pgvector 命中旧 statement）。
2. 用户 reject 一条 fact → SQLite 标 `archived` → pgvector 仍可被 Search 命中 → tool memory_search 返回该条 → Agent 错引。
3. 双写失败时无任何业务告警，索引漂移悄然累积。

### 1.2 设计方案

#### 1.2.1 引入 Fact 索引一致性等级

`memory_facts` 增加列：

```sql
ALTER TABLE memory_facts ADD COLUMN index_status TEXT
  NOT NULL DEFAULT 'fresh'
  CHECK(index_status IN ('fresh','stale','rebuilding','disabled'));
ALTER TABLE memory_facts ADD COLUMN index_synced_at INTEGER; -- unix ms
ALTER TABLE memory_facts ADD COLUMN index_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_facts ADD COLUMN index_last_error TEXT;
```

| 状态 | 含义 | 触发 |
|------|------|------|
| `fresh` | 索引与 SQLite 一致 | 同步 sync 成功 |
| `stale` | 索引可能旧 | 任意写后 sync 失败、Cascade rename 后 sync 失败 |
| `rebuilding` | 异步 reconciler 处理中 | Reconciler 拿锁后置位 |
| `disabled` | 跳过该 fact 的向量检索（黑名单） | 永久错误 / 用户隐私要求 |

#### 1.2.2 写路径错误处理升级

`MemoryFactIndexSyncer` 全部调用点（含 Cascade `Approve`、`UpsertFactRow`、L4 rename、archive）改为：

```go
if err := indexSync.SyncFactIndexFromRow(ctx, raw); err != nil {
    _ = store.MarkFactIndexStatus(ctx, factID, "stale", err.Error())
    event.SessionSysLogWarn(ctx, "", "memory.index_sync_fail", err.Error(), ...)
    metric.MemoryIndexSyncFail.Inc()
    // 业务流不阻断，但状态写入
}
```

#### 1.2.3 读路径一致性校验

`internal/memory/trpc/sqlite_adapter.go::SearchMemories`：

```text
向量命中 hits = pgvector.Recall(q, k*2)            // 多取一倍
for each hit:
    row = sqlite.GetFact(hit.fact_id)
    if row == nil OR row.status != 'active' OR row.index_status != 'fresh':
        skip 并 enqueue index_reconcile(fact_id)
    else if row.statement != hit.statement:        // 统计冲突
        replace hit.statement with row.statement
        enqueue index_reconcile(fact_id)
return top-K from filtered
```

代价：每次 pgvector 命中多一次 SQLite point lookup（已有 `embedding_blob`，可避免额外 query）。容量足够。

#### 1.2.4 离线 Reconciler

新增 `MemoryFactIndexReconciler`（cronrunner）：

| 配置 | 默认 |
|------|------|
| 周期 | 6 h（与 episode backfill 对齐） |
| 批大小 | 200 facts / 轮 |
| 选择条件 | `index_status='stale' AND index_attempts < 5` |
| 退避 | 第 N 次失败后 wait = 2^N minutes |
| 永久失败 | 5 次后置 `disabled` 并发 `memory.index_disabled` 事件 |

#### 1.2.5 用户可见性

- `MemoryWorkerStatus` RPC 新增字段：`fact_index_stale_count` / `fact_index_disabled_count`。
- Memory Center → 设置 Tab 增加"索引健康度"卡片：fresh / stale / disabled 占比 + 最近 reconcile 时间。

### 1.3 迁移路径

1. **Phase 0**：DDL 加列默认 `fresh`，不影响存量。 ✅
2. **Phase 1**：写路径错误捕获 + 状态标记上线（行为兼容）。 ✅
3. **Phase 2**：读路径校验启用 feature flag `MEMORY_READ_CONSISTENCY_CHECK=1`，灰度。 ✅
4. **Phase 3**：Reconciler cron 启用；feature flag 默认开。 ✅

**实施代码锚点**：

| 文件 | 说明 |
|------|------|
| `internal/data/sql/memory_chain.sql` | DDL: `index_status`, `index_synced_at`, `index_attempts`, `index_last_error` 列 |
| `internal/data/memory_fact_index_sync.go` | Phase 1: `MarkFactIndexStale` / `MarkFactIndexSynced` |
| `internal/memory/trpc/sqlite_adapter.go` | Phase 2: `SearchMemories` 读路径一致性校验（feature flag 控制） |
| `internal/data/sessionmemory/store_fact_embedding.go` | Phase 2: `GetFactConsistencyRow` / `CountFactsByIndexStatus` |
| `internal/cronrunner/jobs/memory_fact_index_reconciler.go` | Phase 3: 定期 Reconciler（6h 周期，max 5 次后 disabled） |
| `internal/service/memory_recall.go` | `GetMemoryWorkerStatus` 扩展 `fact_index_stale_count` / `fact_index_disabled_count` |

### 1.4 验收标准

| 指标 | 目标 |
|------|------|
| Cascade Approve 后向量索引收敛时延 | ≤ 15 s（同步成功）/ ≤ 6 h（同步失败 + reconciler） |
| `index_status=stale` 占比 | < 0.5%（稳态） |
| 命中 stale 的 SearchMemories 自动修正率 | 100%（不返回 stale 内容） |
| 集成测：`TestCascadeApprove_PGVectorEventuallyConsistent` | ✅ |

---

## 2. MEM-OPT-02：L4 实体衰减全局调度 + 强化因子

### 2.1 现状与业务问题

| 现状 | 落差 |
|------|------|
| `L4GraphUsecase.RunDecay` 仅在 `auto_memory.extract()` **写图后**调用 | 长期无写入的 agent → entity 永不衰减 |
| 衰减系数 0.92 / 30d 硬编码 | 不同业务（短期记忆型 chat vs 长期 persona）需要不同曲线 |
| confidence 仅按时间单调衰减 | 用户反复确认的 entity 与冷僻 entity 衰减速率相同 |

**业务后果**：用户半年前提到一次 `"对接人小李"`，半年没提 → 仍以 confidence=0.8 注入 prompt，Agent 主动联想造成困扰；反之高频确认的核心人物（伴侣、上司）confidence 不被强化。

### 2.2 设计方案

#### 2.2.1 独立 `MemoryL4DecayWorker`

新增 `internal/cronrunner/jobs/memory_l4_decay.go`：

| 参数 | 默认 | 来源 |
|------|------|------|
| 周期 | 24 h | env `MEMORY_L4_DECAY_INTERVAL_HOURS` |
| 启用 | true | env `MEMORY_L4_DECAY_DISABLED` |
| 批大小 | 500 entities / 轮 | const |
| 目标范围 | 所有 active agent | `agent_memory_maintenance.go` 列表 |

调度逻辑参照 `MemoryL3DecayWorker`：runOnce + ticker，AfterStart 触发首次。

#### 2.2.2 业务化置信度模型

新置信度公式：

```text
confidence_new = clip(
    base_confidence × time_decay × reinforcement_factor × user_factor,
    min=0.05,
    max=1.0
)

time_decay = exp(-λ × Δdays)                   λ = ln(2) / half_life_days
reinforcement_factor = 1 + α × log(1 + recent_hits / 7)   α ∈ [0, 0.3]
user_factor:
    confirmed (👍) → 1.10
    refuted   (👎) → 0.40
    edited        → 1.05
    no signal     → 1.00
```

`half_life_days` 按 entity_type 分档：

| entity_type | half_life | 解释 |
|-------------|-----------|------|
| `person` (核心：family/colleague) | 180 | 长期人物记忆 |
| `person` (其他) | 60 | 偶发提及 |
| `place` | 365 | 地点稳定 |
| `preference` | 90 | 偏好变化适中 |
| `event` | 14 | 事件时效性强 |
| `concept` | 270 | 知识型 |

均可被 `agent.memory_settings.l4_decay_overrides` 覆盖。

#### 2.2.3 衰减触发后的业务动作

| confidence | 行为 |
|------------|------|
| ≥ 0.6 | 正常 cue 注入 |
| 0.3 ~ 0.6 | cue 注入但标 `tentative`（提示 Agent 用"似乎/记得不准确请提醒"语气） |
| 0.1 ~ 0.3 | 仅在被显式 query 时 recall，不主动注入 |
| < 0.1 | 自动归档到 `entities_archived` 表，从 graph 移除；保留 30d 可恢复 |

#### 2.2.4 强化反馈闭环

`L4GraphRepo` 新增方法：

```go
RecordEntityReinforcement(ctx, entityID string, signal ReinforcementSignal) error
```

调用点：
- `L4MemoryCue` 注入命中且本轮 user 未否认 → `signal=hit`
- 用户 feedback 👍 涉及某 entity → `signal=confirmed`
- 用户 feedback 👎 / 编辑 → `signal=refuted/edited`

入库：`entity_reinforcements(entity_id, signal, occurred_at)`，Worker 评估 `recent_hits = COUNT WHERE occurred_at > now-7d`.

### 2.3 迁移路径

1. **Phase 0**：DDL `entity_reinforcements` 表 + `agent_runtime_settings` 新列（`l4_decay_interval_hours` / `l4_decay_overrides_json`）。 ✅
2. **Phase 1**：Biz 层类型定义（`ReinforcementSignal` / `L4DecayConfig` / `L4DecayResult`）+ `L4GraphRepo` / `L4GraphWriter` 接口扩展。 ✅
3. **Phase 2**：Data 层实现（`RecordEntityReinforcement` / `GetRecentReinforcementCounts` / `ApplyBusinessConfidenceDecay` / `ArchiveLowConfidenceEntities`）。 ✅
4. **Phase 3**：Usecase 层 `RunDecayWithConfig` 业务化衰减模型 + `RecordEntityReinforcement`。 ✅
5. **Phase 4**：Worker 层 `MemoryL4DecayWorker` 增强（`L4DecayConfig` / `MEMORY_L4_DECAY_INTERVAL_HOURS` / 统计 decayed/archived）。 ✅
6. **Phase 5**：Cue 层 `L4MemoryCue` confidence 分级注入（`< 0.3` 过滤 / `0.3~0.6` tentative / `≥ 0.6` 正常）。 ✅
7. **Phase 6**：配置层透传（`MemoryCfg` / `AgentRuntimeSettings` / `MemoryRuntimePolicy` / `AgentMemoryMaintenanceTarget`）。 ✅

**实施代码锚点**：

| 文件 | 说明 |
|------|------|
| `docs/sql/10_memory_l4_reinforcements.sql` | DDL: `entity_reinforcements` 表 |
| `internal/data/memory_l4_reinforcements_patch.go` | Go 迁移代码 |
| `internal/biz/memory_l4.go` | `ReinforcementSignal` / `L4DecayConfig` / `L4DecayResult` + 接口扩展 |
| `internal/data/sessionmemory/entity_lookup.go` | `RecordEntityReinforcement` / `ApplyBusinessConfidenceDecay` / `ArchiveLowConfidenceEntities` |
| `internal/data/memory_l4.go` | `l4GraphRepo` 桥接 + `l4GraphWriterAdapter` 扩展 |
| `internal/biz/memory_l4_usecase.go` | `RunDecayWithConfig` / `RecordEntityReinforcement` |
| `internal/cronrunner/jobs/memory_l4_decay.go` | Worker 增强：`L4DecayConfig` + 环境变量 + 统计 |
| `internal/agent/l4_prompt.go` | confidence 分级注入 + tentative 标记 |
| `internal/data/agent_runtime_patch.go` | DDL patch: `l4_decay_interval_hours` / `l4_decay_overrides_json` |

### 2.4 验收标准

| 指标 | 目标 |
|------|------|
| 长期不活跃（>180 天）`person` entity confidence | ≤ 0.4 |
| 高频确认 entity confidence | 稳定在 0.85+ |
| 自动归档比例 | 6 个月稳态 < 15% |
| 单测 `TestL4Decay_HalfLifeAndReinforcement` | ✅ |
| Memory Center 图谱 Tab 展示 confidence 分布柱图 | ✅ |

---

## 3. MEM-OPT-03：AutoMemoryQueue 优先级 / 容量隔离 / Dead-Letter

### 3.1 现状与业务问题

| 现状 | 落差 |
|------|------|
| 队列容量 256，单 channel，FIFO + 30 s debounce | 单租户高峰挤占所有 slot |
| 满则**静默 drop**，每 100 次 warn 一次 | 用户感受："Agent 莫名失忆"，运维感受："看不见" |
| 无 dead-letter，丢失 job 不可补 | 数据永久丢失 |
| Worker 状态 RPC 仅返回 done/fallback/dead 计数，无 dropped/debounced | 运维不可观测 |

### 3.2 设计方案

#### 3.2.1 优先级队列

替换 `MemoryJobQueue` 单 channel 为三优先级：

| 优先级 | 来源 | 默认容量 |
|--------|------|----------|
| **High** | 用户反馈触发（`FeedbackMemoryEnqueuer`）；偏好提取 | 64 |
| **Normal** | Runner turn 完成（`AutoMemoryEnqueuer`） | 256 |
| **Low** | Episode backfill / 迁移 reconcile | 128 |

Drain 顺序：High → Normal → Low（每轮最多 N 个，避免饿死 Low）。

#### 3.2.2 租户配额隔离

`MemoryJobQueue` 引入 token bucket per `tenant_id`（默认 1 tenant = workspace）：

| 配置 | 默认 |
|------|------|
| 单租户最大并发 in-flight | 32 |
| 单租户单优先级槽位 | 128（占总容量 50%） |

超出配额 → 直接进 Dead-Letter（不阻塞 channel）。

#### 3.2.3 Dead-Letter 表 + Replay

新表 `memory_job_deadletter`：

```sql
CREATE TABLE memory_job_deadletter (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    enqueued_at INTEGER NOT NULL,
    failed_at INTEGER NOT NULL,
    session_id TEXT,
    app_name TEXT,
    user_id TEXT,
    feedback_message_id TEXT,
    payload_json TEXT,
    drop_reason TEXT NOT NULL,    -- queue_full | quota_exceeded | extract_failed_terminal
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','replayed','abandoned'))
);
CREATE INDEX idx_memory_job_dl_state_enq ON memory_job_deadletter(state, enqueued_at);
```

**API**：
- `ListMemoryDeadLetters` RPC：分页 + 过滤 ✅
- `ReplayMemoryDeadLetter(id)`：重入 normal 队列 ✅（通过 `ReplayDeadLetterIntoQueue` 真正重入队）
- `AbandonMemoryDeadLetter(id, reason)`：永久放弃 ✅
- 自动重放：cron `MemoryDeadLetterReplayer`，30 min 一次 ✅

**实施代码锚点**：

| 文件 | 说明 |
|------|------|
| `internal/data/memory_job_deadletter.go` | Dead-Letter 表 + CRUD + `ReplayDeadLetterIntoQueue` |
| `internal/biz/memory_queue_contract.go` | `MemoryDeadLetterSink` 接口 + `MemoryDeadLetterRequest` |
| `internal/memory/trpc/auto_memory_queue.go` | 三优先级队列 + 租户配额 + `writeDeadLetter` |
| `internal/service/memory_deadletter.go` | List/Replay/Abandon RPC |
| `internal/cronrunner/jobs/memory_dead_letter_replayer.go` | 自动重放 cron（30min） |
| `api/kratos/memory/v1/memory.proto` | `MemoryDeadLetterEntry` + 3 RPC + `QueueStats` |

#### 3.2.4 队列可观测

`MemoryWorkerStatus` proto 扩展：

```proto
message MemoryWorkerStatus {
  // 现有字段保留 ...
  message QueueStats {
    int64 capacity = 1;
    int64 in_flight = 2;
    int64 dropped_total = 3;
    int64 debounced_total = 4;
    int64 dead_letter_pending = 5;
    map<string, int64> per_tenant_in_flight = 6;
  }
  QueueStats queue_high = 10;
  QueueStats queue_normal = 11;
  QueueStats queue_low = 12;
  int64 oldest_pending_age_ms = 20;
}
```

Memory Center 设置 Tab → "记忆 Worker 健康度"卡片：实时显示队列占用、dead-letter 数、oldest pending。

#### 3.2.5 用户可见的 Toast

当某 session 超过 5 次 `quota_exceeded` 或 `queue_full`：
- 在 Chat 侧栏顶部展示一次性提示：「记忆系统繁忙，本次对话的部分记忆可能延迟写入。已加入重试队列。」
- 自动重放成功后清除。

### 3.3 验收标准

| 指标 | 目标 |
|------|------|
| 单租户 DOS 场景下其他租户 enqueue 成功率 | ≥ 95% |
| Dropped 事件 100% 进入 dead-letter（不再 silent drop） | ✅ |
| Dead-letter 自动 replay 成功率 | ≥ 80% |
| 集成测 `TestMemoryQueue_TenantIsolation_DeadLetter_Replay` | ✅ |

---

## 4. MEM-OPT-04：PII 分级处理（redact / block / review）

### 4.1 现状与业务问题

`biz.ScanPII` 仅做 redact（在 `redacted_statement` 字段保留脱敏版），**原文仍写入 `memory_facts.statement`**。问题：

| 场景 | 当前 | 期望 |
|------|------|------|
| 金融/医疗合规场景 | 写入原文 → 合规审计不通过 | 应拒绝写入（block） |
| 不确定是否敏感 | 用户/admin 无法干预 | 应进人工审核队列（review） |
| 信任租户（默认） | 写入原文 + redact | 现状（redact） |

### 4.2 设计方案

#### 4.2.1 PII Policy 三档

`MemoryPolicyEngine` 新增 PII 子策略，来源优先级：

```text
agent.memory_settings.pii_action            (per-agent)
    ↓ fallback
workspace_settings.memory_pii_action        (per-workspace)
    ↓ fallback
env MEMORY_PII_ACTION_DEFAULT               (deploy default)
    ↓ fallback
"redact"                                    (code default)
```

| 取值 | 行为 |
|------|------|
| `redact` | 当前行为：写原文 + `redacted_statement`（兼容默认） |
| `block` | 拒写；ActionLog 记 `BLOCKED_PII`；返回 caller error；发 `memory.pii_blocked` 事件 |
| `review` | 写到 `memory_facts_pending_review` 表，等 admin Approve；事件 `memory.pii_pending` |

#### 4.2.2 Pending Review 工作流

新表 `memory_facts_pending_review`：

```sql
CREATE TABLE memory_facts_pending_review (
    id TEXT PRIMARY KEY,
    proposed_at INTEGER NOT NULL,
    agent_id TEXT, user_id TEXT, session_id TEXT,
    statement TEXT NOT NULL,
    redacted_statement TEXT,
    pii_types TEXT,                -- JSON array
    source_kind TEXT,
    source_message_id TEXT,
    proposer TEXT,                  -- consolidator | feedback | admin
    state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','approved','rejected')),
    reviewer TEXT, reviewed_at INTEGER, reason TEXT
);
```

**API**：
- `ListPendingPIIFacts` / `ApprovePendingPIIFact(id, edit?: statement)` / `RejectPendingPIIFact(id, reason)`
- Approve → 走标准 `UpsertFactRow` 路径
- Reject → 状态置 `rejected`，发 `memory.pii_rejected` 事件

#### 4.2.3 用户可见性

- Memory Center 新一级 Tab "**PII 审核**"（仅 admin 可见）
- 普通用户：在 Chat 中触发 PII block → 一次性提示「检测到敏感信息，未存入记忆」并提供"我授权写入"按钮（一次性升级该条到 redact 模式）

#### 4.2.4 类型扩展

`ScanPII` 当前仅 email / phone / id / card。补充：

| 类型 | 正则 / 启发 | 示例 |
|------|-------------|------|
| `bank_account` | `\b\d{16,19}\b` + Luhn | 银行卡 |
| `ssn_like` | `\b\d{3}-\d{2}-\d{4}\b` | 美国 SSN |
| `medical_record` | 医疗号正则 / 字典 | 病历号 |
| `home_address` | 地址词典 + NER（如启用 LLM） | "某某市某区某街..." |
| `secret_key` | `(sk-|pk_|AKIA)[\w-]{16,}` | API key 漏写 |

### 4.3 验收标准

| 指标 | 目标 |
|------|------|
| Block 模式下 PII 写入率 | 0%（含间接路径） |
| Review 模式下 admin 审核延迟（中位） | ≤ 24 h |
| 集成测 `TestPIIAction_Block_Review_Redact` | ✅ |
| 合规模式（block）开关后 30 天内零 PII 数据回流 | ✅ |

---

## 5. MEM-OPT-05：提取协议结构化（function call schema）

### 5.1 现状与业务问题

| 路径 | 当前 | 风险 |
|------|------|------|
| `MemoryLLMExtractor` | prompt 要求 LLM 返回 JSON 文本，正则解析 | 模型 update → JSON 包裹改变 → 解析失败 → 全部 fallback heuristic |
| Heuristic | `^我叫\s*(\S+)` 之类正则 | 中英混杂、口语化、错记（"我叫人去做了"误识为名字"人"） |
| `critic_loop` | 期望末轮 LLM 文案含 `approved` | 文案漂移导致永远 retry |

**业务后果**：用户在客服场景反映"我跟它说了三次我叫王五，它还是叫我先生" → 实际是提取失败但无人察觉。

### 5.2 设计方案

#### 5.2.1 强制 Function Call Schema

LLM 提取调用一律走 tool/function call：

```json
{
  "name": "extract_memory_facts",
  "description": "Extract durable user/agent facts from the conversation turn",
  "parameters": {
    "type": "object",
    "required": ["facts"],
    "properties": {
      "facts": {
        "type": "array",
        "items": {
          "type": "object",
          "required": ["statement", "scope", "confidence"],
          "properties": {
            "statement": {"type": "string", "minLength": 4, "maxLength": 280},
            "scope": {"enum": ["user","agent","session","team","workspace"]},
            "subject_type": {"enum": ["person","preference","fact","event","skill","other"]},
            "confidence": {"type": "number", "minimum": 0, "maximum": 1},
            "ttl_hint_days": {"type": "integer", "minimum": 1, "maximum": 3650},
            "evidence_message_id": {"type": "string"},
            "is_pii_sensitive": {"type": "boolean"}
          }
        }
      },
      "no_facts_reason": {
        "type": "string",
        "description": "If facts is empty, briefly explain why"
      }
    }
  }
}
```

#### 5.2.2 多 Provider 兼容

- **OpenAI / 兼容**：tools + tool_choice=required
- **不支持 function call 的 provider**：fallback 到 "JSON mode" + JSON schema 校验
- **完全裸文本 provider**：fallback heuristic + 显式标记 `extraction_quality='low'`

#### 5.2.3 提取质量评分

每条 fact 入库时计算：

```text
quality = base(source) × confidence × evidence_factor × user_signal

base(LLM_function_call)     = 1.0
base(LLM_json_mode)         = 0.85
base(heuristic_regex)       = 0.50
base(user_explicit_pin)     = 1.10

evidence_factor:
    has evidence_message_id → 1.0
    no evidence             → 0.8
```

`memory_facts.quality_score` 持久化，recall rerank 加入权重。

#### 5.2.4 替换 `critic_loop` 字符串协议

新增 `critic_decision` tool：

```json
{
  "name": "critic_decision",
  "parameters": {
    "type": "object",
    "required": ["decision"],
    "properties": {
      "decision": {"enum": ["approved","retry","escalate","abort"]},
      "score": {"type": "number"},
      "notes": {"type": "string"}
    }
  }
}
```

`graph_compile.go::critic_loop` 改为读 tool 调用结果（不再 substring 匹配 `"approved"`）。

#### 5.2.5 提取失败可观测

新事件：

| event_key | 触发 |
|-----------|------|
| `memory.extract_failed` | LLM 调用失败 / schema 校验失败 |
| `memory.extract_fallback` | 降级到 heuristic |
| `memory.extract_empty_with_reason` | LLM 返回 0 facts 但给了 `no_facts_reason` |

Monitor → Events Tab 可过滤这些 key。

### 5.3 迁移路径

1. **Phase 0**：tool schema 定义 + 双轨提取（schema 优先，旧 prompt 兼容）。
2. **Phase 1**：观察 `extract_quality` 分布 2 周。
3. **Phase 2**：default `extraction_mode=function_call`，旧文本路径仅兼容。
4. **Phase 3**：移除文本 JSON 解析代码。

### 5.4 验收标准

| 指标 | 目标 |
|------|------|
| LLM 提取解析成功率 | ≥ 99.5% |
| Heuristic fallback 占比（稳态） | ≤ 5% |
| `extraction_quality < 0.6` 的 fact 不进入 prompt 主路径 | ✅ |
| critic_loop 进入 retry 由字符串解析触发 | 0% |

---

## 6. MEM-OPT-06：Cascade Saga 化 + Dry-Run 预览

### 6.1 现状与业务问题

`L4CascadeUsecase.Approve` 内部线性执行：
1. `UpsertEntity(newName)`
2. `touchAffectedEntities`
3. `ReplaceNameInAgentFacts`
4. `SyncFactIndexFromRow`（`_ =` 静默）
5. `UpdateCascadeProposalStatus`

任意一步失败：
- 部分步骤已落，部分未落 → **数据不一致**且无回滚记录。
- 用户不知道哪些 fact 被改了。
- 重新 Approve 可能重复执行已成功的步骤。

### 6.2 设计方案

#### 6.2.1 Saga 步骤化

新表 `cascade_saga_steps`：

```sql
CREATE TABLE cascade_saga_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    proposal_id TEXT NOT NULL,
    step_index INTEGER NOT NULL,
    step_name TEXT NOT NULL,        -- upsert_entity | touch_affected | replace_facts | sync_index
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK(state IN ('pending','running','succeeded','failed','compensated','skipped')),
    attempts INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER, finished_at INTEGER,
    payload_json TEXT,              -- 该步骤输入
    result_json TEXT,               -- 该步骤产出（如更新的 fact_ids 列表）
    error TEXT,
    UNIQUE(proposal_id, step_index)
);
```

`Approve` 改为：

```text
1. 创建 saga 行（all pending）
2. for each step:
     mark running → execute → mark succeeded/failed
     if failed AND step.is_critical:
         enqueue compensation
         break
3. all succeeded → UpdateCascadeProposalStatus('applied')
   any failed     → UpdateCascadeProposalStatus('partial' | 'failed')
```

#### 6.2.2 补偿（Compensation）

| 步骤 | 补偿动作 |
|------|----------|
| `upsert_entity` | rename back 或 mark inactive |
| `touch_affected` | 重置 affected entity status |
| `replace_facts` | revert via `original_statement` 列（new！） |
| `sync_index` | 重新触发 reconciler，标 `stale` |

`memory_facts` 新增 `last_cascade_original_statement` 列保留可回滚版本（仅最近一次）。

#### 6.2.3 Dry-Run 预览

新 RPC `PreviewCascadeApprove(proposal_id) → CascadePreview`：

```protobuf
message CascadePreview {
  int32 affected_entities_count = 1;
  int32 affected_facts_count = 2;
  repeated FactDiff fact_diffs = 3;     // 最多 50 条
  repeated EntityRename entity_renames = 4;
  EstimatedImpact estimated_impact = 5;  // duration_ms / index_resync_count
}
message FactDiff {
  string fact_id = 1;
  string before_statement = 2;
  string after_statement = 3;
  string scope = 4;
}
```

UI：Approve 按钮改为 "**预览 → 确认 → 执行**" 两步。

#### 6.2.4 重入安全

`Approve` 接 `idempotency_key`（默认 `proposal_id`）；同 key 重复调用 → 返回已有 saga 状态而非重新执行。
失败的 saga 可调 `RetryCascadeApprove(proposal_id)` 从首个非 `succeeded` 步骤恢复。

#### 6.2.5 用户可见性

Memory Center → Cascade Tab：

- 历史列表加 "**详情**" 抽屉，展示 saga steps 状态 + 每步耗时 + 错误信息。
- Partial / failed 提案高亮，支持 Retry / Compensate / Abandon 三按钮。

### 6.3 验收标准

| 指标 | 目标 |
|------|------|
| Approve 中任一步失败的可恢复率 | ≥ 95%（不可逆失败 < 5%） |
| Dry-Run 预览与实际执行 fact diff 一致率 | ≥ 99% |
| 同 proposal_id 重复 Approve 不重复修改 facts | ✅ |
| 集成测 `TestCascadeSaga_PartialFail_RetrySuccess` | ✅ |

---

## 7. 跨方案的统一原则

| 原则 | 落地点 |
|------|--------|
| **任何 best-effort 写都要可观测** | OPT-01 index_status / OPT-03 dead-letter / OPT-05 extract event |
| **业务逻辑用结构化协议而非字符串** | OPT-05 function call / OPT-06 saga state |
| **用户对长期记忆有最终控制权**（呼应 `memory.md` §1.3.2） | OPT-04 PII review / OPT-06 dry-run / OPT-02 confidence 用户反馈强化 |
| **Multi-tenant 隔离要业务可见** | OPT-03 per-tenant quota + Stats RPC |
| **每个优化项可独立 ship、可回滚** | 所有 DDL 加列默认值；行为开关有 env flag |

---

## 8. 排期建议（参考）

| 迭代 | 内容 | 预估 | 状态 |
|------|------|------|------|
| Sprint A（2 周） | OPT-01 Phase 0–3（DDL + 写路径错误 + 读校验灰度 + Reconciler） + OPT-03 优先级队列 + Dead-Letter + Replay RPC + 自动重放 cron | M | ✅ 已落地 |
| Sprint B（2 周） | OPT-02 Worker + 强化因子（无 UI） | M | ✅ 已落地 |
| Sprint C（2 周） | OPT-05 function call schema 双轨 + OPT-04 PII block 模式 + Dead-Letter 前端 | M | ✅ 已落地 |
| Sprint D（3 周） | OPT-06 Saga + Dry-Run + 前端 Cascade Tab 升级 + L4 置信度/衰减 UI + PII 策略配置 UI | L | ✅ 已落地 |
| Sprint E（2 周） | OPT-04 PII review 工作流 + 前端审核 Tab + OPT-02 用户反馈强化打通 | M | 📐 待开始 |

---

## 9. 不在本方案范围

| 项 | 理由 |
|----|------|
| L0 / L1 / L2 模型变更 | 现有模型业务上稳定，本轮聚焦 L3 / L4 与队列 |
| 真实 Cross-Encoder rerank 模型 | 涉及模型服务部署，记入 `memory-development.md` backlog |
| pgvector 多租户深度隔离 | OPT-03 队列隔离已部分覆盖；多租户向量库另立专题 |
| Frontend 全面改版 | 仅描述新增/调整 Tab；详细 UI 见 `memory.md` 后续版本 |

---

## 10. 关联文档

- 业务总则：[`memory.md`](./memory.md)
- 当前实现设计：[`memory.design.md`](./memory.design.md)
- 进度跟踪（本方案落地后追加章节）：[`memory-development.md`](./memory-development.md)
- 代码 Review（问题来源）：[`2026-05-26-Memory-Code-Review.md`](../../review/2026-05-26-Memory-Code-Review.md)
- 历史 Review：[`memory-review.md`](../../review/memory-review.md)
