# Agent 创建 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
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

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 创建弹窗 QDialog | 🟡 待验证 | 需确认前端是否已实现 `CreateAgentDialog.vue` |
| 业务分类级联选择 | 🟡 待验证 | 依赖 `AgentCategoryCascade.vue`（模块4） |
| Provider/Model 下拉 | 🟡 待验证 | 依赖 `/providers` 接口（模块9） |
| 模板芯片行 | ❌ 未实现 | 需求 3.6/3.7 要求模板选择，前端无对应组件 |
| 模型检查按钮 | ❌ 未实现 | 需求 3.5 要求"检查"按钮，后端无校验接口 |
| agent_key 实时查重 | ❌ 未实现 | 仅提交时后端校验唯一性 |
| 头像选择器 | 🟡 待验证 | 依赖 `AgentAvatarPicker`（模块50） |

---

## 3. 差距与优化

1. **P2**：Agent 创建时无 Provider/Model 可用性实时校验（需求 3.4/3.5 要求"检查"按钮），前端仅静态列表。
2. **P2**：模板芯片行（需求 3.7）未实现，用户无法从预设模板快速创建。
3. **P3**：Agent 标识（agent_key）唯一性校验仅在提交时做，无前端防抖实时检查。
4. **P2**：创建失败时缺少结构化错误提示策略（如 agent_key 冲突、Provider 不可用等），前端仅展示通用错误。
5. **P3**：创建成功后的跳转行为未明确（需求 §1 写"关闭弹窗并刷新列表或跳转详情"，两种行为需产品决策）。

---

## 4. 开发阶段

- **Phase 1**：Provider/Model 可用性校验接口 + 前端"检查"按钮
- **Phase 2**：Agent 模板系统（预设模板 → 自动填充字段）
- **Phase 3**：agent_key 前端防抖实时查重

---

## 5. 任务清单

| # | 任务 | 层 | 优先级 | EP | 需求回溯 |
|---|------|-----|--------|-----|----------|
| 1 | 新增 `CheckProviderModel` RPC（`POST /v1/agents/validate-model`） | 后端 | P2 | — | 需求 §3.5 |
| 2 | 新增 `CheckAgentKeyAvailability` RPC（`GET /v1/agents/check-key?key=`） | 后端 | P3 | — | 需求 §3.2 |
| 3 | Agent 模板种子数据 + 模板列表 API | 后端 | P2 | — | 需求 §3.6 |
| 4 | 前端创建弹窗：模型检查按钮 + 结果展示 | 前端 | P2 | — | 需求 §3.5 |
| 5 | 前端创建弹窗：模板芯片行 + 自动填充 | 前端 | P2 | — | 需求 §3.6/3.7 |
| 6 | 前端创建弹窗：agent_key 防抖查重 + inline error | 前端 | P3 | — | 需求 §3.2 |
| 7 | 前端创建弹窗：结构化错误提示（冲突/不可用/校验失败） | 前端 | P2 | — | 需求 §4 |
| 8 | 单测覆盖 AgentUsecase CRUD | 后端 | P1 | EP-TEST-01 | — |

---

## 6. 验收标准

- [ ] 前端创建 Agent 时可实时检查 Provider/Model 可用性
- [ ] 模板选择后自动填充名称/描述/Provider/Model
- [ ] agent_key 输入后 500ms 防抖查重，冲突时 inline error
- [ ] 创建失败时前端展示结构化错误（非通用 toast）
- [ ] `go test ./internal/biz/... -run TestAgent` 通过

---

## 7. 依赖与风险

### 7.1 跨模块依赖

| 依赖模块 | 依赖项 | 说明 |
|----------|--------|------|
| 模块4 Agent分类 | `AgentCategoryCascade.vue` | 创建弹窗业务分类级联选择组件 |
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

| 场景 | HTTP 状态码 | 错误码 | 前端行为 |
|------|------------|--------|----------|
| agent_key 已存在（未软删） | 409 Conflict | `AGENT_KEY_CONFLICT` | inline error：标识已被使用 |
| Provider 不可用 | 400 Bad Request | `PROVIDER_UNAVAILABLE` | 模型检查按钮结果：Provider 不可用 |
| Model 不属于该 Provider | 400 Bad Request | `MODEL_NOT_FOUND` | 模型检查按钮结果：模型不存在 |
| 必填字段为空 | 400 Bad Request | `FIELD_REQUIRED` | 对应字段 inline error |
| agent_key 格式不合法 | 400 Bad Request | `AGENT_KEY_INVALID` | 输入时正则校验 + inline error |
