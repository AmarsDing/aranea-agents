# Multi-Agent 编排 — 开发计划

> **版本**：2026-05-23 | **状态**：✅ Phase 1–3 已完成；**M53 Phase 7**：Team Run 默认 GraphAgent（Native 仅 `ARANEA_TEAM_NATIVE=1` 应急）— 见 [53-team-graph-orchestration-development.md §Phase 7](./53-team-graph-orchestration-development.md#phase-7--单链终态p3)
> **需求**：[11 multi-agent.md](./11%20multi-agent.md) · **设计**：[11 multi-agent.design.md](./11%20multi-agent.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-05 · EP-TEST-01
> **变更**：[changelog/2026-05-21-Multi-Agent-P1-P3.md](../changelog/2026-05-21-Multi-Agent-P1-P3.md) · [Review P0–P1](../changelog/2026-05-21-Multi-Agent-Review-P0-P1.md) · [Review P2](../changelog/2026-05-21-Multi-Agent-Review-P2.md)

---

## 1. 模块定位

Multi-Agent 编排：Team 模式（sequential / parallel / coordinator / critic_loop / swarm / adaptive），Agent 间协作、运行观测与 Chat 集成。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| Proto | `api/kratos/team/v1/team.proto` |
| Service | `internal/service/team.go`（14 RPC，含 GetTeamRunSummary） |
| Biz | `internal/biz/team_usecase.go` · `team_types.go` · `team_summary.go`（GetRunSummary） |
| Data | `internal/data/team_repo.go` |
| Runtime | `internal/team/` · `summary.go`（WS map） |
| 事件 | `internal/agent/event_projector.go` · `turn_helpers.go`（MemberToolCalls） |
| 前端 | `TeamsPage` · `TeamTestDialog` · `TeamRunsDialog` · `ChatTeamMemberStrip` |

---

## 2. 现状评估（2026-05-21）

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

### 2.2 前端

| 项 | 状态 |
|----|------|
| RunTeamTest UI（TeamTestDialog） | ✅ |
| TeamRuns 汇总 RPC + WS 回放 banner | ✅ |
| Chat Team 成员 strip | ✅ |
| adaptive ↔ Swarm 文案 | ✅ |

---

## 3. 差距与优化（后续）

| ID | 优先级 | 待优化项 |
|----|--------|----------|
| TEAM-10 | P3 | TeamRuns 自动加载汇总（展开 run 时） |
| TEAM-11 | P3 | 历史 run 无 tool_call_count 时 Usage 回填 |
| TEAM-12 | P4 | swarm 独立 UI 模式（与 adaptive 二选一或合并） |

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

---

## 5. 验收标准

### Phase 3（已完成）

- [x] Team 管理页可 RunTeamTest 并看到回复
- [x] TeamRuns 步骤有 started/finished 与工具调用数
- [x] `GET /v1/team-runs/{id}/summary` 返回结构化汇总
- [x] Chat Team 会话顶部展示成员流式 chip
- [x] Runner persistStep 单测覆盖 started/finished Envelope

---

## 6. 依赖与风险

- 历史 run 在 tool_call_count 字段上线前步骤计数为 0；**已有库**执行 `docs/sql/03_session_team_run_steps_tool_call_count.sql`
- GetTeamRunSummary 经 `TeamUsecase.GetRunSummary`；与 WS `team_summary` 共用 `biz.BuildTeamRunSummaryData`（RPC 经 `toProtoTeamRunSummary`，WS 经 `SummaryMapFromData`），字段由 parity 测试保障一致
