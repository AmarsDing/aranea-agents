# Session — 开发计划

> **版本**：2026-06-17 | **状态**：🟢 Phase 1–2 核心已落地 · Phase 3+ 待办见 §8
> **需求**：[10-session.md](./10-session.md) · **设计**：[10-session.design.md](./10-session.design.md)
> **规范**：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)

---

## 1. 模块定位

Session 管理：用户与 Agent/Team 的对话会话（创建、列表、删除、归档、标题、上下文压缩、轮次、状态、批量治理、消息检索、Timeline、导出、Runs/Participants 观测、子会话树、活动列表）。

### 1.1 代码锚点

#### Service 层（`internal/service/`）

| 文件 | 行数 | 职责 |
|------|------|------|
| `session.go` | 571 | SessionService：CRUD、Timeline、消息、Turn、Pin/Unpin、Export |
| `session_batch.go` | 90 | BatchPreview / BatchArchive / BatchDelete + Audit |
| `session_observability.go` | 103 | Export / ListSessionRuns / ListSessionParticipants / ListChildSessions / **GetSessionTree** / ListActivities |
| `session_status_guard.go` | 85 | 状态守卫（running/awaiting_confirmation 不可删/归档） |
| `session_context_window.go` | 9 | 上下文窗口解析（薄封装） |
| `session_projection.go` | 66 | Runner 事件投影 |
| `session_run_durable_worker.go` | 73 | Run 持久化 worker |
| `session_run_escalation_notifier.go` | 243 | Run 升级通知 |
| `session_title_llm.go` | 77 | LLM 标题生成 |
| `session_revision_publish.go` | 8 | Session 修订发布（**publish 已 DELETED**，仅保留 BumpSessionRevision） |

#### Biz 层（`internal/biz/session/`）

