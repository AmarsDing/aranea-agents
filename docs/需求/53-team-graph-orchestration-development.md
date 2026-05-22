# M53: Team × Graph 编排融合 — 开发计划

> **版本**：2026-05-23 | **状态**：✅ Phase 0.5–4 已落地；**执行层仍双轨**（Native 默认 + Graph feature flag）；Phase 5–7 收敛至单链  
> **需求**：[53 team-graph-orchestration.md](./53%20team-graph-orchestration.md) · **设计**：[53 team-graph-orchestration.design.md](./53%20team-graph-orchestration.design.md)  
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-TG-01

---

## 1. 模块定位

Team 与 Graph 编排融合：统一 OrchestrationSpec、Agent 状态观测、Kanban 看板、编译器与 Graph 运行时收敛。

**代码锚点**：

| 层级 | 路径 | 阶段 |
|------|------|------|
| Biz 状态 | `internal/biz/orchestration_status.go` | 0.5 ✅ |
| Team 投影 | `internal/team/status_projector.go` | 0.5 ✅ |
| Team 编译 | `internal/team/graph_compile.go` | 2 |
| Event | `internal/event/envelope.go` | 0.5 ✅ |
| Service | `internal/service/orchestration_observatory.go` | 1 |
| Graph 挂钩 | `internal/service/graph.go` | 1 |
| 前端 | `web/src/features/orchestration/` | 1 |
| Proto | `api/kratos/team/v1/team.proto` | 2 |

---

## 2. 现状评估（2026-05-23）

| 项 | 状态 | 证据 |
|----|------|------|
| Team 六种 mode 运行时 | ✅ | `internal/team/trpc_build.go` |
| Team 前端 graph 预览（假拓扑） | ✅ | Observatory / Compile 统一 `BuildCompileSnapshot` |
| Graph Vue Flow + Run 页 | ✅ | `GraphEditorPage` / `GraphRunPage` |
| Graph EventBridge | ✅ | `internal/graph/trpc/event_bridge.go` |
| ExecutionSummary | ✅ | `execution_summary.go` |
| 统一 Agent 状态 | ✅ | Phase 0.5 StatusProjector |
| Kanban UI | ✅ | Phase 1 Observatory 页 |
| mode→Graph 编译器 | ✅ | Phase 2 `graph_compile.go` |
| GraphAgent 统一 Team Run | 🟡 | Phase 3 可选路径；**默认仍 Native**（`trpc_build.go`） |

