# Agent 设置 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用；🟡 ToolOverride 缺失
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
- `internal/agent/trpc_build.go` — BuildTRPCLLMAgent（装配链）

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
| ToolOverride | 🟡 | proto 有 `agent_override_count` 字段，但无 biz Usecase / Repo / Service |
| **A2A Endpoint Tab** | ✅ | `AgentSettingsA2AEndpointTab.vue` + Proxy Tab |
| 系统提示模式切换 | ✅ | `system_prompt_mode` 字段 + `FilesForMode` |
| Prompt 预览 | ✅ | `GetAgentPromptPreview` RPC |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 设置页整体布局（QTabs） | 🟡 待验证 | 需确认 `AgentSettingsPage.vue` 是否已实现 |
| 系统提示模式四卡片 | 🟡 待验证 | 需确认 complete/task/minimized/none 卡片选择 |
| Agent 个性区 | 🟡 待验证 | 需确认名称/描述/状态等编辑表单 |
| 模型与预算区 | 🟡 待验证 | 需确认 Provider/Model 下拉 |
| 子 Agent 配置区 | 🟡 待验证 | 需确认 subagents_* 字段表单 |
| 工具策略区 | 🟡 待验证 | 需确认 allow/deny/concurrent 多选 |
| 记忆配置区 | 🟡 待验证 | 需确认 L0-L4 各层参数表单 |
| ToolOverride 管理 | ❌ 未实现 | 无前端页面，无后端 CRUD |
| 记忆分组折叠 | ❌ 未实现 | L0-L4 字段繁多，无分组折叠 UI |
| `other_config` 深度 merge | 🟡 待验证 | PATCH 时需深度合并 JSON，避免覆盖 |

---

## 3. 差距与优化

1. **P2（EP-BIZ-06）**：`tool_agent_override` 表已存在（Ent schema），但无 `biz/tool_override.go` Usecase、无 Repo 实现、无 Service CRUD。Agent 设置页无法管理工具级别的参数覆盖。
2. **P3**：Agent 设置页"记忆配置"区域（L0-L4 各层参数）字段繁多，前端无分组折叠，用户体验差。
3. **P2**：`other_config` PATCH 时需深度合并 JSON，当前可能覆盖整块配置。需明确 merge 策略（RFC 7396 JSON Merge Patch 或自定义）。
4. **P3**：系统提示模式切换后，"文件"Tab 应联动显示当前模式下哪些文件生效，当前未实现联动。
5. **P3**：Agent 设置页各分区缺少"重置为默认值"功能。
6. ~~**P1（A2A）**：Agent 设置页 A2A Tab~~ → ✅ 已实现

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-06）**：补 `biz/tool_override.go` + Repo + Service CRUD + 前端页面
- **Phase 2**：`other_config` JSON Merge Patch 策略 + 记忆配置分组折叠
- **Phase 2（A2A）**：`AgentSettingsA2ATab.vue` — Endpoint / Proxy 视图（与 [26-a2a-development.md](./26-a2a-development.md) 2.6 对齐）
- **Phase 3**：系统提示模式联动 + 重置默认值

---

## 5. 任务清单

| # | 任务 | 层 | 优先级 | EP | 需求回溯 |
|---|------|-----|--------|-----|----------|
| 1 | `biz/tool_override.go`：模型 + Repo 接口 + Usecase | 后端 | P2 | EP-BIZ-06 | 需求 §9.2 |
| 2 | `data/tool_agent_override.go`：Repo 实现 | 后端 | P2 | EP-BIZ-06 | — |
| 3 | `service/tool.go`：增加 ToolOverride CRUD RPC | 后端 | P2 | EP-BIZ-06 | — |
| 4 | proto 增加 ToolOverride 相关 RPC | 后端 | P2 | EP-BIZ-06 | — |
| 5 | 前端 Agent 设置页工具覆盖管理 | 前端 | P2 | EP-BIZ-06 | 需求 §9.2 |
| 6 | `other_config` PATCH 深度合并策略实现 | 后端 | P2 | — | 需求 §5 |
| 7 | 记忆配置分组折叠 UI | 前端 | P3 | — | 需求 §9 |
| 8 | 系统提示模式与文件 Tab 联动 | 前端 | P3 | — | 需求 §5 |
| 9 | 各分区"重置为默认值"按钮 | 前端 | P3 | — | — |
| 10 | Agent 设置 A2A Tab（Endpoint + Proxy） | 前端 | P1 | — | [5 agent-setting.md](./5%20agent-setting.md) §10 | ✅ |

---

## 6. 验收标准

- [ ] Agent 设置页可管理每个工具的参数覆盖
- [ ] 覆盖参数在 `BuildTRPCLLMAgent` 装配链中生效
- [ ] `other_config` PATCH 不覆盖未提交的字段
- [ ] 记忆配置区可折叠/展开各层参数
- [ ] `go test ./internal/biz/... -run TestToolOverride` 通过

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
