# M53: Team × Graph 编排融合 — 开发计划

> **版本**：2026-07-24 | **状态**：✅ Phase 0.5–8.9 已落地（含 BL-05/BL-09、Swarm Graph 安全、CB 持久化）+ Phase 9 Graph Engineering 评审增强（critic_loop 收敛 / 运行时契约校验 / 并行文件隔离）+ Phase 9.1 critic_loop 运行时收敛修复 + Phase 9.2 收敛二次强化（F1–F6）
> **需求**：[53-team-graph-orchestration.md](./53-team-graph-orchestration.md) · **设计**：[53-team-graph-orchestration.design.md](./53-team-graph-orchestration.design.md)

---

## 1. 模块定位

Team 与 Graph 编排融合：统一 OrchestrationSpec、Agent 状态观测、Kanban 看板、编译器与 Graph 运行时收敛。

**代码锚点**：

| 层级 | 路径 | 阶段 |
|------|------|------|
| Biz 状态 | `internal/biz/orchestration_status.go` | 0.5 ✅ |
| Biz 状态机 | `internal/biz/team_run_state_machine.go` | 8.1 ✅ |
| Team 投影 | `internal/team/status_projector.go`（已重构为 ActivityProjector） | 0.5 ✅ |
| Team 编译 | `internal/team/graph_compile.go` | 2 |
| Event | `internal/event/activityevent/bus.go`（ActivityEventBus）+ `internal/biz/activity_event.go`（原 `envelope.go` 已删除，详见 ADR-03） | 0.5 ✅ |
| Service | `internal/service/team_observatory.go` | 1 |
| Graph 挂钩 | `internal/service/graph.go` | 1 |
| 前端 | `web/src/features/orchestration/` | 1 |
| Proto | `api/kratos/team/v1/team.proto` | 2 |

---

## 2. 现状评估（2026-06-06）

| 项 | 状态 | 证据 |
|----|------|------|
| Team 六种 mode 运行时 | ✅ | `internal/team/trpc_build.go`（Native 路径已移除） |
| Team 前端 graph 预览 | ✅ | Observatory / Compile 统一 `BuildCompileSnapshot` |
| Graph Vue Flow + Run 页 | ✅ | `GraphEditorPage` / `GraphRunPage` |
| Graph EventBridge | ✅ | `internal/graph/trpc/event_bridge.go` |
| ExecutionSummary | ✅ | `execution_summary.go` |
| 统一 Agent 状态 | ✅ | Phase 0.5 StatusProjector（已重构为 ActivityProjector） + ActivityHistory |
| Kanban UI | ✅ | Phase 1 Observatory 页 |
| mode→Graph 编译器 | ✅ | Phase 2 `graph_compile.go` + Phase 8 模板注册表 |
| GraphAgent 统一 Team Run | ✅ | Phase 7 Native 移除 + Phase 8 单轨化 |
| Activity 时间线 | ✅ | `activity_history[]` + `orchestration_steps` 表 + Timeline RPC + 前端 Tab |
| Graph 属性面板 | ✅ | RetryPolicy / Destinations / Mapper |
| 架构优化 | ✅ | Phase 8.1–8.8 状态机/协议化/单轨化/模板/配置化/错误规范化 |
| trace_id 持久化 | ✅ | `team_runs.trace_id` 字段 + `UpdateTeamRunTraceID` |

