# L0 — 开发计划

> **需求**：[`L0.md`](./L0.md) · **设计**：[`L0.design.md`](./L0.design.md)

---

## 现状

| 项 | 状态 | 证据 |
|----|------|------|
| SessionCompressor | ✅ | `internal/service/session_compress.go` |
| L0 agent 配置字段 | ✅ | `agent_runtime_settings` |
| assembly snapshots 表 | ✅ | `memory_l0_assembly_snapshots` |
| ListL0Snapshots API | ✅ | `internal/service/memory.go` |
| Prompt Preview UI | 🟡 | 部分 |
| Team 独立 L0 | 🟡 | 需 Team Turn 回归 |

---

## 待办

| # | 任务 | 状态 |
|---|------|------|
| L0-1 | Team 多 Agent L0 隔离验收 | ✅ `ListL0SnapshotRows` 已支持 agent_id 过滤（2026-07-11 验证） |
| L0-2 | Prompt Preview 完整分段 UI | 🟡 |
| L0-3 | `GetL0Snapshot` HTTP 路由对齐 proto | ❌ |

---

## 代码锚点

- `internal/service/session_compress.go`
- `internal/service/memory.go` → ListL0Snapshots
- `internal/data/sessionmemory/store*.go`

---

## 附录：原落地阶段 / 运行时（迁移自分层需求文）

## 11. 落地实施阶段

### Phase 1（基础装配 / 与现有路径打平，1～2 周）

- [ ] 新增 `MemoryL0Service` + `MemoryL0Repository`，落库 §3.3 表。
- [ ] `agent_runtime_settings` ALTER（§3.2）。
- [ ] `MessageRepository.ListLatestByTokens` 实现。
- [ ] `ChatService.SendMessage` / `SendMessageStream` 接入 `Assemble` + `RecordActual`。
- [ ] `TeamRuntime` 每个 step 接入 `Assemble`。
- [ ] 暴露 §6.1、§6.2 的 GET 接口。
- [ ] 单元测试覆盖：滑动窗口、超额裁剪、摘要触发、snapshot 写入。

### Phase 2（摘要与可观测，1 周）

- [ ] 新增 `SummaryService.SummarizeRange`，接入 `session_summaries`。
- [ ] `truncate_strategy = hybrid` 实现（drop_tool_results → summary → drop_oldest）。
- [ ] `session_trace_spans.memory_recall` span 写入与详情联查。
- [ ] 前端 §8.2 上下文 Tab。

### Phase 3（前端调试与扩展，1 周）

- [ ] §8.1 设置面板。
- [ ] §8.3 Trace 详情 memory_recall 节点。
- [ ] §8.4 Prompt 调试器（P2）。

### Phase 4（与 L1/L3/L4 联调，跨阶段）

- 在 L1 文档 §6 接入 `inject_l1`；
- 在 L3 文档 §6 接入 `inject_l3`；
- 在 L4 文档 §6 接入 `inject_l4`。

---


## 14. 运行时实现与演进方向

> **实现真相**以 [`memory-development.md`](./memory-development.md) §1–§2 为准。本节描述 L0 在运行时的装配与压缩；术语对齐 **trpc-agent-go**（非 ADK）。

### 14.1 运行时记忆装配

| 层次 | 已实现 | 说明 |
|------|--------|------|
| Runner `memory.Service` | ✅ | `internal/memory/trpc/sqlite_adapter.go` — `NewSQLiteMemoryService(sessionmemory.Store)`；无 Store 时框架 `InMemoryService` |
| SQLite 会话记忆链 | ✅ | `internal/data/sessionmemory` + `memory_chain.sql`（L0 快照、L1–L4、entities） |
| 管理/观测 API | ✅ | `internal/service/memory.go` → `MemoryAdminUsecase`；**不** import `pkg/trpc-agent-go` |
| L0 压缩 | ✅ | `internal/service/session_compress.go`（`SessionCompressor.AfterNativeTurn`） |
| L4 Prompt 注入 | ✅ | `internal/agent/l4_prompt.go` via `trpc_build.go` |
| Postgres pgvector | 🟡 可选 | `internal/biz/memory.go` — 独立业务线，**不在**默认 Chat Turn 自动挂载 |
| 框架会话同步 | 🟡 | `AddSessionToMemory` 与 L2/L3 写入对齐仍待产品化 |

