# Multi-Agent 编排 — 开发计划

> **版本**：2026-06-06 | **状态**：✅ Phase 1–3 已完成；✅ M53 Phase 0.5–7 已完成；**M53 Phase 8 进行中**（架构优化）
> **需求**：[11 multi-agent.md](./11%20multi-agent.md) · **设计**：[11 multi-agent.design.md](./11-multi-agent.design.md)
> **M53 编排融合**：[53-team-graph-orchestration.development.md](./53-team-graph-orchestration.development.md) — Graph 编译、观测台、HITL、容错详细计划
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-05 · EP-TEST-01
> **变更**：[changelog/2026-05-21-Multi-Agent-P1-P3.md](../changelog/2026-05-21-Multi-Agent-P1-P3.md) · [Review P0–P1](../changelog/2026-05-21-Multi-Agent-Review-P0-P1.md) · [Review P2](../changelog/2026-05-21-Multi-Agent-Review-P2.md)

---

## 1. 模块定位

Multi-Agent 编排：Team 模式（sequential / parallel / coordinator / critic_loop / swarm / adaptive），Agent 间协作、运行观测与 Chat 集成。Graph 为默认执行路径，Native 已 Deprecated。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| Proto | `api/kratos/team/v1/team.proto`（20 RPC） |
| Service | `internal/service/team.go` + `team_observatory.go` + `team_compile.go` + `team_resume.go` + `team_dead_letter.go` |
| Biz | `internal/biz/team_usecase.go` · `team_types.go` · `team_summary.go`（窄接口 + GetRunSummary） |
| Data | `internal/data/team_repo.go`（含 OrchestrationStep / TaskDeadLetter） |
| Runtime | `internal/team/`（55 文件；graph_compile / graph_runtime / status_projector / summary 等） |
| 事件 | `internal/agent/turn_helpers.go`（MemberToolCalls） · `internal/team/status_projector.go` |
| 前端 | `TeamsPage` · `TeamOrchestratePage` · `TeamRunObservatoryPage` · `TeamTestDialog` · `TeamRunsDialog` · `ChatTeamMemberStrip` |

---

## 2. 现状评估（2026-06-06）

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
| Graph 为默认执行路径 | ✅ |
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

---

## 4. 开发阶段

### Phase 1–2（✅）— 核心编排与运行管理

见 [changelog](../changelog/2026-05-21-Multi-Agent-P1-P3.md) 之前迭代。

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
- ✅ Phase 7：Native 退役（BuildTRPCTeam Deprecated，仅 ARANEA_TEAM_NATIVE=1 应急）

### Phase 8（🔄 M53 架构优化）— 进行中

见 [53-team-graph-orchestration.development.md §Phase 8](./53-team-graph-orchestration.development.md#phase-8)。

- 🔄 状态机协议化
- 🔄 单轨化（移除 Native fallback）
- 🔄 mode → template 映射
- 🔄 配置化
- 🔄 错误处理规范化

---

## 5. 验收标准

### Phase 3（已完成）

- [x] Team 管理页可 RunTeamTest 并看到回复
- [x] TeamRuns 步骤有 started/finished 与工具调用数
- [x] `GET /v1/team-runs/{id}/summary` 返回结构化汇总
- [x] Chat Team 会话顶部展示成员流式 chip
- [x] Runner persistStep 单测覆盖 started/finished Envelope

### Phase 4–7（已完成）

- [x] Graph 为默认执行路径；Native 仅 ARANEA_TEAM_NATIVE=1 应急
- [x] 编排画布页可视化编辑 + 编译预览
- [x] 运行观测台（Agent 看板 / Timeline / Summary / HITL / 任务看板）
- [x] HITL 审核 + ResumeTeamRunExecution
- [x] FailurePolicy 容错（RetryPolicy / CircuitBreaker / 节点级覆盖）
- [x] embedded graph 五种节点类型
- [x] OrchestrationSpec v2
- [x] 死信队列（TaskDeadLetter）
- [x] 行业分组与拖拽排序

---

## 6. 依赖与风险

- 历史 run 在 tool_call_count 字段上线前步骤计数为 0；**已有库**执行 `docs/sql/03_session_team_run_steps_tool_call_count.sql`
- GetTeamRunSummary 经 `TeamUsecase.GetRunSummary`；与 WS `team_summary` 共用 `biz.BuildTeamRunSummaryData`（RPC 经 `toProtoTeamRunSummary`，WS 经 `SummaryMapFromData`），字段由 parity 测试保障一致
- Native 执行栈（`BuildTRPCTeam`）已 Deprecated 但未移除；Phase 8 单轨化后将完全删除
- 前端 TECH-DEBT：findActiveTeamRun 使用前端过滤（需后端 RPC）；拖拽排序仅本地（需后端持久化 API）
