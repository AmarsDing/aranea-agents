# M53: Team × Graph 编排融合 — 实现设计

> 对应需求：[53-team-graph-orchestration.md](./53-team-graph-orchestration.md)
> 遵循：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)
> **实现差距与迭代计划**以 [53-team-graph-orchestration.development.md](./53-team-graph-orchestration.development.md) 为准

---

## 一、模块概述

### 1.1 设计定位

**OrchestrationSpec** 为 Team 与 Graph 的统一编排真相源：

- **Team 简视图**：mode + members + 参数 → 编译器生成 graph 拓扑
- **Graph 高级视图**：Vue Flow 自由编辑，可 `linked_graph_id` 引用
- **运行态**：ActivityProjector 将 `ActivityEvent`（`biz.ActivityEvent`，Domain=chat）投影为 `AgentNodeState`；Kanban 与 Graph 共用。原 `StatusProjector` 投影异构 EnvelopeType 的实现已替换（详见 ADR-03）。

**终态（执行单链）**：所有 Team Run 经 `CompileToGraphRuntimeConfig` → `GraphAgent` 执行；Native 路径已移除，`ARANEA_TEAM_GRAPH_RUNTIME=0` 为 Graph 执行熔断开关。

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
  orchestration_status.go              ← AgentNodeStatus + ApplyActivityEvent（纯领域，无 trpc）
  team_usecase.go                      ← HasActiveRun 锁定校验（Phase 1）
        ↓
internal/team/
  graph_compile.go                     ← CompileToGraphBuildConfig（Phase 2）
  status_projector.go                  ← 订阅 ActivityEventBus → 投影 ActivityEvent → WS（Phase 0.5；已重构为 ActivityProjector）
  runner_team_trpc.go                  ← 启动/停止投影器（含 buildTeamProjectMeta 填充 SpiritSessionID）
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
| `internal/team` | 新增/修改 | 编译器、ActivityProjector（原 StatusProjector）、Runner 挂钩、Coordinator、模板注册表 |
| `internal/service` | 新增 | Observatory RPC、Run 锁定 |
| `internal/event` | 扩展 | `orchestration_agent_status` 改为 `ActivityKind=team_stage`（stage=agent_status），通过 `ActivityEventBus` 传输（Domain=chat）；详见 ADR-03 |
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

用于将 `member_*` ActivityEvent（`ActivityKind=reply`，带 `agent_key`）的 author/agent_key 映射到 graph 节点。

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

实现：`OrchestrationStatusStore.ApplyActivityEvent` 纯领域方法（原 `ApplyEnvelope`，已重构为接收 `ActivityEvent`），便于单测。

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

## 四、StatusProjector（已重构为 ActivityProjector）

> **架构变更**：原 `StatusProjector` 投影异构 `EnvelopeType` 已替换为 `ActivityProjector`，订阅 `ActivityEventBus` 上的 `biz.ActivityEvent`（Domain=chat）并投影为 `AgentNodeState`。详见 ADR-03。

### 4.1 职责（单一）

订阅 Session 级 `ActivityEventBus`，将 `ActivityEvent` **归约**为 `AgentNodeState`，发布 `ActivityKind=team_stage`（stage=agent_status）的 `ActivityEvent`。

**不负责**：持久化（由 `ActivityStepFlusher` 异步批 flush 到 `orchestration_steps` 表）、Graph 构建、Team Run 生命周期。

**设计要点**：`activity_history[]`（上限 20 条）+ `current_activity`；`ActivityStepFlusher` 异步批 flush。

### 4.2 事件映射

| 当前 `ActivityKind`（legacy EnvelopeType） | 状态更新 |
|--------------|----------|
| `team_stage`（step_started） / `team_step_started` | → `running`, phase=`doing` |
| `reply`（streaming，agent_key=member） / `member_message_start` | → `thinking` |
| `action`（tool） / `tool_call` | → `tool_running`, 记录 CurrentActivity |
| `action`（tool result） / `tool_result` | 完成 Activity；若仍 streaming → `thinking` |
| `reply`（completed） / `member_message_done` | → `success`, phase=`delivered` |
| `team_stage`（step_finished） / `team_step_finished` | step.status → success/failed |
| `graph_stage`（node_start） / `graph_node_start` | node_id → `running` |
| `graph_stage`（node_end） / `graph_node_end` | → `success` |
| `graph_stage`（node_error） / `graph_node_error` | → `failed` |
| `team_stage`（transfer） / `transfer` | 源 idle, 目标 `transferring`→`running` |
| `team_stage`（checkpoint） / `checkpoint` | → `waiting_input` |
| `session`（cancelled） / `run_status` (cancelled) | 全部活跃 → `cancelled` |

### 4.3 输出 ActivityEvent

