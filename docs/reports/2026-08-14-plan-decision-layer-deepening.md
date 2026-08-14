# 决策层深度分析（Deep Dive）与 P4 深化方案

> 日期：2026-08-14
> 类型：代码深潜分析 + 前沿对照 + 实施方案
> 前置：[2026-08-14-plan-orchestration-upgrade.md](./2026-08-14-plan-orchestration-upgrade.md)（P0-P3 已全部完成）
> 方法：逐文件精读决策点代码（非关键词猜测），对照前沿论文/系统，产出差距矩阵与方案

---

## 一、决策层六决策点现状（代码事实）

### D1 策略选择（direct / parallel / dag）

| 维度 | 事实 |
|------|------|
| 决策权威 | **Spirit LLM 唯一权威**——[DECISION.md](../internal/scenario/system/prompts/DECISION.md) 指示 LLM 必须显式传 `mode`；后端 [determineStrategy](../internal/agent/task_planner_impl.go#L814) 是纯透传 |
| 兜底 | LLM 不配合时 `detectTeamIntent` 关键词扫描升级模式（团队词→dag、并行词→parallel）；`detectTeamCount` 提取数量硬约束（"2个团队"） |
| 编排缓存 | `queryMemory` 命中时直接复用历史 topology → strategy（含 legacy coordinator/sequential→dag 映射） |
| 六维复杂度评分 | 仍在计算（semantic 0.25 / structural 0.15 / domain 0.15 / tool 0.10 / context 0.10 / historical 0.25），但**不驱动策略**——仅用于 thinking effort 路由（P2-5）与 metrics |

**缺陷**：策略决策理由（为何选此 mode、fallback 是否触发、六维分数）不落流程日志，决策不可审计，MAST 标注缺输入。

### D2 任务分解

| 维度 | 事实 |
|------|------|
| 机制 | 单发 LLM 流式调用（`buildDecompositionPrompt` 含 teamCount 硬约束），[llmDecomposeAttempt](../internal/agent/task_planner_impl.go#L1113) |
| 重试 | 错误分类熔断（fatal markers 立即失败 / retriable markers 指数退避），上限 5 次（F8） |
| 校验 | [validateSubTaskDAG](../internal/agent/task_planner_impl.go#L1692) **仅结构校验**：重复 ID、环检测、前向引用解析 |

**缺陷**：**无计划可行性校验**——不校验子任务 `RequiredCapabilities` 是否有 agent 能满足、不校验隐含工具需求是否可用、不校验分解粒度。计划质量完全依赖 LLM 单发自觉，失败要等到执行期才暴露。

### D3 成员分配（成熟，非短板）

三层级联已实现：[agent_allocator_impl.go](../internal/agent/agent_allocator_impl.go)
1. mission match → 2. 历史性能最优 → 3. 精确能力匹配（overlap×0.7+成功率×0.3，阈值 0.5）→ 4. embedding 语义匹配（阈值 0.3，可叠加历史成功率）→ 5. LLM 冷启动 → 6. 最佳精确兜底

**结论**：无需改动。

### D4 重规划 ⚠️ 重大发现：双轨断裂

| 轨道 | 状态 | 事实 |
|------|------|------|
| **生产路径** | 在用 | [failure_recovery.go](../internal/graph/trpc/failure_recovery.go) `failureRecoveryAfterNode`——**静态声明式**：节点定义写死 `FailureAction=skip` 或 `FallbackAgent=X`，AfterNode 回调机械执行 |
| **智能轨道** | **死代码** | [runtime_replanner.go](../internal/graph/runtime_replanner.go) `RuntimeReplanner`（关键词规则分析 → `ReplanAction{retry/reroute/insert_fallback/rebuild_subgraph}`，maxReplanAttempts=3）——**生产零调用**（全仓 grep 确认 `OnNodeFailure` 仅测试文件引用）；[ControlCommand](../internal/graph/control_command.go#L22) 注释明示"stash in state for observability, NOT returned as AfterNode result" |

即：调研对标的"重规划决策"在生产中**从未发生过**——智能轨道是观测性空壳；且其 `insert_fallback` 的 prev/next 节点 ID 从 `failedNode+"_prev"` 命名约定派生，即使接通也接不上真实拓扑。

### D5 终止/质量门

| 维度 | 事实 |
|------|------|
| 机制 | [runner_team_turn.go:223](../internal/team/runner_team_turn.go#L223) `deliverableGate`——DAG 团队收尾时查 `HasRealDeliverable`（有无成员调用过 `set_deliverable`） |
| 粒度 | **二元**：有交付物即成功，**不评估内容质量**；不达标仅否决成功，无"打回重做"通道 |
| 防护 | `tool_team_completion_guard`：团队运行中时确定性阻断 `synthesize_results`（WP-2a，良好） |

### D6 Leader 综合

- DECISION.md 流程：`plan_and_execute` → 后台监控推送完成通知 → `synthesize_results` 合成
- **中途无反馈环**：成员执行期间 Leader 不介入；P2-3 已铺好 steer（下一 step 边界消费）语义路基，但 Leader 侧无产出中途纠偏消息的逻辑

---

## 二、前沿理论对照（决策层专属）

| 理论/系统 | 核心机制 | 对应差距 |
|-----------|---------|---------|
| **LLMCompiler**（2023, UC Berkeley） | Planner→TaskFetcher→**Joiner**；Joiner 每步决定 finish-or-**replan**，重规划是 LLM 决策 | G2：我们的 replan 是关键词规则且未接线 |
| **Reflexion**（NeurIPS 2023） | 失败原因写成 verbal 反馈注入下次尝试的情景记忆 | G2/G3：重试不携带失败上下文 |
| **Self-Refine**（NeurIPS 2023） | 同模型 generate→feedback→refine **有界**循环 | G1：分解无自修复环 |
| **LLM-as-Judge / Prometheus** | rubric 化评分替代二元判断 | G3：质量门二元 |
| **Ensemble QSP checklist 注入**（已调研） | 评审环节自动注入领域校验清单，不靠模型自觉 | G3：deliverable 无 checklist 校验 |
| **pi-agentteam Leader-owned board**（已调研） | Worker `report_done` → Leader **评审后**显式启动下游 | G4/D6：Leader 只有终点合成 |
| **LATS**（MCTS+Reflexion） | 搜索式规划 | 成本过高，**明确不采纳**（生产默认路径禁同步搜索） |

---

## 三、差距矩阵与 P4 方案

| # | 差距 | 等级 | 方案 | 理论依据 |
|---|------|------|------|---------|
| **G1** | 计划无可行性校验，执行期才暴雷 | **高** | **计划校验门**：分解后、board 发布前插入 `PlanVerifier`——规则层校验 RequiredCapabilities 对 AgentCapability 清单可满足性 + 粒度/数量合理性；违例写回 prompt 有界重分解 1 次（Self-Refine），仍失败则带违例详情降级 direct | Self-Refine 有界自修复 |
| **G2** | 重规划双轨断裂，智能轨道死代码 | **高** | **重规划统一接线**：节点失败先走节点静态声明（failure_recovery 保留为先验），未声明或声明耗尽时上调 `RuntimeReplanner`；决策**真正落地**（retry=RetryPolicy 重入队；fallback=切换 agent 重执行节点；skip=标记+下游依赖评估）；失败上下文（error+节点输入摘要）注入重试 prompt（Reflexion）；关键词分析保留为快路径，unknown 可挂 LLM 分析（可选档） | LLMCompiler Joiner |
| **G3** | 质量门二元，无内容评估 | 中 | **交付物质量门**：deliverableGate 扩展 verdict `{pass / revise(feedback) / fail}`——rubric 评分（完整性/与任务契约一致性），revise 把反馈 steer 回成员（复用 P2-3 路基），上限 2 轮防循环 | LLM-as-Judge + Ensemble QSP checklist |
| **G4** | Leader 中途无反馈环 | 中 | **中途纠偏**：成员 milestone/异常事件 → Leader 评估 → steer 消息注入成员下一 step 边界 | pi-agentteam Leader 评审 |
| **G5** | 策略决策不可审计 | 低 | **决策证据链**：mode/fallback 触发/teamCount/六维分数落 FlowLog（`spirit.planner.decision`），喂 MAST 标注 | DSH 可观测性铁律 |

### 实施约束（承接既有红线）

1. G1/G3 的 LLM 调用必须**有界**（校验 1 次重试、revise 2 轮），禁无限自循环（风险 #4：多 Agent 墙钟）
2. G2 接线保持 **fail-closed**：replanner 异常时退回现有静态路径，不得软吞错误
3. G2 的 LLM 分析档默认关闭（先规则快路径），对齐 P1-2 "先观察后阻断"策略
4. 所有新决策点落 FlowLog step 并登记 `stepTitleRegistry`（DOC-SYNC）
5. 每任务 TDD，批次门禁同前五批

### 建议批次

| 批次 | 内容 | 预估改动面 | 状态 |
|------|------|-----------|------|
| Batch-7 | G5（证据链，极小）+ G1（计划校验门） | task_planner_impl + FlowLog 注册 | ✅ 完成（2026-08-14） |
| Batch-8 | G2（重规划统一接线，架构级，先 ADR-F） | runtime_replanner + graph executor + failure_recovery | ✅ 完成（2026-08-14） |
| Batch-9 | G3（质量门 verdict）+ G4（Leader 中途纠偏） | runner_team_turn + synthesis + team steer | ✅ 完成（2026-08-14） |

#### Batch-9 设计定稿（ADR-G，2026-08-14）

**G4 框架边界（事实核查结论）**：框架 steer 为 invocation-scoped（`pkg/trpc-agent-go/internal/state/steer`：runner 为活动 invocation 挂队列，llmflow 在 step 边界 drain）；graph executor **无 steer 通道**（`pkg/trpc-agent-go/graph` 零匹配），图成员节点 invocation 不注册进 RunRegistry——**成员运行中 steer 注入不可行**（除非改框架，本批次明确不做）。据此 G4 收敛：**中途纠偏评审点收敛到质量门**——成员中途异常（interrupted/failed step）作为 J4 规则输入带进评审（pi-agentteam「Leader 评审后验收」语义在门侧落地）；中途可见性由 G2 的 `graph.replan.applied` / `graph.node_error` 事件承担。后续若需真·中途 steer，需框架级 graph executor steer 队列（另立 ADR）。

**G3 交付物质量门 verdict**（rule-based v1，LLM judge 留作可选后续档，对齐 G2 约束3「先规则快路径」）：

- **biz 判定** `SpiritTeamUsecase.EvaluateDeliverableQuality(ctx, team) (QualityGateResult, error)`：非 DAG 团队直接 pass；读自有交付物（graph state − 上游种子，排除 summary/cognition/ack 保留键）；规则：
  - J2 充分性：全 topic 文本总长 < 80 runes → revise「内容过于简略」
  - J3 占位/拒答：命中 TODO/占位/待定/无法完成/作为AI 等标记 → revise
  - J4 成员异常：成员 session interrupted 或存在 failed step（MemberExecutionEvidence）→ revise 并点名成员
  - 全过 → pass；infra 读错 → 返回 error（runner 侧 fail-open）
- **runner 集成**（`finalizeTeamRun`，既有二元门之后、success 转换之前）：pass → 照常；revise/fail → 修订预算（team+session 内存计数，上限 2）内：计数++ → followup 入队（P2-3 路基，反馈消息随当前 turn 结束后作为新 turn 输入）→ run 标 failed（原因=质量门打回）；预算耗尽 → **fail-open 放行** + warn FlowLog（防回归：今天二元门会放行的交付物不被质量门卡死）；judge infra error → fail-open + warn。
- **端口**（规避 wire 环：ChatUsecase 晚于 team.Runner 构建）：`SetQualityGate` / `SetRevisionEnqueuer` setter 模式（同 SetDeliverableGate），biz `team_ports.go` 接口增补，orchestrator 装配点 `chat_orchestrator.go:598` 旁。
- **FlowLog**：`team.quality_gate.revise`（warn，打回修订）、`team.quality_gate.bypass`（warn，预算耗尽/判分异常放行）。

#### Batch-9 完成记录（2026-08-14）

- **G3 交付物质量门 verdict**：
  - biz 判定 [team_quality_gate.go](../internal/biz/team_quality_gate.go) `EvaluateDeliverableQuality`——非 DAG / 无 state 通道 / 空交付物直接 pass；自有交付物（graph state − 上游种子，排除 cognition/ack 保留键）拍平后过 J2 充分性（<80 runes）/ J3 占位拒答标记 / J4 成员异常；`SpiritTeamController` 接口增补同名方法；
  - runner 集成 [runner_quality_gate.go](../internal/team/runner_quality_gate.go) `qualityGateBlocks`（`finalizeTeamRun` 二元门之后、success 转换之前）：revise/fail → 修订预算（team+session 内存计数，上限 `maxQualityRevisions=2`）内 followup 入队 + `finishRunErr`；预算耗尽 / 判分 infra error / 未装配 enqueuer / 入队被拒 → fail-open + warn（`team.quality_gate.bypass` 四类 `reason`）；
  - 端口与装配：`team_ports.go` `TeamGraphRunFinisherPort` 增补 `SetQualityGate` / `SetRevisionEnqueuer`；[chat_orchestrator.go](../internal/service/chat_orchestrator.go) 装配（revisionEnqueuer 走 `EnqueueUserMessageWithKind{ChatEnqueueKindFollowup}` P2-3 路基，当前 turn 结束后作为新 turn 输入；拒收转 error 触发 fail-open）；`teamRunnerWireAdapter` 透传两方法（关键：缺转发则编排器接线静默空操作，本批次实测拦截）。
- **G4 Leader 中途纠偏**：按 ADR-G 框架边界结论收敛为 J4 规则随 G3 落地——成员 session interrupted / failed step（`MemberExecutionEvidence`）作为质量门规则输入并点名成员（pi-agentteam「Leader 评审验收」语义门侧落地）；真·中途 steer 需框架级 graph executor steer 队列，另立 ADR。
- **观测**：`team.quality_gate.revise` / `team.quality_gate.bypass` 已登记 `stepTitleRegistry` + [52-flow-logger.design.md §5.1](../development/52-flow-logger.design.md)。
- **门禁**（干净 GOCACHE，默认缓存现 biz 导出数据幻影）：`go build ./cmd/... ./internal/... ./api/... ./pkg/...` 绿；`go test ./internal/team/ ./internal/biz/ ./internal/service/ -count=1` 全绿（新增 13 个用例：biz 判定 7 + runner 集成 6）。

#### Batch-7 完成记录（2026-08-14）

- **G5 决策证据链**：`taskPlannerImpl.emitPlannerDecision`（[task_planner_impl.go](../internal/agent/task_planner_impl.go)）在策略路由后发射 `spirit.planner.decision` FlowLog，字段含 `decision_source`（llm_mode/keyword_fallback/complexity_auto/memory_cache）/ `mode` / `strategy` / `complexity_score` / `team_count` / `fallback_triggered`；memory 命中路径同样落证据。step 已登记 `stepTitleRegistry` + [52-flow-logger.design.md §5.1](../development/52-flow-logger.design.md)。
- **G1 计划校验门**：[plan_verifier.go](../internal/agent/plan_verifier.go) `verifyPlanFeasibility`（纯函数，R1 空定义 / R2 能力不可满足 / R3 病态规模>12）+ `applyPlanVerifyGate`（违例 → 反馈写回 prompt 有界重分解恰好 1 次 → 仍违例降级 direct 走 `decompose_failed{reason=verify_failed}`）。fail-open 约束：capBuilder nil 或能力清单构建失败时整体跳过。`NewTaskPlanner` 新增第 9 参 `agentReader biz.AgentReader`（wire 已重生成）。已知折衷：流式路径下修复后计划不重发中间 PlanStep 动画，终态 Board 由既有补发机制保证。
- **门禁**：`go build ./cmd/... ./internal/... ./api/... ./pkg/...` 绿；`go test ./internal/agent/... ./internal/event/... -count=1` 全绿（新增 13 个用例：G1 纯函数 5 + 门集成 5 + G5 证据链 3）。

#### Batch-8 完成记录（2026-08-14）

- **ADR-F**：[2026-08-14-review-adr-replan-unified-wiring.md](./2026-08-14-review-adr-replan-unified-wiring.md)——仲裁顺序（静态 per-node 先、智能 global 后，框架事实 `mergeNodeCallbacks`）+ 落地语义（fail-closed）+ 生命周期（per-execution 计数收口）。
- **G2 重规划统一接线**：智能轨死代码接通生产。
  - 提取包级构造 [replan_callbacks.go](../internal/graph/adapter/replan_callbacks.go) `NewReplanNodeCallbacks`（graph run 域与 team 域共用），`runtime_adapter.go` 原 C-23 实现改为委托；
  - `applyReplanControl` 落地语义：retry → **Reflexion 智能重试**（失败反馈注入 `user_input` 浅拷副本同步重执行 agent 节点，原 state 零污染防 checkpoint 污染；非 agent 节点 fail-closed 传播原始错误）；reroute → 退化 skip（`SkippedNodesStateKey` 标记）；insert_fallback → `InterruptError` HITL；rebuild_subgraph/未知/replanner 异常 → fail-closed 传播；
  - team 域接线 [runner_replan_wiring.go](../internal/team/runner_replan_wiring.go)：graph 模式经 run 级 `RuntimeState[StateKeyNodeCallbacks]` 注入回调，`RunnerConfig.Replanner` 由 wire 注入（与 graph run 域共享实例），`ReleaseExecution` 随 run 收口 defer 释放（含 HITL 暂停路径）。
- **观测**：新 step `graph.replan.applied`（ok=智能重试成功 / warn=重试失败、reroute 降级 skip）已登记 `stepTitleRegistry` + [52-flow-logger.design.md §5.1](../development/52-flow-logger.design.md)；[53-team-graph-orchestration.design.md §6.2](../development/53-team-graph-orchestration.design.md) 同步设计小节。
- **门禁**：`go build ./cmd/... ./internal/... ./api/... ./pkg/...` 绿；`go test ./internal/graph/... ./internal/team/... ./internal/event/... -count=1` 全绿（新增 11 个用例：applyReplanControl 6 + 构造函数 2 + team 接线 3）。

---

## 四、信息来源

- 代码事实：本文 §一 全部为逐文件精读结果（文件:行号已标注），非推测
- LLMCompiler: [arXiv:2307.05760](https://arxiv.org/abs/2307.05760)；Reflexion: [arXiv:2303.11366](https://arxiv.org/abs/2303.11366)；Self-Refine: [arXiv:2303.17651](https://arxiv.org/abs/2303.17651)；LLM-as-Judge: [arXiv:2306.05685](https://arxiv.org/abs/2306.05685)
- 既有调研：DeepSeek Harness / Ensemble QSP / pi-agentteam / HiClaw 见 [plan 文档 §二](./2026-08-14-plan-orchestration-upgrade.md)
