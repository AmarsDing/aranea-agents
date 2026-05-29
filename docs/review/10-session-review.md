# 10 Session Review

> **评分**：79 / 100（基线）| **风险等级**：P1  
> **Phase 2 增量 Review**（2026-05-24）：**83 / 100** — 见 [2026-05-24-Session-Phase2-Review.md](./2026-05-24-Session-Phase2-Review.md)（Pin / Export / Timeline UNION / Runs / Participants）
> **O7 接口拆分增量 Review**（2026-05-29）：**86 / 100** — Repository 接口拆分 + Deprecated 清理 + Compressor 窄接口 + 前端分层修复
> **文档**：[10 session.md](../需求/10%20session.md) · [10 session.design.md](../需求/10%20session.design.md) · [10-session-development.md](../需求/10-session-development.md)  
> **代码锚点**：`internal/service/session.go` · `internal/service/session_observability.go` · `internal/biz/session/` · `internal/data/session_timeline.go`  
> **审查时间**：2026-05-21（基线）· 2026-05-24（Phase 2 增量）

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | +1：接口拆分提升可维护性 |
| 架构一致性 | 23 | 25 | +2：窄接口替代宽依赖、Compressor AgentKeyLookup |
| 后端实现质量 | 18 | 20 | +1：MessageStatusWriter 消除类型断言 |
| 前端实现质量 | 14 | 15 | +1：分层修复 + Store 瘦身 |
| 测试与验证 | 7 | 10 | +1：编译期接口检查完备 |
| 文档一致性 | 7 | 10 | +1：设计文档同步更新 |

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

### O7 接口拆分增量（2026-05-29）

| 功能 | 状态 |
|------|------|
| SessionWriter/MessageReader/SummaryRepo/SessionRunRepo 接口拆分（红线 #15 合规） | ✅ |
| SessionRepository → SessionRepo（移除 Deprecated） | ✅ |
| Compressor.Agents → AgentKeyLookup 窄接口 | ✅ |
| MessageStatusWriter 新接口（消除运行时类型断言） | ✅ |
| 前端 buildTimelineStats 从 components 迁移至 features 层 | ✅ |
| 前端 Store 移除 turns/timeline/messages 冗余子资源状态 | ✅ |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| SESS-P1-01 | `session_usecase.go` 领域规则密集；接口拆分已完成（红线 #15 ✅），但 usecase 方法数仍偏高 | 🟡 接口拆分已完成；续拆 usecase 方法至独立文件 |
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
| SESS-P2-09 | `NewCompressor` 接收 `SessionRepo` 聚合接口但仅用 7 个子接口 | 为 Compressor 定义专用窄聚合接口 `CompressorDeps` |

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