```go
// 投影后发射 ActivityEvent（替代原 NewEnvelope）
evt := biz.ActivityEvent{
    Event: biz.ActivityEventUpdated,  // 状态更新事件
    Activity: biz.Activity{
        Kind:      biz.ActivityKindTeamStage,
        Stage:     "agent_status",
        SessionID: sessionID,
        TeamID:    teamID,
        AgentKey:  state.AgentKey(),
        Meta: map[string]any{
            "run_id":           runID,
            "node_id":          state.NodeID,
            "agent_id":         state.AgentID,
            "status":           string(state.Status),
            "display_status":   state.DisplayStatus,
            "phase":            string(state.Phase),
            "input_preview":    state.InputPreview,
            "output_preview":   state.OutputPreview,
            "current_activity": state.CurrentActivity,
            "retry_count":      state.RetryCount,
        },
    },
    Domain: biz.ActivityDomainChat,  // 持久化到 activities 表 + WS 推送
}
activityEventBus.Publish(ctx, evt)  // FilterKey 由前端按 runID/nodeID 过滤
```

### 4.4 生命周期

```go
// internal/team/status_projector.go
func StartOrchestrationStatusProjector(
    ctx context.Context,
    activityBus *activityevent.Bus,  // 原 event.Bus → ActivityEventBus
    cfg OrchestrationProjectorConfig,
) context.CancelFunc
```

由 `runner_team_trpc.runTeamTRPC` 在 `team_stage`（stage=run_started）之后启动，`defer cancel()` 于 Run 结束。

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

- runtime_engine 切换（graph / native，**仅平台管理员调试入口**；Team 编辑器已移除该选项，见 §十二 ADR-08 A3）
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

首屏 REST + WS `ActivityKind=team_stage`（stage=agent_status）增量（经 `ActivityEventBus` 推送）。

**RPC 契约**：`GetTeamRunObservatory` RPC + `GetTeamRunObservatoryTimeline` RPC（Activity 时间线）。

---

## 九、与关联模块

