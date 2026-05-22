# M53: Team × Graph 编排融合 — 实现设计

> 对应需求：[53 team-graph-orchestration.md](./53%20team-graph-orchestration.md)  
> 遵循：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)  
> **实现差距与迭代计划**以 [53-team-graph-orchestration-development.md](./53-team-graph-orchestration-development.md) 为准

---

## 一、模块概述

### 1.1 设计定位

**OrchestrationSpec** 为 Team 与 Graph 的统一编排真相源：

- **Team 简视图**：mode + members + 参数 → 编译器生成 graph 拓扑
- **Graph 高级视图**：Vue Flow 自由编辑，可 `linked_graph_id` 引用
- **运行态**：StatusProjector 将异构 Envelope 投影为 `AgentNodeState`；Kanban 与 Graph 共用

**终态（执行单链）**：所有 Team Run 经 `CompileToGraphRuntimeConfig` → `GraphAgent` 执行；`BuildTRPCTeam`（Native）退役。当前 Phase 0.5–4 已完成编译/观测收敛，执行仍双轨 — **差距与 Phase 5–7 任务**见 [53-team-graph-orchestration-development.md §8](./53-team-graph-orchestration-development.md#8-终态路线图team-规格--graph-执行单链)。

### 1.2 分层与依赖

```
api/kratos/team/v1/team.proto          ← Team 扩展字段（linked_graph_id 等，Phase 2+）
api/kratos/graph/v1/graph.proto        ← Graph 既有 RPC
        ↓
internal/service/
  team.go · graph.go
  orchestration_observatory.go         ← GetTeamRunObservatory（Phase 1）
        ↓
internal/biz/
  orchestration_status.go              ← AgentNodeStatus + ApplyEnvelope（纯领域，无 trpc）
  team_usecase.go                      ← HasActiveRun 锁定校验（Phase 1）
        ↓
internal/team/
  graph_compile.go                     ← CompileToGraphBuildConfig（Phase 2）
  status_projector.go                  ← 订阅 Bus → 投影 WS（Phase 0.5）
  runner_team_trpc.go                  ← 启动/停止投影器
        ↓
internal/graph/trpc/                   ← Graph 构建与 EventBridge（既有）
        ↓
pkg/trpc-agent-go/graph · team         ← 框架真相源
```

**红线**：`internal/biz` 不 import `pkg/trpc-agent-go`；Graph 构建仅在 `internal/graph/trpc` + `internal/team`。

### 1.3 影响域

| 包 | 变更类型 | 说明 |
|----|----------|------|
| `internal/biz` | 新增 | 状态枚举、归约器、Observatory 读模型 |
| `internal/team` | 新增/修改 | 编译器、StatusProjector、Runner 挂钩 |
| `internal/service` | 新增 | Observatory RPC、Run 锁定 |
| `internal/event` | 扩展 | `orchestration_agent_status` EnvelopeType |
| `internal/graph/trpc` | 修改 | Graph Run 启动投影器（Phase 1） |
| `web/src/features/orchestration` | 新增 | 类型、Kanban、store |
| `web/src/components/graph` | 扩展 | 节点细态、边状态 |
| `api/kratos/team/v1` | 扩展 | Phase 2+ Proto 字段 |

**不改动**：`internal/server` 直连 runtime；`internal/data` 除 Phase 2 快照字段外 Phase 0.5 无 schema 变更。

---

## 二、OrchestrationSpec 数据模型

### 2.1 Team Definition 扩展（JSON）

```json
{
  "version": 2,
  "source": "preset",
  "mode": "sequential",
  "members": [],
  "graph": {
    "version": 1,
    "layout": "linear",
    "entry_point": "start",
    "finish_point": "end",
    "nodes": [],
    "edges": []
  },
  "linked_graph_id": "",
  "failure_policy": {
    "default": "retry_then_block",
    "retry": { "max_attempts": 3 }
  }
}
```

与现有 `definition_json` 向后兼容：`version` 缺省为 1，无 `graph` 时由编译器生成。

### 2.2 节点注册表（运行期）

Run 开始时由 members + graph.nodes 构建 `OrchestrationRegistry`：

| 键 | 值 |
|----|-----|
| `agent_key` / `agent_id` | `node_id`, `role`, `display_name` |

用于将 `member_*` Envelope 的 author/agent_key 映射到 graph 节点。

### 2.3 TeamRun 扩展（Phase 1 持久化）

| 字段 | 类型 | 说明 |
|------|------|------|
| `definition_snapshot_json` | TEXT | Run 开始时 OrchestrationSpec 冻结 |
| `graph_execution_id` | UUID | 关联 graph_executions（Phase 3） |

---

## 三、Agent 状态模型

### 3.1 细态枚举 `AgentNodeStatus`

```go
// internal/biz/orchestration_status.go
type AgentNodeStatus string

const (
    AgentNodeStatusIdle           AgentNodeStatus = "idle"
    AgentNodeStatusQueued         AgentNodeStatus = "queued"
    AgentNodeStatusScheduled      AgentNodeStatus = "scheduled"
    AgentNodeStatusRunning        AgentNodeStatus = "running"
    AgentNodeStatusThinking       AgentNodeStatus = "thinking"
    AgentNodeStatusToolRunning    AgentNodeStatus = "tool_running"
    AgentNodeStatusTransferring   AgentNodeStatus = "transferring"
    AgentNodeStatusRetrying       AgentNodeStatus = "retrying"
    AgentNodeStatusWaitingInput   AgentNodeStatus = "waiting_input"
    AgentNodeStatusWaitingReview  AgentNodeStatus = "waiting_review"
    AgentNodeStatusWaitingAssign  AgentNodeStatus = "waiting_assign"
    AgentNodeStatusBlocked        AgentNodeStatus = "blocked"
    AgentNodeStatusSuccess        AgentNodeStatus = "success"
    AgentNodeStatusFailed         AgentNodeStatus = "failed"
    AgentNodeStatusSkipped        AgentNodeStatus = "skipped"
    AgentNodeStatusCancelled      AgentNodeStatus = "cancelled"
    AgentNodeStatusTimedOut       AgentNodeStatus = "timed_out"
)
```

### 3.2 UI 聚合态 `DisplayStatus`

| DisplayStatus | 包含细态 |
|---------------|----------|
| `waiting` | idle, queued, scheduled |
| `active` | running, thinking, tool_running, transferring, retrying |
| `suspended` | waiting_input, waiting_review, waiting_assign, blocked |
| `success` | success |
| `failed` | failed, timed_out |
| `skipped` | skipped |
| `cancelled` | cancelled |

### 3.3 优先级覆盖（同节点多信号）

```
blocked / waiting_* > retrying > tool_running > thinking > running > queued > idle
终态不可被非 retry/resume 信号覆盖
```

实现：`biz.ApplyOrchestrationEnvelope` 纯函数，便于单测。

### 3.4 工作阶段 `WorkPhase`

```go
type WorkPhase string

const (
    WorkPhaseReceived  WorkPhase = "received"
    WorkPhaseDoing     WorkPhase = "doing"
    WorkPhaseDelivered WorkPhase = "delivered"
)
```

---

## 四、StatusProjector

### 4.1 职责（单一）

订阅 Session 级 EventBus，将异构 Envelope **归约**为 `AgentNodeState`，发布 `orchestration_agent_status`。

**不负责**：持久化、Graph 构建、Team Run 生命周期。

### 4.2 事件映射

| EnvelopeType | 状态更新 |
|--------------|----------|
| `team_step_started` | → `running`, phase=`doing` |
| `member_message_start` | → `thinking` |
| `tool_call` | → `tool_running`, 记录 CurrentActivity |
| `tool_result` | 完成 Activity；若仍 streaming → `thinking` |
| `member_message_done` | → `success`, phase=`delivered` |
| `team_step_finished` | step.status → success/failed |
| `graph_node_start` | node_id → `running` |
| `graph_node_end` | → `success` |
| `graph_node_error` | → `failed` |
| `transfer` | 源 idle, 目标 `transferring`→`running` |
| `checkpoint` | → `waiting_input` |
| `run_status` (cancelled) | 全部活跃 → `cancelled` |

### 4.3 输出 Envelope

```go
env := event.NewEnvelope(event.EnvelopeTypeOrchestrationAgentStatus, "orchestration-projector", sessionID)
env.TeamID = teamID
env.Metadata = map[string]any{
    "run_id": runID,
    "node_id": state.NodeID,
    "agent_id": state.AgentID,
    "status": string(state.Status),
    "display_status": state.DisplayStatus,
    "phase": string(state.Phase),
    "input_preview": state.InputPreview,
    "output_preview": state.OutputPreview,
    "current_activity": state.CurrentActivity,
    "retry_count": state.RetryCount,
}
env.Channel = "team" // graph run 时可为 "graph"
env.FilterKey = fmt.Sprintf("orchestration/%s/%s", runID, nodeID)
```

### 4.4 生命周期

```go
// internal/team/status_projector.go
func StartOrchestrationStatusProjector(
    ctx context.Context,
    bus event.Bus,
    cfg OrchestrationProjectorConfig,
) context.CancelFunc
```

由 `runner_team_trpc.runTeamTRPC` 在 `team_run_started` 之后启动，`defer cancel()` 于 Run 结束。

Graph Run：`internal/service/graph.go` Execute 路径同样启动（Phase 1）。

---

## 五、mode → Graph 编译器（Phase 2）

### 5.1 入口

```go
// internal/team/graph_compile.go
func CompileToGraphBuildConfig(def Definition) (graph.GraphBuildConfig, error)
```

对称于前端 `buildGraphFromDefinition()`（`web/src/components/teams/teamUtils.ts`）。

### 5.2 映射表

| mode | 拓扑 | Graph 模板 ID |
|------|------|---------------|
| sequential | 链式 | pipeline |
| parallel | fan-out / join | parallel_review |
| coordinator | 星形 + AgentTool 边 | dispatch |
| critic_loop | generate→critic→router | review_loop |
| adaptive/swarm | entry + transfer 边 | dispatch |
| graph | linked 或内嵌 | — |

Member → `NodeDef{type: agent, agent_name: agent_key}`。

---

## 六、FailurePolicy（Phase 4）

```go
type FailurePolicy struct {
    Default       string                       // retry_then_block | skip | fail_fast
    Retry         RetryPolicyDef
    NodeOverrides map[string]NodeFailureOverride
    ParallelFail  string                       // continue | abort
}
```

映射 trpc `graph.RetryPolicy`；skip 写 state `_skipped_nodes`；fallback 切换 registry 中 agent_id；`parallel_fail: continue` 时并行 join 分支失败自动 `skip_on_failure`（`ApplyParallelFailContinue`）。

#### 6.1 ParallelFail 与 generic diamond 边界

当 `parallel_fail: continue` 且编译产物含 **fan-out / join（含 generic diamond）** 拓扑时：

```
        member-1 (fork)
       /              \
 member-2 (fail)    member-3 (pass)
       \              /
        member-4 (join)
```

- **作用范围**：`ApplyParallelFailContinue` 仅对 **并行扇出分支上的 worker 节点**（compile 时标记为 parallel branch member）注入 `failure_action: skip_on_failure`；join / synthesizer 节点本身不自动 skip。
- **分支失败**：失败分支在 `AfterNode` 回调中走 `skip_on_failure`，写入 `_skipped_nodes`，图执行 **继续**（不 abort 整图）。
- **Join 语义**：join 节点仍按 trpc graph 调度执行；已 skip 的分支不会重复跑 agent，但 join 可消费其它分支输出。若 join 依赖被 skip 分支的唯一产物且 state 中无替代值，join 可能空跑或失败——**不在 Phase 4 自动补全**，需设计时保证 synthesizer 容忍部分输入缺失。
- **与 `policy: skip` 区别**：compile-time `policy: skip` 节点 **从不执行**；`skip_on_failure` 是 **运行时** 失败后降级，且会发 `GraphNodeError` 再投影 `GraphNodeEnd(skipped=true)`。
- **与 `parallel_fail: abort` 对比**：任一并行分支未恢复失败即 **整图失败**，无 `_skipped_nodes` 写入。
- **回归用例**：`internal/graph/trpc/parallel_fail_test.go`（diamond：`member-1`→`member-2`/`member-3`，`member-2`→`member-3` join）验证失败分支 skip 后图仍可 `Done`。

---

## 七、前端架构

### 7.1 路由（Phase 1）

| 路由 | 组件 | 模式 |
|------|------|------|
| `/teams/:id/orchestrate` | `TeamOrchestratePage` | 设计态，Vue Flow |
| `/teams/:id/runs/:runId` | `TeamRunObservatoryPage` | 运行态只读 |

### 7.2 组件

```
web/src/features/orchestration/
  types.ts                 ← AgentNodeState, DisplayStatus
  agentNodeStatusStyles.ts ← 聚合/细态 token（对齐 UX.md）
  useOrchestrationStream.ts
  api.ts                   ← GetTeamRunObservatory

web/src/components/orchestration/
  OrchestrationKanban.vue
  OrchestrationKanbanCard.vue
  OrchestrationStatusChip.vue
```

### 7.3 Kanban 布局

- **行** = Agent 节点（`node_id`）
- **列** = received / doing / delivered
- **行首** = `OrchestrationStatusChip`（DisplayStatus + 细态 caption）
- 与 `GraphEditorCanvas` 共享 Pinia `useOrchestrationStore`

### 7.4 Graph 节点扩展

扩展 `GraphFlowNode.vue`：

- 聚合 badge（右上圆点）
- 细态 subtitle（`tool_running` 时显示工具名）
- CSS modifier：`--thinking`, `--tool`, `--retry`, `--skipped`

复用并扩展 `EXECUTION_STATUS_STYLES` → `AGENT_NODE_STATUS_STYLES`。

---

## 八、Observatory API（Phase 1）

```protobuf
message GetTeamRunObservatoryRequest {
  string run_id = 1;
}
message AgentNodeStateView { /* mirrors biz */ }
message GetTeamRunObservatoryResponse {
  string run_id = 1;
  string status = 2;
  repeated AgentNodeStateView nodes = 3;
  repeated EdgeStateView edges = 4;
}
```

首屏 REST + WS `orchestration_agent_status` 增量。

---

## 九、与关联模块

| 模块 | 关系 |
|------|------|
| 11 Team | Definition、Run、member_* 事件源 |
| 36 Graph | GraphAgent、graph_node_*、Checkpoint |
| 51 Message | Envelope 通道；`orchestration_agent_status` 路由 team/graph |
| 52 FlowLogger | `domain=team|graph` span 与 status 对齐 |
| 17 Channel | Phase 4：`async_graph_id` 与编译路径统一 |

---

## 十、测试策略

| 层 | 文件 | 覆盖 |
|----|------|------|
| Biz | `orchestration_status_test.go` | ApplyEnvelope 优先级、终态、transfer |
| Team | `status_projector_test.go` | 事件序列 → WS 输出 |
| Service | `team_observatory_test.go` | RPC 首屏 |
| 前端 | `agentNodeStatusStyles.test.ts` | 样式映射 |

Runner E2E：EP-TEST-TG-01（Team sequential Run + WS status 序列）。
