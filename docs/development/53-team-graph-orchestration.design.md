# M53: Team × Graph 编排融合 — 实现设计

> 对应需求：[53 team-graph-orchestration.md](./53%20team-graph-orchestration.md)
> 遵循：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)
> **实现差距与迭代计划**以 [53-team-graph-orchestration.development.md](./53-team-graph-orchestration.development.md) 为准

---

## 一、模块概述

### 1.1 设计定位

**OrchestrationSpec** 为 Team 与 Graph 的统一编排真相源：

- **Team 简视图**：mode + members + 参数 → 编译器生成 graph 拓扑
- **Graph 高级视图**：Vue Flow 自由编辑，可 `linked_graph_id` 引用
- **运行态**：StatusProjector 将异构 Envelope 投影为 `AgentNodeState`；Kanban 与 Graph 共用

**终态（执行单链）**：所有 Team Run 经 `CompileToGraphRuntimeConfig` → `GraphAgent` 执行；`BuildTRPCTeam`（Native）已退役，仅 `ARANEA_TEAM_NATIVE=1` 紧急熔断。

> 实施进度与剩余差距详见 [53-team-graph-orchestration.development.md §2 现状评估](./53-team-graph-orchestration.development.md#2-现状评估2026-06-06)

### 1.2 分层与依赖

```
api/kratos/team/v1/team.proto          ← Team 扩展字段（linked_graph_id 等，Phase 2+）
api/kratos/graph/v1/graph.proto        ← Graph 既有 RPC
        ↓
internal/service/
  team.go · graph.go
  team_observatory.go              ← GetTeamRunObservatory（Phase 1）
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
| `internal/team` | 新增/修改 | 编译器、StatusProjector、Runner 挂钩、Coordinator、模板注册表 |
| `internal/service` | 新增 | Observatory RPC、Run 锁定 |
| `internal/event` | 扩展 | `orchestration_agent_status` EnvelopeType |
| `internal/graph/trpc` | 修改 | Graph Run 启动投影器、failure_recovery |
| `web/src/features/orchestration` | 新增 | 类型、Kanban、store、Timeline |
| `web/src/components/graph` | 扩展 | 节点细态、边状态、属性面板（Retry/Destinations/Mapper） |
| `api/kratos/team/v1` | 扩展 | Proto 字段 |
| `internal/data` | 新增 | `orchestration_steps` 表、`team_graph_sessions` 表 |

**不改动**：`internal/server` 直连 runtime。

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

### 2.3 TeamRun 扩展

| 字段 | 类型 | 说明 |
|------|------|------|
| `definition_snapshot_json` | TEXT | Run 开始时 OrchestrationSpec 冻结 |
| `graph_execution_id` | UUID | 关联 graph_executions |
| `trace_id` | TEXT | 跨域 trace 关联 |

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

实现：`OrchestrationStatusStore.ApplyEnvelope` 纯领域方法，便于单测。

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

**不负责**：持久化（由 `ActivityStepFlusher` 异步批 flush 到 `orchestration_steps` 表）、Graph 构建、Team Run 生命周期。

**设计要点**：`activity_history[]`（上限 20 条）+ `current_activity`；`ActivityStepFlusher` 异步批 flush。

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

Graph Run：`internal/service/graph.go` Execute 路径同样启动。

---

## 五、mode → Graph 编译器

### 5.1 入口

```go
// internal/team/graph_compile.go
func CompileToGraphBuildConfig(def Definition) (graph.GraphBuildConfig, error)
```

对称于前端 `buildGraphFromDefinition()`（`web/src/components/teams/teamUtils.ts`）。

**编译路径（统一）**：不再按 mode 分发，而是通过 `generateGraphSpecFromMode` 生成 embedded graph spec 后统一走 `compileFromEmbeddedGraph`。模板注册表（`template_registry.go`）提供 5 个内置模板。

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

## 六、FailurePolicy

```go
type FailurePolicy struct {
    Default       string                       // retry_then_block | skip | fail_fast
    Retry         RetryPolicyDef
    NodeOverrides map[string]NodeFailureOverride
    ParallelFail  string                       // continue | abort
}
```

映射 trpc `graph.RetryPolicy`；skip 写 state `_skipped_nodes`；fallback 切换 registry 中 agent_id；`parallel_fail: continue` 时并行 join 分支失败自动 `skip_on_failure`（`ApplyParallelFailContinue`）。

**设计范围**：retry / skip / fallback_agent / parallel_fail continue / failure_recovery.go。

**预留未实现**：circuit_breaker（类型预留）、死信表。

> 实施状态详见 [53-team-graph-orchestration.development.md §8.3 差距清单](./53-team-graph-orchestration.development.md#83-差距清单phase-0588-已解决项--待实施项)

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
- **Join 语义**：join 节点仍按 trpc graph 调度执行；已 skip 的分支不会重复跑 agent，但 join 可消费其它分支输出。若 join 依赖被 skip 分支的唯一产物且 state 中无替代值，join 可能空跑或失败——**不在 ParallelFail 自动补全**，需设计时保证 synthesizer 容忍部分输入缺失。
- **与 `policy: skip` 区别**：compile-time `policy: skip` 节点 **从不执行**；`skip_on_failure` 是 **运行时** 失败后降级，且会发 `GraphNodeError` 再投影 `GraphNodeEnd(skipped=true)`。
- **与 `parallel_fail: abort` 对比**：任一并行分支未恢复失败即 **整图失败**，无 `_skipped_nodes` 写入。
- **回归用例**：`internal/graph/trpc/parallel_fail_test.go`（diamond：`member-1`→`member-2`/`member-3`，`member-2`→`member-3` join）验证失败分支 skip 后图仍可 `Done`。

---

## 七、前端架构

### 7.1 路由

| 路由 | 组件 | 模式 |
|------|------|------|
| `/teams/:id/orchestrate` | `TeamOrchestratePage` | 设计态，Vue Flow |
| `/teams/:id/runs/:runId` | `TeamRunObservatoryPage` | 运行态只读 |

### 7.2 组件

```
web/src/features/orchestration/
  types.ts                 ← AgentNodeState, DisplayStatus, WorkPhase, ActivitySnapshot
  agentNodeStatusStyles.ts ← 聚合/细态 token（对齐 UX.md）
  useOrchestrationStream.ts
  api.ts                   ← GetTeamRunObservatory / GetTeamRunObservatoryTimeline
  compileApi.ts            ← CompileTeamGraph RPC
  teamGraphAdapter.ts      ← teamDefinitionToGraphDef + displayStatusToExecStatus
  teamNodeDisplay.ts       ← 节点展示标签逻辑

web/src/components/orchestration/
  OrchestrationKanban.vue
  OrchestrationKanbanCard.vue
  OrchestrationStatusChip.vue
  OrchestrationActivityTimeline.vue    ← Activity 时间线 Tab
  OrchestrationFailureBanner.vue       ← 失败横幅（重试/fallback/审核/终止）
  OrchestrationHitlReviewDialog.vue    ← HITL 人工审核对话框

web/src/features/teams/
  orchestrationSpec.ts     ← OrchestrationSpec 类型 + toOrchestrationSpec / fromOrchestrationSpec
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

### 7.5 Graph 属性面板

`GraphPropertyPanel.vue`：

- **RetryPolicy**：`max_attempts` + `failure_action` + `fallback_agent`（agent/router 节点）
- **Destinations**：多选 GoTo 目标节点（agent/router 节点）
- **Mapper**：Input/Output JSON 编辑 + `isolatedMessages` / `inputFromLastResponse` toggle（agent 节点）

### 7.6 编排面板

`TeamOrchestrateRuntimePanel.vue`（功能等价于原设计 `TeamRuntimeSection.vue`）：

- runtime_engine 切换（graph / native）
- failure_policy 配置
- 编译状态展示

---

## 八、Observatory API

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

**RPC 契约**：`GetTeamRunObservatory` RPC + `GetTeamRunObservatoryTimeline` RPC（Activity 时间线）。

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
---

## 附录：企业级蓝图与 AI 落地指南

> 原 `需求/53%20team-graph-orchestration.design.md#附录企业级蓝图与-ai-落地指南` 已并入本文。
> **实施进度与任务清单**详见 [53-team-graph-orchestration.development.md](./53-team-graph-orchestration.development.md)。

## 0. 本文是什么

本文是 **M53（Team × Graph 编排融合）企业级落地纲领**，把成果推到 **企业产品级** 终态：

> **OrchestrationSpec 是唯一编排真相源；GraphAgent 是唯一运行时；Team / Graph / Multi-Agent 是同一引擎的三种产品视图。**

它做三件事：

1. **诊断**：把分散在三套文档（11/36/53）与代码里的痛点收敛为 6 类系统级问题。
2. **设计**：给出 **执行层 / 编排规格 / 观测层 / 容错层 / UX 层** 的目标态与契约。
3. **落地**：给 AI 编码代理一份按 ID 可执行的任务卡清单（含代码锚点、验收、风险、回滚剧本）。

> 当前实施进度详见 [53-team-graph-orchestration.development.md §2 现状评估](./53-team-graph-orchestration.development.md#2-现状评估2026-06-06)

**红线不变**：

- `internal/biz` 不 import `trpc-agent-go`
- 运行时装配只在 `internal/service` / `internal/team` / `internal/graph`，server 不直调 Runner
- 编译真相源是 biz（`graph_compile.go` / `embedded_graph.go` / `failure_policy.go`），前端不伪造拓扑
- Channel 与 Web 共用 Session；Team Run / Graph Execution 与 Channel Job 共用同一 `channel_turn_jobs`

---

## 1. 问题陈述（6 类核心症结）

| # | 症结 | 用户可感知 | 根因层 |
|---|------|-----------|--------|
| **T-1** | 执行层双轨：默认 Native，Graph 仅 feature flag | 同一 Team 在 chat 与 channel async 路径走两套执行栈；故障表现不一致 | 架构 |
| **T-2** | OrchestrationSpec 产品化不完整 | 用户改不到 `runtime_engine` / `failure_policy`；前端 `parseDefinition` 白名单丢字段 | 协议/UX |
| **T-3** | 节点能力缺口（Agent/Router/HITL/Subgraph） | Team 编译只输出 `agent` 节点；Graph 编辑器无属性面板编辑 RetryPolicy / Destinations / Mapper | 后端/前端 |
| **T-4** | 观测断片：Activity 只有 current，无历史时间线 | Kanban 列「进行中」只显示最后一个工具；之前的工具消失 | 业务 |
| **T-5** | 容错单一：FailurePolicy 已编译 retry/skip，但 fallback / 熔断 / HITL 接管未一等 | 关键 Agent 失败时不能切备用 / 暂停 / 等人工 | 业务/UX |
| **T-6** | 跨域可观测性：跨 Team / Channel async / Chat 子 turn 无统一 trace | 排障靠 grep；找不到一条 trace 看穿 Team→GraphAgent→子 Agent→Tool→Channel | 平台 |

> T-1 / T-2 / T-3 是 **结构性缺口**；T-4 / T-5 是 **业务表达不够**；T-6 是 **平台化债务**。本文按这六类系统级整理目标态与任务。
> 各症结的解决状态详见 [53-team-graph-orchestration.development.md §8.3 差距清单](./53-team-graph-orchestration.development.md#83-差距清单phase-0588-已解决项--待实施项)

---

## 2. 目标态（企业产品级）

### 2.1 执行层 — 单链终态（解决 T-1）

```
┌─────────────────────────────────────────────────────────────────┐
│  编辑视图（用户）                                                  │
│  ├─ Team Editor（mode 模板 + 成员）                                │
│  ├─ Graph Editor（自由拓扑 / Vue Flow）                            │
│  └─ Embedded Graph（OrchestrationSpec custom 区段）                │
└─────────────┬───────────────────────────────────────────────────┘
              │ 编辑保存 → definition_json
              ▼
┌─────────────────────────────────────────────────────────────────┐
│  OrchestrationSpec v2（唯一真相源，biz 类型）                       │
│  - members[]                                                     │
│  - mode (sequential / parallel / coordinator / critic_loop /     │
│         adaptive / custom)                                       │
│  - graph: { nodes[], edges[], conditional_edges[], entry_point } │
│  - linked_graph_id?                                              │
│  - failure_policy { retry, skip, fallback_agent, circuit }       │
│  - runtime_engine: "graph"（默认）                                 │
│  - turn_timeout_sec / first_byte_timeout_sec                     │
└─────────────┬───────────────────────────────────────────────────┘
              │ CompileToGraphBuildConfig / CompileToGraphRuntimeConfig
              ▼
┌─────────────────────────────────────────────────────────────────┐
│  GraphBuildConfig（biz 中间表达）                                  │
│  - 节点 / 边 / mapper / retry / cache / destinations             │
│  - 用于 Compile→GraphAgent，也用于 Observatory `compiled_topology`│
└─────────────┬───────────────────────────────────────────────────┘
              │ adapter.BuildTeamGraphRoot / graph.BuildStateGraphWithRegistry
              ▼
┌─────────────────────────────────────────────────────────────────┐
│  GraphAgent（trpc-agent-go 唯一执行引擎）                          │
│  - BSP / DAG · Checkpoint · HITL · Retry · Cache                 │
│  - 入口：Chat Turn / Channel async / Cron / RunTest              │
└─────────────────────────────────────────────────────────────────┘
```

**入口分流（产品视角）**：

| 触发 | 编译路径 | 运行时 |
|------|---------|--------|
| Web Team Chat | `CompileToGraphRuntimeConfig` | GraphAgent |
| Web Graph Run | 直接 GraphDefinition | GraphAgent |
| Channel `async_team_id` | `CompileToGraphRuntimeConfig` | GraphAgent（已落地） |
| Channel `async_graph_id` | GraphDefinition | GraphAgent |
| Cron 触发 Team | 同上 | GraphAgent |
| Team RunTest | 同上 | GraphAgent |

**Native 路径定位**：

- `BuildTRPCTeam` 标记 Deprecated，仅 `ARANEA_TEAM_NATIVE=1` 紧急熔断时使用。
- 编译器统一走 `compileFromEmbeddedGraph`，不再按 mode 分发。

> 退役实施进度详见 [53-team-graph-orchestration.development.md §8.2 已完成](./53-team-graph-orchestration.development.md#82-已完成一条链的完整实现)

### 2.2 编排规格 v2 产品化（解决 T-2）

#### 2.2.1 类型对齐

后端已存在的字段（`runtime_engine` / `failure_policy` / `embedded graph` / `linked_graph_id`）必须**前端一等公民化**：

```ts
// web/src/features/teams/types.ts（目标态）
export interface OrchestrationSpec {
  version: 2;
  mode: TeamMode;
  members: TeamMember[];
  graph?: EmbeddedGraph;                 // OrchestrationSpec custom 区段
  linked_graph_id?: string;
  runtime_engine: "graph" | "native";    // 默认 graph
  failure_policy?: FailurePolicySpec;
  turn_timeout_sec?: number;
  first_byte_timeout_sec?: number;
  intent_anchor_agent_id?: string;
  // ... swarm 子段
}
```

`web/src/features/teams/teamUtils.ts` 的 `parseDefinition` 改为 **raw merge** 策略：白名单解析已知字段，未知字段（含 `runtime_engine` 等）保留到 `__raw__` 字段并在保存时回写，**避免 silent drop**。

#### 2.2.2 编辑器目标态

| 入口 | 职责 | 锚点 |
|------|------|------|
| `/teams/:id/edit` | 成员、模式、`failure_policy`、`runtime_engine` 下拉 | `TeamEditorDialog.vue` + `TeamOrchestrateRuntimePanel.vue` |
| `/teams/:id/orchestrate` | OrchestrationSpec 自由编辑（Vue Flow）；Run 中 readonly | `TeamOrchestratePage.vue`（已存在） |
| `/graphs/:id/edit` | 独立 Graph 编辑 + Mapper / Destinations / Retry 属性面板 | `GraphPropertyPanel.vue` |
| `/team-runs/:id/observatory` | Kanban + Graph 双视图（只读） | 已存在；扩展 §2.3 |

#### 2.2.3 编排来源（保持现有定义）

| 值 | 编辑入口 | 运行编译 |
|----|---------|---------|
| `preset` | Team Editor | mode 模板 |
| `custom` | Team Editor + Orchestrate Page | mode 模板 ⊕ embedded graph |
| `linked_graph` | Team Editor 选 `graph_id` | `linked_graph_loader` |

### 2.3 节点能力缺口（解决 T-3）

Graph builder 已经支持的能力（`internal/graph/trpc/node_wiring.go`）：

- `llm` / `tool` / `agent` / `router` / `function` ✅
- `WithEndsMap` / `WithRetryPolicy` / `WithNodeCachePolicy` ✅
- Mapper（Input/Output/Isolated/InputFromLastResponse）✅
- Failure recovery（skip / continue / fallback agent）✅

**未补齐**（前端 + Team 编译 + 文档对齐）：

| 缺口 | 任务 | 优先级 |
|------|------|--------|
| 前端属性面板编辑 `RetryPolicy` / `CachePolicy` / `Destinations` / `Mapper` | Phase G-FE | P0 |
| Team 编译生成 `router` 节点（adaptive 模式的 Destinations）| UI 显示 transfer 边 | P1 |
| HITL 节点：与 `InterruptBefore/After` + Chat `AwaitUserReply` 复用 UX | Phase H | P1 |
| Subgraph 节点：Team 嵌套 Team（linked_graph 在节点级） | Phase I | P2 |
| Function 节点 + CodeExecutor 桥接 | Phase J | P2 |

> 实施状态详见 [53-team-graph-orchestration.development.md §8.3 差距清单](./53-team-graph-orchestration.development.md#83-差距清单phase-0588-已解决项--待实施项)

### 2.4 观测层 — 时间线 + 拓扑（解决 T-4）

#### 2.4.1 Activity Timeline

**设计**：`status_projector.go` 把 `current_activity` 升级为 `activity_history[]`，每个 ActivitySnapshot 带 `started_at / finished_at / status`。

**数据结构**：`AgentNodeState.ActivityHistory []ActivitySnapshot`（上限 20 条）+ `ActivityStepFlusher` 异步批 flush 到 `orchestration_steps` 表 + Timeline RPC + 前端 `OrchestrationActivityTimeline.vue`。

```go
// internal/biz/orchestration_status.go（增量）
type AgentNodeState struct {
    NodeID            string
    Status            AgentNodeStatus
    DisplayStatus     DisplayStatus
    Phase             WorkPhase
    CurrentActivity   *ActivitySnapshot   // 仍保留
    ActivityHistory   []ActivitySnapshot  // ★ 新增
    Received          *Snapshot           // 上游输入
    Delivered         *Snapshot           // 节点输出
    LastTransitionAt  time.Time
}
```

`OrchestrationObservatory` API 返回 `activity_history`，前端 Kanban 「进行中」列改为时间轴渲染（最多 5 条，超过折叠）。

#### 2.4.2 Trace 统一

`team_run.id` / `graph_execution.id` / chat `run_id` 必须共享 `trace_id`：

- 当 Chat Turn 触发 Team Run（`owner_type=team`）：`trace_id` 来自 chat turn 的 root span，team run 与子 graph execution 都 inherit。
- Channel async Team：`trace_id` 来自 `channel_turn_job` 创建时；后续 Graph Execution 共用。
- Observatory API 返回顶层 `trace_id`；前端 "View Trace" 跳到 Monitor Trace UI（与 FlowLog `chat.turn.*` 关联）。

> `team_runs.trace_id` 字段已在 Ent Schema 中定义（`internal/data/ent/schema/team_run.go`）。

#### 2.4.3 拓扑统一

| 数据来源 | 用户视图 |
|---------|---------|
| Team `definition.graph` + `embedded_graph` | OrchestratePage Vue Flow |
| `CompileTeamGraph` 后端真相 | Observatory `compiled_topology` |
| GraphExecution.summary | Run 完成态 ExecutionSummary |
| Team Run 中 GraphExecution 实时 step | Observatory Step Tab |
| Cross-Team trace（Team 调子 Team） | Monitor cross-trace |

### 2.5 容错层 — FailurePolicy 完整化（解决 T-5）

| 策略 | 设计 |
|------|------|
| `retry { max_attempts, backoff }` | `WithRetryPolicy(WithSimpleRetry(N))`；暴露 backoff strategy（exp / linear） |
| `skip` | 编译期 `SkipNodeFuncRef`；UI 展示 skip 边 + Kanban "skipped" 状态 |
| `fallback_agent` | `node_wiring.go` Agent 节点支持 + `failure_recovery.go`；UI 编辑；envelope `agent_failover` |
| `continue_on_failure` | ParallelFail；UI 标注；统计 partial-success |
| **circuit_breaker** | 类型预留（`TeamFailurePolicy.circuit_breaker`）；目标：阈值 N 次连续失败 → 节点冻结 + WS alert |
| **HITL 接管** | InterruptBefore/After + `OrchestrationFailureBanner`；错误时进入 `waiting_review`；前端 banner + 审核 |
| **死信** | 目标：失败 N 轮后写入 `task_dead_letters` 表 / Job dashboard |

> 实施状态详见 [53-team-graph-orchestration.development.md §8.3 差距清单](./53-team-graph-orchestration.development.md#83-差距清单phase-0588-已解决项--待实施项)

**统一错误模型**（前端契约）：

```ts
type NodeFailure = {
  node_id: string;
  attempt: number;
  max_attempts: number;
  reason: "agent_error" | "tool_error" | "timeout" | "cancel" | "circuit_open";
  error_code: string;
  error_message: string;
  recovery: "retry" | "skip" | "fallback" | "halt" | "await_review";
  fallback_agent?: string;
  next_node_id?: string;
};
```

### 2.6 UX 层 — 三视图协同（解决 T-3+T-4 复合）

```
                ┌─────────────────────────────────────────────────┐
                │ Team Workspace（/teams/:id）                       │
                ├─────────────────────────────────────────────────┤
                │ ┌─────────────┬───────────────────────────────┐ │
                │ │ Editor      │ Orchestrate (Vue Flow)        │ │
                │ │ - members   │ - 编译实时预览                  │ │
                │ │ - mode      │ - 节点细配置（subroute）        │ │
                │ │ - failure   │ - Run 期间 readonly            │ │
                │ │   policy    │                               │ │
                │ │ - runtime   │                               │ │
                │ └─────────────┴───────────────────────────────┘ │
                │ ┌──────────────────────────────────────────────┐│
                │ │ Runs Tab                                    │ │
                │ │ - 列表 + ExecutionSummary + 深链 Observatory  │ │
                │ └──────────────────────────────────────────────┘│
                └─────────────────────────────────────────────────┘

                ┌─────────────────────────────────────────────────┐
                │ Run Observatory（/team-runs/:id/observatory）    │
                ├─────────────────────────────────────────────────┤
                │ Tabs:                                           │
                │  - Graph（Vue Flow + 实时节点状态）              │
                │  - Kanban（received/doing/delivered + history）│
                │  - Activity Timeline（顶层 Trace 视图）          │
                │  - HITL（等待审核 / 错误接管入口）                │
                │  - ExecutionSummary（成本 / 时长 / 工具调用）    │
                └─────────────────────────────────────────────────┘
```

**信息架构原则**：

- **一种状态、多个投影**：Agent 节点状态在 Graph 节点 chip / Kanban chip / Activity timeline 三处展示，但全部读自 `AgentNodeState`。
- **导航联动**：Kanban 选中 Agent → Graph fitView + highlight；Graph 节点点击 → Kanban scrollTo + 详情面板；Activity 行点击 → 跳工具卡片或 Trace。
- **错误体感**：失败 / 等审核 / 熔断 → 全局 banner（顶部）+ Kanban 红边 + Graph 节点红色 + Activity 红行；用户 1 屏即知。

---

## 3. 架构契约（提交前必须读）

### 3.1 分层与依赖

```
api/kratos/team/v1/team.proto         ← OrchestrationSpec v2 协议
api/kratos/graph/v1/graph.proto       ← GraphDefinition 与执行 RPC
        ↓
internal/server                       ← WS 帧路由（薄）
internal/service                      ← 装配点：team / graph / orchestration_observatory
internal/team                         ← Team 运行时（Compile + Project + Run via GraphAgent）
internal/graph                        ← Graph 运行时（trpc Builder + adapter）
internal/biz                          ← OrchestrationSpec / GraphBuildConfig / Status / FailurePolicy
internal/data                         ← Ent ORM（team_runs / graph_executions / orchestration_steps）
internal/event                        ← Envelope + Bus + Buffer
```

**红线检查**：

- ✅ `internal/biz` 不 import `trpc-agent-go`（编译期类型 `GraphBuildConfig` 是 biz 中间表达）
- ✅ `internal/team` 不直接构造 `trpcgraph.StateGraph`；走 `adapter.TeamGraphRootBuilder`
- ✅ `internal/server` 不直调 Runner / GraphAgent

### 3.2 数据模型增量

| 表 | 字段 | 用途 |
|----|------|------|
| `team_runs` | `definition_snapshot_json` | Run 期间快照 |
| `team_runs` | `graph_execution_id` | 链入 Graph Run |
| `team_runs` | `trace_id` | 跨域 trace |
| `orchestration_steps` | `id / team_run_id / graph_execution_id / node_id / activity_snapshot_json / status / started_at / finished_at / created_at` | 持久化 Activity 时间线 |
| `team_graph_sessions` | `exec_id / team_run_id / team_id / session_id / input_preview / definition_json / status / registered_at / last_activity_at` | 进程重启恢复 |
| `team_run_steps` | `tool_call_count` · `duration_ms` | — |

**迁移**：

1. Ent schema 增加 `team_runs.trace_id`、`orchestration_steps` 表；
2. `make wire && make api && ent generate`；
3. 旧记录 `trace_id` 默认空字符串，前端兼容。

> 迁移实施状态详见 [53-team-graph-orchestration.development.md §8.2 已完成](./53-team-graph-orchestration.development.md#82-已完成一条链的完整实现)

### 3.3 Envelope 协议增量

```go
type Envelope struct {
    // 已有字段...
    TraceID         string `json:"trace_id,omitempty"`         // ★ 已存在于 EnvelopeTrace；目标提升到顶层
    GraphExecutionID string `json:"graph_execution_id,omitempty"` // ★ 新增
    NodeID          string `json:"node_id,omitempty"`          // 已用于 orchestration_agent_status；统一覆盖 graph_node_*
}
```

新类型：

```go
const (
    EnvelopeTypeOrchestrationActivity EnvelopeType = "orchestration_activity"   // 单次 activity 起止
    EnvelopeTypeAgentFailover          EnvelopeType = "agent_failover"           // fallback / retry 触发
    EnvelopeTypeCircuitOpened          EnvelopeType = "circuit_opened"           // 熔断
)
```

### 3.4 API 增量

```proto
// api/kratos/team/v1/team.proto
service TeamService {
  // 已有 ...
  // ★ 新增：Activity 时间线（Observatory Tab 用）
  rpc GetTeamRunObservatoryTimeline(GetTeamRunObservatoryTimelineRequest)
      returns (GetTeamRunObservatoryTimelineResponse);
}

message GetTeamRunObservatoryTimelineRequest {
  string run_id = 1;
  optional string node_id = 2;  // 按节点过滤
  optional int32 limit = 3;
}

message ActivityRow {
  string node_id = 1;
  string kind = 2;            // tool / skill / mcp / subagent
  string display_label = 3;
  string status = 4;
  string started_at = 5;
  string finished_at = 6;
  int64 duration_ms = 7;
  string trace_id = 8;
}

message GetTeamRunObservatoryTimelineResponse {
  repeated ActivityRow rows = 1;
}
```

```proto
// api/kratos/graph/v1/graph.proto
message ResumeGraphExecutionRequest {
  string execution_id = 1;
  bytes state_patch_json = 2;  // 可选：编辑后恢复
}
```

```proto
// 在 team.proto 加入 OrchestrationSpec v2 显式字段（与 definition_json 平行；服务端可二选一）
message OrchestrationSpec {
  int32 version = 1;
  string mode = 2;
  repeated TeamMember members = 3;
  optional EmbeddedGraph graph = 4;
  optional string linked_graph_id = 5;
  optional FailurePolicy failure_policy = 6;
  optional string runtime_engine = 7;
  optional int32 turn_timeout_sec = 8;
}
```

> **兼容策略**：在保留 `definition_json` 的同时提供 `orchestration_spec` 字段。前端 v2 客户端使用 strong-typed；旧客户端继续 raw JSON。

---

## 4. 业务流程（端到端）

### 4.1 Team Chat Turn（GraphAgent 默认）

```
用户消息 → ChatService.runNativeAgentTurn(sess.OwnerType=team)
  → team.Runner.runTeamTRPC
      ├─ CreateTeamRun（含 definition_snapshot_json + trace_id）
      ├─ CompileToGraphRuntimeConfig
      ├─ adapter.BuildTeamGraphRoot → GraphAgent
      ├─ runner.StoreRunner
      ├─ StartOrchestrationStatusProjector
      │     ├─ member_message_start → status=thinking
      │     ├─ tool_call_start      → status=tool_running + activity push
      │     ├─ tool_call_end        → activity finish
      │     ├─ member_message_done  → status=success
      │     └─ graph_node_error     → status=failed → FailurePolicy 决策
      ├─ RunTRPCUserTurn → ConsumeEventStream → EventProjector → EventBus
      │     → WS:
      │         channel:chat（member_* / text_delta）
      │         channel:team（orchestration_agent_status / team_summary）
      │         channel:graph（graph_node_* / graph_execution_done）
      └─ persistStep + UpdateTeamRun + flushActivityHistory（★ 新增）
```

### 4.2 Channel Async Team Job（与 Chat 同编译链）

```
Channel webhook（async_team_id 配置）
  → ChannelIngress.dispatchAsyncInbound
  → CreateTurnJob（accepted, trace_id 生成）
  → ResolveChannelAsyncGraphTarget → team_graph
  → graphs.ExecuteGraphBuildConfig（输入：team CompileToGraphRuntimeConfig）
  → GraphAgent 执行（与 §4.1 同一引擎）
  → enqueueOutboundReply（"后台任务已创建（Job: X）"）
  → watchAsyncGraphCompletion（短期）/ Worker deadline（Phase F）
  → 完成 → outbound 通知 + Job done envelope + Web Observatory 自动刷新
```

### 4.3 失败接管路径（FailurePolicy 完整化）

```
Agent 节点失败
  ├─ retry: 自动重试 → envelope `agent_retry` + Kanban "retrying"
  ├─ skip: 跳过 + envelope `node_skipped` + Graph 灰线 + ParallelFail.continue_on_failure
  ├─ fallback_agent: 切备用 → envelope `agent_failover` + Kanban 显示新 Agent
  ├─ circuit_open（连续 N 失败）: 节点冻结 + envelope `circuit_opened` + 全局 banner
  ├─ halt: TeamRun 失败收口
  └─ await_review（HITL）: status=waiting_review
        ↓
     前端展示 banner + 一键审核（通过 / 拒绝 / 重写输入）
        ↓
     ResumeGraphExecution(state_patch_json)
        ↓
     继续执行
```

### 4.4 跨 Team / Cross-Trace 编排

```
Team A 内某 Agent 调 call_agent(b)
  ├─ a2a.Invoke（远端或同进程）
  ├─ Trace 注入：child_trace_id 共享 parent trace_id
  ├─ 远端 Team B 启动 TeamRun（带 parent trace_id）
  └─ Observatory「跨 Team」Tab：trace tree 展示父子关系
```

---

## 5. UX 规范（企业级最低门槛）

### 5.1 三视图协同（参考 §2.6 拓扑图）

#### Team Workspace

- 左：Members & Mode（与 mode 模板对应的图标，hover 提示语义）
- 右：Vue Flow 实时编译预览（OrchestratePage 子路由）
- 底部：`FailurePolicySection` 表单（retry / skip / fallback_agent 选择）+ `RuntimeEngineSelector`（默认 graph，仅高级用户能切 native，需 admin 权限）

#### Run Observatory

- 顶栏：`{ trace_id · status · started_at · duration · cost }`，trace_id 可点击跳 Monitor。
- Tabs：Graph / Kanban / Timeline / HITL（badge）/ Summary。
- 失败状态：顶部红 banner + "查看错误" / "切 fallback" / "审核接管" 三按钮（按 FailurePolicy 出现）。

### 5.2 状态与色板

| 状态 | 色（昼）| 色（夜）| 图标 |
|------|-------|-------|------|
| idle / queued / scheduled | 灰 `--neutral-400` | 灰 `--neutral-600` | clock |
| running / thinking / tool_running / transferring | 主色 `--color-accent` 脉冲 | 青 `--color-accent-night` | pulse |
| waiting_input / waiting_review | 黄 `--state-warning` | 同 | hourglass |
| retrying | 蓝 | 蓝 | refresh |
| success | 绿 `--state-success` | 绿 | check |
| failed / circuit_open | 红 `--state-danger` | 红 | error |
| skipped / cancelled | 灰描边 | 灰描边 | skip / close |
| timed_out | 橙 | 橙 | timer_off |

### 5.3 实时性 SLA

- WS envelope 到达 → Kanban / Graph chip 状态 < 50ms（rAF batch）
- Observatory 首屏 30 节点 < 500ms（已有目标）
- Activity Timeline 增量更新无闪烁（使用 `<TransitionGroup>` + key=`activity.id`）

### 5.4 A11y

- 所有状态 chip 带 `aria-label`（含文本与图标）
- Kanban 列与 Graph 节点支持键盘导航（tab + arrow keys）
- 失败 banner role="alert"

### 5.5 i18n

- 新增 namespace：`orchestration.status.*` / `orchestration.failure.*` / `team.run.observatory.*` / `graph.node.policy.*`
- 中英双语

---

## 6. AI 落地任务清单（按 ID 执行）

> **任务清单（含 Phase 划分、代码锚点、验收标准、状态标记）已迁移至开发计划文档。**
> 详见 [53-team-graph-orchestration.development.md §3 开发阶段](./53-team-graph-orchestration.development.md#3-开发阶段) 与 [§Phase 8 架构优化](./53-team-graph-orchestration.development.md#phase-8--架构优化2026-05-28启动)
>
> 任务编号承接 M53 / M36 已有 ID（TG-RT-* / G-* / TEAM-*）+ 新增前缀 `OPS-` 表示运维/可观测、`UX-` 表示前端体验。
> 待实施任务（Phase 8.9）：BL-05（Step 持久化事件驱动统一）、BL-09（Observer 单订阅化）、FP-02（Circuit Breaker）、FP-04（死信表）。
> **注意**：OPS-TRACE-01（trace_id 持久化）已在 Ent Schema 中实现（`team_runs.trace_id` 字段）。

---

## 7. 验证矩阵

### 7.1 CI 必跑

```bash
make wire && make wire-clean && make api && make build && make test && make lint && make runtime-boundary
cd web && pnpm i && pnpm lint && pnpm test && pnpm build
```

### 7.2 单测与集成测试

| 测试 | 位置 | 验证 |
|------|------|------|
| `TestParityNativeVsGraph_<mode>` | `internal/team/parity_test.go` | 六模式 parity |
| `TestActivityHistoryProjection` | `internal/team/status_projector_test.go` | history capped + ordering |
- **验收**：六种 mode（sequential/parallel/coordinator/critic_loop/swarm/adaptive）输出 parity 报告；diff 文档化。
- **风险**：Graph 路径事件序列与 Native 不完全 1:1（Graph 多 `graph_node_*`）；接受这一类有文档的 diff。

#### TG-RT-UI：runtime_engine 前端编辑

- **目标**：
  - `web/src/features/teams/types.ts` 扩展 `OrchestrationSpec.runtime_engine`。
  - `web/src/features/teams/teamUtils.ts` 的 `parseDefinition` 改 raw merge（保留未知字段）。
  - `TeamEditDialog.vue` 添加 `RuntimeEngineSelector`（admin 权限可见）。
- **代码锚点**：`web/src/features/teams/teamUtils.ts` · `web/src/components/teams/TeamEditDialog.vue`。
- **验收**：保存 Team 不丢 `runtime_engine` / `failure_policy`；切换值后 reload 仍显示正确。
- **风险**：parseDefinition 改动影响所有 Team 编辑；先在 PR 标注必跑 `pnpm test` 覆盖 `teamUtils.spec.ts`。

#### TG-RT-UI-RO：GraphEditorCanvas readonly 模式

- **目标**：`web/src/components/graph/GraphEditorCanvas.vue` 增加 `readonly` prop；Run 期间 OrchestratePage 与 Observatory Graph Tab 设 `readonly=true`，禁止拖拽 / 连边 / 删除。
- **验收**：Run 中拖节点不动；删除快捷键无效；右键菜单隐藏 destructive 项。
- **风险**：Vue Flow 默认允许交互；需 `nodesDraggable=false`、`edgesUpdatable=false`、`elementsSelectable=true`（保留选中查看）。

#### TG-RT-METRICS：runtime 路径指标

- **目标**：Prometheus 增加 `team_run_runtime_total{engine=native|graph, outcome=success|fallback|error}`；Grafana panel `Team Runtime Path Distribution`。
- **代码锚点**：`internal/metrics/vars.go`（已有 `TeamGraphRuntimeTotal`）+ Grafana JSON。
- **验收**：每个 TeamRun 完成后指标 +1；Grafana 看板 Canary 阶段可见 Graph % 增长。

#### TG-RT-FLAG：生产 Canary rollout playbook

- **目标**：在 `docs/devlog/` 新增 `2026-XX-XX-Team-Graph-Rollout-Runbook.md`：
  - Canary：先开 5% Team；
  - 监控：parity 测试通过 + metrics 无 fallback 飙升 + FlowLog 无 error 增多；
  - Stage：50% → 100%；
  - 回滚：env 切回 0，无需代码改动。
- **验收**：Runbook 评审通过；按 Runbook 走 1 周观察期。

### 6.2 Phase 5b · Activity History + 持久化（P0）— ✅ 已完成

#### OPS-OBS-HIST-01：StatusProjector 增加历史

- **目标**：`internal/team/status_projector.go` 在 push activity 时同时追加到 `activity_history`（capped 50 条）；`internal/biz/orchestration_status.go.ActivitySnapshot` 已有；扩展 `AgentNodeState` 加 `ActivityHistory []ActivitySnapshot`。
- **验收**：单测：连续 10 个 tool_call → 全在 history 里，current 是最后一个；finished_at 正确。
- **风险**：内存压力；用环形缓冲（已实现 ringbuffer 模式可参考）。

#### OPS-OBS-HIST-02：持久化到 `orchestration_steps`

- **目标**：新建 Ent schema `orchestration_step`（字段：`id`、`team_run_id`、`graph_execution_id`、`node_id`、`activity_snapshot_json`、`status`、`started_at`、`finished_at`）。Projector 异步批 flush（每 500ms 或 10 条）。
- **代码锚点**：`internal/data/ent/schema/orchestration_step.go`（新建）· `internal/biz/orchestration_step.go`（Repo 接口） · `internal/team/status_projector.go`（flush hook）。
- **验收**：Run 完成后从 DB 可重建 timeline；Observatory API 走 DB 而非内存。
- **风险**：高频写入；用 `sql.WAL` + 批写入。

#### OPS-OBS-HIST-03：Timeline RPC + 前端

- **目标**：
  - 后端 `GetTeamRunObservatoryTimeline` RPC（见 §3.4）。
  - 前端 `OrchestrationActivityTimeline.vue` + Observatory 新 Tab "Activity"。
- **验收**：进入 Run 后看到时间线；过滤节点；点击行可跳工具卡片。

### 6.3 Phase 6 · 默认 Graph + Spec v2 + Checkpoint（P0）— ✅ 已完成

#### TG-RT-DEFAULT：新 Team 默认 Graph

- **目标**：`internal/biz/team_usecase.go` 创建 Team 时若用户未指定 `runtime_engine`，自动写 `"graph"`；前端 wizard 文案说明。
- **代码锚点**：`team_usecase.go` 创建分支 · `TeamCreateDialog.vue`。
- **验收**：新 Team 默认走 Graph；旧 Team 不动。

#### TG-RT-ENV-DEFAULT：env 默认开

- **目标**：`internal/team/graph_runtime.go.envTeamGraphRuntimeGate` 默认返回 `true`，env=0 才关。
- **风险**：必须在 TG-RT-PARITY + metrics 健康观察期 1 周后才能合并；做一次 release note 通告。

#### TG-CMP-V2：OrchestrationSpec v2 类型

- **目标**：
  - Proto 新增 `OrchestrationSpec` 顶层消息（§3.4）。
  - `internal/biz/team_types.go` 增加 `OrchestrationSpec` 结构（与 Definition 并存，转换函数双向）。
  - 前端 `types.ts` 对齐。
- **验收**：Service `GetTeam` 同时返回 `orchestration_spec` + `definition_json`；保存时优先 v2，向后写入 v1。

#### TG-RT-CHECKPOINT：Team Graph Run Checkpoint

- **目标**：在 `adapter.BuildTeamGraphRoot` 传入 `EnableCheckpoint`；服务层暴露 `ResumeTeamRunExecution`（包装 GraphService Resume）。
- **代码锚点**：`internal/graph/adapter/team_graph_root.go` · `internal/service/team.go`。
- **验收**：长时间 Team Run 进程重启后能继续；前端 `TeamRunsDialog` 显示 "可恢复" 状态。

#### TG-RT-HITL：Team Graph Run InterruptBefore/After

- **目标**：OrchestrationSpec embedded graph 节点配置 `interrupt_before` 后，编译期传入；Run 命中后投影为 `waiting_review`；前端 Observatory HITL Tab 可恢复（复用现有 `useGraphRunHitl.ts`）。
- **风险**：与 Chat AwaitUserReply 共用 UX 时避免双弹窗；Observatory HITL Tab 是唯一入口。

### 6.4 Phase 6b · 容错完整化（P1）— 🟡 部分完成

#### FP-01：fallback_agent UI + 事件

- **目标**：前端 Team Editor 节点配置「失败时切到 Agent」下拉；后端 envelope `agent_failover` 在节点 fallback 时发射；Observatory Kanban 显示切换记录。
- **代码锚点**：`internal/graph/trpc/failure_recovery.go` · `internal/biz/orchestration_status.go`。

#### FP-02：Circuit Breaker

- **目标**：
  - `internal/graph/trpc/circuit_breaker.go`（新建）：节点级状态机（closed → half-open → open）。
  - GraphBuildConfig 增加 `CircuitBreakerPolicy { Threshold, CooldownSec }`。
  - 编译期 Apply；运行时连续失败到阈值时切 open，envelope `circuit_opened`。
- **代码锚点**：`internal/biz/failure_policy.go` · `internal/graph/trpc/builder.go`。
- **验收**：单测：5 次连续失败 → open；cooldown 后 half-open；成功 1 次回 closed。

#### FP-03：HITL 错误接管

- **目标**：节点失败且 `failure_policy.on_error="await_review"` 时进入 `waiting_review`；Observatory 弹 banner，提供：
  - "重试"（重新跑节点）
  - "切 fallback"（按配置）
  - "编辑输入后继续"（state_patch_json）
  - "终止" 
- **代码锚点**：`internal/team/status_projector.go` · 前端 `OrchestrationFailureBanner.vue`（新建）。

#### FP-04：死信表

- **目标**：失败超过 retry 且 policy=halt 的 Job 写入 `task_dead_letters`；Web 后台任务面板新增 "失败队列" 标签。
- **依赖**：见 [`m55 chat-channel blueprint §6.4`](../55-chat-channel-cursor-solution.md#9-附录企业级蓝图与-ai-落地指南) Background Jobs 面板。

### 6.5 Phase 6c · Graph 编辑器属性面板（P1）— ✅ 已完成

#### G-RETRY：RetryPolicy 属性面板

- **目标**：`GraphPropertyPanel.vue` 添加「重试」分区：`max_attempts` + `backoff_strategy`。
- **代码锚点**：`web/src/components/graph/GraphPropertyPanel.vue` · `web/src/features/graph/types.ts`。

#### G-GOTO：Destinations 属性面板

- **目标**：节点属性中可编辑 `destinations[]`（多选其他节点 id），保存后编译期传 `WithEndsMap`。
- **风险**：避免与 conditional_edges 冲突；UI 应在节点类型为 router/agent 时显示。

#### G-AGENT-MAP：Mapper 属性面板

- **目标**：节点级 Mapper（Input/Output JSON 编辑）+ JSON 校验；与 `validator.go` 的 `invalid_mapper_json` 错误联动。

### 6.6 Phase 7 · 单链终态（P2）— ✅ 已完成

#### TG-RT-RETIRE：移除 Native 主路径

- **目标**：
  - `internal/team/trpc_build.go` 的 `BuildTRPCTeam` 标记 deprecated；
  - `runner_team_trpc.go` 默认走 Graph；仅当 `ARANEA_TEAM_NATIVE=1` 时走 Native；
  - 删除六模式分支的特殊代码（保留为最终 fallback 一行调用）。
- **代码锚点**：`runner_team_trpc.go:198-211` 区段。
- **验收**：`make test` 全绿；FlowLog 无 `native_fallback` 事件。

#### TG-RT-TASK：Team 编译支持 Task / review 节点

- **目标**：OrchestrationSpec 节点 type 增加 `task`（对应 Graph TaskUsecase + Kanban），用户在 Team 内画"审核任务"节点。
- **依赖**：M54 Hermes Kanban Phase 3 已落地的 `kanban_*` tools。

#### TG-RT-SUBGRAPH：Team 嵌套子图

- **目标**：节点 type `subgraph`，引用另一个 Team / Graph，运行时复用 Mapper 实现父子状态隔离。
- **风险**：循环检测必须在编译期做（`validator.go` 扩展）。

#### TG-0-ARCH / TG-11-SYNC：文档与系统框图

- **目标**：更新 [`0 系统框图.md`](../需求/0%20系统框图.md) Team 执行路径 + [`11 multi-agent.md`](../需求/11%20multi-agent.md) §运行时章节标注 Native 退役。

### 6.7 Phase X · 跨域 Trace 统一（P1，可并行）— 📋 待实施

#### OPS-TRACE-01：trace_id 持久化

- **目标**：`team_runs.trace_id` 字段；Chat Turn 启动 Team 时把 chat run 的 trace_id 写入 team_run；Graph Execution 同步。
- **代码锚点**：`internal/service/chat_native.go`（team 分支）· `internal/team/runner_team_trpc.go`。
- **验收**：DB 查询 `team_runs WHERE trace_id = ?` 可拉出全链路。

#### OPS-TRACE-02：Cross-Team Trace 视图

- **目标**：Monitor Trace UI 增加 "Cross-Team / Cross-Channel" tree 视图，按 trace_id 聚合多个 team_run / graph_execution / channel_turn_job。
- **依赖**：[`m55 chat-channel blueprint`](../55-chat-channel-cursor-solution.md#9-附录企业级蓝图与-ai-落地指南) §Background Job 面板共建。

### 6.8 Phase 8 · 架构优化 — ✅ 8.1–8.8 已完成 / 📋 8.9 待实施

> Phase 8 详细任务见 [53-team-graph-orchestration-development.md §Phase 8](./53-team-graph-orchestration-development.md)。

**已完成（8.1–8.8）**：

| ID | 任务 | 状态 |
|----|------|------|
| BL-07 | TeamRunStatus 常量统一 + 状态机 | ✅ |
| BL-06 | DefinitionJSON 一次解析 | ✅ |
| BL-03 | OrchestrationControlTool 协议化 | ✅ |
| BL-04a | HITL 超时语义拆分 | ✅ |
| BL-01a–d | 单轨化（fallback/graph_runtime/canary/compiler 简化） | ✅ |
| BL-10a–b | 编译器统一走 compileFromEmbeddedGraph + 条件边 | ✅ |
| BL-02 | 模板注册表 + 5 个内置模板 | ✅ |
| BL-04b | team_graph_sessions 持久化 + 进程重启恢复 | ✅ |
| REFACTOR | God Function 拆分 | ✅ |
| TG-Q-04~25 | Review 修复 + 配置化 + 错误规范化 | ✅ |

**待实施（8.9）**：

| ID | 任务 | 优先级 | 说明 |
|----|------|--------|------|
| BL-05 | Step 持久化事件驱动统一：删除 bulk persist 路径 | 中 | 依赖 Native 完全退役 |
| BL-09 | Observer 单订阅化 | 低 | 当前 4 个订阅开销可接受，建议 6+ 时再实施 |
| FP-02 | Circuit Breaker 实现 | P1 | 类型预留，`circuit_breaker.go` 待创建 |
| FP-04 | 死信表 | P2 | 依赖 M55 Background Job 面板 |
| OPS-TRACE-01 | trace_id 持久化 | P1 | `team_runs.trace_id` 字段 |

---

## 7. 验证矩阵

### 7.1 CI 必跑

```bash
make wire && make wire-clean && make api && make build && make test && make lint && make runtime-boundary
cd web && pnpm i && pnpm lint && pnpm test && pnpm build
```

### 7.2 单测与集成测试

| 测试 | 位置 | 验证 |
|------|------|------|
| `TestParityNativeVsGraph_<mode>` | `internal/team/parity_test.go` | 六模式 parity |
| `TestActivityHistoryProjection` | `internal/team/status_projector_test.go` | history capped + ordering |
| `TestOrchestrationStepsFlush` | `internal/biz/orchestration_step_test.go` | 持久化 + 批 flush |
| `TestCircuitBreakerStateMachine` | `internal/graph/trpc/circuit_breaker_test.go` | closed/half-open/open |
| `TestFallbackAgentEnvelope` | `internal/graph/trpc/failure_recovery_test.go` | envelope agent_failover |
| `TestSpecV2RoundTrip` | `internal/biz/orchestration_spec_test.go` | v1 ↔ v2 双向转换 |
| `TestRaw MergeParseDefinition` | `web/src/features/teams/teamUtils.spec.ts` | 未知字段保留 |
| `TestTeamRuntimeEngineSelector` | `web/src/components/teams/TeamEditDialog.spec.ts` | admin 权限切换 |
| `TestObservatoryReadonlyGraph` | `web/src/pages/TeamRunObservatoryPage.spec.ts` | Run 中画布不可拖 |

### 7.3 E2E（手工 / Playwright）

| ID | 场景 | 通过条件 |
|----|------|---------|
| M53-PARITY-01 | Sequential 5 成员，Native 与 Graph 输出相等 | parity report 通过 |
| M53-OBS-01 | 5 工具调用 / Agent，Kanban Activity Timeline 全部可见 | history 5 条都在 |
| M53-FP-01 | Agent 配 fallback_agent，主 fail 时切到备 | UI 显示切换；envelope `agent_failover` |
| M53-FP-02 | Circuit Breaker 阈值 3 | 3 次失败后节点冻结 + banner |
| M53-HITL-01 | InterruptBefore 节点 → 等审核 → 继续 | Banner + Resume 成功 |
| M53-RT-01 | Team Run 进程重启 | Checkpoint 恢复后继续 |
| M53-TRACE-01 | Chat → Team Run → 子 Graph Execution | Monitor Trace tree 显示 3 层 |

### 7.4 性能基线

| 指标 | 目标 | 测试 |
|------|------|------|
| Team Run 首字节（5 成员 sequential） | < 5s | E2E 计时 |
| Observatory 首屏（30 节点）| < 500ms | Lighthouse |
| Activity Timeline 渲染（100 条） | < 100ms | Vue devtools |
| 状态投影延迟（envelope → chip） | < 50ms | DevTools timeline |
| Graph parity 测试套件 | < 5min | CI |

---

## 8. 风险与回滚

| 风险 | 影响 | 缓解 |
|------|------|------|
| Graph 路径 silent fallback Native | 用户以为 Graph 在跑 | metrics + FlowLog warn；strict 模式可选（失败即报错） |
| `parseDefinition` raw merge 破坏旧 Team | 加载 / 保存异常 | 单测覆盖；feature flag 灰度 |
| `orchestration_steps` 高频写入 | DB 锁 / 拖慢 | 异步批 flush + SQLite WAL；可关闭（env `ARANEA_OBS_PERSIST=0`） |
| Circuit Breaker 误触发 | 正常节点被冻结 | 默认阈值高（10 失败）+ half-open 自恢复 |
| Checkpoint 兼容性 | 升级后旧 Run 无法 resume | nullable schema；resume 失败 graceful 报错 |
| Spec v2 切换 | 老客户端编辑后丢字段 | 双协议并存 1 个 sprint；Service 同时写 v1+v2 |
| Cross-Team trace 拖慢 | Monitor 慢查询 | trace_id 加索引；按时间分区 |

**回滚剧本**：

- Native fallback 兜底：env `ARANEA_TEAM_GRAPH_RUNTIME=0` 立即关 Graph。
- Activity 持久化关闭：env `ARANEA_OBS_PERSIST=0`；Observatory 退化到内存模式。
- Spec v2 暂停：Service 优先读 v1；前端切回 raw JSON 编辑。
- Circuit Breaker 全关：FailurePolicy.circuit 不传即不启用。

---

## 9. 文档同步清单

| 文档 | 更新内容 | 时机 | 状态 |
|------|---------|------|------|
| [`53-team-graph-orchestration-development.md`](./53-team-graph-orchestration-development.md) §8 终态路线图 | Phase 5/6/7/8 任务 ID 状态 | 每 Phase 完成 | ✅ 已同步 |
| [`36-graph-development.md`](./36-graph-development.md) | G-RETRY / G-GOTO / G-AGENT-MAP 状态 | Phase 6c | ✅ |
| [`11-multi-agent-development.md`](./11-multi-agent-development.md) | Native 退役标注 | Phase 7 | ✅ |
| [`0 系统框图.md`](../需求/0%20系统框图.md) | Team 执行路径单链 | Phase 7 | ✅ |
| [`51 消息机制.md`](../需求/51%20消息机制.md) §Envelope 类型 | 新增 `orchestration_activity` / `agent_failover` / `circuit_opened` | Phase 5b / 6b | 🟡 部分同步 |
| [`execution-plan.md`](../guides/execution-plan.md) §迭代 TG | 当前 Sprint 任务卡 | 每周 | ✅ |

---

## 10. AI 编码代理执行守则

> 在执行 §6.X 任务前 **必读**。

1. **CodeGraph 优先**：找符号、调用链、影响面用 `codegraph_*`；不要 grep 扫符号。
2. **一卡一 PR**：每个任务（TG-RT-* / OPS-* / FP-* / G-*）一个 PR；跨 Phase 不合并。
3. **测试先行**：每个任务必带单测或集成测试；新增 Envelope 字段必须有 round-trip 测试。
4. **架构红线**：`make runtime-boundary` 必跑；biz import trpc-agent-go 立即拒绝。
5. **wire 三步**：Schema 改动 → `make wire && make wire-clean && make api`；遗漏不可提交。
6. **前端守则**：
   - 颜色 / 字号 / 间距用 `var(--*)`，遵循 `.cursor/rules/glass-dialog.mdc` 与 `frontend-ux.mdc`；
   - `parseDefinition` 类工具改动必带单测覆盖未知字段保留；
   - readonly 模式必须真正禁用交互（不靠 CSS pointer-events 单点）。
7. **FlowLog 必带**：每个新分支用 `event.NewFlowLogger` / `event.CtxFlowLogWarn`；禁止 slog。
8. **指标必带**：新增运行时分支必带 Prometheus counter（参考 `metrics.TeamGraphRuntimeTotal`）。
9. **错误分类**：用户可感知错误必经 `TurnError(code, msg)` / `kerrors.*`，禁止裸 `errors.New`。
10. **Native 保留期**：在 Phase 7 RETIRE 前，**任何 Native 路径修改**必须同步 Graph 路径，避免行为漂移。
11. **跨蓝图协调**：本文与 [m55 chat-channel blueprint](../55-chat-channel-cursor-solution.md#9-附录企业级蓝图与-ai-落地指南) 共用 `trace_id` / Background Job 面板 / Channel async 编译链；改任一处先看另一份。

---

## 11. 速查卡（执行顺序）

```
✅ Phase 5  parity + UI + metrics + Canary — 已完成
✅ Phase 5b Activity History + 持久化 — 已完成
✅ Phase 6  默认 Graph + Checkpoint/HITL + Spec v2 — 已完成
🟡 Phase 6b FP-01 fallback ✅ / FP-02 Circuit Breaker 📋 / FP-03 HITL 接管 ✅ / FP-04 死信 📋
✅ Phase 6c G-RETRY / G-GOTO / G-AGENT-MAP 前端面板 — 已完成
✅ Phase 7  TG-RT-RETIRE / TASK / SUBGRAPH / 文档 — 已完成
✅ Phase 8.1–8.8 架构优化 — 已完成
📋 Phase 8.9 BL-05 / BL-09 / FP-02 / FP-04 / OPS-TRACE-01 — 待实施
```

---

## 12. 与 M55（Chat × Channel）蓝图的协同

> 两份蓝图共用底层契约；下表展示交叉关系。

| 共用机制 | 在 M55 中的作用 | 在 M53+ 中的作用 |
|---------|----------------|------------------|
| `trace_id`（顶层 Envelope） | 跨 Chat Turn / Channel Job 关联 | 跨 Team Run / Graph Execution 关联 |
| Background Job 面板 | Channel async / Chat 长任务列表 | Team async / Graph Run 列表共用 |
| `session_revision` | Chat Web 增量同步 | Team Run UI 同样使用（Observatory 数据来自 Session） |
| FailurePolicy / TurnError 模型 | Channel IM 错误文案 | Team / Graph 错误接管 banner |
| Checkpoint / Resume | Chat 长任务 24h（Phase F） | Team Graph Run resume（Phase 6） |
| HITL UX | Chat AwaitUserReply 复用 | Observatory HITL Tab 复用 |

**协同执行原则**：M55 Phase B（`session_revision`）+ M53 Phase 5b（`trace_id`）应在同一 sprint 完成，因为 envelope 协议改一处其他必须跟上。

---

> **执行守则一句话**：每个 PR 都要回答三个问题——
> 1. 这改变了哪一类问题（T-1…T-6）？  
> 2. 它符合 §3 架构契约的哪一条？  
> 3. 它带了 §7 验证矩阵里的哪一个测试？

如果 PR 描述里没有这三段答案，AI 编码代理应自我拒绝并补齐后再提交。
