# 39 Planner Review

> **评分**：81 / 100 | **风险等级**：P1  
> **文档**：[39-planner-development.md](../需求/39-planner-development.md)  
> **代码锚点**：`internal/agent/planner/` · `internal/biz/agent_usecase.go`（planner_kind）· `web/src/components/agents/AgentPlannerSection.vue`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | 持久化 + 设置 UI + Chat ReAct/A2UI 组件树 + tool 去重 + Review 打磨 ✅；表单可编辑/长尾组件 🟡 |
| 架构一致性 | 22 | 25 | `agent/planner.Select` 在 BuildTRPCLLMAgent 中正确调用；Planner 配置从 biz.Agent 行读取 |
| 后端实现质量 | 17 | 20 | `planner_kind` / `planner_config_json` 持久化到 biz.Agent；builtin_planner 实现完整 |
| 前端实现质量 | 14 | 15 | `AgentPlannerSection.vue` ✅；Chat 页 ReAct 步骤卡 + A2UI 组件树 ✅；Planner 配置表单可编辑待补 |
| 测试与验证 | 6 | 10 | `pkg/trpc-agent-go/planner/builtin/builtin_planner_test.go` ✅；Aranea 侧集成测试待补 |
| 文档一致性 | 6 | 10 | `39-planner-development.md` P2/P3 闭环同步 |

---

## Planner 种类

| kind | 描述 | 状态 |
|------|------|------|
| `""` (空) | 使用 Agent 默认行为（无显式 Planner） | ✅ 三态说明已文档化 |
| `react` | ReAct（思考-行动-观察循环） | ✅ |
| `a2ui` | Agent-to-UI（结构化输出 + 用户交互） | ✅ |
| 自定义 | 通过 `planner_config_json` 扩展 | 🟡 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| `planner_kind` 持久化 | ✅ |
| `planner_config_json` 浅合并（`MergeAgentConfigJSON`） | ✅ |
| `AgentPlannerSection.vue` 设置 UI | ✅ |
| Chat 页 ReAct 步骤卡（`reactToolLinkIndex` 去重） | ✅ |
| A2UI 组件树预览 + `userAction` 回传 | ✅ |
| `planner_kind` 空字符串三态说明 | ✅ |
| Planner 配置 JSON 表单可编辑 | 🟡 P2 |
| 长尾 Planner 组件（自定义/高级） | 🟡 P2 |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| PLN-P1-01 | ReAct 步骤卡中 `reactToolLinkIndex` 去重逻辑若与 A2UI 混用可能出现顺序问题 | 补 ReAct + A2UI 混合场景测试 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| PLN-P2-01 | Planner 配置 JSON 表单可编辑（非 raw JSON 输入）待实现 | 基于 Schema 生成表单（复用 PluginSchemaForm 模式） |
| PLN-P2-02 | 自定义 Planner 注册机制文档化 | 在 `39 planner.md` 中说明如何扩展 Planner |

---

## 建议优化路径

1. 实现 Planner 配置表单可编辑（P2）。
2. 补 ReAct + A2UI 混合场景测试。
3. 文档化自定义 Planner 注册扩展点。