**离终态差距**：Circuit Breaker 实现、死信表、Step 持久化事件驱动统一、Observer 单订阅化。见 [§8.9](#phase-8-待实施)。

---

## 3. 开发阶段

### Phase 0.5 — Agent 状态模型 + StatusProjector（P0，约 1 周）

> **目标**：不切换 Team 运行时，先统一 WS 状态投影，为 Kanban/Graph 提供真相源。
> **架构演进**：原 `StatusProjector` + `EnvelopeType` 已在 ADR-03 重构为 `ActivityProjector` + `ActivityEvent`（Domain=chat）；下方任务描述为 Phase 0.5 原始计划，括号内标注当前实际实现。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| TG-OBS-01 | `biz.AgentNodeStatus` + `ApplyOrchestrationEnvelope`（已重构为 `ApplyActivityEvent`） + 单测 | `internal/biz` | 优先级/终态/transfer 单测绿 |
| TG-OBS-02 | `EnvelopeTypeOrchestrationAgentStatus` + RouteChannel（已重构为 `ActivityKind=team_stage` stage=agent_status + `ActivityEventBus` 订阅，RouteChannel 已删除） | `internal/event` | team/graph 通道路由（现走 ActivityEventBus Domain=chat） |
| TG-OBS-03 | `team.StartOrchestrationStatusProjector`（已重构为 ActivityProjector，订阅 `ActivityEventBus`） | `internal/team` | 订阅 ActivityEventBus（原 session Bus 已删除） |
| TG-OBS-04 | `runner_team_trpc` 启动/停止投影器（含 `buildTeamProjectMeta` 填充 SpiritSessionID） | `internal/team` | Run 期间有 status WS |
| TG-OBS-05 | 前端 `features/orchestration/types.ts` + 样式常量 | `web/src` | 类型与 Graph 对齐 |

**不涉及**：Proto 变更、DB 迁移、Team 运行时替换。

---

### Phase 1 — Kanban + Graph 状态 UI + Observatory RPC（P1，约 2 周）

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| TG-FE-01 | `OrchestrationKanban` + `StatusChip` | `web/src/components/orchestration` | OBS-02 |
| TG-FE-02 | `TeamRunObservatoryPage` 路由 | `web/src/pages` | Run 只读 |
| TG-FE-03 | 扩展 `GraphFlowNode` 细态 subtitle | `web/src/components/graph` | OBS-01 |
| TG-FE-04 | Kanban ↔ Graph focus 联动 | orchestration store | OBS-03 |
| TG-FE-05 | `useOrchestrationStream` 订阅 WS | `web/src/features/orchestration` | OBS-04 |
| TG-API-01 | `GetTeamRunObservatory` RPC + service | `api` · `internal/service` | 首屏 < 500ms |
| TG-API-02 | Graph Run 启动 StatusProjector（已重构为 ActivityProjector） | `internal/service/graph.go` | graph 通道 status（经 ActivityEventBus） |
| TG-API-03 | `TeamRun.definition_snapshot_json` 字段 | `data` · `team.proto` | ✅ Run 创建冻结；Observatory 读快照 |
| TG-API-04 | `HasActiveRun` 锁定 UpdateTeam | `internal/biz` | 运行中 PATCH 拒绝 |

---

### Phase 2 — 编排编译器 + 统一画布（P1，约 2 周）

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| TG-CMP-01 | `CompileToGraphBuildConfig` | `internal/team/graph_compile.go` | 六 mode 编译单测 |
| TG-CMP-02 | `CompileTeamGraph` 预览 RPC | `internal/service/team_compile.go` | ✅ |
| TG-CMP-03 | `/teams/:id/orchestrate` Vue Flow | `web/src/pages/TeamOrchestratePage.vue` | ✅ |
| TG-CMP-04 | `ExportTeamStructure` 改走编译器 | `internal/team/graph_structure.go` | ✅ |
| TG-CMP-05 | `linked_graph_id` Proto + CRUD | `team.proto` · definition_json | ✅ |

---

### Phase 3 — 运行时统一（P2，约 3 周，feature flag）✅ 已落地

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| TG-RT-01 | Team Run 可选 GraphAgent 路径 | `internal/team` · `internal/graph` | ✅ `TeamGraphRuntimeEnabled` + `SetGraphRootBuilder` |
| TG-RT-02 | `TeamRun.graph_execution_id` | `data` · `team.proto` | ✅ ent schema + `UpdateTeamRunGraphExecutionID` |
| TG-RT-03 | coordinator/swarm 编译为 Graph 边 | `graph_compile.go` | ✅ `edge_kind=transfer` |
| TG-RT-04 | Swarm runtime edge overlay | 前端 Graph | ✅ 虚线 + label transfer |

---

### Phase 4 — FailurePolicy + Task 深度集成（P2，约 2 周）✅ 已落地

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| TG-FP-01 | FailurePolicy schema + 编译 Retry | `biz` · `graph/trpc` | ✅ `ApplyFailurePolicy` + `WithRetryPolicy` |
| TG-FP-02 | fallback_agent / skip 策略 | `graph_compile` · `graph/trpc` | ✅ 编译期 skip + 运行时 recovery + ParallelFail continue |
| TG-FP-03 | Task review_required → waiting_review | StatusProjector（ActivityProjector） | ✅ `graph_task_status` ActivityEvent + `applyGraphTaskStatus` |
| TG-FP-04 | Channel async_graph 与编译路径对齐 | `internal/service/channel` | ✅ `async_team_id` + `ResolveChannelAsyncGraphTarget` + team_graph 执行 |
| TG-OPT-01 | Observatory `compiled_topology` 走后端 Compile | `team` · `service` · `web` | ✅ 移除前端假拓扑 |
| TG-OPT-02 | embedded `definition.graph` 编译 | `team/embedded_graph.go` | ✅ agent 节点/边参与 Compile |
| TG-OPT-03 | linked_graph Team Run 加载 | `graph/adapter` · `team/graph_runtime_config.go` | ✅ `GraphBuildConfigLoader` |
| TG-OPT-04 | 前端 `has_active_run` 编排只读 | `team.proto` · `TeamOrchestratePage` | ✅ banner + 保存禁用 |

---

### Phase 5 — 执行收敛准备（P2+）

> **目标**：Graph 路径在生产证明 **行为 parity** 后，扩大 rollout；补齐运维与前端配置，**不立即移除 Native**。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| TG-RT-PARITY | 六 mode Native vs Graph **对比 E2E** | `internal/team` · `internal/service` | 同输入下终态/summary 等价或 documented diff |
| TG-RT-UI | 前端 `runtime_engine` 下拉 + `parseDefinition` 保留字段 | `web` · `teamUtils.ts` | 保存 Team 不丢 `runtime_engine` / `failure_policy` |
| TG-RT-UI-RO | `GraphEditorCanvas` 原生 `readonly` | `web/src/components/graph` | 运行中编排页不可拖拽/连边 |
| TG-RT-METRICS | `graph_execution_id` 填充率 + fallback 监控 | `internal/metrics` · Grafana | Canary 仪表盘 |
| TG-RT-FLAG | `ARANEA_TEAM_GRAPH_RUNTIME` Canary → 全量 | 运维 | 文档化 rollout；默认仍 off |
| TG-OBS-HIST | Activity **时间线**（非仅 `current_activity`） | `status_projector` · Observatory | Kanban 列内多段历史 |
| TG-CMP-JOIN | embedded graph **显式 join** 参与 ParallelFail | `embedded_graph.go` · `failure_policy.go` | 减少 `parallelBranchNodeIDs` 启发式依赖 |
| G-RETRY | RetryPolicy 属性面板 + Graph 编辑器 | 36-graph · `web` | 与 Team FailurePolicy 字段对齐展示 |
| G-GOTO | Destinations / GoTo 属性面板 | 36-graph · `web` | Graph 编辑器可编辑；Team adaptive 已编译 Destinations |
| G-AGENT-MAP | Agent Input/OutputMapper 接线 | `graph/trpc/builder.go` | Subgraph 场景 Team linked_graph 可用 |

### Phase 6 — Graph 默认执行（P2+）

> **目标**：新 Team 默认走 Graph 执行；Native 仅作 **编译/构建失败 fallback**。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| TG-RT-DEFAULT | 新 Team `runtime_engine` 默认 `graph` | `definition` · 前端 | 未显式设置时走 Graph（仍受 env gate） |
| TG-RT-ENV-DEFAULT | 生产 env **默认开启** Graph gate | 运维 | 移除对 `ARANEA_TEAM_GRAPH_RUNTIME=1` 的依赖或默认 `1` |
| TG-RT-FALLBACK-ONLY | Native 路径 **仅** fallback | `runner_team_trpc.go` | 非 fallback 场景不调用 Native 构建 |
| TG-RT-DEPRECATE | Native 构建路径标记 deprecated | `trpc_build.go` | godoc + lint 告警 |
| TG-CMP-V2 | OrchestrationSpec **version 2** 类型对齐 | `web/types` · proto 文档 | `linked_graph_id` / `failure_policy` / `graph.entry_point` 一等字段 |
| TG-RT-CHECKPOINT | Team Graph Run **Checkpoint** | `graph/trpc` · `runner` | 长任务可 resume（对齐 M36） |
| TG-RT-HITL | Team Graph Run **InterruptBefore/After** | `graph/trpc` · Team 定义 | 与 Graph Run 页 HITL 一致 |

### Phase 7 — 单链终态（P3）

> **目标**：删除 Native Team 执行栈；Team = OrchestrationSpec 编辑视图，Graph = 唯一运行时。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| TG-RT-RETIRE | 移除 Native Team 主路径 | `internal/team/trpc_build.go` | ✅ Team Run 默认 GraphAgent；Native 路径已移除 |
| TG-RT-TASK | Team 编译支持 **Task / review** 节点 | `embedded_graph` · `team_graph_task_bridge` | ✅ 编译 + Task 创建；resume 待 US-06 |
| TG-RT-SUBGRAPH | Team **嵌套子图** / Router 节点 | `embedded_graph` · linked_graph | ✅ subgraph 编译 + 循环检测 |
| TG-0-ARCH | 更新 [0-system-diagram.md](./0-system-diagram.md) Team 执行路径 | 文档 | ✅ |
| TG-11-SYNC | [11-multi-agent.development.md](./11-multi-agent.development.md) 标注 Native 已移除 | 文档 | ✅ |

**跨模块（M36，不阻塞 Phase 5 但阻塞 US-06 全量）**：G1 LLM 节点 · G2 Tool 节点 · G5–G8 执行监控与校验面板 · G9–G14 见 [36-graph-workflow.development.md](./36-graph-workflow.development.md)。

---

## 4. 任务板（当前冲刺）

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | TG-OBS-01 | biz 状态归约器 | ✅ |
| 2 | TG-OBS-02 | ActivityKind（原 Envelope 类型，已重构） | ✅ |
| 3 | TG-OBS-03 | StatusProjector（已重构为 ActivityProjector） | ✅ |
| 4 | TG-OBS-04 | Runner 挂钩 | ✅ |
| 5 | TG-OBS-05 | 前端类型 | ✅ |
| 6 | TG-FE-01 | Kanban 组件 | ✅ |
| 7 | TG-API-01 | Observatory RPC | ✅ |
| 8 | TG-FE-02 | Observatory 页 + 路由 | ✅ |
| 9 | TG-FE-03 | GraphFlowNode 细态 | ✅ |
| 10 | TG-FE-05 | useOrchestrationStream | ✅ |
| 11 | TG-API-02 | Graph Run StatusProjector（ActivityProjector） | ✅ |
| 12 | TG-API-04 | HasActiveRun 锁定 | ✅ |
| 13 | TG-FE-04 | Kanban ↔ Graph 选中联动 | ✅ |
| 14 | TG-API-03 | definition_snapshot_json | ✅ |
| 14b | TG-OPT-01 | Observatory compiled_topology | ✅ |
| 14c | TG-OPT-02 | embedded definition.graph | ✅ |
| 14d | TG-OPT-03 | linked_graph Run 路径 | ✅ |
| 14e | TG-OPT-04 | 前端 has_active_run 锁 | ✅ |
| 15 | TG-CMP-01 | CompileToGraphBuildConfig | ✅ |
| 16 | TG-CMP-02 | CompileTeamGraph RPC | ✅ |
| 17 | TG-CMP-03 | TeamOrchestratePage | ✅ |
| 18 | TG-CMP-04 | ExportTeamStructure 编译器 | ✅ |
| 19 | TG-CMP-05 | linked_graph_id | ✅ |

**下一冲刺（Phase 5 入口）**：TG-RT-PARITY → TG-RT-UI → TG-RT-METRICS → TG-RT-FLAG。

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 20 | TG-RT-PARITY | Native vs Graph 对比 E2E | ✅ |
| 24 | TG-RT-FLAG | Canary percent + holdout Native | ✅ |
| 21 | TG-RT-UI | runtime_engine 前端 + 字段保留 | ✅ |
| 22 | TG-RT-UI-RO | GraphEditorCanvas readonly | ✅ |
| 23 | TG-RT-METRICS | graph_execution_id / fallback 监控 | ✅ |
| 25 | TG-OBS-HIST | Activity 时间线 | ✅ |
| 26 | TG-CMP-JOIN | embedded join + ParallelFail | ✅ |

---

## 5. 验收标准

### Phase 0.5

- [x] `go test ./internal/biz/... -run Orchestration` 通过
- [x] Team Run 时 WS 推送 `orchestration_agent_status`
- [x] `member_message_start` → status `thinking`；`tool_call` → `tool_running`；`member_message_done` → `success`
- [x] 前端 types 与 biz 枚举一致

### Phase 1

- [x] Graph 节点与 Kanban chip 状态一致（OBS-01/02）
- [x] Run 中编排页只读（OBS-06）— Observatory 页 Graph 只读 + UpdateTeam 运行中拒绝
- [x] `GET /v1/team-runs/{id}/observatory` 可用
- [x] Kanban ↔ Graph 视口 focus 联动（OBS-03）— fitView + Kanban scrollIntoView

---

## 6. 依赖与风险

| 风险 | 缓解 | 状态 |
|------|------|------|
| member_* 无 node_id，仅靠 agent_key 映射 | Run 开始时构建 Registry；约定 node_id=`member-{sort_order}` | ✅ 已解决 |
| Phase 3 切换运行时回归 | feature flag；Phase 0.5–1 双轨观测先行 | ✅ 已解决（Phase 8 单轨化） |
| Kanban 与 Chat 工具卡片重复 | 复用 ActivityMeta 结构，不 duplicate 投影逻辑 | ✅ 已解决 |
| 文档与 11/36 重复 | 53 管融合边界；11/36 管单模块，互链不复制 | ✅ 持续 |
| Graph 路径 silent fallback Native | Phase 8：`fallback_policy.go` 简化，Native 路径已移除 | ✅ 已解决 |
| `teamUtils.parseDefinition` 丢 `runtime_engine` | Phase 5 TG-RT-UI：raw merge 保留未知字段 | ✅ 已解决 |
| trace_id 跨域关联 | `team_runs.trace_id` 字段 + `UpdateTeamRunTraceID` 持久化 | ✅ 已解决 |
| Circuit Breaker 未实现 | ✅ FP-02 已落地（graph 运行时 + blocked 投影） | ✅ |

---

## 8. 终态路线图（Team 规格 + Graph 执行单链）

### 8.1 终态定义

```
OrchestrationSpec (definition_json)
        │
        ├─ CompileToGraphBuildConfig / CompileToGraphRuntimeConfig
        │       （mode 模板 · embedded graph · linked_graph_id · failure_policy）
        │
        ├─ BuildTeamGraphRoot → GraphAgent（唯一执行）
        │
        └─ ActivityProjector（原 StatusProjector） / Observatory / Channel async_team_id
                （同一编译链、同一 WS 状态模型；ActivityEvent Domain=chat 经 ActivityEventBus）
```

**不再存在**：Native 路径按 mode 分发 ChainAgent / ParallelAgent / Swarm 的 **主执行路径**。

### 8.2 已完成（一条链的完整实现）

| 能力 | 状态 | 说明 |
|------|------|------|
| 编译真相源 | ✅ | `graph_compile.go` + `embedded_graph.go` + `linked_graph_loader` + `template_registry.go` |
| Run 快照 | ✅ | `definition_snapshot_json` 冻结；Observatory 读快照 |
| 观测拓扑 | ✅ | `compiled_topology` 后端 Compile；前端不伪造 |
| FailurePolicy 编译 | ✅ | Retry / skip / fallback_agent / parallel_fail → GraphBuildConfig |
| Channel team_graph | ✅ | `CompileToGraphRuntimeConfig` 与 Chat 同编译链 |
| Graph 默认执行 | ✅ | `ARANEA_TEAM_GRAPH_RUNTIME` 默认 true |
| Native 移除 | ✅ | Native 主路径已移除；编译器统一走 `compileFromEmbeddedGraph` |
| Activity 时间线 | ✅ | `activity_history[]` + `orchestration_steps` 表 + Timeline RPC + 前端 Tab |
| Graph 属性面板 | ✅ | RetryPolicy / Destinations / Mapper |
| 架构优化 | ✅ | Phase 8.1–8.8 状态机/协议化/单轨化/模板/配置化/错误规范化 |
| trace_id 持久化 | ✅ | `team_runs.trace_id` 字段 + `UpdateTeamRunTraceID` |

### 8.3 差距清单（Phase 0.5–8.8 已解决项 + 待实施项）

#### A. 执行层（核心）— ✅ 已解决

| 差距 | 现状 | 目标 Phase |
|------|------|------------|
| ~~Team Run **默认 Native**~~ | ✅ Phase 7：Native 主路径移除，统一走 GraphAgent | Phase 6 ✅ / Phase 7 ✅ |
| ~~**双开关** gate~~ | ✅ Phase 8：`graph_runtime.go` 简化，`ARANEA_TEAM_GRAPH_RUNTIME` 默认 true | Phase 6 ✅ / Phase 8 ✅ |
| ~~**Silent fallback**~~ | ✅ Phase 8：`fallback_policy.go` 简化，Native 路径已移除 | Phase 5 ✅ / Phase 8 ✅ |
| ~~**Mode parity 未证明**~~ | ✅ Phase 5：parity E2E + run 级 summary 对比 | Phase 5 ✅ |

#### B. OrchestrationSpec 产品化 — ✅ 已解决

| 差距 | 现状 | 目标 Phase |
|------|------|------------|
| ~~`runtime_engine` 无 UI~~ | ✅ `TeamOrchestrateRuntimePanel` | Phase 5 ✅ |
| ~~前端保存 **剥离未知字段**~~ | ✅ `parseDefinition` raw merge | Phase 5 ✅ |
| ~~Spec **version 2** 未在前端建模~~ | ✅ `orchestrationSpec.ts` + `toOrchestrationSpec` / `fromOrchestrationSpec` | Phase 6 ✅ |
| ~~编排页运行中 **画布仍可拖**~~ | ✅ `GraphEditorCanvas` readOnly prop | Phase 5 ✅ |

#### C. 观测与拓扑语义 — ✅ 已解决

| 差距 | 现状 | 目标 Phase |
|------|------|------------|
| ~~Activity **仅 current_activity**~~ | ✅ `activity_history[]`（上限 20）+ `orchestration_steps` 表 + Timeline RPC + 前端 Tab | Phase 5 ✅ |
| ~~ParallelFail **启发式 join**~~ | ✅ embedded graph 显式 join + `compileEmbeddedEdges` | Phase 5 ✅ |
| ~~member_* → node_id~~ | ✅ `BuildOrchestrationRegistry` + `member-{sort_order}` 约定 | ✅ |

#### D. Graph 引擎能力（M36 × M53）— ✅ 已解决

| 差距 | 现状 | 目标 Phase |
|------|------|------------|
| ~~Checkpoint / HITL on Team Run~~ | ✅ `team_graph_run_coordinator.go` + `team_graph_sessions` 持久化 | Phase 6 ✅ / Phase 8 ✅ |
| ~~Task / review 节点进 Team 编译~~ | ✅ `embedded_graph.go` 支持 task/review 节点 | Phase 7 ✅ |
| ~~LLM / Tool 节点（G1/G2）~~ | ✅ `node_wiring.go` 支持；Team 编译仅 agent 节点（设计决策） | Phase 7 ✅ |
| ~~Destinations 编辑器（G-GOTO）~~ | ✅ `GraphPropertyPanel.vue` Destinations 多选 | Phase 6c ✅ |

#### E. 运维与退役 — ✅ 已解决

| 差距 | 现状 | 目标 Phase |
|------|------|------------|
| ~~Rollout playbook~~ | ✅ Phase 5 TG-RT-FLAG 已完成 | Phase 5 ✅ |
| ~~Native 路径退役~~ | ✅ Phase 8：Native 主路径已移除 | Phase 8 ✅ |
| ~~系统框图 / 11 文档~~ | ✅ Phase 8：编译器统一走 `compileFromEmbeddedGraph` | Phase 8 ✅ |
| ~~灰度桶 / canary~~ | ✅ Phase 8：`graph_runtime_canary.go` 简化 | Phase 8 ✅ |
| ~~Native fallback 决策树~~ | ✅ Phase 8：`fallback_policy.go` 简化 | Phase 8 ✅ |

#### F. Phase 8.9（已完成）

| 差距 | 现状 | 优先级 |
|------|------|--------|
| ~~Circuit Breaker 实现~~ | ✅ FP-02：Pre/Post + 持久化 registry | P1 ✅ |
| ~~死信表~~ | ✅ 表/Repo/API + Teams 死信 Tab | P2 ✅ |
| ~~Step 持久化事件驱动统一~~ | ✅ BL-05：Native bulk persist 已退役 | ✅ |
| ~~Observer 单订阅化~~ | ✅ BL-09：`teamRunPipeline` + status handler | ✅ |
| ~~Swarm Graph 安全~~ | ✅ RepetitiveHandoff / CrossRequest / NodeTimeout | ✅ |

### 8.4 推荐实施顺序

```
✅ Phase 5  parity + UI + metrics + Canary
✅ Phase 6  默认 Graph + Checkpoint/HITL + Spec v2
✅ Phase 7  移除 Native + Task/Subgraph + 文档/arch 图更新
✅ Phase 8.1–8.8  架构优化（状态机 / 协议化 / 单轨化 / mode→template / 配置化 / 错误规范化）
✅ Phase 8.9  BL-05 / BL-09 / FP-02 / FP-04
```

**原则**：Phase 0.5–8.9 已完成执行收敛、架构优化与剩余补全项。

### 8.5 配置速查（当前单轨期）

| 层级 | 配置 | 作用 |
|------|------|------|
| 进程 env | `ARANEA_TEAM_GRAPH_RUNTIME=0` | 平台级关闭 Graph 执行（默认开启） |
| Team JSON | `"runtime_engine":"graph"` 或 `"native"` | Team 级选择（默认 graph） |
| 验证 | Run.`graph_execution_id` 非空 | 确认走 Graph 路径 |
| FlowLog | `team.run.graph` vs `team.run.build` | 构建路径可观测 |


---

## 9. Native vs Graph Parity（TG-RT-PARITY）

> 原 `guides/tg-rt-parity-diff.md` 已并入本文。

# TG-RT-PARITY — Native vs Graph 差异说明

> **状态**：build/runtime 对齐 ✅ · run 级 team_summary 成员指纹对比 ✅ · 全 LLM E2E ⏳  
> **代码**：`internal/team/parity_test.go` · `parity_runtime_test.go` · `parity_run_test.go`

## 已覆盖（自动化）

| 检查项 | Native | Graph |
|--------|--------|-------|
| 编译 entry/finish | ✅ | ✅ |
| agent 节点数量 | ✅ | ✅ |
| member key 与 graph agent_name 对齐 | ✅ | ✅ |
| `BuildStateGraphWithAgents` + `NewGraphAgent` | N/A | ✅ |

## Step 持久化策略（BL-03 后）

| 路径 | team_run_steps 来源 |
|------|---------------------|
| **Native**（`graphExecID==""`） | 流结束后 bulk `persistStep`（每 member） |
| **Graph 首跑** | `StartGraphStepWatch` 订阅 `member_message_done` / `graph_node_end`；无 step 时 anchor fallback |
| **Graph HITL defer** | 首跑 watch 部分 step + resume `FinalizeGraphTeamRun` |
| **Graph resume** | `graphWatchStepsAndFinalize` + `PersistGraphRunStep` |

## 已知可接受 diff（蓝图 §519）

Graph 路径额外 WS ActivityEvent（Native 无）：

- `graph_stage`（stage=node_start/node_end/node_error）
- `graph_stage`（stage=execution_done）
- `team_stage`（stage=agent_status，Observatory 投影）

Native 路径额外 ActivityEvent：

- 同步 `team_stage`（stage=step_started/step_finished）bulk 序列（Graph 改为 per-node 事件驱动）

## Run 级 parity

| 检查项 | 状态 | 说明 |
|--------|------|------|
| `team_summary` 成员指纹（agent_key / tool_call_count / tokens / status） | ✅ | `TestParityRunSummary_AllModes` |
| Native vs Graph WS 独占 ActivityEvent 文档化 | ✅ | `TestParityRunEnvelopeDiff_documented`（legacy 测试名保留，验证 ActivityEvent diff） |
| `team_run.token_in/out` run 级聚合 | ✅ | Native 总量 vs Graph `enrichTeamRunMetricsFromSteps` |
| 真实 LLM stub 六 mode 执行对比 | ✅ | `TestParityRunE2E_stubStreamAllModes` |
| WS 序列 hash 逐条对比（harness） | ✅ | persistStep 双路径 hash 一致 |
| 生产 Graph event-watch WS diff | 📋 | 见 §已知可接受 diff |

Harness：`internal/team/parity_run_test.go`（fixture 级）；全 LLM E2E 待独立 PR。
---

## 7. 关联文档更新

- [11-multi-agent.md](./11-multi-agent.md) — 增加 M53 交叉引用（编排融合）
- [36-graph-workflow.md](./36-graph-workflow.md) — §0.1 增加 Team 融合路径
- [0-system-diagram.md](./0-system-diagram.md) — Team 执行路径单链
- [51-message-mechanism.md](./51-message-mechanism.md) — ActivityEvent 类型同步（原 Envelope 类型同步，详见 ADR-03）

---

## Phase 8 — 架构优化（2026-05-28 启动）

> **目标**：消除 Phase 0.5–7 遗留的架构债，统一编译路径、协议化决策、状态机显式建模。

### Phase 8.1 — 基础加固（✅ 已完成）

| ID | 任务 | BL | 影响域 | 状态 |
|----|------|-----|--------|------|
| BL-07 | `TeamRunStatus` 常量统一真相源 + 状态机 `TeamRunStateMachine` | `biz/team_types.go` · `biz/team_run_state_machine.go` | ✅ |
| BL-06 | `DefinitionJSON` 一次解析 `parseRuntimeOptions`，消除 3 次 `json.Unmarshal` | `team/graph_runtime_options.go` | ✅ |
| BL-03 | `OrchestrationControlTool` 协议化 + `ParseOrchestrationDecision` + `IsApprovedDecision` | `biz/team_types.go` `team/trpc_build.go` `graph/adapter/critic_loop_cond.go` | ✅ |
| BL-04a | HITL 超时语义拆分：`watchTimeout`(30min) vs `hitlSLATimeout`(24h) + `maxHITLSLAExtensions`(3) | `team/team_graph_run_coordinator.go` | ✅ |
| DRY | 共享类型提取到 `biz`：`OrchestrationDecision` / `OrchestrationControlToolName` / `CriticLoopCondFuncRef` / `ExtractScore` | `biz/team_types.go` | ✅ |
| CLEAN | 移除冗余别名 `teamRunStatusWaitingHuman`；删除死代码 `parseOrchestrationCheckpoint` | `team/team_graph_run_coordinator.go` `team/graph_runtime_options.go` | ✅ |
| TEST | 新增 4 个测试文件 22+ 测试用例 | `biz/team_types_test.go` `team/trpc_build_test.go` `graph/adapter/critic_loop_cond_test.go` | ✅ |

### Phase 8.2 — 单轨化 + mode→template（✅ 已完成）

| ID | 任务 | BL | 影响域 | 状态 |
|----|------|-----|--------|------|
| BL-01a | `fallback_policy.go` 简化：`DecideNativeFallback` 移除，仅保留诊断错误 | `team/fallback_policy.go` | ✅ |
| BL-01b | `graph_runtime.go` 简化：删除灰度/桶/百分比逻辑，仅保留 `ARANEA_TEAM_GRAPH_RUNTIME=0` 熔断 | `team/graph_runtime.go` | ✅ |
| BL-01c | `graph_runtime_canary.go` 简化：`TeamGraphRuntimeEnabledForTeam` 直接委托 `TeamGraphRuntimeEnabled` | `team/graph_runtime_canary.go` | ✅ |
| BL-01d | `runner_team_compiler.go` 简化：移除 Native fallback 分支，统一走 GraphAgent | `team/runner_team_compiler.go` | ✅ |
| BL-10a | 编译器统一走 `compileFromEmbeddedGraph`：当 `definition.graph` 为空时 `generateGraphSpecFromMode` 自动生成 spec | `team/graph_compile.go` | ✅ |
| BL-10b | `compileEmbeddedEdges` 新增条件边生成：critic_loop 自动生成 `ConditionalEdgeDef` | `team/embedded_graph.go` | ✅ |
| CLEAN | 删除旧 `compileEdges` / `compileEntryFinish` / `compileSequentialEdges` 等 8 个死代码函数（~170 行） | `team/graph_compile.go` | ✅ |

### Phase 8.3 — 持久化与模板（✅ 已完成）

| ID | 任务 | BL | 影响域 | 状态 |
|----|------|-----|--------|------|
| BL-02 | 模板注册表：`OrchestrationTemplate` 接口 + 5 个内置模板 | `team/template_registry.go` | ✅ |
| BL-04b | `team_graph_sessions` 持久化：新增 raw SQL DDL + Repo + 进程重启恢复 | `biz/team_types.go` `data/team_graph_session_*.go` `team/team_graph_run_coordinator.go` | ✅ |

### Phase 8.4 — 代码质量优化（✅ 已完成）

| ID | 任务 | 影响域 | 状态 |
|----|------|--------|------|
| REFACTOR | God Function 拆分：`resolveAnchorAndAttachments` + `prepareUserTurnOptions` + `finalizeTeamRun` | `team/runner_team_trpc.go` → `team/runner_team_turn.go` | ✅ |
| TG-Q-04 | `persistGraphMemberStepsFromResult` 标记为 `TestOnly` 测试辅助 | `team/runner_finish_steps_test_helpers_test.go` | ✅ |

### Phase 8.5 — Review 修复（✅ 已完成）

| ID | 任务 | 影响域 | 状态 |
|----|------|--------|------|
| TG-Q-15 | `finishRunErr` 幂等保护（终态守卫） | `team/runner_helpers.go` | ✅ |
| TG-Q-16 | 事件码语义修正（`team.session.stale_evicted` / `team.graph.resume_fail`） | `team/team_graph_run_coordinator.go` | ✅ |
| TG-Q-17 | DDL 拆分为独立 ExecContext 调用 | `data/team_graph_session_schema.go` | ✅ |
| TG-Q-18 | `templateRegistry` 添加 `sync.RWMutex` 保护 | `team/template_registry.go` | ✅ |
| TG-Q-19 | `ShouldRun` 冗余调用消除 | `team/runner_team_turn.go` | ✅ |

### Phase 8.6 — Critic Loop 优化（✅ 已完成）

| ID | 任务 | 影响域 | 状态 |
|----|------|--------|------|
| TG-Q-20 | `containsWord` 精确匹配替代 `strings.Contains("approved")` | `graph/adapter/critic_loop_cond.go` | ✅ |
| TG-Q-21 | `DefaultCriticLoopThreshold` 常量化替代硬编码 `0` | `graph/adapter/critic_loop_cond.go` + `runtime_adapter.go` | ✅ |

### Phase 8.7 — Review 修复 + 配置化 + 清理（✅ 已完成）

| ID | 任务 | 影响域 | 状态 |
|----|------|--------|------|
| TG-Q-06 | `adaptiveMaxTransferEdges` 静默裁剪添加 `FlowLog warn` | `team/graph_compile.go` | ✅ |
| TG-Q-09 | `ResumeExecution` 失败添加 run status 更新 + `evictSession` | `team/team_graph_run_coordinator.go` | ✅ |
| TG-Q-12 | 魔法常量提取为 `CoordinatorConfig` 结构体 | `team/team_graph_run_coordinator.go` + `provider.go` | ✅ |
| TG-Q-13 | `recordTeamRunUsage` 表达式优先级显式括号 | `team/usage_record.go` | ✅ |
| BL-05 | `persistNativeBulkMemberSteps` 标记 `Deprecated` | `team/runner_finish_steps.go` | ✅ |
| CLEAN | 清理 `graph_runtime_canary_test.go` 死测试代码 | `team/graph_runtime_canary_test.go` + `team/graph_runtime_test.go` | ✅ |

### Phase 8.8 — 错误处理规范化（✅ 已完成）

| ID | 任务 | 影响域 | 状态 |
|----|------|--------|------|
| TG-Q-22 | `fmt.Errorf` → `kerrors` 替换（8 处） | `team/graph_compile.go` + `team/embedded_graph.go` | ✅ |
| TG-Q-23 | 静默忽略错误添加 `FlowLog warn`（4 处关键：`CreateTaskDeadLetter` / `RollbackToBoundary` / `MarkTeamGraphInterrupt` / `BatchCreateOrchestrationSteps`） | `team/runner_helpers.go` + `team/runner_team_trpc.go` + `team/team_graph_execution_tracker.go` + `team/activity_step_flusher.go` | ✅ |
| TG-Q-24 | adaptive 裁剪告警解耦：改为基于实际边数检测而非 mode 名硬编码 | `team/graph_compile.go` | ✅ |
| TG-Q-25 | `UpdateTeamRun` 静默忽略错误全面消除（6 处） | `team/team_graph_run_coordinator.go` + `team/runner_team_turn.go` + `team/runner_helpers.go` + `team/team_graph_run_finisher.go` | ✅ |

### Phase 8.9 — 已完成

| ID | 任务 | 优先级 | 说明 |
|----|------|--------|------|
| BL-05 | Step 持久化事件驱动统一：删除 Native bulk persist 死路径 | ✅ | Graph 路径仅事件驱动 / finalize fallback |
| BL-09 | Observer 单订阅化：`teamRunPipeline` + `runEventHandler` | ✅ | `status_projector` 经 pipeline；Graph step watch 仍独立（HITL/finalize） |
| FP-02 | Circuit Breaker + `circuit_breaker_states` 持久化 | ✅ | Wire `ProvideNodeCircuitBreakerRegistry` + Pre/Post |
| FP-04 | 死信表 + UI | ✅ | Teams 运行轨迹 Dialog 死信 Tab |
| Swarm | `RepetitiveHandoff*` / `CrossRequestTransfer` / `NodeTimeout` | ✅ | Graph PreNode + session meta + Executor `WithNodeTimeout` |

### Phase 8 关键设计决策

1. **共享类型归 `biz`**：`OrchestrationDecision` / `OrchestrationControlToolName` / `CriticLoopCondFuncRef` / `ExtractScore` 统一定义在 `biz/team_types.go`，`team` 和 `graph/adapter` 包引用 `biz.*`
2. **编译器统一路径**：`compileToGraphBuildConfigWithLoader` 不再按 mode 分发，而是通过 `generateGraphSpecFromMode` 生成 embedded graph spec 后统一走 `compileFromEmbeddedGraph`
3. **Native 路径已移除**：`ARANEA_TEAM_GRAPH_RUNTIME=0` 环境变量为 Graph 执行熔断开关，灰度桶/canary/holdout/Native fallback 逻辑全部删除
4. **HITL SLA 有界延期**：`maxHITLSLAExtensions = 3`，避免无限延期
5. **CoordinatorConfig 可配置化**：`WatchTimeout` / `HITLSLATimeout` / `SessionMaxAge` / `CleanupInterval` 统一收敛到 `CoordinatorConfig` 结构体，默认值与原常量一致，支持构造后覆盖
6. **错误处理规范化**：`fmt.Errorf` 统一替换为 `kerrors.BadRequest`；所有 `_ = xxx.UpdateTeamRun` / `_ = xxx.Create` 静默忽略改为 `FlowLog warn`，确保错误可观测

---

## Phase 9 — Graph Engineering 评审增强（✅ 已完成，2026-07-23）

> 来源：Graph Engineering 评审（文章启发 + 代码评审）输出的三项运行期健壮性增强，按 Phase A/B/C 以 TDD 落地。设计详见 [53-team-graph-orchestration.design.md §十一](./53-team-graph-orchestration.design.md#十一graph-engineering-评审增强2026-07-23-已落地)。

| ID | 任务 | 影响域 | 状态 |
|----|------|--------|------|
| GE-A1 | critic_loop 迭代上限接线：`CriticLoopCondFuncRefForConfig` 参数化 ref（`critic_loop[@<threshold>][#<maxIterations>]`）+ 编译期写入 + `EnsureCriticLoopCondFuncs` 注册；未配置时默认上限 3 | `biz/team_types.go` · `team/embedded_graph.go` · `graph/adapter/critic_loop_cond.go` | ✅ |
| GE-A2 | loop-until-dry 提前收敛：连续两轮 critic 反馈归一化相同且非空 → `approved` | `graph/adapter/critic_loop_cond.go` | ✅ |
| GE-B1 | `read_upstream_deliverable` 运行时契约校验：reader `InputContract` vs 上游 `Deliverables`（name/type/format），结构化 `ContractMismatchError`（LLM-actionable，可自动纠正重试）；任一侧无契约跳过 | `biz/deliverable_contract.go` · `biz/spirit_team_usecase.go` · `tools/deliverable/upstream_reader.go` | ✅ |
| GE-C1 | `IsolationStrategyForTool` 单一打标点：文件写工具（canonical + UI 别名）→ `IsolationStrategyWorktree` | `tools/parallel_executor.go` | ✅ |
| GE-C2 | 并行文件写 E2E：两并发 `save_file` 各自 worktree 提交并合并进主仓（ff + --no-ff），HEAD 前进 | `tools/worktree_isolator_test.go` | ✅ |

**测试覆盖**：

- `graph/adapter/critic_loop_cond_test.go`：`MaxIterationsForcesApproval` / `MaxIterationsNotReached` / `DryConvergence` / `DryNotTriggeredWhenFeedbackDiffers`
- `biz/node_circuit_breaker_test.go`：`TestCriticLoopCondFuncRefForConfig` / `TestParseCriticLoopCondFuncRef`
- `team/graph_compile_test.go`：编译期参数化 ref 接线
- `biz/spirit_team_deliverable_test.go`：Phase B——契约匹配放行 / 三类不匹配聚合为单个结构化错误 / 无契约跳过
- `tools/parallel_executor_test.go`：`TestIsolationStrategyForTool`（canonical/别名/只读/无关分类）
- `tools/worktree_isolator_test.go`：`TestBatchExecuteSpiritTools_ParallelWorktreeFileOps`

**验证证据（2026-07-23）**：`go build ./...` exit 0；`go test -count=1`——`internal/tools` / `internal/biz` / `internal/graph` / `internal/team` / `internal/service` 全 PASS（service 包一例 flaky panic 经重跑确认与本次改动无关，为既有测试间污染）。

**关联文档同步**：[1-chat.design.md §B.10.15.11](./1-chat.design.md)（Phase B 实施记录）、[70-orchestration-longtask-memory.design.md §8.2](./70-orchestration-longtask-memory.design.md)（Phase C 打标点说明）。

---

## Phase 9.1 — critic_loop 运行时收敛修复（✅ 已完成，2026-07-24）

> 运行时验证发现 GE-A1/A2 在 **team 图（agent 节点 critic）** 路径下不生效：max_iterations=2 实际循环 24+ 次不收敛。四个根因联动修复。

| # | 根因 | 修复 | 影响域 | 状态 |
|---|------|------|--------|------|
| FIX-1 | 条件边 `PathMap["approved"]` 映射到 critic 节点自身 → 自循环，图永不终止 | 新增终止哨兵 `biz.EndNodeID`（镜像 trpcgraph.End `__end__`），`approved` 路由到 `__end__`；`validator` 允许哨兵为目标且不参与环检测 | `biz/graph.go` · `team/embedded_graph.go` · `graph/trpc/validator.go` | ✅ |
| FIX-2 | agent 节点 critic 输出只进 `last_response`/`node_responses`，不进 `messages` → cond func 计不到轮次 | 新增 `criticRoundCaptureCallback`（AfterNodeCallback）：将轮次计数 + 最近两轮评审文本写入 state metadata（`critic_loop_rounds` / `*_last_response` / `*_prev_response`），接线到 critic_loop finish 的 agent 节点 | `graph/trpc/critic_round_capture.go` · `graph/trpc/node_wiring.go` · `biz/team_types.go` | ✅ |
| FIX-3 | cond func 只读 messages 路径，metadata 轮次未消费 | cond func 优先读 metadata 轮次/反馈（与 messages 路径取 max），评审文本优先 `last_response` | `graph/adapter/critic_loop_cond.go` | ✅ |
| FIX-4 | 批准判定仅英文 `approved`，中文评审永远 retry 至上限 | 新增中文批准/拒绝词表（拒绝词先判，防「不批准」误判）；裸「通过」不入词表（中文常作介词，误报率高） | `graph/adapter/critic_loop_cond.go` | ✅ |

**测试覆盖**：

- `graph/trpc/critic_round_capture_test.go`（新增 6 例）：finish agent 节点 ID 筛选 / 首轮与次轮 prev 移位 / float64 容错（checkpoint JSON 往返）/ fail-open（nodeErr、非 State 结果、空响应）
- `graph/adapter/critic_loop_cond_test.go`（新增 8 例）：metadata 轮次上限与未达上限 / metadata 干涸收敛与反馈变化 / 中文批准词、拒绝词、介词「通过」防误判 / `last_response` 优先级
- `team/graph_compile_test.go`：两个 critic_loop 编译测试新增 `PathMap["approved"] == biz.EndNodeID` 回归断言

**验证证据（2026-07-24）**：

- `go build ./...` exit 0；`go test -count=1`——`internal/graph/adapter` / `internal/graph/trpc` / `internal/team` / `internal/service` / `internal/biz` / `internal/agent` / `internal/data` 全 PASS
- 运行时：团队 `ge_review_critic_loop_test`（max_iterations=2）run-test HTTP 200 / 23.6s / status=success；pipeline 日志证实 critic（member-2）恰好执行 2 轮后 `critic_loop 达到迭代上限，强制收敛 rounds=2 max_iterations=2`，图经 `approved → __end__` 终止（修复前循环 24+ 次不收敛）

## Phase 9.2 — critic_loop 收敛二次强化（✅ 已完成，2026-07-24）

> 二次评审发现 2 个逻辑错误 + 3 个设计缺陷 + 1 个潜伏契约缺口，逐项修复。

| # | 问题 | 修复 | 影响域 | 状态 |
|---|------|------|--------|------|
| F1 | 中文组合式否定误判：「不能予以通过」含批准词「予以通过」误判批准 | 批准词逐出现位置判定紧邻前缀的中文否定标记（`criticNegationMarkersZH`），第二次非否定命中仍算批准 | `graph/adapter/critic_loop_cond.go` | ✅ |
| F2 | `IsApprovedDecision` 允许 score 推翻显式 `action=retry` | 显式 action 优先：approve 通过 / 其他非空不通过 / 空时 score 兜底 | `biz/team_types.go` | ✅ |
| F3 | dry 收敛先于结构化裁决生效，显式 retry 被错误收敛 | 结构化裁决提到判定链最高优先级 | `graph/adapter/critic_loop_cond.go` | ✅ |
| F4 | 上限兜底收敛与真实批准同返回 `approved`，观测不可区分 | 新增路由键 `approved_forced`（`biz.CriticLoopResultApprovedForced`），PathMap 映射 `EndNodeID` | `biz/team_types.go` · `team/embedded_graph.go` · `graph/adapter/critic_loop_cond.go` | ✅ |
| F5 | 框架 maxSteps 截断与自然完成不可区分 | BSP/DAG 循环返回 truncated，完成事件 `CompletionMetadata.StepsTruncated` 透传 + Warn 日志 | `pkg/trpc-agent-go/graph/executor.go` · `executor_dag.go` · `events.go` | ✅ |
| F6 | 外部（API/Pack）critic_loop 条件边缺 `approved_forced` 键时运行时错 | validator 新增 `critic_path_map_incomplete` 编译期校验（`validateCriticLoopPathMaps`） | `graph/trpc/validator.go` | ✅ |

**测试覆盖**：`graph/adapter/critic_loop_cond_test.go`（否定窗口/action 优先/approved_forced/节点隔离）；`biz/team_types_test.go`（IsApprovedDecision 优先级）；`team/graph_compile_test.go`（approved_forced PathMap 断言）；`graph/trpc/validator_test.go`（新增，缺键报错/有键通过/纯 threshold ref 豁免）。

**验证证据（2026-07-24）**：`go build ./...` exit 0；`internal/biz` / `internal/graph/...` / `internal/team` / `internal/service` / `internal/agent` 全 PASS；`pkg/trpc-agent-go/graph` 8 个失败经 stash 对照（3/3）确认为 dev 分支既有时序/顺序敏感问题，与改动无关。

---

## Phase 10 — ADR-08 团队编排统一（Phase A ✅ 已完成，2026-07-25）

> 来源：团队「编排模式」与 Graph 编排割裂评审。决策详见 [ADR-08](../reports/2026-07-25-review-adr-team-orchestration-unify.md)；设计详见 [53-team-graph-orchestration.design.md §十二](./53-team-graph-orchestration.design.md#十二adr-08-团队编排统一2026-07-25-phase-a-已落地)。
> **核心决策**：embedded graph 为拓扑唯一真相源；mode 退化为模板选择器；角色由 mode + 成员顺序派生；runtime_engine 从 Team 编辑器移除（统一 Graph 运行时，native 仅编排页 admin 调试）。

| ID | 任务 | 影响域 | 状态 |
|----|------|--------|------|
| A1 | 拓扑字段指纹 `definitionTopologyKey`（mode / synthesizer_agent_id / members 拓扑）+ `rebuildDefinitionGraph` 拓扑→graph 单向派生（layout 未变保留节点 x/y）；编辑器 topology watcher 驱动重建 | `web/.../teams/teamUtils.ts` · `features/teams/useTeamsPage.ts` | ✅ |
| A2 | 模板去重：后端 `CompileTeamGraph` 返回 `definition_graph_json`（canonical embedded graph spec）；前端 `definitionGraphFromCompileJSON` 同步，本地 `graphUtils` 降级为离线/失败回退 | `api/kratos/team/v1/team.proto` · `team/graph_compile.go`（`resolveDefinitionGraphSpec` / `DefinitionGraphSpecJSON`）· `service/team_compile.go` · `features/orchestration/compileApi.ts` · `useTeamsPage.scheduleGraphSyncFromBackend` | ✅ |
| A3 | 编辑器联动：mode 选项带模板描述；角色由 `deriveMemberRolesForMode` 派生（sequential 全 worker / parallel 末位 synthesizer 回写 `synthesizer_agent_id` / coordinator 首位 / critic_loop 交替 generator-critic）且派生模式下角色只读展示；parallel 汇总 Agent 派生只读；策略区条件显隐（`parallel_fail` 仅 parallel）；**移除执行引擎选择器**（保存统一 Graph） | `web/.../teams/teamUtils.ts` · `teamConstants.ts` · `TeamEditorDialog.vue` · `TeamsPage.vue` · `useTeamsPage.ts` | ✅ |
| A4 | 校验改造：definition 携带 embedded graph（`graph.nodes` 非空）时跳过 role-mode 耦合校验（角色兼容 / parallel 汇总 / coordinator / critic_loop 角色要求），结构问题由 CompileTeamGraph 编译期报告；保留 enabled 成员 ≥1 与 agent_id 必填。前后端同 PR 镜像 | `biz/team_usecase.go`（`validateTeamDefinition`）· `web/.../teams/teamUtils.ts`（`validateTeamDefinition`） | ✅ |

**测试覆盖**：

- `web/.../teams/__tests__/teamUtils.spec.ts`：`deriveMemberRolesForMode` 各 mode 派生 / 幂等 / 禁用成员跳过 / synthesizer 回写；`definitionTopologyKey` 指纹稳定性；`rebuildDefinitionGraph` 位置保留
- `web/.../teams/__tests__/teamValidation.spec.ts`：custom graph 跳过 role-mode 校验 / 保留 enabled+agent_id 校验 / 空 nodes 不跳过
- `biz/team_usecase_test.go::TestValidateTeamDefinition`：新增 7 例 graph 路径（跳过三类 mode 要求 + 保留 enabled/agent_id + 空 nodes 不跳过）

**验证证据（2026-07-25）**：

- 前端：`pnpm vitest run src/components/teams/__tests__ src/features/teams/__tests__` 71/71 PASS；`pnpm eslint` 改动文件 0 问题
- 后端：`go build ./internal/biz` exit 0；`go test ./internal/biz -run TestValidateTeamDefinition` **31/31 PASS**（0.298s，含 7 个新增 graph 用例）。`internal/agent` 存在并发 WIP 未定义符号（`tryDomainRecipe` / `TopLevelDomain` 等，与本次改动无关）阻塞测试二进制编译（经 `team_graph_linked_test.go` → `internal/team` → `internal/agent` 链），验证时临时移出该文件隔离运行，测后已恢复（见 ADR-08 §验证）

**遗留（Phase B）**：mode 字段只读化（graph 完全接管后 mode 仅存展示）、`definitionToJSON` 中 native 序列化分支清理、`TeamOrchestrateRuntimePanel` native 调试入口随 native 运行时退役移除。
