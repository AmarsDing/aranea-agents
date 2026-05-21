# Agent 模块优化（2026-05-21）

## 摘要

对照 `docs/README.md` 与 Agent 模块 `*-development.md`，收敛 **Builder 依赖分组**、**config_json 深度合并**、**agent_key 实时查重**、**构建链路 FlowLog**，并同步开发计划文档。

## 后端

| 项 | 变更 |
|----|------|
| `internal/agent/builder_deps.go` | `TRPCBuilderDeps` 扁平 DTO + `TRPC*Deps` 分组类型与 `CatalogGroup()` |
| `internal/agent/trpc_build_router.go` | `system.agent.build` FlowLog（开始/失败/完成） |
| `internal/biz/agent_config_merge.go` | `MergeAgentConfigJSON`；`Update` PATCH 合并 `config_json` |
| `internal/biz/agent_usecase.go` | `CheckAgentKeyAvailability` |
| `api/kratos/agent/v1/agent.proto` | `CheckAgentKey` → `GET /v1/agent-keys/check`（避免与 `/v1/agents/{id}` 冲突） |

## 前端

| 项 | 变更 |
|----|------|
| `web/src/features/agents/api.ts` | `checkAgentKey()` |
| `web/src/features/agents/useAgentsPage.ts` | agent_key 500ms 防抖服务端查重 |

## 文档

- `docs/需求/0-system-development.md` §8.11 Agent 待优化表
- `docs/需求/2-agents-create-development.md`、`5-agent-setting-development.md` 现状对齐
- `docs/guides/execution-plan.md` 迭代 8 Agent 任务板

## 仍待后续

- `AgentSettingsPage.vue`（~1000 行）`page-to-components` 拆分（SYS-06）
- Agent 列表 `last_run_status` 聚合（模块 3）
- LIST-02：`created_by`、创建模板全字段、结构化创建错误及审查修正 — [2026-05-21-Agent-CreatedBy-Templates-Errors.md](./2026-05-21-Agent-CreatedBy-Templates-Errors.md)
- Evolution 运行时（模块 7 占位）
