# Planner / Chat — Review 修复（P0–P2）

**日期**：2026-05-21

## P0 — ReAct 工具卡去重

- `collectReactLinkedToolIds` / `shouldSuppressStandaloneToolCard`：已关联到 ReAct `/*ACTION*/` 的 `tool_call` activity 行不再单独渲染。
- `buildMessagePresentation` 设置 `suppressToolRow`；`ChatMessageRow` 整行隐藏，工具详情仅保留在 `ChatReactSteps` 内嵌 `ChatExecutionCard`。

## P1 — 编排与校验

- `buildMessagePresentation()` 收拢 `ChatMessageRow` 的 presentation / ReAct 关联 / 工具行策略。
- `reactPlannerParse` 移除无意义 `steps: finalAnswer ? steps : steps`。
- `ValidatePlannerConfigJSON(plannerKind, raw)`：按 kind 校验字段白名单与类型（react 仅 `{}`）。

## P2 — 结构与可观测性

- A2UI 拆分：`useA2UIComponent` + `A2UIKindContent` + `A2UIChildList`；`A2UIComponentNode` 为薄壳。
- Chat 单测统一至 `web/src/features/chat/__tests__/`。
- `hydrateAgentSettings` 失败时 `import.meta.env.DEV` 下 `console.warn`。
