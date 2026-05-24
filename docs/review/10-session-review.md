# 10 Session Review

> **评分**：79 / 100（基线）| **风险等级**：P1  
> **Phase 2 增量 Review**（2026-05-24）：**83 / 100** — 见 [2026-05-24-Session-Phase2-Review.md](./2026-05-24-Session-Phase2-Review.md)（Pin / Export / Timeline UNION / Runs / Participants）  
> **文档**：[10 session.md](../需求/10%20session.md) · [10 session.design.md](../需求/10%20session.design.md) · [10-session-development.md](../需求/10-session-development.md)  
> **代码锚点**：`internal/service/session.go` · `internal/service/session_observability.go` · `internal/biz/session/` · `internal/data/session_timeline.go`  
> **审查时间**：2026-05-21（基线）· 2026-05-24（Phase 2 增量）

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | 基础 CRUD、摘要、压缩、批量 bulk-ops ✅；批量治理 UI ✅ |
| 架构一致性 | 21 | 25 | biz/data 分层合理；`session_batch.go` 拆文件分责；trpc session adapter 位于 `internal/session/trpc` 正确位置 |
| 后端实现质量 | 17 | 20 | 压缩、摘要、标题生成均已实现；批量操作接口已有；`IncrementInvocationCounts` 工具/MCP/Skill 调用计数同步 |
| 前端实现质量 | 13 | 15 | 会话列表 + `SessionsBulkToolbar` 批量操作 ✅；详情页三 Tab ✅ |
| 测试与验证 | 6 | 10 | `session_batch_test.go` + `session_timeline_test.go` ✅ |
| 文档一致性 | 6 | 10 | Session 三件套对齐；Bulk Ops 与 UI 已同步 |

---

## 模块定位

Session 管理用户与 Agent/Team 的对话上下文，包括：
- 会话 CRUD（列表、归档、删除）
- 消息持久化与检索（FTS）
- 会话摘要与自动标题生成
- 上下文压缩（`NativeTurnCompressor`）
- 批量治理（批量归档/删除）
- trpc `session.Service` 适配（L0 感官记忆层）
- 调用计数同步（`mcp_call_count`、`tool_call_count`、`skill_call_count`）

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| 会话 CRUD + 归档 | ✅ |
| 消息 FTS 全文搜索 | ✅ |
| 自动标题生成（LLM） | ✅ |
| 上下文压缩 | ✅ |
| trpc Session 适配 | ✅ |
| `IncrementInvocationCounts` | ✅ |
| 批量 bulk-ops Phase 1b 后端 | ✅ |
| 批量治理前端 UI（`SessionsBulkToolbar.vue`、`SessionRetentionDialog.vue`）| ✅ |
| 会话分页 + 筛选（前端） | ✅ |
| 摘要卡展示（前端） | ✅ |
| 跳转 Chat 继续会话 | ✅ |

### Phase 2 增量（2026-05-24）

| 功能 | 状态 |
|------|------|
| Session 置顶（服务端 `pinned_at`） | ✅ |
| Session 导出 Markdown/JSON | ✅ |
| Timeline SQL UNION 分页（无 2000 cap） | ✅ |
| ListSessionRuns + Runs 详情 Tab | ✅ |
| ListSessionParticipants + Team Tab（读时 Sync） | 🟡 |
| 详情 Timeline 服务端分页 | ✅ |
| Chat `SessionTimelineDialog` 服务端分页 | ❌ FE-TL-01 |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| SESS-P1-01 | `session_usecase.go` 领域规则密集 | 🟡 `session_timeline.go` 已拆分 Timeline；压缩/摘要续拆 |
| SESS-P1-02 | 上下文压缩路径（`NativeTurnCompressor`）无专项测试；`sessionCompressMinGap` 硬编码 10min | 补压缩单测；将 `minGap` 提升为可配置参数 |
| SESS-R-P1-01 | Participants 每次 List 全量 Sync messages + DELETE/INSERT | **F6-a** turn 增量 upsert — [Phase2 Review](./2026-05-24-Session-Phase2-Review.md) |
| SESS-R-P1-02 | Export 全量进内存、proto inline content | **ARCH-03** 异步 job 或 size 上限 |
| SESS-R-P1-03 | Chat Timeline 对话框全量拉取（移除 cap 后更险） | **FE-TL-01** |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| SESS-P2-01 | 动态 MCP 挂载工具名未入 catalog 时仅计 `mcp_call` 而非具体工具名 | 文档化此限制，长期在 catalog 层解决 |
| SESS-P2-02 | 会话详情页 Turns Tab 缺乏筛选 | ✅ Export 已覆盖导出；Runs/Participants Tab 已加 |
| SESS-R-P2-01~08 | Runs  bypass biz、MCP 启发式、limits dead code、测试缺口 | 见 [Phase2 Review §3](./2026-05-24-Session-Phase2-Review.md#3-问题与风险) |

---

## 批量治理差距

**已完整实现**：
- `BatchDeleteSessions` / `BatchArchiveSessions` RPC（后端 biz + data）
- `SessionsBulkToolbar.vue` — 批量操作工具栏
- `SessionRetentionDialog.vue` — 按保留天数配置
- 按 ID 批量或按天数归档/删除均已可用

**剩余改进点**：`sessionCompressMinGap` 仍硬编码；`session_usecase.go` 体量大。

---

## 建议优化路径

1. **F6-a** Participants turn 增量写（P1，[SESS-R-P1-01](./2026-05-24-Session-Phase2-Review.md)）。
2. **FE-TL-01** Chat Timeline 对话框服务端分页（P1）。
3. **ARCH-03** Export 大会话限流/异步（P1）。
4. 补上下文压缩单测；删 `limits.go` dead code。
5. 将 `IncrementInvocationCounts` 的动态 MCP 工具名限制文档化。
