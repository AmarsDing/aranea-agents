# Agent 设置 — 开发计划

> **版本**：2026-06-06 | **状态**：✅ 端到端可用；Composable 大幅拆分；页壳 ~298 行；9 Tab
> **文档同步**：[changelog/2026-05-21-Agent-Modules-2-8-DocSync.md](../changelog/2026-05-21-Agent-Modules-2-8-DocSync.md)
> **需求**：[5 agent-setting.md](./5%20agent-setting.md) · **设计**：[5 agent-setting.design.md](./5%20agent-setting.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-06

---

## 1. 模块定位

Agent 设置页：管理 Agent 的详细配置，包括系统提示、工具选择、记忆配置、进化设置、RuntimeSettings 等。

**代码锚点**：
- `api/kratos/agent/v1/agent.proto` — UpdateAgent / UpdateAgentRuntimeSettings
- `internal/service/agent.go` — AgentService
- `internal/biz/agent_usecase.go` — AgentUsecase
- `internal/biz/agent_settings.go` — AgentSettings（effective tools / MCP）
- `internal/biz/agent_effective_tools.go` — effective tools engine（9 tool groups, 8 profiles）
- `internal/biz/agent_settings_helpers.go` — helpers（withSettingDefaults, settingsFromLegacyConfig, etc.）
- `internal/agent/trpc_build.go` — BuildTRPCLLMAgent（装配链）
- `web/src/features/agents/useAgentSettingsPage.ts` — settings page composable
- `web/src/features/agents/useAgentSettingsPersistence.ts` — persistence logic
- `web/src/features/agents/useAgentRuntimeConfig.ts` — runtime config form
- `web/src/features/agents/agentRuntimeConfig.ts` — defaults + options
- `web/src/features/agents/agentRuntimeConfigHydrate.ts` — form hydration
- `web/src/features/agents/agentRuntimeConfigSerialize.ts` — form serialization
- `web/src/features/agents/useAgentProviderModelPicker.ts` — Provider/Model picker
- `web/src/features/agents/useAgentChannelRefs.ts` — channel references
- `web/src/features/agents/useAgentPlannerForm.ts` — Planner form
- `web/src/features/agents/useAgentRalphLoopForm.ts` — Ralph Loop form
- `web/src/features/agents/useAgentSkillCatalog.ts` — Skill catalog
- `web/src/features/agents/useAgentToolsCatalog.ts` — Tools catalog
- `web/src/features/agents/fieldGuides.ts` — FieldGuide registry（10 scopes）
- `web/src/components/agents/AIRefineButton.vue` — AI Refine button
- `web/src/components/agents/AgentPlannerSection.vue` — Planner section
- `web/src/components/agents/AgentRalphLoopSection.vue` — Ralph Loop section
- `web/src/components/agents/AgentChannelRefsSection.vue` — Channel refs section
- `web/src/components/agents/AgentUsageQuotaPanel.vue` — Usage quota panel
- `web/src/components/agents/AgentLearningLoopPanel.vue` — Learning loop panel

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| Agent 基础设置 CRUD | ✅ | UpdateAgent RPC |
| RuntimeSettings CRUD | ✅ | UpdateAgentRuntimeSettings RPC |
| Effective Tools 计算 | ✅ | `agent_effective_tools.go` |
| Effective MCP 计算 | ✅ | `agent_mcp_effective.go` |
| PromptFile 管理 | ✅ | 独立 RPC + 表 |
| ToolOverride | ✅ | `tool.proto` CRUD + `ApplyAgentToolOverrides` + `trpc_build.applyRuntimeToolConfigs` |
| **A2A Endpoint Tab** | ✅ | `AgentSettingsA2AEndpointTab.vue` + Proxy Tab |
| 系统提示模式切换 | ✅ | `system_prompt_mode` 字段 + `FilesForMode` |
| Prompt 预览 | ✅ | `GetAgentPromptPreview` RPC |
| Planner config | ✅ | `AgentRuntimeSettings` planner_type/react/a2ui fields |
| Ralph Loop | ✅ | `AgentRuntimeSettings` ralph_loop_* fields |
| Agent variant/kind/source | ✅ | proto fields 28-33 |
| `MergeAgentConfigJSON` | ✅ | shallow merge for PATCH |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 设置页 QTabs | ✅ | `AgentSettingsPage.vue`（~298 行页壳）+ 9 tabs（agent/memory/files/permissions/skills/evolution/learning/hooks/a2a） |
| 顶栏 | ✅ | `AgentSettingsHeader.vue` |
| 系统提示模式四卡片 | ✅ | `AgentSettingsPromptSection` / agent Tab |
| Agent 个性 / 模型 / 工具 / 记忆 | ✅ | 各 section 于 agent/memory Tab |
| ToolOverride | ✅ | `AgentToolOverridesPanel` |
| 文件 Tab | ✅ | `AgentFilesPanel`（见 [6-agent-setting-file-development.md](./6-agent-setting-file-development.md)） |
| 进化 Tab | ✅ | `AgentEvolutionPanel`（见模块 7） |
| A2A Tab | ✅ | `AgentSettingsA2ATab` / Endpoint |
| 高级对话框 | ✅ | `AgentAdvancedDialog.vue` |
| PlannerSection | ✅ | `AgentPlannerSection.vue` |
| RalphLoopSection | ✅ | `AgentRalphLoopSection.vue` |
| LearningLoopPanel | ✅ | `AgentLearningLoopPanel.vue` |
| UsageQuotaPanel | ✅ | `AgentUsageQuotaPanel.vue` |
| ChannelRefsSection | ✅ | `AgentChannelRefsSection.vue` |
| AIRefineButton | ✅ | `AIRefineButton.vue` |
| FieldGuide system | ✅ | 10 scopes with word budgets and examples |
| Runtime Config system | ✅ | 3-file split（defaults/hydrate/serialize） |
| 记忆分组折叠 | ✅ | L0-L4 via `MemoryLevelSection.vue` |
| `config_json` PATCH merge | ✅ | `MergeAgentConfigJSON` |

---

## 3. 差距与优化

1. ~~**P2（EP-BIZ-06）**：ToolOverride CRUD~~ → ✅ 见 `tool.proto` 与 `AgentToolOverridesPanel`。
2. ~~**P3**：记忆配置区分组折叠 UI~~ → ✅ `MemoryLevelSection.vue` L0-L4 分组折叠。
3. ~~**P2**：`config_json` PATCH 覆盖~~ → ✅ 顶层键浅合并 `MergeAgentConfigJSON`；嵌套 `other_config` 对象仍为整对象替换（若需 RFC7396 再开任务）。
4. **P3**：系统提示模式切换后，"文件"Tab 应联动显示当前模式下哪些文件生效，当前未实现联动。
5. **P3**：Agent 设置页各分区缺少"重置为默认值"功能。
6. ~~**P1（A2A）**：Agent 设置页 A2A Tab~~ → ✅ 已实现
7. **P3**：Debug trace 清理（运行时调试日志残留）。
8. **P3**：Learning loop 文档与代码实现对齐。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-06）**：补 `biz/tool_override.go` + Repo + Service CRUD + 前端页面
- **Phase 2**：`other_config` JSON Merge Patch 策略 + 记忆配置分组折叠
- **Phase 2（A2A）**：`AgentSettingsA2ATab.vue` — Endpoint / Proxy 视图（与 [26-a2a-development.md](./26-a2a-development.md) 2.6 对齐）
- **Phase 3**：系统提示模式联动 + 重置默认值

---

## 5. 任务清单

| # | 任务 | 层 | 优先级 | EP | 需求回溯 | 状态 |
|---|------|-----|--------|-----|----------|------|
| 1 | `biz/tool_override.go`：模型 + Repo 接口 + Usecase | 后端 | P2 | EP-BIZ-06 | 需求 §9.2 | ✅ |
| 2 | `data/tool_agent_override.go`：Repo 实现 | 后端 | P2 | EP-BIZ-06 | — | ✅ |
| 3 | `service/tool.go`：增加 ToolOverride CRUD RPC | 后端 | P2 | EP-BIZ-06 | — | ✅ |
| 4 | proto 增加 ToolOverride 相关 RPC | 后端 | P2 | EP-BIZ-06 | — | ✅ |
| 5 | 前端 Agent 设置页工具覆盖管理 | 前端 | P2 | EP-BIZ-06 | 需求 §9.2 | ✅ |
| 6 | `other_config` PATCH 深度合并策略实现 | 后端 | P2 | — | 需求 §5 | ✅ |
| 7 | 记忆配置分组折叠 UI | 前端 | P3 | — | 需求 §9 | ✅ |
| 8 | 系统提示模式与文件 Tab 联动 | 前端 | P3 | — | 需求 §5 | — |
| 9 | 各分区"重置为默认值"按钮 | 前端 | P3 | — | — | — |
| 10 | Agent 设置 A2A Tab（Endpoint + Proxy） | 前端 | P1 | — | [5 agent-setting.md](./5%20agent-setting.md) §10 | ✅ |
| 11 | Planner config（planner_type/react/a2ui） | 后端+前端 | P2 | — | — | ✅ |
| 12 | Ralph Loop config（ralph_loop_* fields） | 后端+前端 | P2 | — | — | ✅ |
| 13 | FieldGuide system（10 scopes） | 前端 | P2 | — | — | ✅ |
| 14 | Runtime Config 3-file split（defaults/hydrate/serialize） | 前端 | P2 | — | — | ✅ |
| 15 | Learning loop panel + composable | 前端 | P2 | — | — | ✅ |
| 16 | Debug trace 清理 | 前端 | P3 | — | — | — |
| 17 | Learning loop 文档对齐 | 文档 | P3 | — | — | — |

---

## 6. 验收标准

- [x] Agent 设置页可管理每个工具的参数覆盖
- [x] 覆盖参数在 `BuildTRPCLLMAgent` 装配链中生效
- [x] `config_json` PATCH 顶层键合并（未提交键保留）
- [x] 记忆配置区可折叠/展开各层参数（`MemoryLevelSection.vue`）
- [x] 设置页 9 Tab 布局（agent/memory/files/permissions/skills/evolution/learning/hooks/a2a）
- [x] Planner / Ralph Loop / Channel Refs / Usage Quota / Learning Loop 各 section 可用
- [x] FieldGuide 系统 10 scope 可用
- [x] Runtime Config 3-file split（defaults/hydrate/serialize）可用
- [ ] `go test ./internal/biz/... -run TestToolOverride` 通过
- [ ] 系统提示模式与文件 Tab 联动
- [ ] 各分区"重置为默认值"功能

---

## 7. 依赖与风险

### 7.1 跨模块依赖

| 依赖模块 | 依赖项 | 说明 |
|----------|--------|------|
| 模块2 Agent创建 | Provider/Model 校验逻辑 | 设置页变更 Provider/Model 需复用校验 |
| 模块6 Agent文件 | PromptFile 管理 | "文件"Tab 数据源 |
| 模块7 Agent进化 | 进化开关 + 指标 | "进化"Tab 数据源 |
| 模块8 Agent标题 | 顶栏 + Prompt 预览 | 设置页顶栏组件 |
| 模块9 Provider | Provider/Model 列表 | 模型下拉数据源 |
| 模块23 Tools | 工具注册表 | 工具策略 allow/deny 选项数据源 |

### 7.2 风险

- ToolOverride 与 Tool 系统紧耦合，需确保覆盖优先级正确（Override > Agent 默认 > 全局默认）
- M2 多租户后需 workspace_id 隔离
- `other_config` 深度合并需考虑并发写入冲突

---

## 8. 错误处理规格

| 场景 | HTTP 状态码 | 错误码 | 前端行为 |
|------|------------|--------|----------|
| ToolOverride 引用不存在的工具 | 400 Bad Request | `TOOL_NOT_FOUND` | inline error：工具不存在 |
| `other_config` JSON 格式错误 | 400 Bad Request | `CONFIG_JSON_INVALID` | Toast：配置格式错误 |
| RuntimeSettings 字段越界 | 400 Bad Request | `SETTING_OUT_OF_RANGE` | 对应字段 inline error |
| 并发更新冲突 | 409 Conflict | `VERSION_CONFLICT` | Toast：数据已被修改，请刷新 |
| Agent 不存在 | 404 Not Found | `AGENT_NOT_FOUND` | 跳转列表页 |
