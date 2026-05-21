# Agent 标题与顶栏 — 开发计划

> **版本**：2026-05-21 | **状态**：✅ 顶栏 + Prompt 预览；🟡 标签推导简化；标题自动生成未通
> **需求**：[8 agent-title.md](./8%20agent-title.md) · **设计**：[8 agent-title.design.md](./8%20agent-title.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

Agent 设置顶栏（身份、标签、操作）与系统提示词预览。Session 标题生成与 Agent 顶栏分离。

**代码锚点**：
- `web/src/components/agents/AgentSettingsHeader.vue` — 设置顶栏（非 `AgentHeader.vue`）
- `web/src/components/agents/AgentAdvancedDialog.vue` — 「高级」通道/工作区等
- `internal/service/agent.go` — `GetAgentPromptPreview`
- `internal/service/session_title_llm.go` — Session 标题 LLM（**非** Agent display_name）

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| Prompt 预览 | ✅ | `GetAgentPromptPreview` + `composePromptPreview` |
| Session 标题 LLM | ✅ | `LLMSessionTitleGenerator` |
| Agent 标题自动生成 | ❌ | 无 `GenerateAgentTitle` RPC |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 设置顶栏 | ✅ | `AgentSettingsHeader`：`showEvolving` = pending + self_evolve |
| 系统提示词对话框 | ✅ | `AgentSettingsPage` `promptDialog` + 模式 Tab |
| 高级设置模态 | ✅ | `AgentAdvancedDialog`（通道/Chat/工作区片段） |
| 列表卡片标签 | ✅ | `AgentCard` 进化 chip 同 `isAgentEvolving` |
| Agent 标题自动生成 | ❌ | 创建仍手动填 `display_name` |

---

## 3. 差距与优化

| ID | 优先级 | 待优化项 |
|----|--------|----------|
| TITLE-01 | P2 | 顶栏/列表「进化中」与 pending 建议对齐（✅ 迭代 9） |
| TITLE-02 | P3 | `GenerateAgentTitle` RPC（复用 Session 标题生成器） |
| TITLE-03 | P3 | 大文本预览性能（虚拟滚动/分段） |
| TITLE-04 | P3 | 高级设置与需求 §8 全文对齐（供应商级联等待补） |

---

## 4. 顶栏标签推导（当前实现）

| 标签 | 当前条件 | 目标（需求） |
|------|----------|--------------|
| 模式 chip | `system_prompt_mode` → `promptModeLabel` | ✅ |
| 进化中 | `self_evolve === true` | 建议：`self_evolve` **且** 存在 pending 建议 |
| 状态 | `agents.status` | ✅ |
| A2A | `agent_kind` / `a2a_endpoint_enabled` | ✅（列表 `AgentCard`） |

---

## 5. 验收标准

- [x] 设置顶栏可打开 Prompt 预览与高级对话框
- [x] Prompt 预览可按 complete/task/minimized/none 切换
- [ ] 「进化中」标签与建议状态一致
- [ ] （可选）创建时可 LLM 生成 display_name
