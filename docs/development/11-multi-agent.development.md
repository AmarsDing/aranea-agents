# Multi-Agent 编排 — 开发计划

> **版本**：2026-06-17 | **状态**：✅ Phase 1–3 已完成；✅ M53 Phase 0.5–7 已完成；✅ M53 Phase 8 单轨化已完成
> **需求**：[11 multi-agent.md](./11%20multi-agent.md) · **设计**：[11-multi-agent.design.md](./11-multi-agent.design.md)
> **M53 编排融合**：[53-team-graph-orchestration.development.md](./53-team-graph-orchestration.development.md) — Graph 编译、观测台、HITL、容错详细计划

---

## 1. 模块定位

Multi-Agent 编排：Team 模式（sequential / parallel / coordinator / critic_loop / swarm / adaptive），Agent 间协作、运行观测与 Chat 集成。Graph 为唯一执行路径（Native 已完全移除）。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| Proto | `api/kratos/team/v1/team.proto`（24 RPC） |
| Service | `internal/service/team.go` + `team_observatory.go` + `team_compile.go` + `team_compile_view.go` + `team_resume.go` + `team_dead_letter.go` + `team_orchestration_spec.go` + `team_run_registry_adapter.go` + `team_runner_wire_adapter.go` + `team_turn_hooks.go` |
| Biz | `internal/biz/team_usecase.go` · `team_types.go` · `team_summary.go` · `team_state_machine.go` · `team_run_state_machine.go` · `team_ports.go` · `team_interfaces.go` · `team_fallback.go` · `team_compiler.go` · `team_graph.go` · `team_graph_constants.go` · `team_graph_plugin.go` · `team_graph_knowledge.go` · `team_agent_ports.go` |
| Data | `internal/data/team_repo.go` · `team_graph_session_repo.go` · `team_graph_session_schema.go` |
| Schema | `internal/data/ent/schema/team.go` · `team_run.go` · `team_run_step.go` · `orchestration.go` · `orchestration_step.go` · `task_dead_letter.go` |
| Runtime | `internal/team/`（55+ 文件；graph_compile / graph_runtime / runner / status_projector / summary / state_machine 等） |
| 事件 | `internal/event/activityevent/bus.go`（ActivityEventBus） · `internal/biz/activity_event.go`（ActivityEvent / ActivityKind / ActivityEventType） · `internal/team/status_projector.go`（已重构为 ActivityProjector，投影 ActivityEvent Domain=chat） · `internal/event/monitor/eventbus.go`（MonitorEventBus，监控事件 Domain=system） |
| 前端 | `TeamsPage` · `TeamOrchestratePage` · `TeamRunObservatoryPage` · `TeamTestDialog` · `TeamRunsDialog` · `ChatTeamMemberStrip` · `TeamProgressSection` · `TeamPanel` |

---

## 2. 现状评估（2026-06-17）

### 2.1 后端

| 项 | 状态 |
|----|------|
| Team CRUD + 六种 mode 运行时 | ✅ |
| RunTeamTest / CancelTeamRun | ✅ |
| team_step_started / team_step_finished | ✅ |
| GetTeamRunSummary RPC | ✅ |
| team_summary WS + tool_call_count 落库 | ✅ |
| call_agent / member_* WS | ✅ |
| Runner 集成单测（persistStep 事件） | ✅ |
| Graph 为唯一执行路径（Native 已完全移除） | ✅ |
| CompileTeamGraph 编译预览 | ✅ |
| GetTeamRunObservatory / Timeline | ✅ |
| ResumeTeamRunExecution（HITL / Checkpoint） | ✅ |
| ListTaskDeadLetters / ResolveTaskDeadLetter | ✅ |
| OrchestrationSpec v2 + FailurePolicy + CircuitBreaker | ✅ |
| embedded graph 五种节点（agent/task/review/subgraph/function） | ✅ |
| StatusProjector（已重构为 ActivityProjector，`orchestration_agent_status` → `ActivityKind=team_stage` stage=agent_status） | ✅ |
| OrchestrationStep 持久化 | ✅ |
| Team Kind/Source/Readonly 分类 | ✅ |
| ResolveMemberAgentKeys / SaveTeamWithGraph（Pack 导入） | ✅ |
| Team 状态机（team_state_machine.go） | ✅ |
| TeamRun 状态机（team_run_state_machine.go） | ✅ |
| ListSpiritTeams / SynthesizeResults / ArchiveTeam / RetryTeam | ✅ |
| TeamGraphSession 持久化（team_graph_session_schema.go） | ✅ |
| fallback_policy.go 仅诊断错误（不执行 fallback） | ✅ |
| template_registry.go Mode 模板注册 | ✅ |