**离终态「Team 编排规格 + Graph 执行一条链」**：编译 / 观测 / Channel async 已收敛；**Chat Team Run 执行**仍依赖双开关（`ARANEA_TEAM_GRAPH_RUNTIME` + `runtime_engine=graph`），Native 路径未退役。差距清单见 [§8 终态路线图](#8-终态路线图team-规格--graph-执行单链)。

---

## 3. 开发阶段

### Phase 0.5 — Agent 状态模型 + StatusProjector（P0，约 1 周）

> **目标**：不切换 Team 运行时，先统一 WS 状态投影，为 Kanban/Graph 提供真相源。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| TG-OBS-01 | `biz.AgentNodeStatus` + `ApplyOrchestrationEnvelope` + 单测 | `internal/biz` | 优先级/终态/transfer 单测绿 |
| TG-OBS-02 | `EnvelopeTypeOrchestrationAgentStatus` + RouteChannel | `internal/event` | team/graph 通道路由 |
| TG-OBS-03 | `team.StartOrchestrationStatusProjector` | `internal/team` | 订阅 session Bus |
| TG-OBS-04 | `runner_team_trpc` 启动/停止投影器 | `internal/team` | Run 期间有 status WS |
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
| TG-API-02 | Graph Run 启动 StatusProjector | `internal/service/graph.go` | graph 通道 status |
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
| TG-FP-03 | Task review_required → waiting_review | StatusProjector | ✅ `graph_task_status` envelope + `applyGraphTaskStatus` |
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
| TG-RT-FALLBACK-ONLY | Native 路径 **仅** fallback | `runner_team_trpc.go` | 非 fallback 场景不调用 `BuildTRPCTeam` |
| TG-RT-DEPRECATE | `BuildTRPCTeam` 标记 deprecated | `trpc_build.go` | godoc + lint 告警 |
| TG-CMP-V2 | OrchestrationSpec **version 2** 类型对齐 | `web/types` · proto 文档 | `linked_graph_id` / `failure_policy` / `graph.entry_point` 一等字段 |
| TG-RT-CHECKPOINT | Team Graph Run **Checkpoint** | `graph/trpc` · `runner` | 长任务可 resume（对齐 M36） |
| TG-RT-HITL | Team Graph Run **InterruptBefore/After** | `graph/trpc` · Team 定义 | 与 Graph Run 页 HITL 一致 |

### Phase 7 — 单链终态（P3）

> **目标**：删除 Native Team 执行栈；Team = OrchestrationSpec 编辑视图，Graph = 唯一运行时。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| TG-RT-RETIRE | 移除 `BuildTRPCTeam` 主路径 | `internal/team/trpc_build.go` | Team Run 仅 GraphAgent；保留 emergency `ARANEA_TEAM_NATIVE=1` 可选 |
| TG-RT-TASK | Team 编译支持 **Task / review** 节点 | `graph_compile` · StatusProjector | US-06 验收 |
| TG-RT-SUBGRAPH | Team **嵌套子图** / Router 节点 | `graph_compile` · linked_graph | 复杂协作不离开 Team 资产 |
| TG-0-ARCH | 更新 [0 系统框图](../需求/0%20系统框图.md) Team 执行路径 | 文档 | `Compile → GraphAgent` 单箭头 |
| TG-11-SYNC | [11-multi-agent-development.md](./11-multi-agent-development.md) 标注 Native 已退役 | 文档 | 交叉引用 M53 §8 |

**跨模块（M36，不阻塞 Phase 5 但阻塞 US-06 全量）**：G1 LLM 节点 · G2 Tool 节点 · G5–G8 执行监控与校验面板 · G9–G14 见 [36-graph-development.md](./36-graph-development.md)。

---

## 4. 任务板（当前冲刺）

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | TG-OBS-01 | biz 状态归约器 | ✅ |
| 2 | TG-OBS-02 | Envelope 类型 | ✅ |
| 3 | TG-OBS-03 | StatusProjector | ✅ |
| 4 | TG-OBS-04 | Runner 挂钩 | ✅ |
| 5 | TG-OBS-05 | 前端类型 | ✅ |
| 6 | TG-FE-01 | Kanban 组件 | ✅ |
| 7 | TG-API-01 | Observatory RPC | ✅ |
| 8 | TG-FE-02 | Observatory 页 + 路由 | ✅ |
| 9 | TG-FE-03 | GraphFlowNode 细态 | ✅ |
| 10 | TG-FE-05 | useOrchestrationStream | ✅ |
| 11 | TG-API-02 | Graph Run StatusProjector | ✅ |
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
| 20 | TG-RT-PARITY | Native vs Graph 对比 E2E | ⏳ |
| 21 | TG-RT-UI | runtime_engine 前端 + 字段保留 | ⏳ |
| 22 | TG-RT-UI-RO | GraphEditorCanvas readonly | ⏳ |
| 23 | TG-RT-METRICS | graph_execution_id / fallback 监控 | ⏳ |
| 24 | TG-RT-FLAG | 生产 Canary rollout | ⏳ |
| 25 | TG-OBS-HIST | Activity 时间线 | ⏳ |
| 26 | TG-CMP-JOIN | embedded join + ParallelFail | ⏳ |

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

| 风险 | 缓解 |
|------|------|
| member_* 无 node_id，仅靠 agent_key 映射 | Run 开始时构建 Registry；document 约定 node_id=`member-{sort_order}` |
| Phase 3 切换运行时回归 | feature flag；Phase 0.5–1 双轨观测先行 |
| Kanban 与 Chat 工具卡片重复 | 复用 ActivityMeta 结构，不 duplicate 投影逻辑 |
| 文档与 11/36 重复 | 53 管融合边界；11/36 管单模块，互链不复制 |
| Graph 路径 silent fallback Native | Phase 5 指标 + FlowLog `team.graph_runtime.*` 告警；parity 测试后再扩 rollout |
| `teamUtils.parseDefinition` 丢 `runtime_engine` | Phase 5 TG-RT-UI：扩展类型或 raw merge 未知字段 |

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
        └─ StatusProjector / Observatory / Channel async_team_id
                （同一编译链、同一 WS 状态模型）
```

**不再存在**：`BuildTRPCTeam` 按 mode 分发 ChainAgent / ParallelAgent / Swarm 的 **主执行路径**。

### 8.2 已完成（一条链的「上半段」）

| 能力 | 状态 | 说明 |
|------|------|------|
| 编译真相源 | ✅ | `graph_compile.go` + `embedded_graph.go` + `linked_graph_loader` |
| Run 快照 | ✅ | `definition_snapshot_json` 冻结；Observatory 读快照 |
| 观测拓扑 | ✅ | `compiled_topology` 后端 Compile；前端不伪造 |
| FailurePolicy 编译 | ✅ | Retry / skip / parallel_fail → GraphBuildConfig |
| Channel team_graph | ✅ | `CompileToGraphRuntimeConfig` 与 Chat 同编译链 |
| Graph 可选执行 | ✅ | feature flag + `graph_execution_id` + fallback |

### 8.3 未完成（离终态差距）

#### A. 执行层（核心）

| 差距 | 现状 | 目标 Phase |
|------|------|------------|
| Team Run **默认 Native** | `BuildTRPCTeam` 无 flag 即走 | Phase 6 默认 Graph；Phase 7 移除 Native |
| **双开关** gate | env + `runtime_engine` | Phase 6 env 默认开；Phase 7 仅 Team 级 opt-out |
| **Silent fallback** | 编译/构建失败回 Native，用户无感 | Phase 5 指标 + 可选 strict 模式（失败即报错） |
| **Mode parity 未证明** | 仅 compile E2E + 单测 | Phase 5 TG-RT-PARITY 六 mode 对比 |

#### B. OrchestrationSpec 产品化

| 差距 | 现状 | 目标 Phase |
|------|------|------------|
| `runtime_engine` 无 UI | 需 API 手改 JSON | Phase 5 TG-RT-UI |
| 前端保存 **剥离未知字段** | `parseDefinition` 白名单 | Phase 5 扩展 `TeamDefinition` 或 raw merge |
| Spec **version 2** 未在前端建模 | 设计 §2.1 有，`types.ts` 缺 | Phase 6 TG-CMP-V2 |
| 编排页运行中 **画布仍可拖** | 仅禁保存 | Phase 5 TG-RT-UI-RO |

#### C. 观测与拓扑语义

| 差距 | 现状 | 目标 Phase |
|------|------|------------|
| Activity **仅 current_activity** | 无完整时间线 | Phase 5 TG-OBS-HIST |
| ParallelFail **启发式 join** | `parallelBranchNodeIDs` 推断 | Phase 5 TG-CMP-JOIN + 设计 §6.1 显式 join |
| member_* → node_id | agent_key 映射；无 graph node id 时靠约定 | 文档化 + Registry 增强（随 parity 测试） |

#### D. Graph 引擎能力（M36 × M53）

| 差距 | 现状 | 目标 Phase |
|------|------|------------|
| Checkpoint / HITL on Team Run | Graph 模块有，Team Graph Run 未接 | Phase 6 |
| Task / review 节点进 Team 编译 | Graph Task 仅独立 Graph Run | Phase 7 TG-RT-TASK |
| LLM / Tool 节点（G1/G2） | Team 编译仅 agent 节点 | Phase 7 + US-06 |
| Destinations 编辑器（G-GOTO） | Team adaptive 编译已写 Destinations；Graph UI 未编 | Phase 5 |

#### E. 运维与退役

| 差距 | 现状 | 目标 Phase |
|------|------|------------|
| Rollout  playbook | changelog 提及，无 Runbook | Phase 5 TG-RT-FLAG |
| `trpc_build.go` 退役 | 六种 mode 仍维护 | Phase 7 TG-RT-RETIRE |
| 系统框图 / 11 文档 | 仍写 `BuildTRPCTeam` 主路径 | Phase 7 TG-0-ARCH |

### 8.4 推荐实施顺序

```
Phase 5  parity + UI + metrics + Canary
    ↓
Phase 6  默认 Graph + Checkpoint/HITL + Spec v2
    ↓
Phase 7  移除 Native + Task/Subgraph + 文档/arch 图更新
```

**原则**：每阶段 **扩大 Graph 执行占比**，指标达标后再进入下一阶段；Native 保留至 Phase 7 前均为 **安全网**。

### 8.5 配置速查（当前双轨期）

| 层级 | 配置 | 作用 |
|------|------|------|
| 进程 env | `ARANEA_TEAM_GRAPH_RUNTIME=1` | 平台级允许 Graph 执行 |
| Team JSON | `"runtime_engine":"graph"` 或 `"team_graph_runtime":true` | Team 级启用 |
| 验证 | Run.`graph_execution_id` 非空 | 未 fallback Native |
| FlowLog | `team.run.graph` vs `team.run.build` | 构建路径可观测 |

详见 [2026-05-23 Phase4 Optimization changelog](../changelog/2026-05-23-Team-Graph-M53-Phase4-Optimization.md) §Feature flag。

---

## 7. 关联文档更新

- [11 multi-agent.md](./11%20multi-agent.md) — 增加 M53 交叉引用（编排融合）
- [36 graph-workflow.md](./36%20graph-workflow.md) — §0.1 增加 Team 融合路径
- [execution-plan.md](../guides/execution-plan.md) — 迭代 TG 任务板
- [README-development.md](./README-development.md) — 索引 M53
