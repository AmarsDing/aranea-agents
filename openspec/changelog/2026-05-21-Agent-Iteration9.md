# Agent 迭代 9 — 列表运行态 · Token 估算 · 进化标签

> **日期**：2026-05-21  
> **计划**：[2-8-agent-modules-development.md](../需求/2-8-agent-modules-development.md) · **任务板**：[execution-plan.md](../guides/execution-plan.md) 迭代 9

## 摘要

补齐 Agent 模块 P2 体验三项：列表卡片展示最近运行态、提示文件侧栏对接服务端 Token 估算、「进化中」标签与待处理进化建议计数对齐。

## 变更

### 后端

- `api/kratos/agent/v1/agent.proto`：`Agent` 增加 `last_run_status`、`last_run_at`、`pending_evolution_count`
- `internal/data/agent_list_extras.go`：`ListExtrasForAgents`（最新 session + pending evolution 计数）
- `internal/biz/agent_usecase.go`：`List` / `hydrate(Get)` 合并 extras
- `internal/service/agent.go`：`toProtoAgent` 映射新字段

### 前端

- `agentUi.ts`：`formatLastRunContext`、`isAgentEvolving`
- `AgentsListSection` / `AgentSettingsHeader`：运行态底栏与进化 chip
- `estimateAgentTokens` + `AgentFilesPanel` / `useAgentSettingsPage`：文件 Tab 服务端 Token

## 验证

```bash
make api
go build ./internal/biz/ ./internal/data/   # 注：全仓 make build 仍受 conf/runtime 既有错误影响
cd web && pnpm lint && pnpm build
```

## 后续（迭代 10）

已完成，见 [2026-05-21-Agent-Iteration10.md](./2026-05-21-Agent-Iteration10.md)。
