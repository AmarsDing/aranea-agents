# 2026-05-23 Team Graph M53 Phase 7 — Native 路径退役 + Task/Subgraph

## 摘要

Team Run 执行单链收敛：GraphAgent 为默认路径；Native `BuildTRPCTeam` 仅保留 `ARANEA_TEAM_NATIVE=1` 应急 fallback。Embedded graph 支持 `task`/`review`/`subgraph` 节点编译；Team Graph 运行时经 Task bridge 创建人工任务。

## 变更

### 运行时（TG-RT-RETIRE）

- `TeamGraphRuntimeEnabled`：不再要求 `ARANEA_TEAM_GRAPH_RUNTIME=1`（默认开启；`ARANEA_TEAM_GRAPH_RUNTIME=0` 可全平台关闭 Graph）
- `runner_team_trpc.go`：Graph 编译/构建失败时，**不再 silent fallback**；仅当 `ARANEA_TEAM_NATIVE=1` 时调用 `BuildTRPCTeam`
- `BuildTRPCTeam` 标记 **Deprecated**（godoc）

### Task / Subgraph（TG-RT-TASK / TG-RT-SUBGRAPH）

- `embedded_graph.go`：编译 `task`/`review`（`InterruptAfter`）与 `subgraph`（`GraphBuildConfigLoader` + 循环检测）
- `team_graph_task_bridge.go`：`graph_node_start` → `TaskUsecase.CreateTask`（**仅 task/review 节点**，P0 修复）
- `TeamGraphRunCoordinator`（P1）：register / HITL defer / task resume / `team_run_finished` 收尾
- `ChatService` + `WireGraphTaskRuntime` 共享单例 coordinator

### 指标

- `aranea_team_graph_runtime_total{path="graph",result=...}` — 主路径
- `path="native",result="native_emergency|native_fallback"` — 应急 Native

### 前端

- Team 编辑器 hint 与 `definitionToJSON` 默认 `runtime_engine=graph`

### 文档

- `docs/需求/0 系统框图.md` §5.2、`11 multi-agent.design.md` §6 更新为 Graph 主路径

## 运维

| 变量 | 默认 | 说明 |
|------|------|------|
| `ARANEA_TEAM_GRAPH_RUNTIME` | （空=开） | 设为 `0` 关闭 Graph 平台 gate |
| `ARANEA_TEAM_NATIVE` | off | 设为 `1` 启用 Native 应急（含 Graph 失败 fallback） |

## 已知缺口

- ~~Resume 后 `persistStep` / `team_summary`~~ → ✅ Resume finisher + obs store（见 Phase7-Optimization changelog）
- ~~FP-04 Admin UI~~ → ✅ List/Resolve + Chat 死信 Tab
- TG-RT-PARITY：**build 级** runtime 单测 ✅；run 级 token/steps/WS 对比仍待 P2
- Graph 非 HITL 完成仍 bulk persist 全 member（BL-03，P2）→ ✅ 事件 watch + anchor fallback（见 Phase7-P2 changelog）

## 验证

```bash
go test ./internal/team/ -run 'GraphRuntime|embedded|Subgraph|TeamGraph|Parity|Finisher' -count=1
make wire && go build ./...
```
