# Multi-Agent Review P2 — 2026-05-21

## 读路径收拢

- `TeamUsecase.GetRunSummary`：`GetRun` + `ListRunSteps` + `BuildTeamRunSummaryData`
- `GetTeamRunSummary` Service 统一经 `mapTeamErr` 返回错误

## 聚合与映射分层

- `internal/biz/team_summary.go`：`BuildTeamRunSummaryData`（单一聚合源）
- `internal/team/summary.go`：`SummaryMapFromData`（WS）
- `internal/service/team.go`：`toProtoTeamRunSummary`（RPC，符合 `toProtoXxx` 命名）
- 删除 `internal/team/summary_proto.go`

## 测试

- `internal/biz/team_summary_test.go`
- `internal/service/team_summary_parity_test.go`：WS map ↔ RPC proto 字段对齐

## 文档

- `11 multi-agent.design.md` §6.3 同步
