# Agent 设置 — 开发计划

> **版本**：2026-06-17 | **状态**：✅ 端到端可用；Composable 大幅拆分；页壳 ~298 行；9 Tab
> **文档同步**：[changelog/2026-05-21-Agent-Modules-2-8-DocSync.md](../changelog/2026-05-21-Agent-Modules-2-8-DocSync.md)
> **需求**：[5 agent-setting.md](./5%20agent-setting.md) · **设计**：[5 agent-setting.design.md](./5%20agent-setting.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-06

---

## 1. 模块定位

Agent 设置页：管理 Agent 的详细配置，包括系统提示、工具选择、记忆配置、进化设置、RuntimeSettings 等。

**代码锚点**：

### 1.1 后端代码锚点

| 文件 | 职责 |
|------|------|
| `api/kratos/agent/v1/agent.proto` | Proto 契约（AgentService、Agent / AgentRuntimeSettings / AgentPromptFile 消息） |
| `internal/service/agent.go` | AgentService（RPC 实现 + Proto/Biz 转换） |
| `internal/biz/agent_usecase.go` | AgentUsecase + 窄接口（AgentReader/Writer/Settings/Files 等） |
| `internal/biz/agent_types.go` | 领域模型（Agent / AgentRuntimeSettings / AgentPromptFile） |
| `internal/biz/agent_settings.go` | 领域视图访问器（IdentityCfg / MemoryCfg / ToolsCfg 等） |
| `internal/biz/agent_effective_tools.go` | Effective Tools 计算（9 tool groups, 8 profiles） |
| `internal/biz/agent_mcp_effective.go` | Effective MCP 计算 |
| `internal/biz/agent_settings_helpers.go` | helpers（withSettingDefaults, settingsFromLegacyConfig 等） |
| `internal/biz/agent_defaults.go` | Agent 默认值 |
| `internal/biz/planner.go` | Planner 配置 |
| `internal/biz/ralph_loop.go` | Ralph Loop 配置 |
| `internal/biz/tool_agent_override_runtime.go` | ToolOverride 运行时 |
| `internal/data/agent_repo.go` | Data 层 Repo 实现 |
| `internal/data/agent_runtime_patch.go` | Runtime Settings 补丁逻辑 |
| `internal/data/ent/schema/agent_runtime_setting.go` | Ent Schema（140+ 字段） |
| `internal/data/ent/schema/agent_prompt_file.go` | Ent Schema（提示文件） |
| `internal/agent/trpc_build.go` | BuildTRPCLLMAgent（装配链） |
| `internal/agent/prompt.go` | BuildSystemPrompt（系统提示构建） |

### 1.2 前端代码锚点

| 文件 | 职责 |
|------|------|
| `web/src/pages/AgentSettingsPage.vue` | 主页面（QTabs + QTabPanels，9 Tab） |
| `web/src/pages/agent-settings/AgentSettingsAgentTab.vue` | Agent 属性 Tab |
| `web/src/pages/agent-settings/AgentSettingsMemoryTab.vue` | 记忆 Tab（L0-L4） |
| `web/src/pages/agent-settings/AgentSettingsSkillsTab.vue` | Skill / 工具 Tab |
| `web/src/pages/agent-settings/AgentSettingsPromptSection.vue` | 系统提示模式分区 |
| `web/src/pages/agent-settings/AgentChannelRefsSection.vue` | Channel 引用分区 |
| `web/src/components/agents/AgentSettingsHeader.vue` | 顶栏 |
| `web/src/components/agents/AgentSettingsA2ATab.vue` | A2A Tab |
| `web/src/components/agents/AgentSettingsA2AEndpointTab.vue` | A2A Endpoint 子组件 |
| `web/src/components/agents/AgentAdvancedDialog.vue` | 高级对话框 |
| `web/src/components/agents/AgentEvolutionPanel.vue` | 进化面板 |
| `web/src/components/agents/AgentFilesPanel.vue` | 文件面板 |
| `web/src/components/agents/AgentHooksPanel.vue` | 钩子面板 |
| `web/src/components/agents/AgentPlannerSection.vue` | Planner 分区 |
| `web/src/components/agents/AgentToolsSection.vue` | 工具分区 |
| `web/src/components/agents/AgentUsageQuotaPanel.vue` | 用量配额面板 |
| `web/src/components/agents/AgentLearningLoopPanel.vue` | 学习闭环面板 |
| `web/src/components/agents/MemoryLevelSection.vue` | 记忆分层折叠 |
| `web/src/components/agents/AIRefineButton.vue` | AI Refine 按钮 |
| `web/src/components/agents/FieldGuideHint.vue` | 字段引导提示 |
| `web/src/features/agents/useAgentSettingsPage.ts` | 设置页主 Composable |
| `web/src/features/agents/useAgentRuntimeConfig.ts` | 运行时配置表单 |
| `web/src/features/agents/agentRuntimeConfig.ts` | 默认值 + 选项 |
| `web/src/features/agents/agentRuntimeConfigHydrate.ts` | 表单填充 |
| `web/src/features/agents/agentRuntimeConfigSerialize.ts` | 表单序列化 |
| `web/src/features/agents/useAgentA2AEndpointTab.ts` | A2A Endpoint Composable |
| `web/src/features/agents/useAgentA2AProxyTab.ts` | A2A Proxy Composable |
| `web/src/features/agents/useAgentPlannerForm.ts` | Planner 表单 |
| `web/src/features/agents/useAgentRalphLoopForm.ts` | Ralph Loop 表单 |
| `web/src/features/agents/useAgentSkillCatalog.ts` | Skill 目录 |
| `web/src/features/agents/useAgentToolsCatalog.ts` | Tools 目录 |
| `web/src/features/agents/useAgentToolOverrides.ts` | Tool 覆盖 |
| `web/src/features/agents/useAgentChannelRefs.ts` | Channel 引用 |
| `web/src/features/agents/useAgentEvolutionPanel.ts` | 进化面板 |
| `web/src/features/agents/useAgentHooksPanel.ts` | 钩子面板 |
| `web/src/features/agents/useAgentPromptFiles.ts` | 提示文件 |
| `web/src/features/agents/useAgentPromptPreview.ts` | 提示预览 |
| `web/src/features/agents/useAgentAvatarIcon.ts` | 头像图标 |
| `web/src/features/agents/useLearningLoopPanel.ts` | 学习闭环面板 |
| `web/src/features/agents/fieldGuides.ts` | FieldGuide 注册表（10 scopes） |
| `web/src/features/agents/plannerConfig.ts` | Planner 配置 |
| `web/src/features/agents/ralphLoopConfig.ts` | Ralph Loop 配置 |
| `web/src/features/agents/wireNormalize.ts` | Wire 数据规范化 |
| `web/src/features/agents/types.ts` | TypeScript 类型 |
| `web/src/features/agents/api.ts` | API 调用 |
| `web/src/features/agents/api.learning.ts` | 学习闭环 API |

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| Agent 基础设置 CRUD | ✅ | UpdateAgent RPC |
| RuntimeSettings CRUD | ✅ | UpsertAgentRuntimeSettings RPC |
| Effective Tools 计算 | ✅ | `agent_effective_tools.go` |
| Effective MCP 计算 | ✅ | `agent_mcp_effective.go` |
| PromptFile 管理 | ✅ | 独立 RPC + 表 |
| ToolOverride | ✅ | `tool.proto` CRUD + `ApplyAgentToolOverrides` + `trpc_build.applyRuntimeToolConfigs` |
| **A2A Endpoint Tab** | ✅ | `AgentSettingsA2AEndpointTab.vue` + Proxy Tab |
| 系统提示模式切换 | ✅ | `system_prompt_mode` 字段 + `FilesForMode` |
| Prompt 预览 | ✅ | `GetAgentPromptPreview` RPC |
| Planner config | ✅ | `AgentRuntimeSettings` planner_kind/planner_config_json 字段 |
| Ralph Loop | ✅ | `AgentRuntimeSettings` ralph_loop_* 字段 |
| Agent variant/kind/source | ✅ | proto 字段 28-33 |
| `MergeAgentConfigJSON` | ✅ | shallow merge for PATCH |
| ToggleFavorite | ✅ | `ToggleFavorite` RPC + `uc.ToggleFavorite` |
| DuplicateAgent | ✅ | `DuplicateAgent` RPC |
| CheckAgentKey | ✅ | `CheckAgentKey` RPC |
| Evolution Metrics/Suggestions | ✅ | `GetAgentEvolutionMetrics` / `GetAgentEvolutionSuggestions` / `ApplyEvolutionSuggestion` / `RejectEvolutionSuggestion` RPC |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 设置页 QTabs | ✅ | `AgentSettingsPage.vue`（~298 行页壳）+ 9 tabs（agent/memory/files/permissions/skills/evolution/learning/hooks/a2a） |
| 顶栏 | ✅ | `AgentSettingsHeader.vue` |
| 系统提示模式四卡片 | ✅ | `AgentSettingsPromptSection.vue` / agent Tab |
| Agent 个性 / 模型 / 工具 / 记忆 | ✅ | 各 section 于 agent/memory Tab |
| ToolOverride | ✅ | `AgentToolOverridesPanel`（via `useAgentToolOverrides.ts`） |
| 文件 Tab | ✅ | `AgentFilesPanel`（见 [6-agent-setting-file-development.md](./6-agent-setting-file-development.md)） |
| 进化 Tab | ✅ | `AgentEvolutionPanel`（见模块 7） |
| A2A Tab | ✅ | `AgentSettingsA2ATab` / Endpoint |
| 高级对话框 | ✅ | `AgentAdvancedDialog.vue` |
| PlannerSection | ✅ | `AgentPlannerSection.vue` |
| RalphLoopSection | ✅ | `AgentRalphLoopSection.vue`（via `useAgentRalphLoopForm.ts`） |
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
9. **P3**：`AgentRuntimeSetting` Schema 140+ 字段超标（DB-DEBT-01），后续应拆分。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-06）**：补 `biz/tool_override.go` + Repo + Service CRUD + 前端页面
- **Phase 2**：`other_config` JSON Merge Patch 策略 + 记忆配置分组折叠
- **Phase 2（A2A）**：`AgentSettingsA2ATab.vue` — Endpoint / Proxy 视图（与 [26-a2a-development.md](./26-a2a-development.md) 2.6 对齐）
- **Phase 3**：系统提示模式联动 + 重置默认值 + Schema 拆分评估

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
| 7 | 记忆配置分组折叠 UI | 前端 | P3 | — | 需求 §10 | ✅ |
| 8 | 系统提示模式与文件 Tab 联动 | 前端 | P3 | — | 需求 §5 | — |
| 9 | 各分区"重置为默认值"按钮 | 前端 | P3 | — | — | — |
| 10 | Agent 设置 A2A Tab（Endpoint + Proxy） | 前端 | P1 | — | 需求 §13 | ✅ |
| 11 | Planner config（planner_kind/planner_config_json） | 后端+前端 | P2 | — | — | ✅ |
| 12 | Ralph Loop config（ralph_loop_* fields） | 后端+前端 | P2 | — | — | ✅ |
| 13 | FieldGuide system（10 scopes） | 前端 | P2 | — | — | ✅ |
| 14 | Runtime Config 3-file split（defaults/hydrate/serialize） | 前端 | P2 | — | — | ✅ |
| 15 | Learning loop panel + composable | 前端 | P2 | — | — | ✅ |
| 16 | Debug trace 清理 | 前端 | P3 | — | — | — |
| 17 | Learning loop 文档对齐 | 文档 | P3 | — | — | — |
| 18 | `AgentRuntimeSetting` Schema 拆分评估（DB-DEBT-01） | 后端 | P3 | — | — | — |

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
- [x] A2A Tab（Endpoint + Proxy）可用
- [x] ToggleFavorite / DuplicateAgent / CheckAgentKey 可用
- [ ] `go test ./internal/biz/... -run TestToolOverride` 通过
- [ ] 系统提示模式与文件 Tab 联动
- [ ] 各分区"重置为默认值"功能
- [ ] Debug trace 清理
- [ ] Learning loop 文档对齐

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
| 模块26 A2A | AgentCard CRUD + 远程发现 | A2A Tab 数据源 |

### 7.2 风险

- ToolOverride 与 Tool 系统紧耦合，需确保覆盖优先级正确（Override > Agent 默认 > 全局默认）
- M2 多租户后需 workspace_id 隔离
- `other_config` 深度合并需考虑并发写入冲突
- `AgentRuntimeSetting` Schema 140+ 字段（DB-DEBT-01），认知复杂度高，后续拆分需评估迁移成本

---

## 8. 改动文件清单（最近迭代）

| 文件 | 改动类型 | 说明 |
|------|----------|------|
| `internal/biz/agent_types.go` | 扩展 | AgentRuntimeSettings 新增 Planner/Ralph Loop/Context 压缩等字段 |
| `internal/biz/agent_settings.go` | 扩展 | 新增领域视图访问器 |
| `internal/biz/agent_usecase.go` | 重构 | 拆分为窄接口 + AgentUsecaseDeps |
| `internal/service/agent.go` | 扩展 | 新增 ToggleFavorite/Duplicate/CheckAgentKey/Evolution RPC |
| `api/kratos/agent/v1/agent.proto` | 扩展 | Agent 消息新增 agent_kind/a2a_proxy_config 等字段；AgentRuntimeSettings 新增 130+ 字段 |
| `internal/data/ent/schema/agent_runtime_setting.go` | 扩展 | Schema 新增 140+ 字段 |
| `web/src/pages/AgentSettingsPage.vue` | 重构 | 9 Tab 布局 |
| `web/src/pages/agent-settings/` | 新增 | Tab 子组件拆分 |
| `web/src/features/agents/` | 扩展 | Composable 大幅拆分（30+ 文件） |

> 错误处理规格见设计文档 §十「错误处理规格」。