**接入点**：`internal/agent/trpc_build.go`、`internal/team` Runner；`biz.RuntimeSet`（`TRPC` + `Admin`）经 `PersistenceSet.Memory` 注入。`load_memory` / `preload_memory` 使用 Runner 挂载的 `trpcmemory.Service`。

### 14.2 会话上下文压缩

当历史达到一定规模时，将一段可追溯的对话区间交给「压缩模型」梳理为结构化摘要，后续轮次仅注入摘要 + 近期原文。

**核心概念**：

| 术语 | 含义 |
|------|------|
| 滚动摘要（Rolling Summary） | 覆盖区间 `[from_turn, to_turn]` 的 Markdown 文本，存于 `session_summaries` |
| 当前有效摘要（Active Summary） | 某 session 在装配时刻用于头部的摘要集合 |
| 滑动窗口（Tail） | 摘要区间之后、尚未被摘要覆盖的最近 K 轮或最近 T tokens 的原始对话 |
| 压缩模型（Compressor Model） | 执行摘要生成的 LLM 调用，可与对话模型同厂商或降级为更小规格 |

**触发条件（可组合配置）**：

| 策略 | 条件 | 说明 |
|------|------|------|
| 比例触发 | `context_used_ratio ≥ summary_threshold` | 与现有 `sessions.context_*` 字段对齐 |
| 轮次触发 | 自上次摘要以来新增 `Δturn ≥ compress_every_n_turns` | 防窗口很大但比例尚未告警时长对话不摘要 |
| Token 估算触发 | 未摘要前缀估算 token ≥ `compress_prefix_token_budget` | 与滑动窗口预算联动 |
| 手动触发 | UI「生成会话摘要」或 API | 便于调试与关键节点强制固化 |

**防抖**：同一 session 短时间窗口内（如 5～10 分钟）最多触发 N 次摘要任务。

**压缩输出 schema（推荐固定章节）**：

1. 用户意图与目标
2. 已确认事实 / 结论（含数字、版本、路径、API 名等硬信息）
3. 约束与偏好（语言、风格、禁止项）
4. 未完成事项 / 待澄清问题
5. 重要工具结果摘录（表格或列表）
6. 术语与别名

**装配顺序（与 L0 一致）**：

1. 系统 / 开发者固定段（SOUL、策略等）
2. `session_summaries` 合并摘要（标记 `source: session_summaries:<id>`）
3. L1 工作记忆字段（若有）
4. 滑动窗口内原始 messages（摘要区间之后）
5. L3/L4 检索段（若有）
6. 本轮 user 输入

**与 trpc 会话持久态的协同**：首版采用**装配层优先**——不改变会话快照内全量 events，仅在构造发往 LLM 的 messages 时应用摘要 + tail。实现集中、可逆、与现有 `messages` 账本一致。

**Team 会话**：每个子 Agent 应有独立的摘要区间与 `session_summaries` 维度，避免 Host 与子 Agent 上下文串扰。

**API / 配置**：扩展 `agent_runtime_settings`：`summary_threshold`、`compress_every_n_turns`、`recent_window_turns`、`recent_window_tokens`、`compressor_model_profile`；另设 `l0_compress_provider` / `l0_compress_model`（可选），指定专用压缩调用。

### 14.3 待完善项与演进方向

| 方向 | 现状 | 建议 |
|------|------|------|
| MemoryService 可插拔 | 有 Store 时已注入 SQLite 适配器 | 无 Store 时仍回退 `InMemoryService`；Composite `SearchMemory`（SQLite + pgvector）待 P2 |
| 记忆工具业务含义 | `load_memory`/`preload_memory` 走 trpc 默认语义 | `RuntimeCapabilityCue` 需如实描述 Store 是否就绪 |
| 向量记忆与 Runner 统一 | pgvector 独立业务线 | P2：显式工具或统一 `SearchMemory` 聚合 |
| 闭环会话记忆 | `AddSessionToMemory` 对齐待完善 | 与 L2/L3 写入路径一致；`memory/v1` 作观测面 |
| 摘要多条记录合并 | ✅ 已实现单条滚动：LLM 压缩传入 `PriorSummary` 吸收合并历史摘要，事务内删除旧摘要行、写入单行合并行（`compressor.go` + `session_repo_summaries.go`） | ~~二期可演进为单条滚动~~ 已完成（2026-07-20，Grok 借鉴 Phase 2） |
| 压缩调用成本 | 压缩模型与对话模型共用 | 可配置更小模型、更长触发间隔、批量区间合并以控 latency/cost |