| 文件 | 行数 | 职责 |
|------|------|------|
| `usecase.go` | 677 | SessionUsecase 主入口 + SessionRepo 聚合接口（17 子接口） |
| `status.go` | 26 | SessionStatus 枚举 + SessionStatusReason + IsProtectedStatus |
| `status_machine.go` | 48 | SessionStatusMachine 实现（合法转换表） |
| ~~`session_state_machine.go`~~ | — | 已删除：TEST_ONLY 僵尸实现（生产仅经 `status_machine.go` 驱动），2026-08-14 死代码清理 |
| `compression.go` | 68 | SessionCompressionUsecase（窄接口：CompressRepo/ContextUpdater/SummaryReader/SummaryWriter） |
| `batch.go` | 252 | cutoff 解析、scope 扫描、批量命中 |
| `export.go` | 117 | Session 导出（Markdown/JSON） |
| `pin.go` | 19 | Pin/Unpin 业务逻辑 |
| `timeline.go` | 6 | Timeline 入口（薄封装） |
| `timeline_items.go` | 170 | Timeline 条目构建 |
| `timeline_usecase.go` | 204 | Timeline 聚合用例 |
| `message_usecase.go` | 336 | 消息子用例 Facade（Activity 适配读取 + 标题生成 + metrics 累积 + state/turn/participant 委托） |
| `messages.go` | 69 | 消息辅助逻辑 |
| `turns.go` | 18 | Turn 入口（薄封装） |
| `turn_usecase.go` | 72 | Turn 用例（Create/Update/List） |
| `title.go` | 44 | 自动标题生成（截取 + LLM 双策略） |
| `summary.go` | — | 摘要业务逻辑 |
| `state.go` | 18 | State KV 入口 |
| `state_usecase.go` | 52 | State 用例（Get/Save/ApplyStateDelta） |
| `participant.go` | 27 | Participant 入口 |
| `participant_usecase.go` | 69 | Participant 用例 |
| `participants_list.go` | 9 | Participant 列表辅助 |
| `metrics.go` | 114 | Metrics 业务逻辑 |
| `metrics_delta.go` | 24 | Metrics 增量计算 |
| `metrics_flush.go` | 32 | Metrics 刷新 |
| `metrics_repo.go` | 53 | SessionRuntimeWriter/SessionMetricsReader/Writer 拆分接口 |
| `recovery.go` | 52 | Session 恢复逻辑 |
| `limits.go` | 41 | 分页/扫描限制常量 |
| **`types.go` / `status.go`** | — | **SessionType 枚举**（spirit/team/agent/standalone，member DELETED）— 详见 [design §3.6.1](./10-session.design.md#361-sessiontype-枚举父子树角色) |
| **`tree_validate.go`** | — | **validateDepth 深度校验**（subagents_max_generation_depth 相对 + max_session_depth 绝对）— 详见 [design §3.6.4](./10-session.design.md#364-深度校验-validatedepth) |
| **`activity.go`**（`internal/biz/`） | — | Activity 领域模型 + ActivityKind(10) + ActivityEventType(7) + ActivityEvent — 详见 [design §3.6.7](./10-session.design.md#367-activity-模型单一真相源) |
| **`activity_event.go`**（`internal/biz/`） | — | ActivityEvent 定义（Event + Activity + Domain chat\|system） |
| **`llm_context_builder.go`**（`internal/biz/`） | — | BuildLLMContext：Activity → LLM 消息角色映射（task→user/reply→assistant/action→tool/notice→system） |
| **`activity_repo.go`**（`internal/data/`） | — | Activity Ent 持久化实现（替代 message_repo.go） |

#### Data 层（`internal/data/`）

| 文件 | 行数 | 职责 |
|------|------|------|
| `session_repo.go` | 843 | 主表 Ent 实现（SessionReader/Writer/Mutator/MessageReader/Writer 等） |
| `session_message_repo.go` | 409 | 消息子接口实现（AppendChatTurn/搜索） |
| `session_state_repo.go` | 86 | State KV 实现 |
| `session_timeline.go` | 257 | Timeline SQL UNION 分页 |
| `session_participant_repo.go` | 216 | participants 表 + 读时 Sync（SyncFromSession） |
| `session_participant_schema.go` | — | Participant Schema 辅助 |
| `session_run_repo.go` | 434 | M55 `session_runs` 生命周期 |
| `session_run_schema.go` | — | Run Schema 辅助 |
| `session_run_checkpoint_repo.go` | — | Run Checkpoint 实现 |
| `session_runtime_repo.go` | 117 | SessionRuntime 拆分表实现 |
| `session_metrics_repo.go` | 173 | SessionMetrics 拆分表实现 |
| `session_metrics_cache.go` | — | Metrics 缓存 |
| `session_repo_batch.go` | 192 | 批量归档/删除实现 |
| `session_repo_summaries.go` | 104 | session_summaries 原生 SQL CRUD |
| `session_message_feedback.go` | — | 消息反馈实现 |
| `session_status_migrate.go` | — | 状态迁移辅助 |
| `session_turn_repo.go` | — | Turn Repo 实现 |

#### 运行时层

| 文件 | 职责 |
|------|------|
| `internal/biz/native_turn_compressor.go` | NativeTurnCompressor 接口（AfterNativeTurn） |
| `internal/session/compressor.go` | 压缩触发器（调用 biz SessionCompressionUsecase） |
| `internal/session/compress_policy.go` | 压缩策略（L0CompressMinGapSec 防抖） |
| `internal/session/runtime.go` | trpc Runtime 桥接 |
| `internal/session/memory_compact.go` | 记忆压缩 |
| `internal/session/compress_quality.go` | 摘要质量门（退化检测/减量守卫/错误分类纯函数） |
| `internal/session/compress_suppress.go` | 压缩失败抑制（deterministic sticky + transient 退避） |
| `internal/session/context_update.go` | 上下文更新 |
| `internal/session/snapshot.go` | Runner Snapshot 同步 |
| `internal/session/token_estimate.go` | Token 估算 |
| `internal/session/provider.go` | Provider 辅助 |
| `internal/session/key.go` | Key 辅助 |
| `internal/agent/trpc_runtime.go` | trpc Runtime 集成 |
| `internal/agent/event_projector.go` | 事件投影（**DELETED** — 已被 `activity_projector.go` 替换，详见 §AF-01） |
| `internal/agent/activity_projector.go` | ActivityProjector：运行时事件 → ActivityEvent（Activity-First 架构核心，详见 [design §3.6.8](./10-session.design.md#368-双总线架构activityeventbus--monitoreventbus)） |
| `internal/agent/activity_persist.go` | 活动持久化（**DELETED** — 已并入 `ActivityEventBus` 内置 `persistChan` 并行异步机制 + 死信缓冲，详见 ADR-02 D1） |
| `internal/agent/l0_snapshot_persist.go` | L0 Snapshot 持久化 |

#### 前端（`web/src/`）

| 文件 | 职责 |
|------|------|
| `features/session/api.ts` | Kratos API 封装 + 类型定义 |
| `features/session/types.ts` | 类型定义（BatchPreviewResult, BulkProgress 等） |
| `features/session/useSessionsPage.ts` | 会话列表页 composable |
| `features/session/useSessionDetailPage.ts` | 会话详情页 composable |
| `features/session/useSessionTimelinePanel.ts` | Timeline 面板 composable（服务端分页） |
| `features/session/useSessionTimelineInspector.ts` | Timeline 检查器 composable |
| `features/session/useSessionTurnsPanel.ts` | Turn 面板 composable |
| `features/session/useSessionRunsPanel.ts` | Run 面板 composable |
| `features/session/useSessionParticipantsPanel.ts` | 参与者面板 composable |
| `features/session/useSessionMessagesPanel.ts` | 消息面板 composable |
| `features/session/timelineHelpers.ts` | Timeline 辅助函数 |
| `features/session/sessionSort.ts` | 排序逻辑 |
| `features/session/downloadExport.ts` | 导出下载 |
| `features/session/contextMetrics.ts` | 上下文指标（阈值与 Go llmcontext/metrics.go 同步） |
| `features/session/batchNotify.ts` | 批量操作通知 |
| `components/chat/ChatSessionSidebar.vue` | Chat 页右侧 Session 列表 |
| `components/chat/SessionTimelineDialog.vue` | 历史追踪弹窗（服务端分页） |
| `components/chat/SessionEventInspectorPanel.vue` | 事件检查器面板 |
| `components/sessions/sessionUi.ts` | 工具函数（格式化、颜色、列定义） |
| `components/sessions/Sessions*.vue` | 管理页组件（Hero/SummaryCards/FilterBar/TableSection/BulkToolbar 等） |
| `components/sessions/Session*.vue` | 详情组件（StatusBadge/RunsPanel/TurnsPanel/ParticipantsPanel/MessagesPanel/TimelinePanel 等） |
| `pages/SessionsPage.vue` | Session 管理页面 |

> 完整组件设计、UX 规范、API 契约详见 [10-session.design.md §8 Web 前端设计](./10-session.design.md#八web-前端设计)

---

## 2. 现状评估（2026-06-17 · 2026-06-26 更新）

| 项 | 状态 | 证据 |
|----|------|------|
| Session CRUD / 归档 / 恢复 / 部分更新 | ✅ | Proto + Ent + Service `session.go` (571 行) |
| 列表删除 + 批量治理（Phase 1b） | ✅ | Batch* RPC + `session_batch.go` + `useSessionsPage` |
| 消息 DB 分页 + FTS 搜索 | ✅ | `ListSessionMessages`（基于 Activity 表向后兼容，含 after_revision）/ `SearchSessionMessages` |
| Timeline 混合 kind SQL UNION | ✅ | `session_timeline.go` (257 行)；全量无 cap |
| 上下文压缩 + L0 防抖可配置 | ✅ | `compress_policy.go` · `L0CompressMinGapSec` |
| Session 置顶 | ✅ | `pinned_at` + Pin/Unpin RPC；Search 排序；聊天侧栏 + 管理页 |
| Session 导出 Markdown/JSON | ✅ | `ExportSession`；详情页 + 列表详情卡片 |
| Session Turns / State KV / Summaries | ✅ | 已有 RPC + 表 + biz 用例 |
| Runner Snapshot（Ent + trpc KV） | ✅ | `stateDeltaHandler` · `SyncStateDelta` |
| M55 session_runs 生命周期 | ✅ | CC-R-01~05；Chat 写入；`session_run_repo.go` (434 行) |
| ListSessionRuns + Runs 详情 Tab | ✅ | 读 M55 表 |
| ListSessionParticipants + Participants Tab | ✅ | 读时 Sync（`SyncFromSession`） |
| ListChildSessions（会话树，单层） | ✅ | `SessionTreeReader.ListByParentSessionID` |
| **GetSessionTree（完整递归树）** | ✅ | Phase D — 一次查询 + 内存构树，任意深度（详见 [design §3.6.5](./10-session.design.md#365-getsessiontree-rpc-设计)） |
| ListActivities（活动列表） | ✅ | Activity-First 单一真相源 |
| **Session 父子树字段（10 字段 50-59）** | ✅ | Phase D — session_type/parent_session_id/root_session_id/agent_depth/member_agent_key/member_role/execution_stage/completed_steps/total_steps/progress_pct |
| **Activity-First 单一真相源** | ✅ | AF-01 — `messages` 表 DELETED，`activities` 表承载全部内容（详见 [design §3.6.7](./10-session.design.md#367-activity-模型单一真相源)） |
| **ActivityEventBus + MonitorEventBus 双总线** | ✅ | AF-02 — 替换 SessionBus + Envelope MonitorBus（详见 ADR-03） |
| **ActivityEventBus 并行异步持久化 + 三级补偿** | ✅ | AF-03 — persistChan fire-and-forget + 死信缓冲 + API Backfill（详见 ADR-02） |
| CompactSession / GetCompressStatus | ✅ | 手动压缩 + 状态查询 RPC |
| **压缩 tail 保留** | ✅ | `loadCompressBody` 返回 body+tail 穿透事务，近期轮次写入快照（修复 tail 恒为空） |
| **递归滚动摘要** | ✅ | LLM 传入 `PriorSummary` 吸收合并，事务内删旧写新，防止无限拼接 |
| **摘要质量门** | ✅ | `compress_quality.go`：退化检测 + 减量守卫 + 错误分类（纯函数） |
| **压缩失败抑制** | ✅ | `compress_suppress.go`：deterministic sticky + transient 退避 |
| **双锚点 token 校准** | ✅ | `compress/service.go` → `llmcontext.RecordAuthoritativeUsage`，校准共享估算器 |
| Session 状态机（5 状态） | ✅ | `status_machine.go`（~~`session_state_machine.go`~~ 已删除，TEST_ONLY 僵尸） |
| SessionRuntime / SessionMetrics 拆分表 | ✅ | 高频字段拆出，减少写放大 |
| session_participants 增量写 | 🟡 | 表 + 读时 Sync；Team 详情 Tab；**无 turn 增量写** |
| session_run_steps | ❌ | F5 未建表 |
| session_trace_spans / context_snapshots | ❌ | F7–F8 未建表 |
| trpc session 多后端 | 🟡 | SQLite 共用池；in-memory 回退有日志 |
| 前端 Timeline 对话框（Chat） | ✅ | `SessionTimelineDialog` 已实现服务端分页（`useSessionTimelinePanel`） |
| trpc session.Service 桥接 | ❌ | M5-12 未实施（`BizSessionService` 未创建） |

---

## 3. 差距与优化（索引）

完整待办清单见 **§8**。本节保留 ID 与优先级对照。

### 3.1 代码优化

| ID | 项 | 优先级 | 状态 |
|----|-----|--------|------|
| P1 | 消息 DB 分页 + Timeline/压缩/取消路径 | P1 | ✅ |
| P3 | 批量 `ListSessionsByIDs` | P3 | ✅ |
| O1 | summaries repo kerrors | P1 | ✅ |
| O2 | Timeline SQL UNION（含全量无 2000 cap） | P2 | ✅ |
| O3 | Timeline inv limit 随查询缩放 | P2 | ✅ |
| O5 | 压缩防抖 `L0CompressMinGapSec` | P2 | ✅ |
| O4 | `AppendChatTurn` 事务内查询合并 | P3 | 待办 |
| O6 | SessionCompressor 模型选择策略收敛 | P3 | 待办 |
| O7 | Repository 接口拆分 + Deprecated 清理 | P2 | ✅ |
| O8 | CompressorDeps 窄聚合 + 代码质量修复 | P2 | ✅ |
| O9 | SessionRepo 注释 + data 层拆分 + 前端 UX/分层修复 | P2 | ✅ |
| P2-refactor | biz/data 文件拆分收尾 | P2 | ✅（timeline/export/participant 已拆；接口拆分 O7；data 拆分 O9） |

### 3.2 功能差距

| ID | 项 | 优先级 | 状态 |
|----|-----|--------|------|
| F1 | Session 置顶 | P2 | ✅ |
| F2 | Session 导出 | P3 | ✅ |
| F3 | 消息搜索 FTS | P3 | ✅ |
| F4 | session_runs 列表 | P2 | ✅（M55 语义；非 design §4.7 编排 schema） |
| F5 | session_run_steps | P2 | ❌ |
| F6 | session_participants | P2 | 🟡 |
| F7–F9 | trace / context_snapshots / model_summaries | P3 | ❌ |
| F10–F14 | trpc 适配 / Event 分页 / Track / Ingestor / 多后端 | P3–P4 | ❌ |
| F15–F17 | 前端 Trace / Context 趋势 / Team 专属 UI | P2–P3 | 🟡 |

---

## 4. 开发阶段

| 阶段 | 范围 | 状态 |
|------|------|------|
| Phase 1b | 批量治理 | ✅ |
| Phase 1 | O1/O3/O5、F1 置顶 | ✅ |
| Phase 2 | F2 导出、F4 Runs 列表、O2 Timeline UNION、F6 参与者（读时）、O7/O8/O9 代码质量 | ✅ 大部分 |
| Phase 3 | F5 steps、F7 trace、F8 snapshots、F15–F16 前端 | 待办 |
| Phase 4–5 | F9–F14 trpc 多后端 / Ingestor / session.Service 桥接 | 待办 |

> Phase 划分对应 trpc-agent-go 对齐路径 M5-1 至 M5-14，详见 [10-session.design.md §7 trpc-agent-go 对齐路径](./10-session.design.md#七trpc-agent-go-对齐路径)

---

## 5. 任务清单（摘要）

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| O1–O5, P1, P3 | 性能与 Timeline | P1–P2 | ✅ |
| O7–O9 | Repository 拆分 + 代码质量 | P2 | ✅ |
| F1, F2 | 置顶 + 导出 | P2–P3 | ✅ |
| F4 | Runs 列表 RPC + UI | P2 | ✅ |
| F6 | Participants 增量写 + Agent 会话展示 | P2 | 🟡 |
| F5 | session_run_steps | P2 | 待办 |
| F7–F10 | trace / snapshots / trpc 适配 | P3 | 待办 |
| F11–F14 | Event 分页 / Track / Ingestor / 多后端 | P3–P4 | 待办 |
| F15–F17 | Trace 页 / Context 趋势 / Team Handoff UI | P2–P3 | 待办 |

---

## 6. 验收标准

### 已完成 ✅

- [x] Phase 1b 批量治理 + Audit
- [x] 置顶（Search 排序 + 前端操作）
- [x] 压缩防抖可配置（Agent L0）
- [x] Timeline UNION 分页 / 全量无 2000 cap
- [x] Export Markdown/JSON
- [x] Runs 列表（M55 表）
- [x] Timeline 弹窗服务端分页（`useSessionTimelinePanel`，PAGE_SIZE=100）
- [x] Session 状态机（5 状态 + 合法转换 + 受保护状态）
- [x] SessionRuntime / SessionMetrics 拆分表
- [x] ListChildSessions（会话树）+ ListActivities（活动列表）
- [x] CompactSession / GetCompressStatus RPC
- [x] O7 Repository 接口拆分（17 子接口，每个 ≤5 方法）
- [x] O8 CompressorDeps 窄聚合
- [x] O9 SessionRepo 注释 + data 层拆分 + 前端 UX/分层修复

### Phase 2 剩余

- [ ] Participants **turn 完成时增量 upsert**（替代每次 List 全量 Sync）
- [ ] Agent 单会话也展示参与者（owner 行）
- [ ] M55 `session_runs` 与 design §4.7 编排字段对齐（或文档定稿双表策略）
- [ ] `session_run_steps` 表 + 写入 + List RPC

### Phase 3+

- [ ] Trace / Context 快照 / 模型分布
- [ ] 前端 Trace 树形页、Context 趋势线
- [ ] trpc session.Service 可配置切换后端

---

## 7. 依赖与风险

- 消息 FTS 依赖 `messages_fts`；无表时 LIKE 回退。
- **⚠️ 单一真相源（Activity-First）**：原 `messages` 表已 DELETED（DDL 迁移 `20260902`），transcript 真相改由 `activities` 表承载（详见 [design §3.6](./10-session.design.md#36-session-父子树--activity-模型activity-first-重构核心)）；trpc `trpc_*` = Runner 事件/state（见 design §〇）。`ListSessionMessages` / `SearchSessionMessages` RPC 当前基于 Activity 表向后兼容实现。
- **M55 vs F4**：当前 `session_runs` 为 Run 生命周期（phase/budget/checkpoint），与设计文档编排 runs（token 聚合/plan_json）字段不同；扩展前需 schema 决策。
- Participants **读时 Sync** 在大会话上 O(n) 扫 Activity；生产应改增量写。
- Timeline 全量导出/全量 UNION 在超大会话（10w+ 事件）可能慢；必要时加 export 流式或 cursor。
- Export 上限受 `MessageListMaxLimit=500` 分批拉 Activity，Timeline 走 UNION total，整体可完成但耗时长。
- **ActivityEventBus 持久化风险**（ADR-02）：fire-and-forget 通道在异常退出时可能丢未 flush 事件；已通过三级补偿（重试预算 5 次/3100ms → 死信环形缓冲 512 容量 FIFO 驱逐 activityID 去重 → API Backfill 最终一致）兜底。

### 7.1 接口拆分（O7 — 2026-05-29）

**变更**：`SessionRepository` 聚合接口拆分为 17 个子接口，每个 ≤5 方法，符合红线 #15。

| 子接口 | 方法数 | 职责 |
|--------|--------|------|
| SessionReader | 5 | 会话读取（含 ListSessionsByIDs/ListSessionsForBatch） |
| SessionTreeReader | 1 | 会话树读取（ListByParentSessionID） |
| SessionWriter | 5 | 会话写入（Create/UpdateTitle/Update/Restore/BumpRevision） |
| SessionMutator | 5 | 会话变更（Archive/Delete/DeleteByAgent/Pin/Unpin） |
| SessionBatchMutator | 2 | 批量变更（ArchiveByIDs/DeleteByIDs） |
| MessageReader | 5 | 消息读取 |
| MessageSearchReader | 3 | 消息搜索 + 增量拉取（含 ListMessagesAfterRevision） |
| MessageWriter | 4 | 消息写入（含 UpsertChatActivityMessage） |
| MessageStatusWriter | 1 | 消息状态更新 |
| TimelineReader | 4 | 时间线读取 |
| InvocationReader | 2 | 工具/Skill 调用读取 |
| SummaryReader | 3 | 摘要读取 |
| SummaryWriter | 4 | 摘要写入（含 DeleteSessionSummaries 递归合并删除） |
| StateRepo | 3 | KV 状态（Get/Save/Patch） |
| TurnRepo | 4 | Turn 读写 |
| ContextUpdater | 5 | 上下文更新（含 ApplyMetricsDelta） |
| CompressRepo | 2 | 压缩 CAS + 事务 |

**其他变更**：
- `SessionRepository` → `SessionRepo`（保留 Deprecated 标记 + TECH-DEBT(COG): interface_methods=17）
- `Compressor.Agents` 从 `biz.AgentRepository` 改为 `AgentKeyLookup` 窄接口
- 新增 `MessageStatusWriter` 接口，消除 `chatMessageStatusUpdater` 运行时类型断言
- 前端：`buildTimelineStats` 从 components 迁移至 features 层
- 前端：Store 移除 `turns`/`timeline`/`messages` 冗余子资源状态

### 7.2 O8 代码质量修复（2026-05-29）

**变更**：Compressor 构造函数收窄 + data 层规范修复 + 前端统一

| 变更 | 文件 | 说明 |
|------|------|------|
| CompressorDeps | internal/session/compressor.go | 7 子接口窄聚合替代 SessionRepo |
| RawDB() 访问器 | session_repo.go | rawDB → RawDB()，与项目 40+ 处用法一致 |
| kerrors 替换 | channel_peer_session.go | fmt.Errorf → kerrors.BadRequest |
| resolveAgentAuthor | internal/session/compressor.go | 提取重复的 author 解析逻辑 |
| formatSessionDate | sessionUi.ts + 4 组件 | 统一日期格式化函数 |

### 7.3 O9 SessionRepo 注释 + data 层拆分 + 前端 UX/分层修复（2026-05-29）

**变更**：SessionRepo 注释明确 + data 层文件拆分 + 前端 UX 红线修复 + 分层合规

| 变更 | 文件 | 说明 |
|------|------|------|
| SessionRepo 注释 | usecase.go | 添加注释明确仅用于 Wire 绑定，消费者应依赖具体子接口 |
| data 层文件拆分 | session_repo.go → 多文件 | 主表 843 行 + 消息 409 行 + 状态 86 行 + timeline 257 行 + participant 216 行 + run 434 行 + runtime 117 行 + metrics 173 行 + batch 192 行 + summaries 104 行 |
| deep-purple → teal | sessionUi.ts + SessionTurnsPanel.vue | 修复前端红线 #8（禁止日间用霓虹紫作强调色） |
| exportSelectedDetail 迁移 | SessionsPage.vue → useSessionsPage.ts | Page 不直接 import features/api，经 composable |

### 7.4 Activity-First 迁移任务块（2026-06-26 · ADR-02/ADR-03）

> 本节记录 Activity-First 架构重构在 Session 模块的迁移任务，全部完成（✅）。详见 ADR-02（活动事件持久化）与 ADR-03（统一总线架构）。

#### 7.4.1 AF-01：messages 表删除 + activities 单一真相源 ✅

| 任务 | 说明 | 状态 |
|------|------|------|
| DROP `messages` 表 | DDL 迁移 `20260902` | ✅ |
| 删除 `message_repo.go` / `session_message_repo.go` | Ent 实现移除 | ✅ |
| 删除 `event_projector.go` | 替换为 `activity_projector.go` | ✅ |
| 删除 `activity_publish.go` / `activity_persist.go` | 并入 `ActivityEventBus` 内置持久化 | ✅ |
| 保留 `message_usecase.go` 为合法子用例 Facade | 满足 AS-COG-01 复杂度预算 | ✅ |
| `ListSessionMessages` / `SearchSessionMessages` 向后兼容 | 基于 Activity 表实现，proto `ChatMessageRow` 作传输载体 | ✅ |
| `BuildLLMContext` 实现 | `internal/biz/llm_context_builder.go` — task→user/reply→assistant/action→tool/notice→system | ✅ |

#### 7.4.2 AF-02：双总线架构（ActivityEventBus + MonitorEventBus）✅

| 任务 | 说明 | 状态 |
|------|------|------|
| 新增 `ActivityEventBus`（`biz.ActivityEvent`） | chat 域持久化到 `activities`；system 域仅 WS | ✅ |
| 新增 `MonitorEventBus`（`contract.MonitorEvent`） | 替代 Envelope MonitorBus；不持久化 | ✅ |
| 删除 `SessionBus` | 死 publisher 全部移除（Blocker D） | ✅ |
| 删除 Envelope 文件 | `contract/envelope.go` / `buffer.go` / `reliability.go` 删除；`EnvelopeError` / `EnvelopeTokenUsage` 提取到 `envelope_types.go` | ✅ |
| 删除 `event_projector.go` / `activity_publish.go` / `activity_persist.go` | ActivityProjector 替代 | ✅ |
| WSServer 双总线双 pump | ActivityEventBus + MonitorEventBus 独立分发 | ✅ |

#### 7.4.3 AF-03：ActivityEventBus 并行异步持久化 + 三级补偿 ✅

> 详见 ADR-02 D1/D2。

| 任务 | 说明 | 状态 |
|------|------|------|
| 并行异步持久化 | `persistChan` fire-and-forget + 同步发布到总线 | ✅ |
| 三级补偿 — 重试预算 | 5 次重试 / 3100ms 退避 | ✅ |
| 三级补偿 — 死信环形缓冲 | 512 容量，FIFO 驱逐，activityID 去重 | ✅ |
| 三级补偿 — API Backfill | 最终一致兜底（`ListActivities` 拉取补齐） | ✅ |
| OnError 语义重构 | 删除 `ActivityKindError`；根 Task 转 `failed`；无根场景创建最小失败 Task | ✅ |
| 旧 ActivityKind 清理 | 删除 `SubTaskBoard` / `Error` / `Delegate` | ✅ |

#### 7.4.4 Phase D：Session 父子树字段断层补全 ✅

| 任务 | 说明 | 状态 |
|------|------|------|
| Proto 新增字段 53–59 | `session_type` / `member_agent_key` / `member_role` / `execution_stage` / `completed_steps` / `total_steps` / `progress_pct` | ✅ |
| Ent Schema 同步 | `internal/data/ent/schema/session.go` 7 字段补全 | ✅ |
| Service 映射 | `toProtoSession`（`internal/service/session.go`） | ✅ |
| 前端类型同步 | `web/src/features/session/types.ts` + `api.ts` | ✅ |
| 前端树节点增强 | `SessionTreeNode.vue`：session_type 图标、depth 徽章、execution_stage 徽章、进度展示 | ✅ |
| GetSessionTree RPC | 一次查询 + 内存构树，任意深度 | ✅ |

#### 7.4.5 Phase E：子会话 Activity 懒加载缓存 ✅

| 任务 | 说明 | 状态 |
|------|------|------|
| `ensureActivitiesLoaded` 缓存 | `useActivityTimeline.ts` — 缓存命中跳过 API；失败回退 | ✅ |
| `useChatWorkspace.ts` 联动 | 子会话切换时懒加载 | ✅ |
| 单测 | 3 个用例覆盖缓存命中/未命中/失败 | ✅ |

---

## 8. 待优化清单（全部）

> **用法**：实施时更新状态列；权威进度以本节 + §2 为准。

### 8.1 P2 — 近期（Phase 2 收尾）

| ID | 项 | 说明 | 状态 |
|----|-----|------|------|
| F6-a | Participants 增量写入 | Turn/Message 完成时 upsert；List 不再全量 Sync | 待办 · Review **SESS-R-P1-01** |
| F6-b | Agent 会话参与者行 | 单 Agent session 写 owner 参与者 | 待办 |
| F6-c | Team Handoff Badge | 设计 §8.9；消息 options 展示 handoff | 待办 |
| F5 | session_run_steps | 表 + model/tool/skill/mcp step 写入 + List RPC | 待办 |
| F4-schema | M55 runs vs 编排 runs | 扩展字段或独立表；与 Chat/Team 写入对齐 | 待办 |
| FE-TL-01 | Chat `SessionTimelineDialog` 服务端分页 | 对齐 `useSessionTimelinePanel` | ✅ |
| FE-EXP-01 | 聊天侧栏导出入口 | 可选：Session 菜单 Export | 待办 |

### 8.2 P3 — 代码质量

| ID | 项 | 说明 | 状态 |
|----|-----|------|------|
| O4 | AppendChatTurn 查询合并 | 事务内 `maxMessageTurnTx` + session 一次查 | 待办 |
| O6 | Compressor 模型选择收敛 | 统一 L0 provider/model fallback 策略 | 待办 |
| O7 | Repository 接口拆分 | 17 子接口（SessionReader/SessionTreeReader/SessionWriter/SessionMutator/SessionBatchMutator/MessageReader/MessageSearchReader/MessageWriter/MessageStatusWriter/TimelineReader/InvocationReader/SummaryReader/SummaryWriter/StateRepo/TurnRepo/ContextUpdater/CompressRepo） | ✅ |
| O9 | SessionRepo 注释 + data 拆分 + 前端 UX/分层 | SessionRepo 注释明确 Wire 绑定用途; session_repo.go 拆分多文件; deep-purple→teal; exportSelectedDetail 迁移 composable | ✅ |
| P2-refactor | 文件拆分收尾 | `session_usecase` 常量迁出；repo 按域分文件 | ✅ data 层已拆分 |
| SYNC-01 | state_json bulk sync | event_bus 全量 state 写 trpc（现仅 per-key KV） | 待办 |
| SYNC-02 | Ent-only snapshot 路径 | 非 event bus 路径也调 `SyncRunnerSnapshot` | 待办 |

### 8.3 P3 — 可观测性（Phase 3）

| ID | 项 | 说明 | 状态 |
|----|-----|------|------|
| F7 | session_trace_spans | 表 + parent_span_id + Trace API | 待办 |
| F8 | session_context_snapshots | 模型调用后写快照；趋势 API | 待办 |
| F9 | session_model_summaries | 多模型分布汇总 | 待办 |
| F15 | 前端 Trace 链路页 | 树形/瀑布；详情 Tab 或独立路由 | 待办 |
| F16 | 前端 Context 趋势线 | 基于 snapshots 图表 | 待办 |
| F17 | Team Session 专属 UI | Participants 增强 + 内部消息开关 | 🟡 基础 Panel 已有 |

### 8.4 P3–P4 — trpc / 框架对齐（Phase 4–5）

| ID | 项 | 说明 | 状态 |
|----|-----|------|------|
| F10 | trpc session.Service 适配器 | Ent ↔ trpc 桥接（design §7.1） | 待办 |
| F11 | Event 分页 | ListEvents limit/offset | 待办 |
| F12 | Session Track | Track(sessionID, key, value) | 待办 |
| F13 | Session Ingestor | Run 完成后外部记忆摄入 | 待办 |
| F14 | 多后端 | Redis/PG/MySQL/ClickHouse 可配置 | 待办 |
| M5-12 | trpc session.Service 桥接 | `BizSessionService` 实现（design §7.7） | 待办 |
| M5-14 | 压缩迁 trpc 内置 | 与 `internal/session/compressor` 合并策略 | 待办 |

### 8.5 前端 / UX 待办

| ID | 项 | 说明 | 状态 |
|----|-----|------|------|
| FE-FAV | 收藏会话服务端化 | 现 `localStorage` `chat:favorite-sessions` | 待办 |
| FE-TL-02 | Timeline kind 过滤 UX | 详情页已接 API；Inspector 对话框待对齐 | 待办 |
| FE-RUN-01 | Runs 与 Jobs 面板联动 | 详情 Runs ↔ Chat BackgroundJobs 深链 | 待办 |
| FE-EXP-02 | 批量导出 | 多 Session ZIP/合并 Markdown | 待办 |

### 8.6 架构 / 运维

| ID | 项 | 说明 | 状态 |
|----|-----|------|------|
| ARCH-01 | trpc/Ent 边界文档 | `0-system.development.md` Session 行定稿 | 待办 |
| ARCH-02 | in-memory session 告警 | SQLite 失败回退监控指标 | 🟡 已有 log |
| ARCH-03 | Export 大会话限流 | 异步 job + 下载链接 | 待办 · Review **SESS-R-P1-02** |

---

## 9. 已完成项速查（勿重复排期）

| 类别 | 项 |
|------|-----|
| 性能 | P1 消息分页、P3 批量 IDs、O1 kerrors、O2 UNION、O3 inv limit、O5 防抖 |
| 功能 | F1 置顶、F2 导出、F3 FTS、F4 Runs 列表（M55）、ListChildSessions、ListActivities、CompactSession/GetCompressStatus |
| 代码质量 | O7 接口拆分（17 子接口）、O8 CompressorDeps 窄聚合、O9 SessionRepo 注释 + data 拆分 + 前端 UX/分层 |
| 状态机 | SessionStatusMachine（5 状态 + 合法转换 + IsProtectedStatus） |
| 拆分表 | SessionRuntime（运行时字段）、SessionMetrics（指标字段） |
| 前端 | 置顶（聊天+管理页）、导出（详情+列表卡片）、Runs/Participants/Timeline/Turns/Messages 详情 Tab、Timeline 弹窗服务端分页 |
| 基础设施 | Phase 1b 批量、SQLite 父目录自动创建、trpc `DefaultAppName`、KV state delta sync |

---

## 10. 文档同步说明

| 文档 | 同步策略 |
|------|----------|
| **本文** | 待办权威来源（§8） |
| [10-session.design.md](./10-session.design.md) | §7 trpc-agent-go 对齐路径、§9 关键设计原则 — 与本文对齐 |
| [10-session.md](./10-session.md) | 需求文档 — 用户故事/功能需求/验收标准 |
| [0-system.development.md](./0-system.development.md) | Session 架构边界 — ARCH-01 完成后更新 |

**最后同步**：2026-06-26（Activity-First 迁移 AF-01~03 + Phase D/E 完成，详见 §7.4）
