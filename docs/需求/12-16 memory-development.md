# Memory 记忆 — 开发计划

> **版本**：2026-05-18 | **状态**：🟡 L0-L3 部分实现；❌ L4 未实现；❌ MemoryWorker 未实现
> **需求**：[12 L0](./12%20memory-L0-sensory.md) · [13 L1](./13%20memory-L1-working.md) · [14 L2](./14%20memory-L2-episodic.md) · [15 L3](./15%20memory-L3-semantic.md) · [16 L4](./16%20memory-L4-persistent.md) · [38 知识体系](./38%20memory.md) · [31 UX](./31%20memery.md)
> **设计**：[12-16 memory.design.md](./12-16%20memory.design.md)（含原 supplement 内容）
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-04 / EP-BIZ-07 / EP-MEM-01 / EP-MEM-02

---

## 1. 模块定位

Agent 记忆系统：五层架构（L0 上下文压缩 / L1 工作记忆 / L2 情景记忆 / L3 语义记忆 / L4 持久进化记忆）+ MemoryWorker（后台记忆管理单元），为 Agent 提供跨会话的持久化记忆能力与记忆联动更新能力。

**代码锚点**：
- `internal/service/session_compress.go` — L0 上下文压缩
- `internal/biz/memory.go` — MemoryUsecase（L1/L2/L3）
- `internal/data/memory.go` — MemoryRepo
- `internal/agent/trpc_build.go` — Memory 装配（L1-L3）
- `internal/cronrunner/jobs/auto_memory.go` — AutoMemory 后台任务
- `internal/memory/trpc/sqlite_adapter.go` — trpc Memory 持久化
- `pkg/trpc-agent-go/memory/` — trpc-agent-go Memory 框架

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| L0 上下文压缩 | ✅ | `SessionCompressor.AfterNativeTurn` |
| L1 工作记忆（当前会话） | ✅ | trpc `WorkingMemory` |
| L2 情景记忆（最近 N 条） | ✅ | `MemoryUsecase` + Ent 表 |
| L3 语义记忆（向量检索） | ✅ | `MemoryUsecase` + Embedding + 向量搜索 |
| L4 知识图谱 | ❌ | `RuntimeSettings.L4Enabled` 字段存在但无实现 |
| L4 Agent 进化 | ❌ | `EvolutionScanner` / 30min ticker 代码不存在 |
| Memory 类型区分 | ✅ | observation / reflection / plan |
| 记忆注入 | ✅ | `BuildTRPCLLMAgent` 中 `WithMemory` |
| AutoMemory 后台任务 | ✅ | `internal/cronrunner/jobs/auto_memory.go`（extract 体为占位） |
| MemoryWorker（后台记忆管理） | ❌ | 不存在 |
| 记忆联动更新（级联复盘） | ❌ | 不存在 |
| 记忆衰减与遗忘 | ❌ | 不存在 |

---

## 3. 差距与优化

1. **P2（EP-BIZ-04）**：L4 知识图谱层完全未实现。`RuntimeSettings` 中有 `L4Enabled` / `L4GraphInjectNeighbors` / `L4GraphMaxNeighbors` / `L4GraphMaxHops` / `L4IdentityInject` / `L4StrategyInject` 字段，但无任何业务逻辑。
2. **P2（EP-BIZ-07）**：EvolutionScanner 代码不存在。L4 进化档案（AgentIdentity / StrategyProfile / EvolutionEvent / EvolutionProposal）仅 Proto 定义，无运行时实现。
3. **P2（EP-MEM-01）**：无 MemoryWorker 后台记忆管理单元。当前 AutoMemory Cron Job 仅有占位 extract 逻辑，缺少：turn 完成后异步提取、fact 冲突检测、级联更新检查。
4. **P2（EP-MEM-02）**：无记忆联动更新机制。当实体属性变更时（如工作地点从北京变为纽约），无法自动复盘并更新关联记忆（交通方式、天气、时区等）。L3 `superseded_by` 仅支持单条 fact 替换，不支持沿图谱关系的级联传播。
5. **P2**：L3 长期记忆的向量搜索质量未优化，无 re-ranking 机制。
6. **P3**：记忆无过期/衰减机制，长期运行后记忆库膨胀。
7. **P3**：记忆无"遗忘"策略，用户无法手动删除特定记忆。

---

## 4. 开发阶段

### Phase 1：L4 知识图谱基础（EP-BIZ-04）

L4 是 MemoryWorker 和级联更新的基础设施，必须先实现。

- L4 Graph Schema：`memory_entities` / `memory_relations` Ent 表
- L4 Graph Usecase：节点/边 CRUD + BFS 遍历 + 邻居查询
- L4 Graph 注入：`BuildTRPCLLMAgent` 中 `WithGraphMemory`
- L4 Agent 身份：`AgentIdentity` / `StrategyProfile` 读写

### Phase 2：MemoryWorker 后台记忆管理（EP-MEM-01）

在 L4 基础上实现后台记忆管理单元，替代当前 AutoMemory 占位逻辑。

