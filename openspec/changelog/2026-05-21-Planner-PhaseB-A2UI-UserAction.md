# Planner Phase B — A2UI 组件渲染与 userAction

**日期**：2026-05-21

## 摘要

- Chat A2UI：JSONL → `reduceA2UISurface` → 组件树（Text/Button/Row/Column/Image/Icon）。
- Button 交互：`userAction` 经 WS `user_message.content` 单行 JSON 回传（交叉 51 消息机制）。
- 侧栏 Agent 无 `settings` 或 `planner_kind` 为空时，`hydrateAgentSettings` 调用 `getAgent` 补全展示策略。
- 迁移：`docs/sql/02_agent_planner.sql` 已可对 `cmd/data/arenea.sqlite` 执行。

## 前端锚点

| 职责 | 路径 |
|------|------|
| Surface 归约 | `web/src/features/chat/a2uiSurfaceState.ts` |
| 绑定解析 | `web/src/features/chat/a2uiBind.ts` |
| userAction 信封 | `web/src/features/chat/a2uiUserAction.ts` |
| Settings 补全 | `web/src/features/chat/agentPlannerSettings.ts` |
| 组件节点 | `web/src/components/chat/A2UIComponentNode.vue` |
| 发送桥接 | `useChatWorkspace.submitA2UIUserAction` → `useChatSender.sendAgentUserContent` |

## 测试

- `a2uiSurfaceState.spec.ts`、`a2uiUserAction.spec.ts`、`agentPlannerSettings.spec.ts`