### 2.2 前端

| 项 | 状态 |
|----|------|
| RunTeamTest UI（TeamTestDialog） | ✅ |
| TeamRuns 汇总 RPC + WS 回放 banner | ✅ |
| Chat Team 成员 strip | ✅ |
| adaptive ↔ Swarm 文案 | ✅ |
| 编排画布页（TeamOrchestratePage） | ✅ |
| 编译预览（TeamCompilePreview） | ✅ |
| 运行观测台（TeamRunObservatoryPage） | ✅ |
| HITL 审核（approve/reject/fallback） | ✅ |
| 成员看板（TeamMemberKanban） | ✅ |
| 行业分组 + 拖拽排序 | ✅ |
| OrchestrationSpec v2 类型 | ✅ |
| 状态机校验（validStatusTransitions） | ✅ |
| TeamsListSection 组件 | ✅ |
| useTeamCompilePreview composable | ✅ |
| TeamProgressSection / TeamPanel Chat 组件 | ✅ |
| orchestration/teamGraphAdapter / teamNodeDisplay | ✅ |

---

## 3. 差距与优化（后续）

| ID | 优先级 | 待优化项 |
|----|--------|----------|
| TEAM-10 | P3 | TeamRuns 自动加载汇总（展开 run 时） |
| TEAM-11 | P3 | 历史 run 无 tool_call_count 时 Usage 回填 |
| TEAM-12 | P4 | swarm 独立 UI 模式（与 adaptive 二选一或合并） |
| TEAM-13 | P3 | 拖拽排序后端持久化 API（前端已本地排序） |
| TEAM-14 | P3 | findActiveTeamRun 后端 RPC（前端当前用过滤） |
| TEAM-15 | P4 | TaskDeadLetter 前端 UI 入口（API 已有，组件缺失） |
| TEAM-16 | P4 | Team struct 拆分（TECH-DEBT(COG): 字段=23, 上限=15） |
| TEAM-17 | P3 | ListSpiritTeams / SynthesizeResults / ArchiveTeam / RetryTeam 前端 UI 入口（API 已有） |
| TEAM-18 | P2 | Deliverable State 传递：`enable_state_deliverable` + `set_deliverable`/`get_deliverable` 工具（✅ Phase 1 已完成；✅ Phase 2 MDC+ack+MergeReducer 2026-07-28 完成，剩前端 UI 展示） |

---

## 4. 开发阶段

### Phase 1–2（✅）— 核心编排与运行管理

历史迭代已完成 Team CRUD、六种 mode 运行时、Chat 集成、WS 事件推送。

### Phase 3（✅ 2026-05-21）— 产品闭环

- ✅ TEAM-01 RunTeamTest 前端
- ✅ TEAM-02 team_step_started
- ✅ TEAM-03 GetTeamRunSummary RPC
- ✅ TEAM-04 工具调用统计
- ✅ TEAM-05 Chat 子 Agent strip
- ✅ TEAM-07 WS 回放（TeamRuns subscribe onReplayState）
- ✅ TEAM-08 adaptive/Swarm 文案
- ✅ TEAM-06/09 单测

### Phase 4–7（✅ M53 编排融合）— Graph 统一运行时

详细计划见 [53-team-graph-orchestration.development.md](./53-team-graph-orchestration.development.md)。

- ✅ Phase 0.5：Graph 编译器基础（CompileToGraphBuildConfig / CompileToGraphRuntimeConfig）
- ✅ Phase 1：Graph 运行时执行（graph_runtime / runner_team_trpc Graph 优先）
- ✅ Phase 2：观测台（GetTeamRunObservatory / Timeline / ActivityStepFlusher）
- ✅ Phase 3：HITL 人工审核（ResumeTeamRunExecution / RunnerMediator）
- ✅ Phase 4：容错策略（FailurePolicy / RetryPolicy / CircuitBreaker）
- ✅ Phase 5：embedded graph 扩展（task/review/subgraph/function 节点）
- ✅ Phase 6：Checkpoint / Resume（ResumeTeamRunExecution / TeamGraphSession）
- ✅ Phase 7：Native 退役（BuildTRPCTeam 已完全移除）

