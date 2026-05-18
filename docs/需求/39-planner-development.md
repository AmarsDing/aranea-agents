# Planner 规划 — 开发计划

> **版本**：2026-05-19 | **状态**：🟡 运行时选择器已实现；❌ 数据层/配置/前端未完成
> **需求**：[39 planner.md](./39%20planner.md) · **设计**：[39 planner.design.md](./39%20planner.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Planner 规划：为 Agent 提供规划能力，支持 BuiltinPlanner（内置思维）、ReActPlanner（结构化推理）和 A2UIPlanner（A2UI 协议规划）。

**代码锚点**：
- `internal/agent/planner/selector.go` — `Select(dialogMode, plannerKind)` 规划器选择
- `internal/agent/trpc_build.go` — `agentplanner.Select()` 调用 + `plannerKind(ag)` 提取
- `internal/biz/agent_types.go` — `AgentRuntimeSettings.PlannerKind` 字段
- `api/kratos/agent/v1/agent.proto` — `planner_kind = 100` 字段

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| BuiltinPlanner 运行时选择 | ✅ | `selector.go`: `case "builtin"` → `trpcbuiltin.New(trpcbuiltin.Options{})` |
| ReActPlanner 运行时选择 | ✅ | `selector.go`: `case "react"` → `trpcreact.New()` |
| A2UIPlanner 运行时选择 | ✅ | `selector.go`: `case "a2ui"` → `trpca2ui.New()` |
| DialogMode 兼容 | ✅ | `selector.go`: 默认分支 `dialogMode == "plan"` → BuiltinPlanner |
| Agent 构建集成 | ✅ | `trpc_build.go`: `agentplanner.Select(deps.DialogMode, plannerKind(ag))` |
| Proto 字段 | ✅ | `agent.proto`: `planner_kind = 100` |
| Biz 字段 | ✅ | `agent_types.go`: `PlannerKind string` |
| Service 映射 | ✅ | `agent.go`: proto ↔ biz 映射 |
| Ent Schema 字段 | ❌ | `agent_runtime_setting.go` 缺少 `planner_kind` 字段 |
| Data 层映射 | ❌ | `agent_repo.go` 的 `entRuntimeToBiz`/`applyBizRuntimeToCreate` 缺少映射 |
| planner_config_json 字段 | ❌ | 全链路缺失（Proto/Biz/Ent/Data/Service） |
| BuiltinPlanner 参数配置 | ❌ | `selector.go` 始终使用 `trpcbuiltin.Options{}`（空配置） |
| A2UIPlanner 参数配置 | ❌ | `selector.go` 始终使用 `trpca2ui.New()`（无 Option） |
| 前端配置 UI | ❌ | 无规划模式配置组件 |
| Chat 页面规划步骤展示 | ❌ | 无 ReAct 步骤卡片、无 A2UI 渲染预览 |

---

## 3. 差距与优化

1. **P0**：`planner_kind` 数据层断裂 — Proto/Biz/Service 已有字段，但 Ent Schema 和 Data 映射缺失，导致字段无法持久化。这是阻塞性缺陷。
2. **P1**：规划器参数不可配置 — BuiltinPlanner 的 reasoning_effort/thinking_enabled/thinking_tokens 和 A2UIPlanner 的 Schema 均使用默认值，无法按 Agent 定制。
3. **P2**：前端无规划模式配置 UI — 用户无法通过界面配置规划模式。
4. **P2**：Chat 页面无规划步骤展示 — ReAct 步骤和 A2UI 渲染预览缺失。

---

## 4. 开发阶段

- **Phase 1**：修复数据层断裂（Ent Schema + Data 映射 + DB 迁移）
- **Phase 2**：规划器参数配置（planner_config_json 全链路 + selector 扩展）
- **Phase 3**：前端配置面板 + Chat 页面集成

---

## 5. 任务清单

| # | 任务 | 优先级 | EP | 涉及文件 |
|---|------|--------|-----|---------|
| 1 | Ent Schema 增加 `planner_kind` 字段 | P0 | — | `internal/data/ent/schema/agent_runtime_setting.go` |
| 2 | Data 映射增加 `PlannerKind` | P0 | — | `internal/data/agent_repo.go` |
| 3 | 数据库迁移 | P0 | — | `sql/` |
| 4 | Proto 增加 `planner_config_json` 字段 | P1 | — | `api/kratos/agent/v1/agent.proto` |
| 5 | Biz 增加 `PlannerConfigJSON` 字段 + 配置结构 | P1 | — | `internal/biz/agent_types.go`, `internal/biz/agent_settings.go` |
| 6 | Service 增加 `PlannerConfigJSON` 映射 | P1 | — | `internal/service/agent.go` |
| 7 | Ent Schema 增加 `planner_config_json` 字段 | P1 | — | `internal/data/ent/schema/agent_runtime_setting.go` |
| 8 | Data 映射增加 `PlannerConfigJSON` | P1 | — | `internal/data/agent_repo.go` |
| 9 | selector.go 扩展 `Select()` 接受配置参数 | P1 | — | `internal/agent/planner/selector.go` |
| 10 | trpc_build.go 传入配置参数 | P1 | — | `internal/agent/trpc_build.go` |
| 11 | 前端 types + wire 增加 `planner_config_json` | P2 | — | `web/src/features/agents/types.ts`, `wireNormalize.ts` |
| 12 | 前端规划模式配置面板 | P2 | — | `web/src/features/agents/` |
| 13 | Chat 页面 ReAct 步骤展示 | P2 | — | `web/src/features/chat/` |
| 14 | Chat 页面 A2UI 渲染预览 | P2 | — | `web/src/features/chat/` |

---

## 6. 验收标准

- [ ] `planner_kind` 可持久化到数据库（Ent Schema + Data 映射完整）
- [ ] `planner_config_json` 全链路贯通（Proto → Biz → Data → Service）
- [ ] BuiltinPlanner 参数可配置（reasoning_effort/thinking_enabled/thinking_tokens）
- [ ] A2UIPlanner 参数可配置（Instruction/Schema 等）
- [ ] 前端可配置规划模式和参数
- [ ] Chat 页面展示 ReAct 规划步骤
- [ ] Chat 页面展示 A2UI 渲染预览

---

## 7. 依赖与风险

- P0 数据层断裂是阻塞性缺陷：当前 `planner_kind` 通过 API 设置后无法持久化，重启后丢失
- 规划器参数配置依赖 `planner_config_json` 全链路实现
- A2UI 渲染预览组件复杂度较高，需评估是否独立迭代