- MemoryWorker goroutine：`safego.Go` 启动，EventBus 订阅 `turn.completed`
- Turn 完成后异步提取：从对话中提取 fact / episode / entity
- Fact 冲突检测：新 fact 与现有 fact 矛盾时标记冲突
- 巩固管道：L2 episode → L3 fact / L4 entity-relation

### Phase 3：记忆联动更新（EP-MEM-02）

在 MemoryWorker + L4 图谱基础上实现级联更新。

- 级联更新检查：实体属性变更时，BFS 遍历关联实体，生成 `CascadeCheckProposal`
- Proposal 审核：复用 L4 `EvolutionProposal` 机制，用户/Critic 确认后应用
- EvolutionScanner：30min ticker 扫描待处理 Proposal

### Phase 4：L3 检索质量优化

- L3 re-ranking：Cross-Encoder 重排
- 记忆衰减：基于时间的权重递减
- 记忆遗忘：用户手动删除 + 自动过期

### Phase 5：记忆衰减与遗忘

- 基于使用频率与时效的 confidence 衰减
- 用户手动删除 + 自动过期策略
- PII 过滤与隐私保护

---

## 5. 任务清单

### Phase 1：L4 知识图谱（EP-BIZ-04）

| # | 任务 | 优先级 | EP | 依赖 |
|---|------|--------|-----|------|
| 1.1 | L4 Graph Schema：`memory_entities` / `memory_relations` Ent 表 | P2 | EP-BIZ-04 | — |
| 1.2 | L4 Graph Repo：节点/边 CRUD + BFS 遍历 | P2 | EP-BIZ-04 | 1.1 |
| 1.3 | L4 Graph Usecase：邻居查询 + 图遍历 + 实体合并 | P2 | EP-BIZ-04 | 1.2 |
| 1.4 | L4 Graph 注入：`BuildTRPCLLMAgent` 中 `WithGraphMemory` | P2 | EP-BIZ-04 | 1.3 |
| 1.5 | L4 Agent 身份：`AgentIdentity` / `StrategyProfile` 读写 | P2 | EP-BIZ-07 | 1.1 |
| 1.6 | L4 EvolutionProposal：提议 CRUD + 审核 + 回滚 | P2 | EP-BIZ-07 | 1.5 |

### Phase 2：MemoryWorker（EP-MEM-01）

| # | 任务 | 优先级 | EP | 依赖 |
|---|------|--------|-----|------|
| 2.1 | MemoryWorker 启动：`safego.Go` + EventBus 订阅 `turn.completed` | P2 | EP-MEM-01 | — |
| 2.2 | Turn 完成后异步提取：fact / episode / entity 提取逻辑 | P2 | EP-MEM-01 | 2.1 |
| 2.3 | Fact 冲突检测：新 fact 与现有 fact 矛盾时标记冲突 | P2 | EP-MEM-01 | 2.2 |
| 2.4 | 巩固管道：L2 episode → L3 fact / L4 entity-relation | P2 | EP-MEM-01 | 2.2 + 1.3 |
| 2.5 | MemoryWorker 配置：`agent_runtime_settings` 新增 `memory_worker_enabled` / `memory_worker_extract_mode` 等 | P2 | EP-MEM-01 | 2.1 |

### Phase 3：记忆联动更新（EP-MEM-02）

| # | 任务 | 优先级 | EP | 依赖 |
|---|------|--------|-----|------|
| 3.1 | 级联更新检查：实体属性变更 → BFS 关联实体 → 生成 `CascadeCheckProposal` | P2 | EP-MEM-02 | 1.3 + 2.2 |
| 3.2 | Proposal 审核：级联更新走 `EvolutionProposal` 审核，用户/Critic 确认后应用 | P2 | EP-MEM-02 | 1.6 + 3.1 |
| 3.3 | EvolutionScanner：30min ticker 扫描待处理 Proposal | P2 | EP-BIZ-07 | 1.6 + 3.2 |
| 3.4 | 级联更新回滚：任意级联更新可一键回滚 | P3 | EP-MEM-02 | 3.2 |

### Phase 4：L3 检索质量

| # | 任务 | 优先级 | EP | 依赖 |
|---|------|--------|-----|------|
| 4.1 | L3 re-ranking：Cross-Encoder 重排 | P2 | — | — |
| 4.2 | L3 去重优化：文本指纹 + embedding 相似度 | P2 | — | — |

### Phase 5：记忆衰减与遗忘

| # | 任务 | 优先级 | EP | 依赖 |
|---|------|--------|-----|------|
| 5.1 | 记忆衰减：基于时间的 confidence 递减 | P3 | — | — |
| 5.2 | 记忆遗忘：用户手动删除 + 自动过期 | P3 | — | — |
| 5.3 | PII 过滤：写入前 PII 检测与脱敏 | P3 | — | — |

---

## 6. 验收标准

### Phase 1

- [ ] Agent 可使用 L4 知识图谱进行图遍历和邻居注入
- [ ] L4 实体/关系 CRUD 通过单元测试
- [ ] `go test ./internal/biz/... -run TestMemory` 通过

