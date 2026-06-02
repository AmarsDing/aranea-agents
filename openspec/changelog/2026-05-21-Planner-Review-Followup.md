# Planner Review 跟进（P1–P3）

**日期**：2026-05-21

## P1

- `Message` / `ReactToolLinkIndex` / `ReactStepWithTools` 收敛至 `features/chat/types.ts`；组件从 `types` 引类型。
- 删除 `shouldSuppressStandaloneToolCard` O(n²) 回退；生产路径必须传 `buildReactToolLinkIndex`（默认 `emptyReactToolLinkIndex()` 不去重）。
- 单测：`messagePlannerPresentation` 索引路径 + 无索引不 suppress。

## P2

- 空 kind **三态**文案：`AgentPlannerSection` + `39 planner.design.md` §7.2。
- `docs/sql/02_agent_planner_legacy_cleanup.sql` 清理历史脏 config。
- `reasoning_effort` biz 枚举：`low|medium|high|max`。

## P3

- `features/chat/a2ui/a2uiKindRegistry.ts` 表驱动 `A2UIKindContent` 路由。
- ReAct 流式链接策略文档化：`39 planner.design.md` §7.3。

## 二次跟进

- `ChatMessagePanel` / `ChatMessageRow`：`reactToolLinkIndex` 必填；`Message` 从 `features/chat/types` 引入。
- `buildMessagePresentation` 必填 index；`reactStepsForIndex` 仅读缓存，无行内 enrich。
- `reactPlannerTypes.ts` 解耦 `types.ts` ↔ `reactPlannerParse`。
- `validatePlannerForm` 对齐 `reasoning_effort` 枚举。

## 打磨

- `reactStepsForIndex`：`cached !== undefined` 即信任索引（含空数组）。
- `buildMessagePresentation` 移除未使用的 `messages` 参数。
- `plannerFormFromSettings`：非法 `reasoning_effort`  hydrate 时清空。

## 文档同步（同批）

- `docs/需求/39-planner-development.md` — 锚点表、阶段、验收、测试命令
- `docs/需求/39 planner.design.md` §7.1–7.3 — 文件分层与 index 契约
- `docs/需求/39 planner.md`、`README-development.md`、`frontend-pages.md`、`51 消息机制.md` §4.5

## 验证

- `go test ./internal/biz/... -run TestValidatePlanner`
- `pnpm vitest run src/features/chat/__tests__`
- `pnpm vitest run src/features/agents/__tests__/plannerConfig.spec.ts`
