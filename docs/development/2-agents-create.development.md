# Agent 创建 — 开发计划

> **版本**：2026-06-06 | **状态**：✅ 端到端可用；P2 全部完成；Planner/Ralph Loop/FieldGuide/Taxonomy/Duplicate/BatchUpdate 已通
> **变更记录**：[changelog/2026-05-21-Agent-CreatedBy-Templates-Errors.md](../changelog/2026-05-21-Agent-CreatedBy-Templates-Errors.md) · [Modules 2–8 文档同步](../changelog/2026-05-21-Agent-Modules-2-8-DocSync.md)
> **需求**：[2 agents-create.md](./2%20agents-create.md) · **设计**：[2 agents-create.design.md](./2%20agents-create.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Agent 创建弹窗：采集创建 Agent 所需最小字段（名称、标识、分类、Provider/Model、描述、自我进化），提交后写入 `agents` 表并关闭弹窗。

**代码锚点**：
- `api/kratos/agent/v1/agent.proto` — Agent CRUD RPC
- `internal/service/agent.go` — AgentService
- `internal/biz/agent_usecase.go` — AgentUsecase
- `internal/data/agent_repo.go` — AgentRepo
- `internal/agent/trpc_build.go` — BuildTRPCLLMAgent
- `internal/biz/agent_templates.go` — `ListAgentTemplates` 内置预设
- `internal/biz/agent_context.go` — 创建时 `created_by`（auth 上下文，Create 请求不可伪造）
- `internal/biz/agent_kind.go` — `NormalizeAgentKind` / `IsA2AProxyAgent` / `HydrateAgentKind`
- `internal/biz/agent_duplicate.go` — `AgentUsecase.Duplicate` 方法
- `internal/biz/agent_effective_tools.go` — effective tools 引擎
- `web/src/utils/kratosError.ts` — `parseKratosApiError` / `mapAgentCreateFieldErrors`
- `web/src/services/axiosHandler.ts` — `CreateAgent` 自动 `skipErrorNotify`
- `web/src/features/agents/useAgentProviderModelPicker.ts` — Provider/Model picker composable
- `web/src/features/agents/useAgentModelValidation.ts` — 模型校验 composable
- `web/src/features/agents/plannerConfig.ts` — Planner 配置
- `web/src/features/agents/ralphLoopConfig.ts` — Ralph Loop 配置
- `web/src/features/agents/fieldGuides.ts` — FieldGuide 注册表（10 scopes）
- `web/src/components/agents/TaxonomyPicker.vue` — 创建弹窗分类选择器（替代旧 `AgentCategoryCascade.vue`）

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| Agent CRUD | ✅ | `AgentService.CreateAgent/UpdateAgent/...` |
| Agent 分类绑定 | ✅ | `category_position_id` 字段 |
| Provider/Model 选择 | ✅ | `provider` + `model` 字段 |
| 自我进化开关 | ✅ | `self_evolve` 字段 |
| Agent RuntimeSettings | ✅ | `AgentRuntimeSettings` 独立表 + CRUD |
| LRU 缓存 | ✅ | `BuildTRPCLLMAgentCached` |
| 创建者写入 | ✅ | `Create` 空 `created_by` 时 `AgentCreatedByFromContext`；Proto `CreateAgentRequest` 无该字段 |
| 结构化创建错误 | ✅ | `AGENT_KEY_CONFLICT` / `AGENT_KEY_INVALID` / `AGENT`（Kratos `reason` + `message`） |
| Agent 模板 API | ✅ | `GET /v1/agent-templates`（`display_name` / `provider` / `model` / `description`） |
| `agent_kind` / A2A Proxy | ✅ | `agent_kind` 字段 + `A2AProxyConfig` + `HydrateAgentKind` |
| `DuplicateAgent` | ✅ | `AgentUsecase.Duplicate` + `agent_duplicate.go` |
| `BatchUpdateAgents` | ✅ | `AgentUsecase.BatchUpdateAgents`（biz only，无 proto RPC） |
| Planner config | ✅ | `AgentRuntimeSettings` 字段：`planner_kind`、`planner_config_json` |
| Ralph Loop | ✅ | `AgentRuntimeSettings` 字段：`ralph_loop_max_iterations` / `completion_promise` / `verify_command` / `verify_timeout_seconds` / `promise_tag_open` / `promise_tag_close` / `verify_work_dir` |
| Agent variant/kind/source | ✅ | `agent_variant`、`variant_description`、`source`、`kind` 字段 in proto |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 创建弹窗 QDialog | ✅ | `AgentCreateDialog.vue` |
| 业务分类级联选择 | ✅ | `useAgentsPage` 行业/部门/职位三级 |
| Provider/Model 下拉 | ✅ | Platform store + `validateModel` |
| 模板芯片行 | ✅ | `ListAgentTemplates` API；`applyTemplate` 填充 display_name / provider / model / description；API 失败回退 `agentUi` 本地芯片 |
| 结构化创建错误 UI | ✅ | `AgentCreateDialog` inline + banner；`createAgentService().CreateAgent`；4xx 不重复全局 notify |
| 模型检查按钮 | ✅ | `POST /v1/agents/validate-model` + 弹窗「检查」 |
| agent_key 实时查重 | ✅ | `GET /v1/agent-keys/check` + 500ms 防抖（2026-05-21） |
| 头像选择器 | ✅ | `AgentAvatarPicker` |
| Planner section | ✅ | `AgentPlannerSection.vue` + `plannerConfig.ts` + `useAgentPlannerForm.ts` |
| Ralph Loop section | ✅ | `AgentRalphLoopSection.vue` + `ralphLoopConfig.ts` + `useAgentRalphLoopForm.ts` |
| FieldGuide system | ✅ | `fieldGuides.ts` + `AIRefineButton.vue`（5 scopes: `category.industry/department/position` + `agent.description` + `agent.file`×6） |
| Taxonomy picker | ✅ | `TaxonomyPicker.vue`（替代旧 `AgentCategoryCascade.vue`） |
| Provider/Model picker | ✅ | `useAgentProviderModelPicker.ts` + `useAgentModelValidation.ts` |

---

## 3. 差距与优化

1. ~~**P2**：Provider/Model 可用性校验~~ → ✅ `validate-model` + 创建弹窗检查按钮。
2. ~~**P2**：后端 Agent 模板种子 / 列表 API~~ → ✅ `biz.ListAgentTemplates` + `GET /v1/agent-templates`（迭代 10）。
3. ~~**P3**：agent_key 实时查重~~ → ✅ `CheckAgentKey` + 防抖（2026-05-21）。
4. ~~**P2**：创建失败结构化错误~~ → ✅ `reason`（`AGENT_KEY_*` / `AGENT`）→ `kratosError.ts` + 创建弹窗 inline / banner（2026-05-21）。
5. **P3**：创建成功后的跳转行为未明确（需求 §1 写"关闭弹窗并刷新列表或跳转详情"，两种行为需产品决策）。
6. ~~**P1（A2A）**：创建对话框 Agent 类型~~ → ✅ 已实现（LLM / A2A Proxy），见 [changelog/2026-05-20-A2A-Phase1-2.md](../changelog/2026-05-20-A2A-Phase1-2.md)
7. **P3**：`BatchUpdateAgents` proto RPC — biz 层已实现，缺少 HTTP endpoint / proto RPC 定义
8. **P3**：`ReorderAgents` 实际实现 — 当前 `agent_repo.ReorderAgents` 为空 stub（直接 `return nil`）
9. **P3**：trpc-agent-go §8 对齐项（7 项功能待实现）
10. **P3**：Debug trace 清理 — `agent_usecase.go` / `agent_repo.go` 中残留 `#region debug-point` 计时代码需移除

---

## 4. 开发阶段

- **Phase 1**：Provider/Model 可用性校验接口 + 前端"检查"按钮
- **Phase 2**：Agent 模板系统（预设模板 → 自动填充字段）
- **Phase 2（A2A）**：创建对话框 Agent 类型选择 + A2A Proxy 表单（与 [26-a2a-development.md](./26-a2a-development.md) Phase 2 对齐）
- **Phase 3**：agent_key 前端防抖实时查重

---

## 5. 任务清单

| # | 任务 | 层 | 优先级 | EP | 需求回溯 |
|---|------|-----|--------|-----|----------|
| 1 | `ValidateProviderPair`（`POST /v1/agents/validate-model`） | 后端 | P2 | — | 需求 §3.5 | ✅ |
| 2 | `CheckAgentKey`（`GET /v1/agent-keys/check`） | 后端 | P3 | — | 需求 §3.2 | ✅ |
| 3 | Agent 模板种子数据 + 模板列表 API | 后端 | P2 | — | 需求 §3.6 | ✅ |
| 4 | 前端创建弹窗：模型检查按钮 + 结果展示 | 前端 | P2 | — | 需求 §3.5 | ✅ |
| 5 | 前端创建弹窗：模板芯片行 + 全字段自动填充 | 前端 | P2 | — | 需求 §3.6/3.7 | ✅ |
| 6 | 前端创建弹窗：agent_key 防抖查重 + inline error | 前端 | P3 | — | 需求 §3.2 | ✅ |
| 7 | 前端创建弹窗：结构化错误提示（冲突/不可用/校验失败） | 前端 | P2 | — | 需求 §4 | ✅ |
| 8 | 单测覆盖 AgentUsecase CRUD | 后端 | P1 | EP-TEST-01 | — |
| 9 | Proto `agent_kind` + `A2AProxyConfig` | 后端 | P1 | — | 需求 §7 | ✅ |
| 10 | 创建弹窗：Agent 类型 LLM / A2A Proxy | 前端 | P1 | — | 需求 §7 | ✅ |
| 11 | 创建弹窗：远程 URL + 流式/超时 | 前端 | P1 | — | 需求 §7.3 | ✅ |
| 12 | `DuplicateAgent`（复制 Agent） | 后端 | P2 | — | — | ✅ |
| 13 | `BatchUpdateAgents` biz 层 | 后端 | P2 | — | — | ✅ |
| 14 | Planner config 后端字段 | 后端 | P2 | — | — | ✅ |
| 15 | Ralph Loop 后端字段 | 后端 | P2 | — | — | ✅ |
| 16 | Agent variant/kind/source proto 字段 | 后端 | P2 | — | — | ✅ |
| 17 | Planner section 前端 | 前端 | P2 | — | — | ✅ |
| 18 | Ralph Loop section 前端 | 前端 | P2 | — | — | ✅ |
| 19 | FieldGuide system 前端 | 前端 | P2 | — | — | ✅ |
| 20 | TaxonomyPicker 替换 AgentCategoryCascade | 前端 | P2 | — | — | ✅ |
| 21 | Provider/Model picker composable | 前端 | P2 | — | — | ✅ |
| 22 | `BatchUpdateAgents` proto RPC + HTTP endpoint | 后端 | P3 | — | — | 🔲 |
| 23 | `ReorderAgents` 实际实现（当前 stub） | 后端 | P3 | — | — | 🔲 |
| 24 | trpc-agent-go §8 对齐（7 项功能） | 后端 | P3 | — | — | 🔲 |
| 25 | Debug trace 清理（`agent_usecase.go` / `agent_repo.go`） | 后端 | P3 | — | — | 🔲 |

---

## 6. 验收标准

- [x] 前端创建 Agent 时可实时检查 Provider/Model 可用性
- [x] 模板选择后自动填充名称/描述/Provider/Model（`ListAgentTemplates` + `applyTemplate`）
- [x] agent_key 输入后 500ms 防抖查重，冲突时 inline error
- [x] 创建失败时前端展示结构化错误（inline + banner；4xx 跳过全局 toast）
- [x] 可创建 A2A 远程代理 Agent（`agent_kind=a2a_proxy`）并在列表展示 **A2A ↗** 徽章
- [x] `go test ./internal/biz/...` 通过（含 `agent_context_test`、`agent_duplicate` created_by、`agent_create_errors_test`）
- [x] `web` `kratosError.spec.ts` 断言 `AGENT_KEY_CONFLICT` → `reason` 映射
- [x] Planner section 可配置 `planner_kind`（`""` / `builtin` / `react` / `a2ui`）+ `planner_config_json`
- [x] Ralph Loop section 可配置 `ralph_loop_*` 7 个字段
- [x] FieldGuide 系统覆盖 10 个 scope（3 分类 + 1 描述 + 6 文件），`AIRefineButton` 可用
- [x] `TaxonomyPicker.vue` 替代旧 `AgentCategoryCascade.vue`，创建弹窗分类选择正常
- [x] `Duplicate` 方法可复制 Agent（含 settings + prompt files）
- [ ] `BatchUpdateAgents` proto RPC + HTTP endpoint（biz 已实现，缺 proto 层）
- [ ] `ReorderAgents` 实际排序逻辑（当前 stub `return nil`）
- [ ] Debug trace 代码清理（`agent_usecase.go` / `agent_repo.go` 中 `#region debug-point`）

---

## 7. 依赖与风险

### 7.1 跨模块依赖

| 依赖模块 | 依赖项 | 说明 |
|----------|--------|------|
| 模块4 Agent分类 | `TaxonomyPicker.vue` | 创建弹窗业务分类选择组件（替代旧 `AgentCategoryCascade.vue`） |
| 模块9 Provider | `GET /providers` 接口 | Provider 下拉选项数据源 |
| 模块9 Provider | `llm_provider_models` 表 | Provider/Model 可用性校验数据源 |
| 模块50 Avatar | `AgentAvatarPicker` | 头像选择器组件 |
| 模块3 Agent列表 | 列表刷新 | 创建成功后通知列表页刷新 |

### 7.2 风险

- M2 多租户后 Agent 创建需注入 workspace_id
- 模板系统需与 AgentCategory 联动
- Provider/Model 校验接口需考虑缓存策略，避免每次创建都查库

---

## 8. 错误处理规格

> **实现口径**（2026-05-21）：Kratos `BadRequest` 多为 **HTTP 400**，JSON 体含 `reason` + `message`（非全部场景已实现下表独立 reason）。

| 场景 | HTTP | `reason`（实际） | 前端行为 |
|------|------|------------------|----------|
| agent_key 已存在 | 400 | `AGENT_KEY_CONFLICT` | `agent_key` inline |
| agent_key 格式不合法 | 400 | `AGENT_KEY_INVALID` | 正则 + `agent_key` inline |
| 必填 / A2A URL 等校验 | 400 | `AGENT`（message 含字段名） | `mapAgentCreateFieldErrors` 正则分流 → 对应 inline 或 banner |
| Provider/Model 不可用 | 400 | （`validate-model` 响应体，非 Create） | 模型检查按钮 notify |
| 其它 4xx Create | 400 | 缺省 / 未知 | banner `form`；无字段级映射时不重复全局 toast |

**相关**：列表复制见 [3-agent-list-development.md](./3-agent-list-development.md) — `Duplicate` 清空 `created_by` 后按当前用户重建。
