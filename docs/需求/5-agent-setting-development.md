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

| 项 | 状态 | 证据 |
|----|------|------|
| Agent 基础设置 CRUD | ✅ | UpdateAgent RPC |
| RuntimeSettings CRUD | ✅ | UpdateAgentRuntimeSettings RPC |
| Effective Tools 计算 | ✅ | `agent_effective_tools.go` |
| Effective MCP 计算 | ✅ | `agent_mcp_effective.go` |
| PromptFile 管理 | ✅ | 独立 RPC + 表 |
| ToolOverride | 🟡 | proto 有 `agent_override_count` 字段，但无 biz Usecase / Repo / Service |

---

## 3. 差距与优化

1. **P2（EP-BIZ-06）**：`tool_agent_override` 表已存在（Ent schema），但无 `biz/tool_override.go` Usecase、无 Repo 实现、无 Service CRUD。Agent 设置页无法管理工具级别的参数覆盖。
2. **P3**：Agent 设置页"记忆配置"区域（L0-L4 各层参数）字段繁多，前端无分组折叠，用户体验差。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-06）**：补 `biz/tool_override.go` + Repo + Service CRUD + 前端页面
- **Phase 2**：记忆配置区域分组折叠 UI 优化

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `biz/tool_override.go`：模型 + Repo 接口 + Usecase | P2 | EP-BIZ-06 |
| 2 | `data/tool_agent_override.go`：Repo 实现 | P2 | EP-BIZ-06 |
| 3 | `service/tool.go`：增加 ToolOverride CRUD RPC | P2 | EP-BIZ-06 |
| 4 | proto 增加 ToolOverride 相关 RPC | P2 | EP-BIZ-06 |
| 5 | 前端 Agent 设置页工具覆盖管理 | P2 | EP-BIZ-06 |
| 6 | 记忆配置分组折叠 | P3 | — |

---

## 6. 验收标准

- [ ] Agent 设置页可管理每个工具的参数覆盖
- [ ] 覆盖参数在 `BuildTRPCLLMAgent` 装配链中生效
- [ ] `go test ./internal/biz/... -run TestToolOverride` 通过

---

## 7. 依赖与风险

- ToolOverride 与 Tool 系统紧耦合，需确保覆盖优先级正确
- M2 多租户后需 workspace_id 隔离
