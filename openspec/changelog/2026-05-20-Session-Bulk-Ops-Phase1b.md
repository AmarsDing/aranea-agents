# Session Phase 1b — 批量治理实现

> 日期：2026-05-20  
> 关联：SES-1b · [10-session-development.md](../需求/10-session-development.md) · [设计 changelog](./2026-05-20-Session-Bulk-Ops-Design.md)

## 摘要

完成会话历史批量治理 Phase 1b：后端 Batch RPC + 前端列表行删除、批量选择、按保留天数归档/删除。

## 后端

| 层 | 变更 |
|----|------|
| Proto | `BatchPreviewSessions` / `BatchArchiveSessions` / `BatchDeleteSessions` + `SessionBatchScope` |
| Biz | `internal/biz/session_batch.go` — cutoff 解析、running 排除、Preview/BatchArchive/BatchDelete |
| Data | `internal/data/session_repo_batch.go` — 500-id 分块 UPDATE |
| Service | `internal/service/session_batch.go` — RPC 实现 + `RecordAdminAudit` |
| 单测 | `session_batch_test.go` — cutoff 边界、running 排除 |

## 前端

| 组件 | 职责 |
|------|------|
| `useSessionsPage.ts` | 列表状态、选择模式、批量/保留天数流程 |
| `SessionsBulkToolbar.vue` | 批量选择开关 + 按天数入口 |
| `SessionsBulkSelectionBar.vue` | 勾选后归档/删除 |
| `SessionsBulkProgressBar.vue` | 批量操作进度 |
| `SessionDeleteConfirmDialog.vue` | 单条/批量删除确认 |
| `SessionRetentionDialog.vue` | 按天数预览 + 确认 |
| `SessionsTableSection.vue` | checkbox 列 + 行内删除按钮 |
| `SessionsPage.vue` | 接入 composable |

## 验证

- `go test ./internal/biz/... ./internal/service/...` ✅
- `cd web && pnpm build` ✅

## 行为约定

- 删除：UI「永久删除」；后端软删除 `deleted_at`
- 保留天数：按 `last_message_at` → `updated_at` → `created_at` 计算 cutoff
- `running` 会话：单条/批量删除均跳过
- 批量勾选归档：无二次确认，显示进度条
- 批量勾选删除：确认弹窗 + 进度条
- 按天数操作：预览 matched 数 → 确认 → 进度 + 刷新
