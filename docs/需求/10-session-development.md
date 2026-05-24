# Session — 开发计划

> **版本**：2026-05-24 | **状态**：🟢 Phase 1–2 核心已落地 · Phase 3+ 待办见 §8
> **需求**：[10 session.md](./10%20session.md) · **设计**：[10 session.design.md](./10%20session.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **规范**：[docs/README.md](../README.md)

---

## 1. 模块定位

Session 管理：用户与 Agent/Team 的对话会话（创建、列表、删除、归档、标题、上下文压缩、轮次、状态、批量治理、消息检索、Timeline、导出、Runs/Participants 观测）。

**代码锚点**（按职责拆分）：

| 文件 | 职责 |
|------|------|
| `api/kratos/session/v1/session.proto` | RPC 契约 |
| `internal/service/session.go` | SessionService：CRUD、Timeline、消息、Turn |
| `internal/service/session_observability.go` | Export / ListSessionRuns / ListSessionParticipants |
| `internal/service/session_batch.go` | BatchPreview / BatchArchive / BatchDelete + Audit |
| `internal/biz/session/` | 领域用例：CRUD、Timeline、Export、Participants、Pin、Turn |
| `internal/biz/session_state.go` | State KV / ApplyStateDelta |
| `internal/data/session_repo.go` | 主表与消息 |
| `internal/data/session_timeline.go` | Timeline SQL UNION 分页 |
| `internal/data/session_participant_*.go` | participants 表 + 读时聚合 |
| `internal/data/session_run_*.go` | M55 `session_runs` 生命周期 |
| `internal/session/` | trpc Runtime、Compressor、KV sync |
| `web/src/features/session/` | API、`useSessionsPage`、详情页 Tab |

---

## 2. 现状评估（2026-05-24）

| 项 | 状态 | 证据 |
|----|------|------|
| Session CRUD / 归档 / 恢复 / 部分更新 | ✅ | Proto + Ent |
| 列表删除 + 批量治理（Phase 1b） | ✅ | Batch* RPC + `useSessionsPage` |
| 消息 DB 分页 + FTS 搜索 | ✅ | `ListSessionMessages` / `SearchSessionMessages` |
| Timeline 混合 kind SQL UNION | ✅ | `session_timeline.go`；全量无 cap |
| 上下文压缩 + L0 防抖可配置 | ✅ | `compress_policy.go` · `L0CompressMinGapSec` |
| Session 置顶 | ✅ | `pinned_at` + Pin/Unpin；Search 排序；聊天侧栏 + 管理页 |
| Session 导出 Markdown/JSON | ✅ | `ExportSession`；详情页 + 列表详情卡片 |
| Session Turns / State KV / Summaries | ✅ | 已有 RPC + 表 |
| Runner Snapshot（Ent + trpc KV） | ✅ | `stateDeltaHandler` · `SyncStateDelta` |
| M55 session_runs 生命周期 | ✅ | CC-R-01~05；Chat 写入 |
| ListSessionRuns + Runs 详情 Tab | ✅ | 读 M55 表 |
| session_participants | 🟡 | 表 + 读时 Sync；Team 详情 Tab；**无 turn 增量写** |
| session_run_steps | ❌ | F5 未建表 |
| session_trace_spans / snapshots | ❌ | F7–F8 未建表 |
| trpc session 多后端 | 🟡 | SQLite 共用池；in-memory 回退有日志 |
| 前端 Timeline 对话框（Chat） | 🟡 | 详情页已服务端分页；`SessionTimelineDialog` 仍全量拉取 |

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
| P2-refactor | biz/data 文件拆分收尾 | P2 | 🟡 部分（timeline/export/participant 已拆） |

### 3.2 功能差距

| ID | 项 | 优先级 | 状态 |
|----|-----|--------|------|
| F1 | Session 置顶 | P2 | ✅ |
| F2 | Session 导出 | P3 | ✅ |
| F3 | 消息搜索 FTS | P3 | ✅ |
| F4 | session_runs 列表 | P2 | ✅（M55 语义；非 design §4.3 编排 schema） |
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
| Phase 2 | F2 导出、F4 Runs 列表、O2 Timeline UNION、F6 参与者（读时） | 🟡 大部分 ✅ |
| Phase 3 | F5 steps、F7 trace、F8 snapshots、F15–F16 前端 | 待办 |
| Phase 4–5 | F9–F14 trpc 多后端 / Ingestor | 待办 |

---

## 5. 任务清单（摘要）

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| O1–O5, P1, P3 | 性能与 Timeline | P1–P2 | ✅ |
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

### Phase 2 剩余

- [ ] Participants **turn 完成时增量 upsert**（替代每次 List 全量 Sync）
- [ ] Agent 单会话也展示参与者（owner 行）
- [ ] M55 `session_runs` 与 design §4.3 编排字段对齐（或文档定稿双表策略）
- [ ] `session_run_steps` 表 + 写入 + List RPC

### Phase 3+

- [ ] Trace / Context 快照 / 模型分布
- [ ] 前端 Trace 树形页、Context 趋势线
- [ ] trpc session.Service 可配置切换后端

---

## 7. 依赖与风险

- 消息 FTS 依赖 `messages_fts`；无表时 LIKE 回退。
- **双存储**：Ent `messages` = transcript 真相；trpc `trpc_*` = Runner 事件/state（见 design §〇）。
- **M55 vs F4**：当前 `session_runs` 为 Run 生命周期（phase/budget/checkpoint），与设计文档编排 runs（token 聚合/plan_json）字段不同；扩展前需 schema 决策。
- Participants **读时 Sync** 在大会话上 O(n) 扫 messages；生产应改增量写。
- Timeline 全量导出/全量 UNION 在超大会话（10w+ 事件）可能慢；必要时加 export 流式或 cursor。
- Export 上限受 `MessageListMaxLimit=500` 分批拉消息，Timeline 走 UNION total，整体可完成但耗时长。

---

## 8. 待优化清单（全部）

> **用法**：实施时更新状态列；权威进度以本节 + §2 为准。

### 8.1 P2 — 近期（Phase 2 收尾）

| ID | 项 | 说明 | 状态 |
|----|-----|------|------|
| F6-a | Participants 增量写入 | Turn/Message 完成时 upsert；List 不再全量 Sync | 待办 · Review **SESS-R-P1-01** |
| F6-b | Agent 会话参与者行 | 单 Agent session 写 owner 参与者 | 待办 |
| F6-c | Team Handoff Badge | 设计 §7.5；消息 options 展示 handoff | 待办 |
| F5 | session_run_steps | 表 + model/tool/skill/mcp step 写入 + List RPC | 待办 |
| F4-schema | M55 runs vs 编排 runs | 扩展字段或独立表；与 Chat/Team 写入对齐 | 待办 |
| FE-TL-01 | Chat `SessionTimelineDialog` 服务端分页 | 对齐 `useSessionTimelinePanel` | 待办 · Review **SESS-R-P1-03** |
| FE-EXP-01 | 聊天侧栏导出入口 | 可选：Session 菜单 Export | 待办 |

### 8.2 P3 — 代码质量

| ID | 项 | 说明 | 状态 |
|----|-----|------|------|
| O4 | AppendChatTurn 查询合并 | 事务内 `maxMessageTurnTx` + session 一次查 | 待办 |
| O6 | Compressor 模型选择收敛 | 统一 L0 provider/model fallback 策略 | 待办 |
| P2-refactor | 文件拆分收尾 | `session_usecase` 常量迁出；repo 按域分文件 | 🟡 |
| SYNC-01 | state_json  bulk sync | event_bus 全量 state 写 trpc（现仅 per-key KV） | 待办 |
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
| F10 | trpc session.Service 适配器 | Ent ↔ trpc 桥接（design §12.1） | 待办 |
| F11 | Event 分页 | ListEvents limit/offset | 待办 |
| F12 | Session Track | Track(sessionID, key, value) | 待办 |
| F13 | Session Ingestor | Run 完成后外部记忆摄入 | 待办 |
| F14 | 多后端 | Redis/PG/MySQL/ClickHouse 可配置 | 待办 |
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
| ARCH-01 | trpc/Ent 边界文档 | `0-system-development.md` Session 行定稿 | 待办 |
| ARCH-02 | in-memory session 告警 | SQLite 失败回退监控指标 | 🟡 已有 log |
| ARCH-03 | Export 大会话限流 | 异步 job + 下载链接 | 待办 · Review **SESS-R-P1-02** |

---

## 9. 已完成项速查（勿重复排期）

| 类别 | 项 |
|------|-----|
| 性能 | P1 消息分页、P3 批量 IDs、O1 kerrors、O2 UNION、O3 inv limit、O5 防抖 |
| 功能 | F1 置顶、F2 导出、F3 FTS、F4 Runs 列表（M55） |
| 前端 | 置顶（聊天+管理页）、导出（详情+列表卡片）、Runs/Participants/Timeline 详情 Tab |
| 基础设施 | Phase 1b 批量、SQLite 父目录自动创建、trpc `DefaultAppName`、KV state delta sync |

---

## 10. 文档同步说明

| 文档 | 同步策略 |
|------|----------|
| **本文** | 待办权威来源（§8） |
| [10 session.design.md](./10%20session.design.md) | §9 M5 里程碑、§10 优化表 — 与本文对齐 |
| [10 session.md](./10%20session.md) | 需求 Phase 段落 — 置顶/导出/FTS 改为 ✅ |
| [0-system-development.md](./0-system-development.md) | Session 架构边界 — ARCH-01 完成后更新 |
| [2026-05-24-Session-Phase2-Review.md](../review/2026-05-24-Session-Phase2-Review.md) | **代码 Review**（83/100）；P1 风险 SESS-R-P1-01~03 映射 §8 任务 ID |
| [10-session-review.md](../review/10-session-review.md) | 模块 Review 基线 + Phase 2 增量索引 |

**最后同步**：2026-05-24