### Phase 8（✅ M53 架构优化）— 已完成

见 [53-team-graph-orchestration.development.md §Phase 8](./53-team-graph-orchestration.development.md)。

- ✅ 状态机协议化（team_state_machine.go / team_run_state_machine.go）
- ✅ 单轨化（Native fallback 完全移除，fallback_policy.go 仅诊断错误）
- ✅ mode → template 映射（template_registry.go）
- ✅ 配置化（runner_config.go / graph_runtime_options.go）
- ✅ 错误处理规范化（graphRuntimeDiagnosticError）

### Phase AF（✅ Activity-First 迁移 · ADR-02/ADR-03）— 已完成

> 详见 ADR-02（活动事件持久化）与 ADR-03（统一总线架构）。

- ✅ Envelope Bus / SessionBus 删除；Team 事件改走 `ActivityEventBus`（`biz.ActivityEvent`，Domain=chat 持久化 / Domain=system 仅 WS）
- ✅ `member_*` / `team_*` / `graph_*` EnvelopeType → `ActivityKind`（team_stage / graph_stage / reply，成员回复带 `agent_key` + `session_type=agent`）
- ✅ `StatusProjector` → `ActivityProjector`；监控事件走 `MonitorEventBus`（`contract.MonitorEvent`）
- ✅ Session 父子树：Team Run 经 `buildTeamProjectMeta` 填充 SpiritSessionID/ParentSessionID，创建 `session_type=team` Session 挂到 Spirit 根（详见 [10-session.design.md §3.6.6](./10-session.design.md#366-spiritsessionid-传播)）
- ✅ 前端 `teamRunEventFromEnvelope.ts` / `useEnvelopeStream.ts` 内部切换为 ActivityEvent 解析（legacy 文件名保留）

### Phase 9（🔄 Spirit 集成扩展）— 进行中

- ✅ ListSpiritTeams / SynthesizeResults / ArchiveTeam / RetryTeam 后端 RPC
- 🔄 前端 UI 入口（TEAM-17）

### Phase 10（✅ Deliverable State 传递）— Phase 1+2 已完成

**目标**：为 Team 成员提供结构化交付物的显式传递机制（一个 agent 的输出 → 另一个 agent 的输入）。

- ✅ P1-1：修复 `project_rules.md` 中 sequencer 文件路径引用（`activity_event_sequencer.go` → `v2/sequencer.go`）
- ✅ P1-2 Phase 1-a：`Definition` 新增 `EnableStateDeliverable` 字段（TDD）
- ✅ P1-2 Phase 1-b：`finalizeRuntimeGraphConfig` 注入 `deliverable` StateField（Reducer=Cover，幂等注入）
- ✅ P1-2 Phase 1-c：实现 `set_deliverable`/`get_deliverable` 工具（`internal/tools/deliverable/tool.go`）
  - `set_deliverable`：实现 `CallableTool` + `StateDelta`（duck typing），通过框架 flow 层合并到 graph state
  - `get_deliverable`：实现 `CallableTool`，通过 `inv.Session.GetState` 读取
- ✅ P1-2 Phase 1-d：注册 `deliverable` ToolSet 到 `Registry()`（`EnabledByDefault=false`，`Category=team`）
- ✅ Phase 2（2026-07-28）：成员级交付契约（MDC）+ 交付确认 + 并行安全
  - ✅ P2-a：`deliverable` StateField Reducer Cover → Merge（顶层 key 级合并，parallel 经 distinct topic 写安全；`parallelDeliverableAdvisory` 编译期 Warn）
  - ✅ P2-b：MDC（`biz.MemberDeliverableContract`，`deliverable_contract` Definition 字段）——写时强制（`required_keys`/`schema_json`，LLM 可纠错 `MemberContractViolationError`）+ 完成时 advisory（`requiredTopicsMissingFromState` Warn）
  - ✅ P2-c：`ack_deliverable` 工具（accepted/rejected 确认，写 `ack/<topic>` 顶层键；桥接 `marshalNonReservedStateKeys` 排除 `ack/` 前缀，不泄漏进团队间信封）
  - ✅ P2-d：装配收口 `deliverableToolsForDef` → `ToolsWithContract`（仅 team 编译路径注入契约；graph adapter 通用路径无契约）
  - 📋 后续：前端 UI 展示 deliverable 内容（含 ack 状态）

### Phase 11（🔄 12:33 会话修复包）— 成员状态管线 / 结果检索结构化 / 结果导向状态

**背景**：12:33 skill 安装会话暴露四问题——① Graph 成员全部「执行中」与汇报不符；② 精灵检索团队结果困难（read_session_history 考古 + get_deliverable 猜错 topic）；③ 成员执行活动面板无内容；④ 安装指令以 shell 命令下达（成员幻觉调用不存在的 exec_command）、状态按消息生命周期而非执行结果显示。评审报告与决策见 `docs/reports/2026-07-28-review-adr-system-agent-team-membership.md`（ADR-06）。

**深入评审结论（2026-07-28）**：

- P2 原案 F9「复用 VerificationGateExecutor 挂 cli_admin_skill_get 验证门」**不可行**：现有三种门（dept_lead_approval/cross_dept_delivery/borrow_approval）全是 LLM 质量评判门，无法表达「调用工具并断言返回」的确定性事实检查；安装验证是事实断言而非质量评判，LLM 门既不匹配也可被交付物文本欺骗。修正为新增确定性门类型 `tool_assertion`。
- F8 必须三层合力（任一层单修无效）：①精灵 IDENTITY.md 补子任务意图化规则；②分解 prompt 加 system-admin capability 与意图式 description 规则；③system_admin agent **无 prompt 文件**（seed_system_admin.go 仅 seed agent 行），是成员幻觉 exec_command 的直接根因，必须 seed prompt 声明确切工具名。
- F10 归因边界：MDC topic 是团队共享黑板无法按成员归因，第一版交付物证据仅用于单成员团队（12:33 即此场景）；多成员团队用 session 状态 + steps 错误证据。
- 根本解闭环：指令可路由（F8）→ 执行有工具（registryOptInOnlyKeys 已修）→ 结果有机器校验（F9 + 既有 Fix 1 真实交付物闸门）→ 状态以结果为据（F10）→ 汇报基于结构化产出（F7）。

**P0 — 成员状态管线修复（✅ 已完成）**

- ✅ F1：`team/runner_helpers.go` publishTeamStepActivity 补 `TaskID=RootTaskActivityIDFromCtx(ctx)`；Version 按写者权威带定值（created=1 / evidence=2 预留 / outcome=哨兵，见 ADR-09），消除 `VersionLT` 守卫静默拒绝导致的状态卡死
- ✅ F2：系统 Agent 成员生命周期完整化（ADR-06）——删除 `biz/spirit_team_usecase.go` AssembleTeam 系统 Agent 跳过逻辑 + `service/spirit_team.go` publishSpiritTeamAssembled filteredKeys 过滤；过滤点上移到分配层
- ✅ F3：`web/src/stores/chat/activityV2Store.ts` getMemberSessionSteps guard 去除 taskId 条件（路径 1 SessionID 精确匹配不需要 taskId）
- ✅ F4：`service/spirit_team.go` publishV2TeamRunCompletion 兜底——定义成员搜索不到 agent session 时以 team session 发 updated 事件（防御性，防回归）

**P1 — 结果检索结构化（细化）**

- ⏳ F5：MDC 自动生成 + topic 直映射
  - `buildSpiritTeamDefinitionJSON`（biz/spirit_team_usecase.go:1246）增加 `deliverables []DeliverableContract` 参数；非空时写入 definition `"deliverable_contract": {"entries":[...]}`：`topic=contract.Name`（契约名即 topic，杜绝二次映射导致的猜名）、`required=true`、`description=contract.Description`、`schema_json=contract.SchemaJSON` 透传、`required_keys` 从 SchemaJSON 的 `required` 数组推导（非法/空 Schema 则留空，仅靠完成时 advisory）
  - AssembleTeam 调用点透传 `params.Deliverables`；DAG 编译路径（TaskOrchestrator.UpdateTeamDefinitionJSON）若重生成 definition 须保留 deliverable_contract（实施时验证路径）
  - `DeliverableProtocolSuffix`（:2215）契约声明段每项追加显式提交指令 `set_deliverable(topic="<name>", data={...})`，消除成员自由命名 topic 的空间（12:33 精灵猜错 topic 根因）
- ⏳ F6：新增 `get_team_deliverable` 工具挂 spirit profile
  - 新文件 `internal/tools/spirit_deliverable_tool.go`（trpcfunction 模式，仿 spirit_tools.go）。输入 `{team_id?, max_chars?}`；team_id 空 → 返回本 spirit session 团队清单（team_id/名称/状态/任务）引导选择；非空 → `SpiritTeamController.ReadUpstreamDeliverable(ctx, "", teamID, maxChars)`（空 readerSessionID 经 resolveReaderContracts 天然豁免契约校验）；团队非 completed 或读取失败时把 error 作为结构化字段返回（`{team_id, team_name, status, content?, error?}`），不抛 tool error
  - `agent_effective_tools.go` toolProfiles["spirit"] 追加 `"get_team_deliverable"`；工具注册点与 plan_and_execute 相同（Wire 装配）
- ⏳ F7：合成 trigger 结构化注入
  - biz/spirit_synthesis.go 新类型 `TeamDeliverableDigest{TeamName, TaskName, Status, DeliverableSummary}`；`BuildSynthesisSummaryTrigger` 签名扩展 `digests []TeamDeliverableDigest`，trigger 文本在失败事实段前插入「各团队交付物摘要」（含成功团队，LLM 不再考古）
  - biz 新方法 `ListTeamDeliverableDigests(ctx, spiritSessionID)`：ListBySpiritSessionID → 逐团队解析 DeliverablesOutputJSON（DeliverableRef 信封 Summary）拼装
  - service/spirit_team.go checkAllTeamsCompleted（:749 前）收集 digests 传入；更新 spirit_synthesis_report_test.go 两例签名

**P2 — 指令方式 + 结果导向状态（修正后）**

- ⏳ F8：三层指令意图化
  - 层 1 精灵 IDENTITY.md：系统管家任务的子任务描述必须声明意图（做什么+来源 URL+指定 `cli_admin_*` 工具名），禁止 shell 命令文本
  - 层 2 buildDecompositionPrompt（task_planner_impl.go:1159）：required_capabilities 预定义 tag 加 `system-admin`；规则——系统管理类子任务标 `system-admin` 且 description 意图式（禁 shell）
  - 层 3 system_admin prompt seed：为 `__system_admin__` seed prompt 文件（声明 `cli_admin_skill_install_from_url(url)` 等工具清单与用途、完成必须 `set_deliverable` 汇报 status、禁止幻觉 exec_command 等不存在工具）
- ✅ F9：确定性 tool_assertion 验证门（修正原案，2026-07-28 落地）
  - `verification_gate.go` 新增 `GateTypeToolAssertion VerificationGateType = "tool_assertion"`；VerificationGate 扩展字段 `Tool/ArgumentsJSON/AssertPath/AssertEquals`
  - VerificationGateExecutor 新增 `executeToolAssertion`：经工具注册表调用指定工具（第一版白名单仅 `cli_admin_skill_get`），对 JSON 结果按 AssertPath 断言等于 AssertEquals；调用失败/断言不等 → approved=false
  - 门来源：team definition `verification_gates` 字段（resolveVerificationGates 已支持解析）；安装类任务的团队定义生成路径自动挂 `{tool:"cli_admin_skill_get", assert_path:"enabled", assert_equals:"true"}`（key 从任务意图提取）
  - 生产接线（2026-07-28 评审 R-2 修复）：`cmd/admin/wire.go` provideVerificationGateExecutor 注入 `WithToolAssertionInvoker(NewSkillAssertionInvoker(skillUC))`；outcome pass 挂接（R-3 修复）：`HandleTeamTurnResult` 在交付物门后调 `ExecuteVerificationGates`，拒绝/infra 错误 fail-closed 翻转 failed
- ✅ F10：结果导向成员状态（2026-07-28 落地）
  - biz 新方法 `MemberExecutionEvidence(ctx, sessionID) (failed bool, reason string)`：session status=interrupted/failed → failed；steps 含 failed/cancelled → failed（附首个失败 step 摘要）
  - service publishV2TeamRunCompletion 成员循环内：团队 completed 时 per-member 调 MemberExecutionEvidence 覆盖 memberStatus（`resolveMemberOutcomeStatus`）；单成员团队追加交付物证据（HasRealDeliverable=false → failed）；cancelled 保持 skipped
  - 「部分完成 = 失败 + 完成情况说明」由 F7 digest 文案承载；状态只有 等待/执行中/成功/失败，无第三态

**P0+ — 成员终态单写者重设计（✅ 2026-07-28 决策 / 2026-07-29 哨兵化修正，ADR-09）**

- ✅ runner 成员 completed 投影删除（含 finisher `finalizePendingSessionActivities`）：消息生命周期 ≠ 工作结果；runner 只写 created（V=1）
- ✅ 版本权威带（`biz/member_session.go` `MemberSessionVersion*`）：created=1 / evidence=2（预留）/ outcome=终态写者族（service outcome pass / Mode B finish / recovery）
- ✅ 2026-07-29 outcome 哨兵化（1<<40）：修复 pause/resume `Version++` 与固定带 V=3 碰撞导致终态被守卫静默拒绝的 P0 回归；`syncMemberSessionStatus` 终态分支携带哨兵带 + 已终态跳过守卫；recovery `SetVersion(outcome)`；回归测试 `TestMemberSessionV2Repo_Upsert_OutcomeSentinelAlwaysWins`
- ✅ 2026-07-29 standalone（Mode A）终态可达性（F-1/S-1/S-2，ADR-09 §4）：
  - F-1：`HandleTeamTurnResult` 对非 AutoCreated 团队不再早退，拆出 `handleStandaloneTeamTurnResult` 精简终态 pass（复用同一 outcome pass 证据链与哨兵带；剔除编排专属职责）
  - F-3/S-1 聚合根回退：standalone 无 `ParentSessionID`/`team.SpiritSessionID`，统一回退 team session ID 作聚合根（runner `deriveSpiritSessionID` + hooks/dispatch 调用点 + `resolveStandaloneSpiritSessionID` service 内兜底——CancelTeam 唯一可达路径）；`CancelTeam` 守卫放宽
  - S-2：standalone cancelled 时 running 成员 session 一并转 interrupted（与 AutoCreated 同姿态）；`publishTerminalTeamStage` 空聚合根守卫防孤立记录
  - 测试：`TestHandleTeamTurnResult_Standalone_Completed/Failed/Cancelled_TerminalPass` + `_EmptySpiritID_FallbackToTeamSession`

**改动文件清单**：

- `internal/team/runner_helpers.go`（F1 ✅ / 单写者 ✅：completed 投影删除）
- `internal/team/team_graph_run_finisher.go`（单写者 ✅：finalizePendingSessionActivities 删除）
- `internal/biz/spirit_team_usecase.go`（F2 ✅ / F5 / F10 ✅ biz 方法 / F9 ✅ ExecuteVerificationGates 接口）
- `internal/service/spirit_team.go`（F2 ✅ / F4 ✅ / F7 接线 / F9 ✅ 挂接 / F10 ✅ 判定 / outcome 版本带 ✅ / F-1 standalone 终态 pass ✅ / S-1 聚合根兜底+CancelTeam 守卫 ✅ / S-2 cancelled interrupted 对齐 ✅）
- `internal/service/team_turn_hooks.go` / `internal/service/chat_orchestrator_turn_dispatch.go`（F-3 ✅ 聚合根回退）+ `internal/team/runner_team_trpc_phases.go`（F-3 ✅ deriveSpiritSessionID 回退 sess.ID）
- `web/src/stores/chat/activityV2Store.ts`（F3 ✅）
- `internal/biz/member_deliverable_contract.go`（F5 schema required 推导辅助）
- `internal/tools/spirit_deliverable_tool.go`（F6 新工具）+ 工具 Wire 装配点 + `internal/biz/agent_effective_tools.go` spirit profile（F6）
- `internal/biz/spirit_synthesis.go`（F7）+ `spirit_synthesis_report_test.go`
- `internal/scenario/system/prompts/IDENTITY.md`（F8 层 1）+ `internal/agent/task_planner_impl.go`（F8 层 2）+ `internal/data/seed_system_admin.go`（F8 层 3 prompt seed）
- `internal/biz/verification_gate.go`（F9 ✅ 新门类型）+ `cmd/admin/wire.go`（F9 ✅ invoker 注入）
- `internal/biz/member_session.go`（单写者 ✅：版本带常量 + IsMemberSessionTerminal + outcome 哨兵化）
- `internal/service/chat_pause.go` / `internal/service/team_pause.go`（单写者 ✅：生命周期写者版本纪律）
- `internal/data/v2_recovery_repo.go`（单写者 ✅：recovery 终态携带 outcome 带）
- `internal/data/member_session_v2_repo_test.go`（回归测试 ✅）+ `internal/service/spirit_team_handle_result_test.go`（F9/F10/版本带断言 ✅）
- `docs/reports/2026-07-29-review-adr-member-outcome-single-writer.md`（ADR-09）

**验收标准**：

- [ ] DAG 团队（含系统 Agent 成员）完成后，member_sessions_v2 全部终态且 TaskID 非空；Graph 成员状态与汇报一致
- [ ] 成员执行活动面板（MemberSessionPanel）展示步骤流
- [ ] 团队 definition 含 deliverable_contract（topic=契约名）；成员按显式 topic 提交 set_deliverable；精灵 `get_team_deliverable` 一次调用取得结构化结果；合成 trigger 内嵌各团队交付物摘要
- [ ] 「安装 skill X」指令意图式下达、经 `cli_admin_skill_install_from_url` 执行 + tool_assertion 门校验；成员状态以执行结果为准（失败即 failed，无第三种状态）

---

## 5. 验收标准

### Phase 3（已完成）

- [x] Team 管理页可 RunTeamTest 并看到回复
- [x] TeamRuns 步骤有 started/finished 与工具调用数
- [x] `GET /v1/team-runs/{id}/summary` 返回结构化汇总
- [x] Chat Team 会话顶部展示成员流式 chip
- [x] Runner persistStep 单测覆盖 started/finished ActivityEvent（`team_stage` stage=step_started/step_finished）

### Phase 4–7（已完成）

- [x] Graph 为唯一执行路径；Native 已完全移除
- [x] 编排画布页可视化编辑 + 编译预览
- [x] 运行观测台（Agent 看板 / Timeline / Summary / HITL / 任务看板）
- [x] HITL 审核 + ResumeTeamRunExecution
- [x] FailurePolicy 容错（RetryPolicy / CircuitBreaker / 节点级覆盖）
- [x] embedded graph 五种节点类型
- [x] OrchestrationSpec v2
- [x] 死信队列（TaskDeadLetter）
- [x] 行业分组与拖拽排序

### Phase 8（已完成）

- [x] Team 状态机（7 状态 / 8 事件 / 显式转换表）
- [x] TeamRun 状态机（6 状态 / 6 事件 / 显式转换表）
- [x] Native fallback 完全移除（无 BuildTRPCTeam / 无 ARANEA_TEAM_NATIVE）
- [x] fallback_policy.go 仅生成诊断错误
- [x] template_registry.go Mode 模板注册
- [x] 编译/构建失败直接返回错误（不 silent fallback）

### Phase 9（部分完成）

- [x] ListSpiritTeams / SynthesizeResults / ArchiveTeam / RetryTeam 后端 RPC
- [ ] 前端 UI 入口（TEAM-17）

### 待验收

- [ ] 历史 run 工具调用数回填（可选，TEAM-11）
- [ ] TeamRuns 自动加载汇总（展开 run 时，TEAM-10）
- [ ] 拖拽排序后端持久化 API（TEAM-13）
- [ ] Team struct 拆分（TEAM-16，TECH-DEBT(COG)）

---

## 6. 依赖与风险

- 历史 run 在 tool_call_count 字段上线前步骤计数为 0；需 Usage 回填（TEAM-11）
- GetTeamRunSummary 经 `TeamUsecase.GetRunSummary`；与 WS `team_summary` 共用 `biz.BuildTeamRunSummaryData`（RPC 经 `toProtoTeamRunSummary`，WS 经 `SummaryMapFromData`），字段由 parity 测试保障一致
- Native 执行栈已完全移除（`BuildTRPCTeam` 不存在，`ARANEA_TEAM_NATIVE` 环境变量不存在）；`fallback_policy.go` 仅生成诊断错误
- 前端 TECH-DEBT：findActiveTeamRun 使用前端过滤（需后端 RPC）；拖拽排序仅本地（需后端持久化 API）
- Team struct 字段数 23 超过认知复杂度上限 15（TECH-DEBT(COG)），下一迭代拆分为 TeamOrgMeta / TeamOrchestrationMeta
- Spirit 集成 RPC（ListSpiritTeams / SynthesizeResults / ArchiveTeam / RetryTeam）后端已实现，前端 UI 入口待补全（TEAM-17）
