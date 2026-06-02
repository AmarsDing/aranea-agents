# Session Phase 1b — Review Fixes

> 日期：2026-05-20  
> 关联：[Phase 1b 实现](./2026-05-20-Session-Bulk-Ops-Phase1b.md)

## 摘要

按 Code Review 结论修复 P1/P2/P3：retention 分页扫描、SQL running 守卫、not_found/truncated 响应、Service 校验、前端 Store 收敛与 notify 详情。

## P1

| 项 | 修复 |
|----|------|
| 10000 静默上限 | `loadBatchCandidatesByScope` 按 `SessionBatchPageSize=1000` 分页，上限 `SessionBatchMaxScan=100000`；响应 `truncated=true` |
| SQL 未排除 running | `batchUpdateSessions` WHERE 增加 `status != running`；归档额外 `status != archived` |
| 无效 ID 静默丢弃 | `loadBatchCandidates` 统计 `skipped_not_found`；proto/biz/前端全链路暴露 |

## P2

| 项 | 修复 |
|----|------|
| BatchArchive 重复查库 | 统一 `resolveBatchOperation`，单次 load + resolve |
| Service 校验 | `older_than_days >= 1`（无 ids 时）；batch RPC 走 `mapSessionErr` |
| 进度条瞬时完成 | 改为 indeterminate 进度条 |
| failed_ids / skipped | `formatBatchNotifyMessage` + retention 预览文案 |
| SelectionBar loading | `bulkArchiving` / `bulkDeleting` 接线 |
| 选中态跨筛选 | 筛选变更时 `clearSelection()` |
| Store 规范 | batch 方法迁入 `useSessionStore`；composable 经 Store 调用 |
| running 单条归档 | `Archive()` biz guard + 表格禁用 |

## P3

| 项 | 修复 |
|----|------|
| 常量重复 | 统一 `biz.SessionBatchPageSize` / `SessionBatchMaxScan` |
| Proto 注释 | ids + older_than_days 组合语义；`skipped_not_found` / `truncated` 字段 |
| 单测 | `validateBatchParams`、service batch validation / skipped_not_found |

## 验证

- `make api` ✅
- `go test ./internal/biz/... ./internal/service/...` ✅
- `cd web && pnpm build` ✅
