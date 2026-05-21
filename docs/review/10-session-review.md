# 10 Session Review

> **评分**：79 / 100 | **风险等级**：P1  
> **文档**：[10 session.md](../需求/10%20session.md) · [10 session.design.md](../需求/10%20session.design.md) · [10-session-development.md](../需求/10-session-development.md)  
> **代码锚点**：`internal/service/session.go` · `internal/service/session_batch.go` · `internal/service/session_compress.go` · `internal/biz/session_usecase.go` · `internal/biz/session_batch.go` · `internal/data/session_repo.go`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | 基础 CRUD、摘要、压缩、批量 bulk-ops Phase 1b 已完成；前端批量治理 UI 待实现 |
| 架构一致性 | 21 | 25 | biz/data 分层合理；`session_batch.go` 拆文件分责；trpc session adapter 位于 `internal/session/trpc` 正确位置 |
| 后端实现质量 | 17 | 20 | 压缩、摘要、标题生成均已实现；批量操作接口已有；`IncrementInvocationCounts` 工具/MCP/Skill 调用计数同步 |
| 前端实现质量 | 13 | 15 | 会话列表页基本功能完整；批量选择/删除/归档 UI 缺失；`/sessions/:id` 详情页三 Tab 已有 |
| 测试与验证 | 6 | 10 | `session_batch_test.go` 已有；摘要/压缩路径无专项测试 |
| 文档一致性 | 6 | 10 | Session 三件套对齐；Bulk Ops Phase 1b changelog 已同步；批量治理 PRD 与当前 UI 状态存在差距 |

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

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| SESS-P1-01 | `session_usecase.go` 约 1058 行，领域规则密集，可读性降低 | 按功能域（压缩/摘要/批量/消息）拆分为更细粒度的 Usecase 文件 |
| SESS-P1-02 | 上下文压缩路径（`NativeTurnCompressor`）无专项测试；`sessionCompressMinGap` 硬编码 10min | 补压缩单测；将 `minGap` 提升为可配置参数 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| SESS-P2-01 | 动态 MCP 挂载工具名未入 catalog 时仅计 `mcp_call` 而非具体工具名 | 文档化此限制，长期在 catalog 层解决 |
| SESS-P2-02 | 会话详情页（`/sessions/:id`）三 Tab 中 Turns Tab 缺乏筛选和导出能力 | 视需求优先级规划 |

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

1. 实现会话列表批量治理前端 UI（P1，复用已有 biz batch API）。
2. 补上下文压缩单测。
3. 将 `IncrementInvocationCounts` 的动态 MCP 工具名限制文档化。
