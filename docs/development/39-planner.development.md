# Planner 规划 — 开发计划

> **版本**：2026-06-17 | **状态**：🟡 后端 + Agent 设置已落地；Chat ReAct 展示未实现；A2UI 组件已实现但未集成到消息流
> **需求**：[39-planner.md](./39-planner.md) · **设计**：[39-planner.design.md](./39-planner.design.md)

---

## 1. 模块定位

Planner 规划：为 Agent 提供 BuiltinPlanner、ReActPlanner、A2UIPlanner 三种规划能力（运行时 `internal/agent/planner.Select`）。

**代码锚点**：

| 层 | 路径 |
|----|------|
| 后端校验 | `internal/biz/planner.go`、`internal/biz/planner_test.go` |
| 领域模型 | `internal/biz/agent_types.go`（`PlannerKind`/`PlannerConfigJSON`）、`internal/biz/agent_settings.go`（`ContextCfg`） |
| 运行时桥接 | `internal/agent/planner/selector.go`、`internal/agent/planner/build.go`、`internal/agent/planner/config.go`、`internal/agent/planner/selector_test.go` |
| Agent 集成 | `internal/agent/trpc_build.go`（`plannerKind`/`plannerConfigJSON`/`agentplanner.Select`） |
| Service 映射 | `internal/service/agent.go`（`PlannerKind`/`PlannerConfigJSON` ↔ proto） |
| Ent Schema | `internal/data/ent/schema/agent_runtime_setting.go`（`planner_kind`/`planner_config_json` 字段） |
| Data 映射 | `internal/data/agent_repo.go`、`internal/data/agent_runtime_mapping_test.go` |
| SQL 迁移 | `internal/data/sql/migrations/20260607_agent_runtime_patches.sql`（`planner_kind` + `planner_config_json` 列） |
| Proto | `api/kratos/agent/v1/agent.proto`（field 100 `planner_kind`、field 102 `planner_config_json`） |
| 设置表单契约 | `web/src/features/agents/plannerConfig.ts` |
| 设置表单状态 | `web/src/features/agents/useAgentPlannerForm.ts` |
| 设置 UI | `web/src/components/agents/AgentPlannerSection.vue` |
| 设置编排 | `web/src/features/agents/useAgentSettingsPage.ts` |
| 设置页 | `web/src/pages/agent-settings/AgentSettingsAgentTab.vue` |
| Chat 设置补全 | `web/src/features/chat/agentPlannerSettings.ts` |
| Chat 共享类型 | `web/src/features/chat/types.ts` |
| Chat 编排 | `web/src/features/chat/composables/useChatWorkspace.ts`（`activePlannerKind`） |
| Chat 消息面板 | `web/src/components/chat/ChatMessagePanel.vue`（`plannerKind` prop）、`web/src/components/chat/ChatMessageList.vue`（`plannerKind` prop） |
| A2UI 解析 | `web/src/features/chat/a2uiParse.ts` |
| A2UI Surface | `web/src/features/chat/a2uiSurfaceState.ts` |
| A2UI Children | `web/src/features/chat/a2uiChildren.ts` |
| A2UI Bind | `web/src/features/chat/a2uiBind.ts` |
| A2UI 路由 | `web/src/features/chat/a2ui/a2uiKindRegistry.ts` |
| A2UI 组件上下文 | `web/src/features/chat/a2ui/useA2UIComponent.ts` |
| A2UI userAction 构建 | `web/src/features/chat/a2uiUserAction.ts` |
| A2UI userAction 展示 | `web/src/features/chat/a2uiUserActionDisplay.ts` |
| A2UI Chat UI（未集成） | `web/src/components/chat/ChatA2UIPreview.vue`、`web/src/components/chat/ChatA2UISurface.vue`、`web/src/components/chat/A2UIComponentNode.vue` |
| A2UI Kind 路由 | `web/src/components/chat/a2ui/A2UIKindContent.vue` |
| A2UI Kind 组件 | `web/src/components/chat/a2ui/kinds/A2UIKind{Primitive,Form,Layout,Container}.vue` |

> **未实现的代码锚点**（设计文档 §7.3 描述但代码中不存在）：
> `reactPlannerTypes.ts`、`reactPlannerParse.ts`、`reactPlannerToolLink.ts`、`reactToolLinkIndex.ts`、`messagePlannerPresentation.ts`、`ChatReactSteps.vue`、`ChatMessageRow.vue`。

---

## 2. 现状评估（2026-06-17）

> 迁移自原 `39-planner.md` §1 现状分析。

**已具备（后端）**：
- 运行时选择与 `planner_config_json` 参数注入（Builtin / A2UI）；A2UI 集成本地 Pipeline
- `planner_kind` / `planner_config_json` 数据库持久化与 API 往返
- Web types / wire 字段贯通
- 迁移 SQL 合并于 `internal/data/sql/migrations/20260607_agent_runtime_patches.sql`

