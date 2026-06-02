# 2026-05-23 Team×Graph M53: Phase 0.5 状态投影

**影响**：🟡 中 | **模块**：Team / Biz / Event / Frontend types

## 变更摘要

M53 Team×Graph 编排融合文档落盘（需求/设计/开发计划），并实现 Phase 0.5：统一 Agent 节点状态模型与 WS 投影，为 Kanban/Graph 观测提供真相源。

## 关键变更

### 文档

- 新增 [53 team-graph-orchestration.md](../需求/53%20team-graph-orchestration.md)（需求）
- 新增 [53 team-graph-orchestration.design.md](../需求/53%20team-graph-orchestration.design.md)（设计）
- 新增 [53-team-graph-orchestration-development.md](../需求/53-team-graph-orchestration-development.md)（开发计划）
- 更新 `docs/README.md`、`README-development.md`、`execution-plan.md` 迭代 TG 任务板
- `11 multi-agent.md` / `36 graph-workflow.md` 增加 M53 交叉引用

### 后端

- `internal/biz/orchestration_status.go` — 16 细态 + 优先级归约 + Kanban WorkPhase
- `internal/team/status_projector.go` — 订阅 Session Bus → `orchestration_agent_status`
- `internal/team/runner_team_trpc.go` — Team Run 生命周期内启动/停止投影器
- `internal/event/envelope.go` — 新 EnvelopeType + team 通道路由

### 前端

- `web/src/features/orchestration/types.ts`
- `web/src/features/orchestration/agentNodeStatusStyles.ts`

## 下一步（Phase 1）

Kanban UI、`GetTeamRunObservatory` RPC、Graph 节点细态扩展。
