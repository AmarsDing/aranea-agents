# Planner P2/P3 优化

**日期**：2026-05-21

## P2

- **`reactToolLinkIndex`**：`useChatWorkspace` 对 `displayMessages` 一次 `buildReactToolLinkIndex`；`buildMessagePresentation` 读缓存，避免每行 O(n²) enrich。
- **`internal/memory/trpc`**：子包改名为 `package trpcmem`，`sqlite_adapter` 使用 `aramemory.TruncateString`，解除与父包同名导致的编译/链接问题。

## P3

- **A2UI**：`a2ui/kinds/{Primitive,Form,Layout,Container}.vue` + 薄 `A2UIKindContent` 路由。
- **userAction 用户消息**：`a2uiUserActionDisplay` 解析 JSON 行并渲染「A2UI 操作 · name」摘要。
- **遗留语义**：`planner_kind` 为空时 `planner_config_json` 仅允许 `{}`（非空字段 BadRequest）。

## 测试

- `pnpm vitest run src/features/chat/__tests__` — 27 passed
