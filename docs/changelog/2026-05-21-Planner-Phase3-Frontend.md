# Planner Phase 3 — 前端设置与 Chat 展示

**日期**：2026-05-21 · **模块**：Planner (39)

## 摘要

落地 Agent 设置「规划模式」表单与 Chat ReAct 步骤卡 / A2UI JSONL 预览（MVP）；按单一职责拆分 `plannerConfig`、`reactPlannerParse`、`messagePlannerPresentation` 等模块；同步 `39 planner*.md` 设计 §7。

## 前端

| 区域 | 变更 |
|------|------|
| `features/agents/plannerConfig.ts` | 表单 parse/serialize/validate + 单测 |
| `components/agents/AgentPlannerSection.vue` | Builtin / ReAct / A2UI 条件表单 |
| `useAgentSettingsPage.ts` | hydrate + save `planner_kind` / `planner_config_json` |
| `features/chat/reactPlannerParse.ts` | ReAct 标签解析 + 单测 |
| `features/chat/a2uiParse.ts` | JSONL 行解析 |
| `features/chat/messagePlannerPresentation.ts` | 展示模式决策 |
| `ChatReactSteps.vue` / `ChatA2UIPreview.vue` | Chat 展示组件 |
| `ChatMessageRow.vue` | reasoning + steps + 正文叠加 |
| `useChatWorkspace` / `ChatPage` | `activePlannerKind` 下发 |

## 文档

- `39 planner.design.md` §7 已实现说明
- `39-planner-development.md` Phase 3 ✅
- `39 planner.md` / `README-development.md` 接入度更新

## 未包含

- A2UI Text/Button/Row 等组件级渲染与 userAction 回传（Phase B）
