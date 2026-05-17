# Planner 规划 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 BuiltinPlanner 可用；❌ ReAct/A2UI 未实现
> **需求**：[39 planner.md](./39%20planner.md) · **设计**：[39 planner.design.md](./39%20planner.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Planner 规划：为 Agent 提供规划能力，支持 BuiltinPlanner（简单规划）、ReActPlanner（结构化推理）和 A2UIPlanner（A2UI 协议规划）。

**代码锚点**：
- `internal/agent/trpc_build.go` — `DialogMode == "plan"` → BuiltinPlanner
- `pkg/trpc-agent-go/planner/` — trpc-agent-go Planner 框架

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| BuiltinPlanner | ✅ | `trpcbuiltin.New(trpcbuiltin.Options{})` |
| ReActPlanner | ❌ | 未集成 |
| A2UIPlanner | ❌ | 未集成 |
| 自定义规划 prompt | ❌ | 无自定义 prompt 配置 |
| 规划结果结构化 | ❌ | 无结构化处理 |

---

## 3. 差距与优化

1. **P2**：ReActPlanner 未集成，Agent 无法使用结构化推理（PLANNING/REASONING/ACTION/FINAL_ANSWER）。
2. **P2**：A2UIPlanner 未集成，Agent 无法使用 A2UI 协议规划。
3. **P3**：无自定义规划 prompt 配置，用户无法定制规划策略。
4. **P3**：规划结果无结构化处理，无法提取规划步骤。

---

## 4. 开发阶段

- **Phase 1**：集成 ReActPlanner
- **Phase 2**：集成 A2UIPlanner
- **Phase 3**：自定义规划 prompt + 结构化处理

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `trpc_build.go`：集成 ReActPlanner | P2 | — |
| 2 | `trpc_build.go`：集成 A2UIPlanner | P2 | — |
| 3 | Agent RuntimeSettings 增加 planner_type 字段 | P2 | — |
| 4 | 自定义规划 prompt 配置 | P3 | — |
| 5 | 规划结果结构化处理 | P3 | — |

---

## 6. 验收标准

- [ ] Agent 可选择 Builtin/ReAct/A2UI 规划器
- [ ] ReAct 规划器输出 PLANNING/REASONING/ACTION 标签
- [ ] A2UI 规划器输出 A2UI 协议格式

---

## 7. 依赖与风险

- ReAct/A2UI Planner 依赖 trpc-agent-go 框架实现
- 规划器选择需与 Agent 对话模式联动