**已具备（前端 Agent 设置）**：
- Agent 设置页「规划模式」表单（`AgentPlannerSection`）；`reasoning_effort` 前后端枚举校验
- Chat 侧 `agentPlannerSettings.ts` 在列表 API 省略 settings 时补全 `planner_kind`

**已具备（前端 A2UI 组件，独立存在）**：
- A2UI StandardCatalog 组件树（14 种组件：Text/Divider/Image/Icon/Video/Button/TextField/CheckBox/List/Row/Column/Card/Modal/Tabs）
- Button `userAction` 上行构建与用户消息友好摘要
- `ChatA2UIPreview.vue` / `ChatA2UISurface.vue` / `A2UIComponentNode.vue` 组件已实现，但**未集成到 Chat 消息流**（`ChatMessageList` 未引用）

**未实现（前端 Chat ReAct 展示）**：
- ReAct 步骤卡（`ChatReactSteps` 不存在）
- `/*PLANNING*/` 等标签解析（`reactPlannerParse.ts` 不存在）
- ReAct ACTION ↔ tool_call 链接（`reactToolLinkIndex.ts` / `reactPlannerToolLink.ts` 不存在）
- 展示门面 `messagePlannerPresentation.ts` 不存在

**未实现（其他）**：
- 历史脏 config 清理脚本（`02_agent_planner_legacy_cleanup.sql` 不存在）
- A2UI 表单字段可编辑（dataModel 双向绑定）

### 现状评估表

| 项 | 状态 | 证据 |
|----|------|------|
| 后端持久化 + 参数注入 | ✅ | `ValidatePlannerKind` / `ValidatePlannerConfigJSON`（`internal/biz/planner.go`） |
| Agent 设置规划表单 | ✅ | `AgentPlannerSection` + `validatePlannerForm`（含 `reasoning_effort` 枚举） |
| Chat settings 补全 | ✅ | `hydrateAgentSettings`（`web/src/features/chat/agentPlannerSettings.ts`） |
| 空 kind 三态文案 + biz | ✅ | UI banner + 非 `{}` config 400 |
| A2UI userAction 上行构建 | ✅ | `a2uiUserAction.ts`（WS `user_message` 单行 JSON） |
| userAction 用户气泡摘要 | ✅ | `a2uiUserActionDisplay.ts` |
| A2UI 组件树（独立组件） | ✅ | `A2UIComponentNode` + `a2uiKindRegistry` 表驱动（14 种） |
| A2UI 组件集成到 Chat 消息流 | ⏳ | `ChatA2UIPreview` 存在但 `ChatMessageList` 未引用 |
| Chat ReAct 步骤卡 | ❌ | `ChatReactSteps` / `reactPlannerParse.ts` 不存在 |
| ReAct ACTION ↔ tool_call | ❌ | `reactToolLinkIndex.ts` / `reactPlannerToolLink.ts` 不存在 |
| 展示门面 | ❌ | `messagePlannerPresentation.ts` 不存在 |
| 历史脏 config 清理脚本 | ❌ | `02_agent_planner_legacy_cleanup.sql` 不存在 |

---

## 3. 差距与后续（backlog）

| 优先级 | 项 | 说明 |
|--------|-----|------|
| P1 | Chat ReAct 步骤卡 | 实现 `reactPlannerParse.ts` / `ChatReactSteps.vue`，解析 `/*PLANNING*/` 等标签为步骤卡片 |
| P1 | 展示门面 | 实现 `messagePlannerPresentation.ts`，统一 a2ui/react 展示入口 |
| P1 | A2UI 组件集成到 Chat | `ChatMessageList` 引用 `ChatA2UIPreview`，按 `plannerKind` 分发 |
| P2 | ReAct ACTION ↔ tool_call | 实现 `reactToolLinkIndex.ts` / `reactPlannerToolLink.ts`，会话级 O(n) 索引 |
| P2 | `ChatMessagePanel` 必填 `reactToolLinkIndex` | 索引就绪后改为必填 prop |
| P3 | 历史脏 config 清理脚本 | 编写 `02_agent_planner_legacy_cleanup.sql`（`planner_kind=''` 且 config 非 `{}` 的清理） |
| P5 | A2UI 表单可编辑 | TextField/CheckBox + dataModel 双向绑定 |
| P5 | StandardCatalog 长尾 | AudioPlayer / Dropdown / Switch / Carousel / TabBar / WebView 等 |
| P5 | ReAct 链接增强 | 多 ACTION↔多 tool、Team 会话、流式乱序（设计 §7.3 已记录局限） |
| P5 | 性能 | 按 `message.id` memo `parseReactPlannerContent`（列表极长时） |

