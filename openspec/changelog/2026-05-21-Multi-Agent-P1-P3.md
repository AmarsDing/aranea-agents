# Multi-Agent P1–P3 闭环 — 2026-05-21

## 后端

- `team_step_started` Envelope：`persistStep` 持久化前发射
- `GetTeamRunSummary` RPC：`GET /v1/team-runs/{id}/summary`，复用 `BuildTeamRunSummary`
- `MemberToolCalls`：`EventStreamResult` 统计子 Agent 工具调用；`team_run_steps.tool_call_count` 落库
- 单测：`runner_helpers_test`、`team_run_test`（Summary + RunTeamTest 运行时校验）

## 前端

- `TeamTestDialog` + TeamCard「运行测试」→ `RunTeamTest`
- `TeamRunsDialog`：WS 回放 banner、`team_step_started` 步骤态、汇总 RPC、工具调用数
- Chat Team：`ChatTeamMemberStrip` 子 Agent 流式 chip 条
- `adaptive` 模式文案对齐 Swarm 运行时

## 文档

- `11-multi-agent-development.md` 任务 TEAM-01~09 状态更新
