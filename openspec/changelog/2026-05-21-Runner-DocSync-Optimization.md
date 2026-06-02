# Runner 文档对齐与架构优化（2026-05-21）

## 摘要

按 `docs/README.md` 工作流，将 Runner 需求/设计/开发计划与实现对齐；收敛 RunnerManager 装配入口、框架 RunStatus 合并职责，并更新过时差距表。

## 文档

| 文件 | 变更 |
|------|------|
| `docs/需求/40 runner.md` | 已有/缺失能力表与代码一致 |
| `docs/需求/40 runner.design.md` | §一 现状与差距、§六 Service 描述 |
| `docs/需求/40-runner-development.md` | §2 评估、§3 优先级、§5 任务 11 状态 |

## 代码

| 项 | 文件 |
|----|------|
| `TurnDeps.CoalesceRunnerManager()` 统一懒装配 | `internal/runtime/deps.go` |
| Chat/Team/A2A 去掉重复 nil 检查 | `trpc_turn.go`, `runner_team_trpc.go`, `a2a_endpoint.go` |
| `FrameworkRunStatusFromRunner` 合并框架状态 | `internal/runtime/run_status.go` |
| `EnqueueTRPCUserMessage` 委托 `trpcrunner.EnqueueUserMessage` | `internal/agent/trpc_runtime.go` |
| RunRegistry 注释 request_id = session_id | `internal/runtime/run_registry.go` |

## P2–P3 续（同日前续）

| 项 | 实现 |
|----|------|
| AgentLookup | `BizAgentRegistryOptions` + `TurnRunnerSpec.LookupAgents`；Team `BuildTeamMemberAgents` 导出 lookup map |
| SessionIngestor | `BizSessionIngestor` 解析 `IngestOption` 并 FlowLog（不重复 `EnqueueAutoMemoryJob`） |
| RalphLoop | `docs/sql/02_agent_ralph_loop.sql` + Ent 字段 + `RalphLoopConfigFromSettings` |
| Web | `ChatRunnerStatus.vue`、`ChatEnqueueMessage.vue` 接入 `ChatMessagePanel` |

## Agent 设置 + 迁移（续）

| 项 | 实现 |
|----|------|
| Proto `AgentRuntimeSettings` Ralph 字段 | `api/kratos/agent/v1/agent.proto` |
| 启动 patch | `internal/data/agent_runtime_patch.go`（与 `docs/sql/02_agent_ralph_loop.sql` 一致） |
| 新库基线 | `docs/sql/02_agent.sql` |
| 前端 | `AgentRalphLoopSection.vue`、`ralphLoopConfig.ts`；修复 `plannerForm` `defineModel` |

已有库可选手动：`go run ./cmd/sqlmigrate docs/sql/02_agent_ralph_loop.sql`（admin 启动时 patch 会自动补列）。

## Review 跟进（P1–P3）

| 项 | 实现 |
|----|------|
| P1 保存校验 | `biz.ValidateRalphLoopSettings` + `agent_usecase` Create/Update |
| P1 A2A | `a2a_endpoint.go` 注入 `RalphLoop` + `LookupAgents` |
| P2 DRY | `agent.ResolveRalphLoopTurn`（Chat/Team/A2A） |
| P2 文档 | `40 runner.design.md` Team Ralph 策略 |
| P3 前端 | `useAgentRalphLoopForm` / `useAgentPlannerForm`；高级设置横幅 |
| 设置页瘦身 | `useAgentRuntimeConfig`、`useAgentPromptFiles`、`useAgentToolsCatalog` 等；`useAgentSettingsPage.ts` ~360 行 |
| 设置页续拆 | `useAgentPromptPreview`、`useAgentEvolutionSettings`、`useAgentAvatarIcon` |

## 仍待办

- 外部 SessionIngestor 后端（Mem0 等）替换/扩展 `BizSessionIngestor`
- 独立 `CancelRun` RPC（沿用 StopGeneration + WS cancel）

## 验证

```bash
go test ./internal/runtime/... ./internal/agent/... -count=1
go test aranea-agents/internal/service -run "CancelRun|StopGeneration|Enqueue" -count=1
```
