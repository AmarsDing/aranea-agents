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
| 事件 | `internal/event/envelope.go`（Team 相关 Envelope 类型） · `internal/team/status_projector.go` |
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
| StatusProjector（orchestration_agent_status） | ✅ |
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

### Phase 9（🔄 Spirit 集成扩展）— 进行中

- ✅ ListSpiritTeams / SynthesizeResults / ArchiveTeam / RetryTeam 后端 RPC
- 🔄 前端 UI 入口（TEAM-17）

---

## 5. 验收标准

### Phase 3（已完成）

- [x] Team 管理页可 RunTeamTest 并看到回复
- [x] TeamRuns 步骤有 started/finished 与工具调用数
- [x] `GET /v1/team-runs/{id}/summary` 返回结构化汇总
- [x] Chat Team 会话顶部展示成员流式 chip
- [x] Runner persistStep 单测覆盖 started/finished Envelope

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
