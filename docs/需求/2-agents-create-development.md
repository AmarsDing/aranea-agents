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

| 项 | 状态 | 证据 |
|----|------|------|
| Agent CRUD | ✅ | `AgentService.CreateAgent/UpdateAgent/...` |
| Agent 分类绑定 | ✅ | `category_position_id` 字段 |
| Provider/Model 选择 | ✅ | `provider` + `model` 字段 |
| 自我进化开关 | ✅ | `self_evolve` 字段 |
| Agent RuntimeSettings | ✅ | `AgentRuntimeSettings` 独立表 + CRUD |
| LRU 缓存 | ✅ | `BuildTRPCLLMAgentCached` |

---

## 3. 差距与优化

1. **P2**：Agent 创建时无 Provider/Model 可用性实时校验（需求 3.4/3.5 要求"检查"按钮），前端仅静态列表。
2. **P2**：模板芯片行（需求 3.7）未实现，用户无法从预设模板快速创建。
3. **P3**：Agent 标识（agent_key）唯一性校验仅在提交时做，无前端防抖实时检查。

---

## 4. 开发阶段

- **Phase 1**：Provider/Model 可用性校验接口 + 前端"检查"按钮
- **Phase 2**：Agent 模板系统（预设模板 → 自动填充字段）
- **Phase 3**：agent_key 前端防抖实时查重

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | 新增 `CheckProviderModel` RPC | P2 | — |
| 2 | Agent 模板种子数据 + 前端模板选择 | P2 | — |
| 3 | agent_key 防抖查重 API | P3 | — |
| 4 | 单测覆盖 AgentUsecase CRUD | P1 | EP-TEST-01 |

---

## 6. 验收标准

- [ ] 前端创建 Agent 时可实时检查 Provider/Model 可用性
- [ ] 模板选择后自动填充名称/描述/Provider/Model
- [ ] `go test ./internal/biz/... -run TestAgent` 通过

---

## 7. 依赖与风险

- M2 多租户后 Agent 创建需注入 workspace_id
- 模板系统需与 AgentCategory 联动
