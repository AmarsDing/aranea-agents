# 2026-05-23 Team×Graph M53: Phase 4 优化 — 快照 / 编译真相源 / 运行锁

**影响**：🟡 中 | **模块**：Data / Biz / Team / Service / Web

## 变更摘要

闭合 M53 审查 P1/P2 项：Run 启动冻结 `definition_snapshot_json`、Observatory 与 Compile 统一走后端编译器、embedded `definition.graph` 参与编译、linked_graph Team Run 加载路径、前端运行中编排只读锁。

## 关键变更

### TG-API-03 — Run 定义快照

- `internal/data/ent/schema/team_run.go` — `definition_snapshot_json` 字段
- `docs/sql/03_session.sql` — DDL 对齐
- `internal/biz/team_types.go` — `TeamRun.DefinitionSnapshotJSON`
- `api/kratos/team/v1/team.proto` — field 20；Observatory 响应同字段
- `internal/team/runner_team_trpc.go` — CreateTeamRun 写入 `BuildCompileSnapshot`
- `internal/biz/team_usecase.go` — `GetRunObservatory` 优先读快照，fallback 当前 Team 定义

### Observatory / Compile 真相源统一

- `internal/team/compile_snapshot.go` — `BuildCompileSnapshot`（Observatory + API 共用）
- `internal/service/team_observatory.go` — 返回 `compiled_topology`（不再依赖前端 `teamUtils.buildGraphFromDefinition`）
- `web/src/features/teams/useTeamRunObservatoryPage.ts` — 消费后端 `compiled_topology`

### TG-CMP-embedded — definition.graph 编译

- `internal/team/embedded_graph.go` — 解析 `definition.graph` agent 节点/边，跳过 start/end/join 装饰
- `internal/team/graph_compile.go` — `CompileToGraphBuildConfigFromJSON` 优先 embedded graph

### linked_graph Team Run 路径

- `internal/team/graph_loader.go` — `GraphBuildConfigLoader` 接口
- `internal/graph/adapter/linked_graph_loader.go` — 经 `GraphUsecase` 加载 persisted graph
- `internal/team/graph_runtime_config.go` — `CompileToGraphRuntimeConfigFromJSON` linked → embedded → mode 编译链
- `internal/service/chat.go` — Wire `GraphFactory` + `Graphs` 至 `TeamsNative.SetGraphBuildConfigLoader`

### P2 — 运行中编辑锁

- `api/kratos/team/v1/team.proto` — `Team.has_active_run`
- `internal/service/team.go` — GetTeam 投影 `has_active_run`
- `web/src/features/teams/useTeamOrchestratePage.ts` — `readOnly` + 保存禁用
- `web/src/pages/TeamOrchestratePage.vue` — 运行中 banner

### adaptive 运行时 Destinations

- `internal/team/graph_runtime_config.go` — `applyAdaptiveAgentDestinations`：flow + transfer 边 → `NodeDef.Destinations`；运行时经 `FilterVisualizationEdges` 剥离 transfer 边

## 架构边界

| 层 | 职责 |
|----|------|
| `team` | 唯一编译入口：`CompileToGraphBuildConfigFromJSON` / `CompileToGraphRuntimeConfigFromJSON` / `BuildCompileSnapshot` |
| `biz` | Run 快照读取、Observatory 聚合；不 import trpc-agent-go |
| `service` | Proto 映射、`compiled_topology` 组装 |
| `web` | 只读消费后端拓扑；运行中禁止 PATCH（配合 `HasActiveRun`） |

## Feature flag（未变）

Team Graph 运行时仍须 **`ARANEA_TEAM_GRAPH_RUNTIME=1`** 且 Team `runtime_engine=graph`。生产 rollout 见 [53-team-graph-orchestration-development.md §Phase 5](../需求/53-team-graph-orchestration-development.md#phase-5--graph-框架能力补全与-rolloutp2)。

## 验收

```bash
make api && make wire
go test ./internal/biz/... ./internal/team/... ./internal/graph/trpc/... ./internal/service/... -run "Team|Observatory|Compile|Orchestration|GraphRuntime" -count=1
go build ./cmd/admin/...
```

## 待办（Phase 5 backlog）

- G-RETRY / G-GOTO 框架接线（见 [36-graph-development.md](../需求/36-graph-development.md)）
- `ARANEA_TEAM_GRAPH_RUNTIME` 生产默认与 Canary 策略
- GraphEditorCanvas 原生 `readonly` prop（当前仅禁用保存/API）