### Phase 2

- [ ] MemoryWorker 在 turn 完成后自动提取 fact / episode / entity
- [ ] 冲突 fact 被自动标记并等待仲裁
- [ ] L2 episode 可巩固为 L3 fact 和 L4 entity-relation

### Phase 3

- [ ] 实体属性变更时，关联记忆被自动检查并生成 Proposal
- [ ] Proposal 经用户/Critic 审核后应用，可回滚
- [ ] "北京→纽约"场景：工作地点变更后，交通/天气/时区等关联记忆生成更新 Proposal

### Phase 4

- [ ] L3 向量搜索结果经 re-ranking 后相关性提升
- [ ] 去重率 > 85%

### Phase 5

- [ ] 长期未使用记忆 confidence 自动衰减
- [ ] 用户可手动删除/归档记忆
- [ ] PII 敏感内容自动脱敏

---

## 7. 依赖与风险

| 依赖/风险 | 说明 | 缓解 |
|-----------|------|------|
| L4 图谱依赖图数据库 | SQLite 无原生图查询；需 BFS 遍历实现 | 先用 SQLite 邻接表 + 应用层 BFS；规模超 5 万节点时迁移 pgvector / Neo4j |
| 级联更新 LLM 调用成本 | 每次级联检查可能触发 LLM 调用判断关联性 | 限制 BFS 深度（≤ 2 跳）；批量检查减少调用次数；低置信度变更跳过 |
| 级联更新准确性 | LLM 判断"哪些关联记忆需要更新"可能出错 | 所有级联更新走 Proposal 审核，不直接写入；用户拥有最终控制权 |
| MemoryWorker 与 M2 多租户 | 后台处理必须携带 workspace context | 所有 MemoryWorker 操作显式注入 `workspace_id`；Ent Hook 强制隔离 |
| re-ranking 延迟 | Cross-Encoder 增加检索延迟 | 异步 re-ranking；仅对 top-K 候选重排 |
| AutoMemory 占位逻辑替换 | 现有 `auto_memory.go` extract 体为占位 | Phase 2 完成后替换为 MemoryWorker 逻辑 |

---

## 8. MemoryWorker 设计要点

> 来源：`_deprecated/需求/随心记.md`「记忆管家 Memory-Agent」需求，经可行性分析后调整为后台 goroutine 方案。

### 8.1 定位

MemoryWorker 是后台运行的记忆管理 goroutine（非独立进程），可配置开启和关闭。核心目标：确保记忆系统的**认知一致性**——当某个记忆块变动时，自动复盘关联记忆块是否需要更新。

### 8.2 与原需求的关键差异

| 原需求（随心记） | 调整后方案 | 调整理由 |
|-----------------|-----------|---------|
| 独立进程 | 后台 goroutine（`safego.Go`） | 避免跨进程通信/状态同步/部署复杂度；与现有 CronRunner 对齐 |
| 实时监控对话流 | Turn 完成后异步触发 | 逐条消息实时处理 LLM 成本过高；`AfterNativeTurn` 钩子已存在 |
| 自动复盘关联记忆 | 生成 Proposal，用户/Critic 审核后应用 | LLM 判断关联性可能出错；必须加门控防止正确记忆被错误修改 |
| 神经记忆系统 | L4 知识图谱 + 级联更新传播 | 图谱的 BFS 遍历天然支持"关联记忆"查找；与 L4 已有设计对齐 |

### 8.3 核心流程

```
Turn 完成 → EventBus(turn.completed)
                ↓
         MemoryWorker.OnTurnCompleted()
                ↓
    ┌─── 1. 异步提取 fact/episode/entity ───┐
    │                                       │
    │   2. Fact 冲突检测                     │
    │       ├─ 无冲突 → 直接写入 L3/L4      │
    │       └─ 有冲突 → 标记冲突，等待仲裁   │
    │                                       │
    │   3. 级联更新检查（依赖 L4 图谱）      │
    │       ├─ 实体属性变更？                │
    │       │   └─ BFS 关联实体              │
    │       │       └─ 生成 CascadeProposal  │
    │       └─ 无变更 → 跳过                │
    │                                       │
    │   4. 巩固管道                          │
    │       └─ L2 episode → L3 fact / L4    │
    └───────────────────────────────────────┘
```

### 8.4 配置项

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `memory_worker_enabled` | `true` | 是否启用 MemoryWorker |
| `memory_worker_extract_mode` | `auto` | `auto`（自动提取）/ `manual`（仅用户触发）/ `hybrid`（自动+手动确认） |
| `memory_worker_cascade_enabled` | `false` | 是否启用级联更新检查（依赖 L4 图谱） |
| `memory_worker_cascade_max_hops` | `2` | 级联更新 BFS 最大跳数 |
| `memory_worker_cascade_mode` | `proposal` | `proposal`（生成 Proposal 审核）/ `auto`（自动应用，高风险） |
| `memory_worker_batch_size` | `10` | 单次批量处理的最大 turn 数 |
| `memory_worker_consolidation_interval` | `30m` | 巩固管道运行间隔 |
