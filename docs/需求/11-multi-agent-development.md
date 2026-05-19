# Multi-Agent 编排 — 开发规划文档

> **版本**：2026-05-19 | **状态**：✅ 核心功能已完成；🟡 A2A / WS member 事件 / 汇总待实现
> **需求**：[11 multi-agent.md](./11%20multi-agent.md) · **设计**：[11 multi-agent.design.md](./11%20multi-agent.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-05

---

## 1. 模块定位

Multi-Agent 编排：Team 模式（sequential / parallel / coordinator / critic_loop / swarm），Agent 间协作与消息传递。

**代码锚点**：

- `api/kratos/team/v1/team.proto` — Team CRUD + Run 管理 + 高级功能
- `internal/service/team.go` — TeamService（13 个 RPC 方法）
- `internal/biz/team_usecase.go` — TeamUsecase + TeamRepository
- `internal/biz/team_types.go` — 领域模型（Team / TeamRun / TeamRunStep / TeamStructureSnapshot）
- `internal/data/team_repo.go` — TeamRepo 实现
- `internal/team/definition.go` — Definition / SwarmConfigDef / MemberToolDef / CriticLoopConfig
- `internal/team/trpc_build.go` — BuildTRPCTeam / buildSwarmOptions / buildEscalationFunc / buildCoordinatorOptions
- `internal/team/runner_team_trpc.go` — Team 运行时（trpc-agent-go Team 编排）
- `internal/event` + `internal/server/ws.go` — Team 事件统一通过 EventBus / WebSocket Envelope 推送

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Team CRUD | ✅ | List / Create / Get / Update / Delete / Duplicate |
| 五种编排模式 | ✅ | sequential / parallel / coordinator / critic_loop / swarm |
| Team Run 管理 | ✅ | ListTeamRuns / GetTeamRun / CancelTeamRun / ListTeamRunSteps |
| SwarmConfig 安全限制 | ✅ | MaxHandoffs / NodeTimeout / RepetitiveHandoff / CrossRequestTransfer |
| MemberToolConfig | ✅ | StreamInner / InnerTextMode / HistoryScope / SkipSummarization |
| 动态成员管理 | ✅ | UpdateSwarmMembers API |
| 结构导出 | ✅ | ExportTeamStructure API |
| escalationFunc 增强 | ✅ | 支持 ScoreThreshold 结构化评分 |
| Team 对话 | ✅ | Chat WS / HTTP unary → `owner_type == "team"` |
| RunTeamTest | ⏳ | Service 桩实现，返回 501 |
| A2A call_agent 工具 | ❌ | `call_agent` 未注入 Agent 工具集 |
| member_* WS 事件 | ❌ | Team 对话不稳定发射子 Agent 实时流 Envelope |
| Team 运行结果结构化汇总 | ❌ | 无成员贡献度 / 工具调用统计 |

---

## 3. 差距与优化

### P0 — RunTeamTest 端到端

当前 `RunTeamTest` Service 层返回 501 Unimplemented。需要：
- 创建临时 Session（owner_type=team）
- 调用 Team Runtime 执行测试消息
- 返回 TeamRun 和回复内容
- 清理临时 Session

### P1 — A2A call_agent 工具注入

`internal/agent/trpc_build.go` 中无 `call_agent` 注入，Agent 无法在 Team 中调用远程 Agent。需要：
- 在 Agent 工具集中注入 `call_agent` 工具
- 支持跨 Team Agent 调用
- 依赖 M26 a2a-protocol 的 `call_agent` 工具实现

### P2 — member_* WS 事件

Team 对话时 `member_message_start/delta/done` WS Envelope 事件未稳定发射，前端无法展示子 Agent 实时流。需要：
- 在 `chat_native.go` 的 Team turn 中发射 member_* 事件
- 与 EventBus / WS 管道对齐
- 前端接收并渲染子 Agent 增量输出

### P3 — Team 运行结果结构化汇总

Team 运行结果无结构化汇总（如各成员贡献度、工具调用统计）。需要：
- 新增 TeamRunSummary API
- 聚合各成员的 token 消耗、耗时、工具调用次数
- 按成员角色分类统计

---

## 4. 开发阶段

### Phase 1（已完成）— 核心功能

- ✅ Team CRUD + 编辑器
- ✅ 五种编排模式（sequential / parallel / coordinator / critic_loop / swarm）
- ✅ Team Run / Step 持久化
- ✅ Chat Team session
- ✅ SwarmConfig / MemberToolConfig / CriticLoopConfig
- ✅ UpdateSwarmMembers / ExportTeamStructure
- ✅ GetTeamRun / CancelTeamRun
- ✅ escalationFunc ScoreThreshold 支持

### Phase 2（当前）— 运行管理完善

- ⏳ RunTeamTest 端到端实现
- ⏳ CancelTeamRun 实际取消逻辑（当前仅修改状态）

### Phase 3 — A2A 与实时增强

- ❌ A2A call_agent 工具注入
- ❌ member_* WS 事件发射
- ❌ WS 增强：step_started / 进度百分比 / 事件回放

### Phase 4 — 高级分析

- ❌ Team 运行结果结构化汇总 API
- ❌ 成员贡献度 / 工具调用统计
- ❌ 运行对比分析

---

## 5. 任务清单

| # | 任务 | 优先级 | EP | 状态 |
|---|------|--------|-----|------|
| 1 | RunTeamTest 端到端实现 | P0 | EP-BIZ-05 | ⏳ 桩实现 |
| 2 | CancelTeamRun 实际取消逻辑 | P0 | EP-BIZ-05 | ⏳ 仅改状态 |
| 3 | `trpc_build.go`：注入 `call_agent` 工具 | P1 | EP-BIZ-05 | ❌ 未开始 |
| 4 | `chat_native.go`：Team turn 中发射 member_* 事件 | P2 | — | ❌ 未开始 |
| 5 | TeamRunSummary 结构化汇总 API | P3 | — | ❌ 未开始 |
| 6 | WS 增强：step_started / 进度 / 回放 | P3 | — | ❌ 未开始 |
| 7 | 单测覆盖 TeamRunner 关键路径 | P1 | EP-TEST-01 | ❌ 未开始 |

---

## 6. 验收标准

### Phase 1（已完成）

- [x] 用户能创建、编辑、删除 Team
- [x] Team 可绑定多个现有 Agent
- [x] 五种编排模式可完整执行
- [x] 每次运行记录 run 和 step 摘要
- [x] SwarmConfig / MemberToolConfig 已实现
- [x] 动态成员管理和结构导出 API 已实现
- [x] escalationFunc 支持 ScoreThreshold

### Phase 2

- [ ] RunTeamTest 可端到端执行并返回结果
- [ ] CancelTeamRun 可实际取消正在运行的 Team Run

### Phase 3

- [ ] Agent 可在 Team 中通过 `call_agent` 调用其他 Agent
- [ ] Team 对话前端可实时看到子 Agent 增量输出

### Phase 4

- [ ] Team 运行结果可查看结构化汇总（成员贡献度、工具调用统计）

---

## 7. 依赖与风险

- A2A call_agent 依赖 M26 a2a-protocol 的 `call_agent` 工具实现
- member_* 事件需与 EventBus / WS 管道对齐
- RunTeamTest 需要创建临时 Session，需考虑并发安全和资源清理
- CancelTeamRun 需要与 trpc-agent-go 运行时上下文取消机制集成