| 模块 | 关系 |
|------|------|
| 11 Team | Definition、Run、`team_stage`/`reply` ActivityEvent 事件源 |
| 36 Graph | GraphAgent、`graph_stage` ActivityEvent、Checkpoint |
| 51 Message | `ActivityEvent` 通道（Domain=chat）；`orchestration_agent_status` → `ActivityKind=team_stage` 路由 team/graph（详见 ADR-03） |
| 10 Session | Session 父子树：Team Run 创建 `session_type=team` Session 挂到 Spirit 根（详见 [10-session.design.md §3.6](./10-session.design.md#36-session-父子树--activity-模型activity-first-重构核心)） |
| 52 FlowLogger | `domain=team|graph` span 与 status 对齐（监控事件走 `MonitorEventBus`） |
| 17 Channel | Phase 4：`async_graph_id` 与编译路径统一 |

---

## 十、测试策略

| 层 | 文件 | 覆盖 |
|----|------|------|
| Biz | `orchestration_status_test.go` | ApplyActivityEvent（原 ApplyEnvelope）优先级、终态、transfer |
| Team | `status_projector_test.go` | ActivityEvent 序列 → WS 输出（经 ActivityEventBus） |
| Service | `team_observatory_test.go` | RPC 首屏 |
| 前端 | `agentNodeStatusStyles.test.ts` | 样式映射 |

Runner E2E：EP-TEST-TG-01（Team sequential Run + WS status 序列）。
---

## 十一、Graph Engineering 评审增强（2026-07-23 ✅ 已落地）

> 来源：Graph Engineering 评审输出的三项运行期健壮性增强，按 Phase A/B/C 以 TDD 落地。实施任务表见 [53-team-graph-orchestration.development.md Phase 9](./53-team-graph-orchestration.development.md#phase-9--graph-engineering-评审增强-已完成2026-07-23)。

### 11.1 Phase A — critic_loop 收敛语义（迭代上限接线 + loop-until-dry）

**问题**：`CriticLoop.MaxIterations` 此前未接线（死配置），critic 不批准时循环无界；反馈不再有新意见时仍机械迭代。

**设计**：

- **参数化 CondFuncRef**：`biz.CriticLoopCondFuncRefForConfig(threshold, maxIterations)` 生成 `critic_loop[@<threshold>][#<maxIterations>]`，`biz.ParseCriticLoopCondFuncRef` 解析。编译期由 `team/embedded_graph.go` 将 `critic_loop.score_threshold` / `critic_loop.max_iterations`（未配置时默认上限 3）写入 `ConditionalEdgeDef.CondFuncRef`；`graph/adapter.EnsureCriticLoopCondFuncs` 按 ref 注册参数化条件函数实例，`ResolveBuildConfig` 可解析。
- **迭代上限强制收敛**：critic 轮次 = 携带 `orchestration_control` 工具调用的消息数（`collectCriticMessages`，每轮评估一条）；达到 `maxIterations` 仍未批准 → 返回 `approved_forced` 强制收敛（Info 日志），循环有界。`approved_forced` 与真实批准的 `approved` 区分路由键，便于观测「质量收敛」与「上限兜底收敛」；PathMap 须将两键都映射到 `biz.EndNodeID`。
- **loop-until-dry 提前收敛**：最近两轮 critic 反馈的归一化内容相同且非空（`criticFeedbackDry`：小写化 + 空白折叠）→ 继续迭代无收益，提前返回 `approved`（Info 日志）。
- **判定链（显式信号优先于启发式）**：`orchestration_control` 结构化裁决（`IsApprovedDecision`：显式 action 优先于 score，`action=retry` 等非批准值不被分数推翻）→ 评审文本中/英文批准词（中文批准词带紧邻否定窗口判定，见 Phase 9.2）→ `score_threshold` 分数兜底 → 迭代上限 `approved_forced` → loop-until-dry。

**测试**：`graph/adapter/critic_loop_cond_test.go`（`MaxIterationsForcesApproval` / `MaxIterationsNotReached` / `DryConvergence` / `DryNotTriggeredWhenFeedbackDiffers`）；`biz/node_circuit_breaker_test.go`（ref 格式化/解析）；`team/graph_compile_test.go`（编译期 ref 接线）。

**运行时补全（2026-07-24，Phase 9.1）**：上述设计在 team 图（agent 节点 critic）路径下存在四个断点，修复后收敛语义才真正端到端生效：

- **终止哨兵 `biz.EndNodeID`**（`"__end__"`，镜像 `trpcgraph.End`）：`approved` 必须路由到哨兵终止图；映射到 critic 节点会构成自循环，图永不结束。`graph/trpc/validator` 将哨兵视为合法 PathMap 目标且不参与环检测。
- **轮次捕获 callback**（`graph/trpc/critic_round_capture.go`）：agent 节点输出只落 `StateKeyLastResponse` / `StateKeyNodeResponses`，不进 `StateKeyMessages`——messages 路径计不到轮次。`criticRoundCaptureCallback` 作为 AfterNodeCallback 接线到 critic_loop finish 的 agent 节点，将轮次计数与最近两轮评审文本写入 `StateKeyMetadata`。键按 critic 节点隔离（`critic_loop_rounds/<nodeID>` 等，`biz.CriticLoopMetaKeysForNode`），多 critic 图各自独立收敛；裸 key（`critic_loop_rounds` 等）仅作旧 checkpoint 回落读取。team 图未注册 metadata schema 字段，整体 map 覆写语义，失败静默（fail-open）。
- **cond func 双数据源**：轮次取 `max(metadata, messages)`；干涸比较先看 metadata 的 prev/last，再看 messages 路径；评审文本优先 `last_response`（agent 节点路径），回退 messages 末条。
- **中文批准判定**：批准词表（「批准」「评审通过」「审核通过」「结论：通过」等）与拒绝词表（「不批准」「未通过」「驳回」等），拒绝词先判（防「不批准」含「批准」误判）；裸「通过」不入词表（中文常作介词）。英文 `approved` 词界匹配 + 否定窗口判定不变。

**二次强化（2026-07-24，Phase 9.2）**：评审发现 2 个逻辑错误 + 3 个设计缺陷，逐项修复：

- **F1 中文组合式否定误判**：拒绝词表无法枚举全部组合（「不能予以通过」「不予评审通过」含批准词「予以通过」「评审通过」会误判批准）。修复：批准词逐出现位置检查紧邻前缀是否以中文否定标记结尾（`criticNegationMarkersZH`：单字「不/未/非/勿/莫」+ 组合「不予/未予/不能/未能/不可/不应/无法/难以」），同一文本第二次非否定命中仍算批准（「不予通过；修改后予以通过」）。
- **F2 分数推翻显式拒绝**：`IsApprovedDecision` 原实现允许 `action=retry` 但 `score>=threshold` 时判批准。修复：显式 action 优先——`approve` 通过、其他非空值不通过、仅 action 为空时 score 兜底。
- **F3 dry 收敛推翻显式 retry**：原判定链中 loop-until-dry 先于结构化裁决生效，连续两轮相同反馈会把显式 `retry` 裁决错误收敛为 approved。修复：结构化裁决提到最高优先级，显式 retry 不被 dry/关键词/打分推翻。
- **F4 上限收敛与真实批准不可区分**：两者同返回 `approved`，观测上无法分辨「质量收敛」与「迭代上限兜底」。修复：新增路由键 `biz.CriticLoopResultApprovedForced`（`approved_forced`），上限兜底时返回；PathMap 映射到 `EndNodeID`；上限当轮真实批准仍返回 `approved`。
- **F5 MaxSteps 截断不可观测**：框架 `maxSteps` 到顶时图静默停止，与自然完成无法区分。修复：BSP/DAG 循环返回 truncated 标记，完成事件 `CompletionMetadata.StepsTruncated` 透传，并在截断时 Warn 日志。
- **PathMap 契约校验**：外部（API/Pack）定义的 critic_loop 条件边若带 `#<maxIterations>` 但 PathMap 缺 `approved_forced`，达上限时运行时报「target node approved_forced does not exist」。`graph/trpc/validator` 新增 `critic_path_map_incomplete` 编译期错误（`validateCriticLoopPathMaps`），fail-fast 替代运行时失败。

### 11.2 Phase B — 跨团队交付物契约的运行时校验

**问题**：P1 形式契约是 dagRun 启动时的 advisory 校验（仅警告）；运行时下游团队经 `read_upstream_deliverable` 读上游产物时无契约把关——agent 传错 `team_id` 或上下游契约漂移表现为静默读到错误内容。

**设计**：工具调用级校验——reader 团队（由调用方 session 解析）声明了 `InputContract` 时，在全文提取**之前**对上游团队声明的 `Deliverables` 做 name/type/format 匹配；不匹配返回结构化 `*biz.ContractMismatchError`（含双方 teamID + 逐条 `ContractMismatch{missing|type_mismatch|format_mismatch}`），错误文案 LLM-actionable，引导 agent 自动纠正后重试。任一侧无契约声明时跳过校验（legacy 团队保持 advisory）。

详细设计与实施记录见 [1-chat.design.md §B.10.15.11](./1-chat.design.md)。

### 11.3 Phase C — 并行成员文件操作的 Worktree 隔离

**问题**：`ParallelToolExecutor` + `WorktreeIsolator`（模块 70 §八）已具备隔离执行能力，但 `ToolCall.IsolationStrategy` 依赖调用方手工标记，缺少统一分类点，文件写工具并发执行存在互踩风险。

**设计**：

- **单一打标点**：`tools.IsolationStrategyForTool(toolName)`——先经 `alias.RuntimeToolNameAliases` 归一化 UI 别名（`write_file→save_file`、`edit_file→diff_edit`），再匹配文件写工具集 `{save_file, diff_edit, patch_file, replace_content}` → `IsolationStrategyWorktree`；只读文件工具（`read_file`/`list_file`/`search_*`）与无关工具返回 `""`（直接执行）。ToolCall 构造点统一经此函数打标，分类保持一致。
- **执行路由**：`ParallelToolExecutor.executeOne` 按 `IsolationStrategy` 分发——worktree 标记走 `WorktreeIsolator`（成功合并回主仓，失败清理 worktree），空标记直接执行。
- **E2E 验证**：`TestBatchExecuteSpiritTools_ParallelWorktreeFileOps`——两个并发 `save_file` 各自在独立 worktree 提交不同文件，双双合并进主仓（首个 ff、次个 --no-ff）且 HEAD 前进。

**测试**：`tools/parallel_executor_test.go::TestIsolationStrategyForTool`（canonical/别名/只读/无关工具分类）；`tools/worktree_isolator_test.go::TestBatchExecuteSpiritTools_ParallelWorktreeFileOps`。

执行器与隔离器本体设计见 [70-orchestration-longtask-memory.design.md §八](./70-orchestration-longtask-memory.design.md)。

---

## 十二、ADR-08 团队编排统一（2026-07-25 Phase A ✅ 已落地）

> 来源：团队「编排模式」（mode + members + role 等表单字段）与 Graph 编排（nodes/edges）长期割裂——两套拓扑各自维护、互不同步，编辑器中大量字段本应由 graph 联动却需人工设置。决策详见 [ADR-08](../reports/2026-07-25-review-adr-team-orchestration-unify.md)；实施任务表见 [53-team-graph-orchestration.development.md Phase 10](./53-team-graph-orchestration.development.md#phase-10--adr-08-团队编排统一phase-a-已完成2026-07-25)。

### 12.1 核心决策

1. **embedded graph 为拓扑唯一真相源**：`definition.graph` 存在（`nodes` 非空）时，拓扑结构以 graph 为准；mode/members/role 是生成 graph 的输入，不再是独立拓扑。
2. **mode 退化为模板选择器**：选择 mode 即按模板语义生成 graph；mode 选项带模板描述（`teamConstants.modeOptions[].description`），编辑器中不再表达「运行时语义」。
3. **角色派生**：`deriveMemberRolesForMode`（`teamUtils.ts`）按 mode + 成员顺序（sort_order）自动派生启用成员 role 并回写 `synthesizer_agent_id`——sequential 全 worker；parallel 末位启用成员 synthesizer（回写 agent_id）其余 worker；coordinator 首位 coordinator 其余 worker；critic_loop 交替 generator/critic；adaptive/swarm 不派生（角色自由，仅清理残留 synthesizer_agent_id）。幂等，拓扑 watcher 内调用。
4. **runtime_engine 从 Team 编辑器移除**：统一 Graph 运行时；打开编辑器即归一 `runtime_engine='graph'` + `team_graph_runtime=true`（`useTeamsPage.openEdit`）；native 仅保留在编排页 `TeamOrchestrateRuntimePanel` 供平台管理员调试。

### 12.2 拓扑 → graph 单向派生（A1/A2）

- **拓扑指纹** `definitionTopologyKey`：仅覆盖驱动 graph 结构的字段（mode / synthesizer_agent_id / members 拓扑：agent_id·role·name·enabled·sort_order，按 sort_order 稳定排序），非拓扑字段（description / timeout / failure_policy 等）变化不触发重建。
- **watcher 链路**：拓扑指纹变化 → `deriveMemberRolesForMode`（角色派生）→ `rebuildDefinitionGraph`（本地重建，layout 未变保留存活节点 x/y 防画布漂移）→ `scheduleGraphSyncFromBackend`（后端 canonical 同步）。
- **后端 canonical 图（A2 模板去重）**：`CompileTeamGraph` RPC 响应新增 `definition_graph_json`（`team.proto`；`team/graph_compile.go` 的 `resolveDefinitionGraphSpec` / `DefinitionGraphSpecJSON`；`service/team_compile.go` 填充），前端 `definitionGraphFromCompileJSON` 应用为 definition.graph。本地 `graphUtils.buildGraphFromDefinition` 降级为离线/失败回退，前端不再持有独立的模板生成真相。

### 12.3 编辑器联动（A3）

- **派生只读**：派生模式（sequential/parallel/coordinator/critic_loop）下成员「角色」字段只读展示（`roleDerived`）；parallel 模式「汇总 Agent（派生）」只读展示并提示经成员顺序调整。
- **策略区条件显隐**：`parallel_fail` 仅 parallel 模式显示；其余失败策略字段（default/retry/circuit_breaker/on_error）为 graph 运行时全模式通用，无条件显隐需求。扩展区更名为「失败策略」（执行引擎选择器已移除，`isPlatformAdmin` prop 及 `nativeLocked` 逻辑一并清除；`TeamsPage` 同步移除死 prop 与 `useAuthStore`）。

### 12.4 校验改造（A4）

- **前后端镜像规则**：definition 携带 embedded graph（`graph.nodes.length > 0`）时，`validateTeamDefinition` 跳过 role-mode 耦合校验（角色-模式兼容 / parallel 汇总要求 / coordinator 角色要求 / critic_loop generator+critic 要求）——拓扑结构问题由 `CompileTeamGraph` 编译期校验报告（编辑器右侧编译预览面板实时展示）。
- **保留校验**（与 graph 无关的基础约束）：至少一名启用成员；启用成员 agent_id 必填。`graph.nodes` 为空数组不视为携带 graph（不跳过）。
- 后端锚点：`biz/team_usecase.go::validateTeamDefinition`（enabledCount 检查前移至 role-mode 校验之前，错误优先级与前端对齐）；前端锚点：`web/.../teams/teamUtils.ts::validateTeamDefinition`。

### 12.5 Phase B（遗留）

- mode 字段只读化（graph 完全接管后 mode 仅存展示/模板语义）
- `definitionToJSON` native 序列化分支清理
- `TeamOrchestrateRuntimePanel` native 调试入口随 native 运行时退役移除

---

## 附录：企业级蓝图与 AI 落地指南

> 原 `53-team-graph-orchestration.design.md#附录企业级蓝图与-ai-落地指南` 已并入本文。
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

- Native 主路径已移除，`ARANEA_TEAM_GRAPH_RUNTIME=0` 为 Graph 执行熔断开关。
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
| `/teams/:id/edit` | 成员、模式（模板选择器）、`failure_policy`；角色按 mode 派生只读；无 `runtime_engine`（统一 Graph，ADR-08 A3） | `TeamEditorDialog.vue` |
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
| `fallback_agent` | `node_wiring.go` Agent 节点支持 + `failure_recovery.go`；UI 编辑；ActivityEvent `team_stage` stage=agent_failover |
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
internal/event                        ← ActivityEventBus + MonitorEventBus（原 Envelope + Bus + Buffer 已删除，详见 ADR-03）
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

### 3.3 ActivityEvent 协议增量（原 Envelope 协议，已重构）

> **架构变更**：原 `Envelope` struct 与 `EnvelopeType` 常量已删除（详见 ADR-03）。Orchestration 相关事件改用 `biz.ActivityEvent` 在 `ActivityEventBus` 上传输（Domain=chat 持久化）。`TraceID` / `GraphExecutionID` / `NodeID` 等字段改由 `Activity.Meta` 承载。

```go
// 已删除：type Envelope struct { TraceID / GraphExecutionID / NodeID ... }
// 当前：ActivityEvent.Meta 承载以下字段
meta := map[string]any{
    "trace_id":           traceID,           // ★ 原 Envelope.TraceID
    "graph_execution_id": graphExecID,       // ★ 原 Envelope.GraphExecutionID
    "node_id":            nodeID,            // ★ 原 Envelope.NodeID（统一覆盖 graph_node_*）
    "run_id":             runID,
}
```

新 `ActivityKind`（替代原 `EnvelopeType` 常量）：

```go
// 已删除：EnvelopeTypeOrchestrationActivity / EnvelopeTypeAgentFailover / EnvelopeTypeCircuitOpened
// 当前映射：
//   orchestration_activity → ActivityKind=team_stage（stage=activity 起止）
//   agent_failover         → ActivityKind=team_stage（stage=agent_failover，fallback/retry 触发）
//   circuit_opened         → ActivityKind=team_stage（stage=circuit_opened，熔断）
//   graph_node_*           → ActivityKind=graph_stage（stage=node_start/node_end/node_error/node_custom）
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
      ├─ StartOrchestrationStatusProjector（ActivityProjector）
      │     ├─ reply(streaming, agent_key=member) → status=thinking
      │     ├─ action(tool_call_start)            → status=tool_running + activity push
      │     ├─ action(tool_call_end)              → activity finish
      │     ├─ reply(completed)                   → status=success
      │     └─ graph_stage(node_error)            → status=failed → FailurePolicy 决策
      ├─ RunTRPCUserTurn → ConsumeEventStream → ActivityProjector → ActivityEventBus
      │     → WS:
      │         Domain=chat（reply / team_stage / graph_stage）
      │         Domain=system（监控事件走 MonitorEventBus）
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
  → 完成 → outbound 通知 + Job done ActivityEvent（`team_stage` stage=job_done）+ Web Observatory 自动刷新
```

### 4.3 失败接管路径（FailurePolicy 完整化）

```
Agent 节点失败
  ├─ retry: 自动重试 → ActivityEvent(`team_stage` stage=agent_retry) + Kanban "retrying"
  ├─ skip: 跳过 + ActivityEvent(`team_stage` stage=node_skipped) + Graph 灰线 + ParallelFail.continue_on_failure
  ├─ fallback_agent: 切备用 → ActivityEvent(`team_stage` stage=agent_failover) + Kanban 显示新 Agent
  ├─ circuit_open（连续 N 失败）: 节点冻结 + ActivityEvent(`team_stage` stage=circuit_opened) + 全局 banner
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

- WS ActivityEvent 到达 → Kanban / Graph chip 状态 < 50ms（rAF batch）
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
> Phase 8.9 已完成：BL-05（bulk persist 退役）、BL-09（`teamRunPipeline`）、FP-02（CB + 持久化）、FP-04（死信 UI）、Swarm Graph 安全接线。
> **注意**：OPS-TRACE-01（trace_id 持久化）已实现（`team_runs.trace_id` 字段 + `UpdateTeamRunTraceID`）。

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
| `TestFallbackAgentEnvelope` | `internal/graph/trpc/failure_recovery_test.go` | ActivityEvent `team_stage` stage=agent_failover（legacy 测试名保留） |
| `TestSpecV2RoundTrip` | `internal/biz/orchestration_spec_test.go` | v1 ↔ v2 双向转换 |

> 任务级代码锚点、验收标准与状态标记详见 [53-team-graph-orchestration.development.md §3 开发阶段](./53-team-graph-orchestration.development.md#3-开发阶段)

### 7.3 E2E（手工 / Playwright）

| ID | 场景 | 通过条件 |
|----|------|---------|
| M53-PARITY-01 | Sequential 5 成员，Native 与 Graph 输出相等 | parity report 通过 |
| M53-OBS-01 | 5 工具调用 / Agent，Kanban Activity Timeline 全部可见 | history 5 条都在 |
| M53-FP-01 | Agent 配 fallback_agent，主 fail 时切到备 | UI 显示切换；ActivityEvent `team_stage` stage=agent_failover |
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
| 状态投影延迟（ActivityEvent → chip） | < 50ms | DevTools timeline |
| Graph parity 测试套件 | < 5min | CI |

---

## 8. 与 M55（Chat × Channel）蓝图的协同

> 两份蓝图共用底层契约；下表展示交叉关系。

| 共用机制 | 在 M55 中的作用 | 在 M53+ 中的作用 |
|---------|----------------|------------------|
| `trace_id`（顶层 ActivityEvent.Meta） | 跨 Chat Turn / Channel Job 关联 | 跨 Team Run / Graph Execution 关联 |
| Background Job 面板 | Channel async / Chat 长任务列表 | Team async / Graph Run 列表共用 |
| `session_revision` | Chat Web 增量同步 | Team Run UI 同样使用（Observatory 数据来自 Session） |
| FailurePolicy / TurnError 模型 | Channel IM 错误文案 | Team / Graph 错误接管 banner |
| Checkpoint / Resume | Chat 长任务 24h（Phase F） | Team Graph Run resume（Phase 6） |
| HITL UX | Chat AwaitUserReply 复用 | Observatory HITL Tab 复用 |

**协同执行原则**：M55 Phase B（`session_revision`）+ M53 Phase 5b（`trace_id`）应在同一 sprint 完成，因为 ActivityEvent 协议改一处其他必须跟上。

---

> 风险与回滚、文档同步清单、AI 编码代理执行守则、速查卡等开发流程内容详见 [53-team-graph-orchestration.development.md](./53-team-graph-orchestration.development.md)。

---

## 子模块：Team × Graph 一体化（C1 全量物化，2026-07-30）

> 来源：2026-07-30 Team 中的 Graph 与 Graph 工作流（M36）关系评审。评审结论：后端执行单链已打通（OrchestrationSpec → 编译 → GraphAgent），但资产模型、能力面、导航三面割裂——embedded graph 是「二等公民」、`graph_executions.graph_id="team:..."` 合成 ID 产生孤儿执行、linked_graph 前端零入口、Team/Graph 两套观测 UI 互不可达。
> 决策：C1 全量物化（用户已确认）；编辑语义=双路径+覆盖警告；存量迁移=L3 批量+惰性兜底。

### A. 终态定义

**Graph 资产是唯一拓扑真相源；Team 是「成员 + 派生规则 + 运行语义」的壳；一次执行，双视角观测。**

- 每个 Team 持有一个 `graph_definitions` 一等资产（`team_id` 标记属主）
- `definition_json.graph`（embedded）退役为**只读兼容**——不再写入，读取仅用于存量迁移与编译兜底
- Team 只剩 `linked_graph_id` 一种拓扑来源；编译主路径 = linked loader
- `graph_executions.graph_id` = 真实资产 ID；`team:` 合成 ID 仅存量历史保留（不迁移，快照可回放）
- Team 观测台与 Graph 运行页能力对齐（Checkpoint/HITL/Kanban），互相跳转

### B. source 三态（编辑语义）

`OrchestrationSpec` 新增 `source` 字段（`json:"source,omitempty"`），缺省按 `preset` 处理：

| source | 含义 | Team 表单行为 | Graph 编辑器行为 |
|--------|------|--------------|-----------------|
| `preset`（默认） | 表单派生 | 改 mode/members → 物化重建图（新版本，保留未变节点坐标） | 可改，保存后转 custom |
| `custom` | 拓扑被手改过 | 拓扑区显示「已自定义，改 mode/members 将覆盖」+「重置为派生」按钮 | 可改 |
| `linked_external` | 关联独立 Graph 资产（`team_id` 为空或非本 team） | 拓扑只读；成员从图 agent 节点同步 | 在该资产自己的上下文编辑 |

**覆盖警告**：source=custom 的 Team 在表单中修改 mode/members 并保存时，前端弹确认「将丢弃自定义拓扑，按模式重新派生」；确认后物化重建并转回 preset。

### C. 物化器（Materializer）

新增 `internal/team/graph_materialize.go`：

```
MaterializeTeamGraph(ctx, def Definition, existing *biz.GraphDefinition) (*biz.GraphDefinition, error)
```

- 输入：Team Definition（mode/members/failure_policy/enable_checkpoint 等）+ 现有图资产（可为 nil）
- 拓扑来源：**复用现有编译器**（`CompileToGraphBuildConfig` 同一生成器，避免两套拓扑逻辑），产出 canonical `biz.GraphBuildConfig`
- 转换为 `biz.GraphDefinition` 持久化格式（nodes/edges/conditional_edges/state_fields/entry_point/finish_point/enable_checkpoint）
- layout 保留：existing 非 nil 时，未变更节点 ID 的坐标从 `metadata.layout`（`Record<nodeID,{x,y}>`）继承；新增节点自动布局
- `metadata.team_source` = preset/custom 镜像（GraphsPage badge 辅助）
- 保存走 `GraphDefinitionUsecase.SaveDefinition` → 自动获得 `_version_history` 版本快照（上限 50），天然支持回滚

### D. Team 保存流（钩子）

`team_usecase.CreateTeam / UpdateTeam`：

1. source=preset（或缺省）→ 物化器重建 team 图资产 → definition_json 写 `linked_graph_id` + `source`，**不再写 `graph` 字段**
2. **D1：物化与 team 保存同一事务**（`Data.ExecInTx`）；物化失败（无启用成员/校验不过）→ 保存整体失败，返回具体校验错误，不静默降级
3. 受 `HasActiveRun` 运行锁定约束（既有逻辑不变）
4. source=custom 且表单改了拓扑字段 → 前端确认后才到达后端；后端按 preset 重建（前端确认责任，后端不二次拒绝）
5. 换绑 external：旧 owned 图（`team_id`=本 team）**D2：直接删除**（历史 run 靠 `definition_snapshot_json` 回放；`GraphExecutionsPage` 对悬空 graph_id 友好降级「资产已删除」）

### E. 反向同步（Graph 编辑器 → Team）

`GraphDefinitionUsecase.SaveDefinition` 保存 team-owned 图（`team_id` 非空）后：

1. 回写属主 team definition_json：`source=custom`
2. members 从图 agent 节点派生重写（提取 `NormalizeOrchestrationSpec` 的 backfill 逻辑为共享函数 `DeriveMembersFromGraphNodes`）；agent key → agent_id 反向解析经 Agent reader；解析失败的节点跳过并记 warn
3. mode 字段保留原值（仅作模板选择器语义展示）
4. 属主 team 有活跃 Run 时 → 拒绝保存（与 Team 侧锁定对称）

### F. 编译/运行路径收敛

- `graph_runtime_config.go`：linked_graph_id 存在 → loader 加载（唯一主路径）；为空（存量未迁移）→ embedded/mode 模板兼容路径 + warn 日志（迁移完成后可标记退役）
- `RegisterTeamGraphExecution`：`graph_id` = team 的 linked_graph_id（真实资产）；linked 为空时保留 `team:` 合成 ID 兜底
- Team 多次 Run 共享同一资产 ID → `/graphs/:id/executions` 自然展示该 team 的全部执行历史（一体化核心收益）

### G. 存量迁移（L3）

注册 data migration `20260730_team_graph_materialize`：

1. 扫描 `teams.definition_json` 含非空 `graph` 且 `linked_graph_id` 为空的 team
2. 逐队物化 + 回写 `linked_graph_id` + `source`（有 graph 字段的存量 team 标记为 custom——其拓扑可能已被视为自定义；纯模板生成的标记 preset。判定：`graph.nodes` 与 mode 模板重新生成结果拓扑等价 → preset，否则 custom）
3. 幂等：linked_graph_id 非空跳过；单 team 失败记 warn 继续，不阻塞启动
4. 惰性兜底：team 保存/运行时 linked 仍为空 → 先物化再继续

### H. 删除保护

| 操作 | 规则 |
|------|------|
| 删 team-owned 图（属主 team 存在） | 拒绝，提示先删 team 或换绑 |
| 删被 external 引用的独立图 | 拒绝并列出引用 team |
| 删 team | owned 图（`team_id`=本 team）级联删（既有逻辑）；external 图只解绑不删 |

### I. 前端设计

| 模块 | 改动 |
|------|------|
| TeamEditorDialog | source=custom 警告条 +「重置为派生」按钮；「关联 Graph」选择器（高级区，列出独立图资产）；`enable_checkpoint` 开关；移除三处 `enableCheckpoint: false` 硬编码（teamGraphAdapter/compileApi/useTeamRunObservatoryPage，改从编译响应/图资产读取） |
| TeamOrchestratePage | 工具栏「在 Graph 编辑器中打开」（跳 `/graphs/:id`）；校验错误接入 R2 节点联动（复用 GraphEditorCanvas 校验面板）；画布保持只读（**D3**：编辑只走表单/Graph 编辑器两个入口，不开第三入口） |
| GraphsPage | team-owned 图显示「Team 编排」badge + 属主 team 名；过滤 chips（全部/独立/Team 关联）；行内操作加「打开 Team 编排」 |
| GraphEditorPage | 保存 team-owned 图时确认提示「此图属于 Team X，保存后 Team 将标记为自定义编排」 |
| TeamRunObservatoryPage | 新增 Checkpoint tab（ListCheckpoints/GetStateSnapshot/EditState/ResumeGraph，enable_checkpoint 时启用）；工具栏「Graph 执行视角」跳转 `/graphs/:graphId/run/:execId` |
| GraphRunPage | team 执行（图 team_id 非空）显示 Kanban 视角 tab（复用 OrchestrationKanban）；悬空 graph_id 友好降级（从 definition_snapshot/执行 steps 渲染只读拓扑 +「资产已删除」提示） |

### J. 端到端数据流

- **创建**：表单 mode+members → 物化图 v1 → run → `graph_executions(graph_id=资产ID)` → Team 观测台 ↔ Graph 运行页互跳
- **自定义**：编排页 → Graph 编辑器改拓扑 → team source=custom + members 同步 → 下次 run 生效
- **回归派生**：Dialog「重置为派生」→ 确认 → 物化重建（新版本，保留坐标）→ source=preset
- **Pack**：team 导出经既有 linked graph 导出路径自动包含 owned 图（`pack/exporter.go` 已支持 linked 导出 + importer ID 映射），零新增改动

### K. 边界与错误处理

- 物化失败 → 保存失败（D1），错误含节点级校验明细
- 运行中：物化/换绑/重置/Graph 编辑器保存 team 图均拒绝（双向锁定对称）
- 成员与图节点不一致（external 图含 team 外 agent）→ 保存时校验警告，members 以图节点为准同步
- 迁移：单 team 失败不阻塞；迁移后 embedded 字段保留不物理删除（只退役写入）
- 循环关联防御：external 关联目标不得是 team-owned 图（避免 team A 的图被 team B 关联后级联删除误伤）；校验在换绑时执行

### L. 测试策略

| 层 | 测试 |
|----|------|
| biz 单测 | 物化器六 mode / 坐标保留 / source 转换 / members 派生 / 删除保护 |
| data PG 集成 | L3 迁移幂等 / preset-custom 判定 |
| service 集成 | 保存→物化→编译→执行→观测全链路；换绑/重置/双路径 |
| 前端组件 | badge / 警告条 / Checkpoint tab / 互跳 / 选择器 |
| E2E | 创建 team→运行→双视角；Graph 编辑器改拓扑→team 运行验证 |

### M. 明确不做（YAGNI）

- embedded 字段不物理删除（只退役写入）；`team:` 历史执行不迁移（快照可回放）
- 不为 team 图加 Graph 编辑器内的编辑限制（双路径+警告即可）
- 不做多 team 共享 owned 图（external linked 已覆盖共享需求）
- Team 编排页不开第三编辑入口（D3）
