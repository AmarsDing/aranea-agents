# Session — 开发计划

> **版本**：2026-05-21 | **状态**：🟢 Phase 1b 已完成 · P1/P3 性能优化已落地
> **需求**：[10 session.md](./10%20session.md) · **设计**：[10 session.design.md](./10%20session.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **规范**：[docs/README.md](../README.md)

---

## 1. 模块定位

Session 管理：用户与 Agent/Team 的对话会话（创建、列表、删除、归档、标题、上下文压缩、轮次、状态、批量治理、消息检索）。

**代码锚点**（按职责拆分，避免 `session_usecase.go` 无限膨胀）：

| 文件 | 职责 |
|------|------|
| `api/kratos/session/v1/session.proto` | RPC 契约 |
| `internal/service/session.go` | SessionService：CRUD、Timeline、消息、Turn、SearchMessages |
| `internal/service/session_batch.go` | BatchPreview / BatchArchive / BatchDelete + Audit |
| `internal/service/session_compress.go` | 异步上下文压缩（`NativeTurnCompressor`） |
| `internal/biz/session_usecase.go` | 领域用例：CRUD、Timeline、消息、Turn、State |
| `internal/biz/session_batch.go` | 批量 cutoff / scope 扫描（单一职责） |
| `internal/biz/session_state.go` | State KV / ApplyStateDelta |
| `internal/biz/session_title.go` | `SessionTitleGenerator` |
| `internal/data/session_repo.go` | 主表与消息 |
| `internal/data/session_repo_batch.go` | 批量 UPDATE |
| `internal/data/session_repo_summaries.go` | `session_summaries` 原生 SQL |
| `internal/data/session_turn_repo.go` | `session_turns` |
| `internal/data/message_search.go` | FTS5 / LIKE 消息搜索 |
| `web/src/features/session/` | API + `useSessionsPage` 编排 |

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Session CRUD | ✅ | Create/Get/Update/Delete/Search |
| Session 归档/恢复 | ✅ | ArchiveSession/RestoreSession |
| Session 部分更新 | ✅ | UpdateSession（多字段 PATCH） |
| 列表行内删除 + 批量治理 | ✅ | DeleteSession + Batch* RPC + 前端 Phase 1b |
| 消息列表 | ✅ | ListSessionMessages + **DB** limit/offset（`CountMessagesBySession`） |
| 消息搜索 | ✅ | SearchSessionMessages（`messages_fts` 或 LIKE） |
| Timeline 聚合 | ✅ | GetSessionTimeline；消息最多拉取 `TimelineMessageMaxFetch`（2000）条后与 tool/skill 合并 |
| 上下文压缩 | ✅ | SessionCompressor.AfterNativeTurn |
| 标题生成 | ✅ | LLMSessionTitleGenerator + 截取 |
| Session Turns | ✅ | CreateTurn/UpdateTurn/ListTurns |
| Session State KV | ✅ | Get/Save/ApplyStateDelta |
| Runner Snapshot | ✅ | UpdateRunnerSnapshotJSON |
| Session Summaries | ✅ | Insert/List/MaxToTurn |
| Session 类型 | ✅ | agent / team |
| session_runs / trace / participants | ❌ | 表与 Chat/Team 写入未接 |
| Session 置顶 / 导出 | ❌ | Proto 未定义 |
| trpc session.Service 多后端 | 🟡 | `internal/session/trpc` 部分能力，业务仍以 Ent 为主 |

---

## 3. 差距与优化

### 3.0 架构质量（持续）

| 项 | 说明 | 状态 |
|----|------|------|
| biz 不 import trpc-agent-go | Session 领域与框架解耦 | ✅ |
| 批量逻辑独立 `session_batch.go` | 降低 CRUD 文件变更影响面 | ✅ |
| Chat 写会话不经 SessionService RPC | 经 `ChatService` → `SessionUsecase` 仓储方法 | ✅ |
| data 层 kerrors 一致 | `session_repo_summaries` 等于其他 repo | ✅ O1 |

### 3.1 代码优化（审查 P1–P3）

| ID | 项 | 优先级 | 状态 |
|----|-----|--------|------|
| **P1** | 消息 DB 分页：`ListMessagesPaged` / `CountMessagesBySession`；Timeline `ListMessagesRecent`；压缩 `ListMessagesAfterTurn`；取消卡片 `ListMessagesByStatus` | P1 | ✅ |
| **P3** | 批量 ids：`ListSessionsByIDs` 替代 N×`GetSessionByID` | P3 | ✅ |
| O1 | `session_repo_summaries.go` 使用 kerrors | P1 | ✅ |
| O2 | Timeline 超长会话：当前 cap 2000 条；完整 UNION 分页待办 | P2 | 部分 |
| O3 | Timeline 工具/Skill limit 随查询缩放 | P2 | ✅ |
| O4 | `AppendChatTurn` 事务内查询合并 | P3 | 待办 |
| O5 | 压缩防抖 `sessionCompressMinGap` 可配置（Agent L0 设置） | P2 | 待办 |
| O6 | SessionCompressor 模型选择策略收敛 | P3 | 待办 |
| **P2** | 拆分 `session_timeline.go` / `session_repository.go` 独立文件 | P2 | 待办（逻辑已收敛到 usecase 常量与方法） |

**常量**（`internal/biz/session_usecase.go`）：`MessageListMaxLimit=500`、`TimelineMessageMaxFetch=2000`、`CompressMessageMaxRows=512`、`ActivityCancelScanLimit=64`。

### 3.2 功能差距

| ID | 项 | 优先级 |
|----|-----|--------|
| F1 | Session 置顶（`pinned_at` + Pin/Unpin RPC） | P2 |
| F2 | Session 导出（Markdown/JSON） | P3 |
| F4–F6 | session_runs / steps / participants + Team UI | P2 |
| F7–F9 | trace_spans / context_snapshots / model_summaries | P3 |
| F10–F14 | trpc session.Service 适配器、多后端、Ingestor | P3–P4 |

---

## 4. 开发阶段

### Phase 1b — 会话历史批量治理 ✅

| ID | 任务 | 状态 |
|----|------|------|
| SES-1b-01 | Proto：BatchPreview/BatchArchive/BatchDelete | ✅ |
| SES-1b-02 | `session_batch.go` + `session_repo_batch.go` | ✅ |
| SES-1b-03 | Service + Audit | ✅ |
| SES-1b-04–07 | 前端删除/批量/RetentionDialog/`useSessionsPage` | ✅ |

### Phase 1 — 近期优化

- O1 ✅ · O3 ✅
- O5：压缩防抖可配置
- F1：Session 置顶

### Phase 2–5

见 [10 session.design.md §10.3](./10%20session.design.md#103-开发阶段建议) 与下文任务表。

---

## 5. 任务清单

| # | 任务 | 优先级 | 阶段 | 状态 |
|---|------|--------|------|------|
| O1 | summaries repo kerrors | P1 | 1 | ✅ |
| P1 | 消息 DB 分页 + Timeline/压缩/取消路径 | P1 | 1 | ✅ |
| P3 | 批量 `ListSessionsByIDs` | P3 | 1 | ✅ |
| O3 | Timeline inv limit | P2 | 1 | ✅ |
| O5 | 压缩防抖可配置 | P2 | 1 | 待办 |
| 1 | `pinned_at` + Pin/Unpin | P2 | 1 | 待办 |
| 2–4 | runs / steps / participants | P2 | 2 | 待办 |
| 6–10 | trace / snapshots / 前端 Trace | P3 | 3 | 待办 |
| 11–12 | 导出 / 搜索增强 | P3 | 4 | 搜索 RPC ✅，导出待办 |
| 13 | Phase 1b 批量治理 | P1 | 1b | ✅ |
| 14–17 | trpc 适配 / Ingestor / 多后端 | P3–P4 | 5 | 待办 |

---

## 6. 验收标准

### Phase 1b ✅

- [x] 行内删除、批量勾选归档/删除、按天数预览与执行
- [x] Batch RPC + Audit；`running` 排除

### Phase 1

- [x] `session_repo_summaries` 使用 kerrors
- [ ] 压缩防抖可从 Agent L0 设置读取
- [ ] 置顶会话列表优先

### Phase 2–5

- [ ] Run/Step/参与者可查询；Team 参与者 UI
- [ ] Trace/Context 趋势/模型分布
- [ ] 导出 Markdown/JSON
- [ ] trpc session.Service 可按配置切换后端

---

## 7. 依赖与风险

- 消息 FTS 依赖 `messages_fts` 迁移；无表时自动 LIKE 回退（`message_search.go`）。
- `session_runs` 写入须与 `ChatService` / Team Runner 生命周期同一事务边界设计。
- trpc `session.Service` 与 Ent 业务 `sessions` 双写期需明确 transcript 真相源（见 `0-system-development.md` Session 行）。
- Timeline 消息已 cap 2000；超过部分不进入合并（summary 统计窗口内条目）。完整游标分页见 O2。
- Ent `sessions` 与 trpc `trpc_*` session 表并存：业务 transcript 以 `messages` 为准（见 design §〇）。
