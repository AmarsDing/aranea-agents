# Plugin Phase 6 Review 修复 — 变更摘要

**日期**：2026-05-21  
**模块**：Plugin / Chat / Agent

## 摘要

按 Phase 6 Code Review 的 P1–P3 项完成修复：cost_guard 双路径分桶一致、await 元数据持久化与缓存、high_risk 跳过反思、SRP 拆分与前端 i18n/常量。

## P1

| 项 | 修复 |
|----|------|
| cost_guard BeforeModel vs ModelSelector 分桶不一致 | `CostGuardPlugin` 改为 `Runtime.BudgetTrackerForContext(ctx)` 动态解析 per-agent tracker |

## P2

| 项 | 修复 |
|----|------|
| `ChatRunStatusPersister` 仅写 `reply` | 接口扩展 `PersistAwaitMarkers(..., biz.ChatAwaitMeta)` |
| GetRunStatus await 元数据 race | 内存 `awaitMetaCache` + tool confirm 路径同步 persist |
| `high_risk_tools_need_confirm` 未生效 | `retry_and_reflect` 跳过 confirmation_guard 匹配工具 |
| 前端 token / i18n | `awaitConstants.ts`、`useAwaitReply.ts`、zh/en i18n |

## P3

| 项 | 修复 |
|----|------|
| `tool_confirmation.go` SRP | 拆出 `tool_invocation_recorder.go` |
| rules 编辑器 | stable `id` key、regex 校验、i18n |

## 验证

```bash
go test ./internal/agent/... ./internal/plugin/trpc/... ./internal/service/...
go build ./...
cd web && pnpm build
```
