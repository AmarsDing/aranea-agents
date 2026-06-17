# Agent 标题与顶栏 — 开发计划

> **版本**：2026-06-06 | **状态**：✅ 顶栏 + Prompt 预览 + KindBadge + 通道绑定（内嵌于高级模态）；🟡 标题自动生成未通
> **需求**：[8 agent-title.md](./8%20agent-title.md) · **设计**：[8 agent-title.design.md](./8-agent-title.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

Agent 设置顶栏（身份、标签、操作）与系统提示词预览。Session 标题生成与 Agent 顶栏分离。

**代码锚点**：
- `web/src/components/agents/AgentSettingsHeader.vue` — 设置顶栏（非 `AgentHeader.vue`）
- `web/src/components/agents/AgentAdvancedDialog.vue` — 「高级」模态（通道/工作区/Reasoning/压缩/沙箱，**内嵌**通道绑定，无独立 `AgentChannelRefsSection.vue`）
- `web/src/components/agents/AgentPromptPreviewDialog.vue` — preview dialog（非 `PromptPreviewDialog.vue`）
- `web/src/components/agents/KindBadge.vue` — ownership kind badge
- `web/src/components/agents/agentUi.ts` — `promptModeLabel` / `promptModes` / `statusLabel`
- `web/src/components/agents/AgentCard.vue` — 列表卡片（含「进化中」chip）
- `web/src/features/agents/useAgentPromptPreview.ts` — preview composable
- `web/src/features/agents/types.ts` — `Agent` / `AgentPromptPreview` 类型
- `web/src/stores/agents/detail.ts` — `useAgentDetailStore`（`fetchPromptPreview`）
- `web/src/pages/AgentSettingsPage.vue` — 组装顶栏 + 高级模态 + 预览对话框
- `internal/service/agent.go` — `GetAgentPromptPreview`（调用 `chatagent.BuildPreviewReport`）
- `internal/agent/prompt.go` — `BuildSystemPrompt` + `BuildIndustryContext` + `StaticRuntimeCapabilityCue` / `DynamicRuntimeCapabilityCue`（`RuntimeCapabilityCue` 已 Deprecated）
- `internal/agent/prompt_preview.go` — `BuildPreviewReport` + `PreviewReport`
- `internal/biz/agent_settings_helpers.go` — `FilesForMode`（**非** `agent_defaults.go`）
- `internal/service/session_title_llm.go` — Session 标题 LLM（**非** Agent display_name）
- `api/kratos/agent/v1/agent.proto` — `Agent` / `GetAgentPromptPreviewRequest|Response` / `PromptSectionEstimate`

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| Prompt 预览 | ✅ | `GetAgentPromptPreview` + `chatagent.BuildPreviewReport`（返回 summary/instruction/sections/token 估算） |
| `BuildSystemPrompt` | ✅ | `internal/agent/prompt.go`（含 `categoryResponsibility` + `position_key`/`agent_variant` 注入） |
| `BuildIndustryContext` | ✅ | taxonomy context injection |
| `StaticRuntimeCapabilityCue` / `DynamicRuntimeCapabilityCue` | ✅ | 拆分后的运行时能力提示 |
| `RuntimeCapabilityCue` | 🟡 Deprecated | 等价于 Static + Dynamic，新代码应使用拆分函数 |
| `position_key` / `agent_variant` | ✅ | proto fields 29-30 |
| `kind` / `source` / `readonly` | ✅ | proto fields 28/32/33 |
| `planner_kind` | ✅ | proto field 100 |
| Session 标题 LLM | ✅ | `LLMSessionTitleGenerator` |
| Agent 标题自动生成 | ❌ | 无 `GenerateAgentTitle` RPC |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 设置顶栏 | ✅ | `AgentSettingsHeader.vue`：`showEvolving` = pending + self_evolve；`KindBadge` + 模式 chip + 状态 badge |
| KindBadge | ✅ | `KindBadge.vue` ownership type badge（内置/预设/商城/认证） |
| PromptPreviewDialog | ✅ | `AgentPromptPreviewDialog.vue` with mode tabs and token breakdown（构建期 + 运行时 + sections 分解） |
| 通道绑定 | ✅ | 内嵌于 `AgentAdvancedDialog.vue`（Channel `QSelect` + Chat ID `QInput` 级联） |
| 系统提示词对话框 | ✅ | `AgentSettingsPage` `promptDialog` + 模式 Tab |
| 高级设置模态 | ✅ | `AgentAdvancedDialog.vue`（通道/Chat/工作区/Reasoning/压缩/沙箱） |
| 列表卡片标签 | ✅ | `AgentCard.vue` 进化 chip（`evolving` prop） |
| Agent 标题自动生成 | ❌ | 创建仍手动填 `display_name` |

---

## 3. 差距与优化

| ID | 优先级 | 待优化项 |
|----|--------|----------|
| TITLE-01 | P2 | 顶栏/列表「进化中」与 pending 建议对齐 | ✅ |
| TITLE-02 | P3 | `GenerateAgentTitle` RPC（复用 Session 标题生成器） |
| TITLE-03 | P3 | 大文本预览性能（虚拟滚动/分段） |
| TITLE-04 | P3 | 高级设置与需求 §8 全文对齐（Provider/Model cascade ✅, Channel cascade ✅） |
| TITLE-05 | P3 | KindBadge + source/readonly display alignment |

---

## 4. 顶栏标签推导（当前实现）

| 标签 | 当前条件 | 目标（需求） |
|------|----------|--------------|
| 模式 chip | `system_prompt_mode` → `promptModeLabel` | ✅ |
| 进化中 | `self_evolve && pending_evolution_count > 0` | ✅ |
| 状态 | `agents.status` → `statusLabel` | ✅ |
| A2A | `agent_kind` / `a2a_endpoint_enabled` | ✅（列表 `AgentCard`） |
| KindBadge | `kind` field → ownership type badge | ✅ |
| Source badge | `source` field | ✅ |

---

## 5. 验收标准

- [x] 设置顶栏可打开 Prompt 预览与高级对话框
- [x] Prompt 预览可按 complete/task/minimized/none 切换
- [x] 「进化中」标签与建议状态一致
- [x] KindBadge 显示 ownership type
- [x] 通道绑定可用（内嵌于高级模态）
- [x] PromptPreviewDialog with mode tabs and token breakdown
- [ ] （可选）创建时可 LLM 生成 display_name