---

## 4. 开发阶段

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1 | Ent + Data + SQL + Biz 校验 | ✅ |
| Phase 2 | `planner_config_json` + runtime build | ✅ |
| Phase 3 | 设置 UI + Chat ReAct/A2UI MVP 预览 | 🟡 设置 UI ✅；Chat ReAct ❌；A2UI 组件 ✅ 但未集成 |
| Phase B | A2UI 组件树 + userAction 上行 + settings hydrate | 🟡 组件树 + userAction + hydrate ✅；Chat 集成 ❌ |
| Phase C | StandardCatalog 余量 + ReAct↔tool_call 内嵌卡片 | ❌ ReAct↔tool_call 未实现 |
| Review | `reactToolLinkIndex`、去 O(n²)、类型统一、三态、枚举校验 | ❌ 依赖 Phase C |

---

## 5. 任务清单（摘要）

| # | 任务 | 状态 |
|---|------|------|
| 1–10 | 后端 P0–P1（持久化 + 校验 + 运行时选择） | ✅ |
| 11–15 | 前端设置 UI + Chat MVP 展示 | 🟡 设置 UI ✅；Chat MVP ❌ |
| 16–19 | A2UI 组件树 + tool 链接 + 去重 | 🟡 A2UI 组件树 ✅；tool 链接 ❌ |
| 20 | `reactToolLinkIndex` 会话级索引 | ❌ |
| 21 | `messagePlannerPresentation` 单一入口 + 必填 index | ❌ |
| 22 | `reactPlannerTypes` + `a2uiKindRegistry` | 🟡 `a2uiKindRegistry` ✅；`reactPlannerTypes` ❌ |
| 23 | `reasoning_effort` 前后端枚举 + hydrate 清洗 | ✅ |

---

## 6. 验收标准

- [x] `planner_kind` / `planner_config_json` 持久化与 API 往返
- [x] Builtin / A2UI 参数可在 Agent 设置页配置并保存
- [x] 空 `planner_kind` 仅允许 `{}`；非法 `reasoning_effort` 前后端拒绝
- [ ] Chat ReAct 步骤卡 + ACTION 内嵌工具卡；独立 tool activity 行去重
- [ ] Chat A2UI 组件树集成到消息流；Button `userAction` 上行；用户消息友好摘要
- [ ] `ChatMessagePanel` / `ChatMessageRow` 必填 `reactToolLinkIndex`
- [x] 侧栏选 Agent 时 `getAgent` 补全 `planner_kind`（无 settings 或 kind 为空）

---

## 7. 依赖与风险

- 新库执行 `internal/data/sql/migrations/20260607_agent_runtime_patches.sql`（planner_kind + planner_config_json 列）；无独立 legacy cleanup 脚本。
- **空 kind 三态**：API 保存 / 运行时 Builtin / Chat 展示启发式 — 须在 UI 区分（见 design §7.2）。
- **ReAct 链接**：流式过程中工具行可能短暂重复，索引随 `displayMessages` 刷新后收敛（索引尚未实现）。
- 列表 API 省略 `settings` 时依赖 `hydrateAgentSettings`；正文标签仍作 `planner_kind` 为空时的展示兜底（展示组件尚未实现）。
- **A2UI 组件未集成**：`ChatA2UIPreview` / `ChatA2UISurface` / `A2UIComponentNode` 已实现但 `ChatMessageList` 未引用，需在 Phase 3 后续补齐集成。

---

## 8. 测试

| 范围 | 命令 |
|------|------|
| 前端 planner 表单 | `cd web && pnpm vitest run src/features/agents/__tests__/plannerConfig.spec.ts` |
| 前端 Chat settings 补全 | `cd web && pnpm vitest run src/features/chat/__tests__/agentPlannerSettings.spec.ts` |
| 前端 A2UI 解析/路由/surface/userAction | `cd web && pnpm vitest run src/features/chat/__tests__/a2uiKindRegistry.spec.ts src/features/chat/__tests__/a2uiSurfaceState.spec.ts src/features/chat/__tests__/a2uiUserAction.spec.ts src/features/chat/__tests__/a2uiUserActionDisplay.spec.ts src/features/chat/__tests__/a2uiChildren.spec.ts` |
| 后端 biz | `go test ./internal/biz/... -run TestValidatePlanner` |
| 后端 selector | `go test ./internal/agent/planner/...` |

> **未实现的测试**（依赖对应源码尚未实现）：
> `reactPlannerParse.spec.ts`、`reactToolLinkIndex.spec.ts`、`reactPlannerToolLink.spec.ts`、`messagePlannerPresentation.spec.ts`。
