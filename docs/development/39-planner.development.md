# Planner 规划 — 开发计划

> **版本**：2026-05-21 | **状态**：🟢 P0–P3 + Phase B/C + Review 打磨已落地
> **需求**：[39 planner.md](./39%20planner.md) · **设计**：[39 planner.design.md](./39%20planner.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **变更**：[changelog/2026-05-21-Planner-Review-Followup.md](../changelog/2026-05-21-Planner-Review-Followup.md)

---

## 1. 模块定位

Planner 规划：为 Agent 提供 BuiltinPlanner、ReActPlanner、A2UIPlanner 三种规划能力（运行时 `internal/agent/planner.Select`）。

**代码锚点**：

| 层 | 路径 |
|----|------|
| 后端校验 | `internal/biz/planner.go` |
| 运行时桥接 | `internal/agent/planner/{selector,build,config}.go` |
| SQL | `internal/data/sql/migrations/20260607_agent_runtime_patches.sql`（planner_kind + planner_config_json 列） |
| 设置表单 | `features/agents/plannerConfig.ts`、`components/agents/AgentPlannerSection.vue` |
| ReAct 类型/解析 | `features/chat/reactPlannerTypes.ts`、`reactPlannerParse.ts` |
| ReAct 工具链接 | `features/chat/reactPlannerToolLink.ts`、`reactToolLinkIndex.ts` |
| 展示门面 | `features/chat/messagePlannerPresentation.ts` |
| Chat 类型 | `features/chat/types.ts`（`Message`、`ReactToolLinkIndex`） |
| A2UI | `a2uiParse.ts`、`a2uiSurfaceState.ts`、`a2ui/a2uiKindRegistry.ts`、`components/chat/a2ui/kinds/*`、`components/chat/a2ui/A2UIKindContent.vue`、`a2uiUserAction.ts`、`a2uiUserActionDisplay.ts` |
| Chat 编排 | `features/chat/composables/useChatWorkspace.ts`（`buildReactToolLinkIndex`、`activePlannerKind`） |
| Chat UI | `ChatMessagePanel`（**必填** `reactToolLinkIndex`）、`ChatMessageRow`、`ChatReactSteps`、`ChatA2UIPreview`、`ChatA2UISurface`、`A2UIComponentNode` |

---

## 2. 现状评估（2026-05-21）

| 项 | 状态 | 证据 |
|----|------|------|
| 后端持久化 + 参数注入 | ✅ | `ValidatePlannerKind` / `ValidatePlannerConfigJSON` |
| Agent 设置规划表单 | ✅ | `AgentPlannerSection` + `validatePlannerForm`（含 `reasoning_effort` 枚举） |
| Chat ReAct 步骤卡 | ✅ | `ChatReactSteps` + `reactPlannerParse` |
| ReAct ACTION ↔ tool_call | ✅ | `reactToolLinkIndex` O(n) + `buildMessagePresentation` 去重 |
| Chat A2UI 组件树 | ✅ | `A2UIComponentNode` + `a2uiKindRegistry` 表驱动 |
| A2UI userAction 上行 | ✅ | WS `user_message` 单行 JSON（[51 消息机制](./51%20消息机制.md) §4.5） |
| userAction 用户气泡摘要 | ✅ | `a2uiUserActionDisplay.ts` |
| Chat settings 补全 | ✅ | `hydrateAgentSettings` |
| 空 kind 三态文案 + biz | ✅ | UI banner + 非 `{}` config 400 |
| 历史脏 config 清理脚本 | ✅ | `02_agent_planner_legacy_cleanup.sql` |

---

## 3. 差距与后续（backlog）

| 优先级 | 项 | 说明 |
|--------|-----|------|
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
| Phase 3 | 设置 UI + Chat ReAct/A2UI MVP 预览 | ✅ |
| Phase B | A2UI 组件树 + userAction 上行 + settings hydrate | ✅ |
| Phase C | StandardCatalog 余量 + ReAct↔tool_call 内嵌卡片 | ✅ |
| Review | `reactToolLinkIndex`、去 O(n²)、类型统一、三态、枚举校验 | ✅ |
| 打磨 | 必填 index、无行内 enrich、`reactPlannerTypes`、`cached !== undefined` | ✅ |

**Changelog 索引**：`2026-05-21-Planner-DocSync-P0-P1`、`Phase3-Frontend`、`PhaseB-A2UI-UserAction`、`PhaseC-Catalog-ReactTools`、`P2-P3-Optimization`、`Review-Fixes`、`Review-Followup`。

---

## 5. 任务清单（摘要）

| # | 任务 | 状态 |
|---|------|------|
| 1–10 | 后端 P0–P1 | ✅ |
| 11–15 | 前端设置 + Chat MVP 展示 | ✅ |
| 16–19 | A2UI 组件树 + tool 链接 + 去重 | ✅ |
| 20 | `reactToolLinkIndex` 会话级索引 | ✅ |
| 21 | `messagePlannerPresentation` 单一入口 + 必填 index | ✅ |
| 22 | `reactPlannerTypes` + `a2uiKindRegistry` | ✅ |
| 23 | `reasoning_effort` 前后端枚举 + hydrate 清洗 | ✅ |

---

## 6. 验收标准

- [x] `planner_kind` / `planner_config_json` 持久化与 API 往返
- [x] Builtin / A2UI 参数可在 Agent 设置页配置并保存
- [x] 空 `planner_kind` 仅允许 `{}`；非法 `reasoning_effort` 前后端拒绝
- [x] Chat ReAct 步骤卡 + ACTION 内嵌工具卡；独立 tool activity 行去重
- [x] Chat A2UI 组件树；Button `userAction` 上行；用户消息友好摘要
- [x] `ChatMessagePanel` / `ChatMessageRow` 必填 `reactToolLinkIndex`
- [x] 侧栏选 Agent 时 `getAgent` 补全 `planner_kind`（无 settings 或 kind 为空）

---

## 7. 依赖与风险

- 新库执行 `internal/data/sql/migrations/20260607_agent_runtime_patches.sql`（planner_kind + planner_config_json 列）；无独立 legacy cleanup 脚本。
- **空 kind 三态**：API 保存 / 运行时 Builtin / Chat 展示启发式 — 须在 UI 区分（见 design §7.2）。
- **ReAct 链接**：流式过程中工具行可能短暂重复，索引随 `displayMessages` 刷新后收敛。
- 列表 API 省略 `settings` 时依赖 `hydrateAgentSettings`；正文标签仍作 `planner_kind` 为空时的展示兜底。

---

## 8. 测试

| 范围 | 命令 |
|------|------|
| 前端 planner 表单 + Chat planner 相关 | `cd web && pnpm vitest run src/features/agents/__tests__/plannerConfig.spec.ts src/features/chat/__tests__/reactPlannerParse.spec.ts src/features/chat/__tests__/reactToolLinkIndex.spec.ts src/features/chat/__tests__/reactPlannerToolLink.spec.ts src/features/chat/__tests__/messagePlannerPresentation.spec.ts src/features/chat/__tests__/a2uiKindRegistry.spec.ts src/features/chat/__tests__/a2uiSurfaceState.spec.ts src/features/chat/__tests__/a2uiUserAction.spec.ts src/features/chat/__tests__/a2uiUserActionDisplay.spec.ts` |
| 后端 biz | `go test ./internal/biz/... -run TestValidatePlanner` |
| 后端 selector | `go test ./internal/agent/planner/...` |
