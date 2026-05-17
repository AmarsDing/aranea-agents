# Multi-Agent 编排 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用；🟡 A2A 未连通
> **需求**：[11 multi-agent.md](./11%20multi-agent.md) · **设计**：[11 multi-agent.design.md](./11%20multi-agent.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-05

---

## 1. 模块定位

Multi-Agent 编排：Team 模式（sequential/parallel/coordinator/critic_loop/swarm），Agent 间协作与消息传递。

**代码锚点**：
- `api/kratos/team/v1/` — Team CRUD + RunTurn
- `internal/service/team.go` — TeamService
- `internal/biz/team.go` — TeamUsecase + TeamRepo
- `internal/agent/team_runner.go` — team.Runner（trpc-agent-go Team 编排）
- `internal/agent/trpc_build.go` — BuildTRPCTeam

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Team CRUD | ✅ | Create/Update/Delete/Get/List |
| Team 模式 | ✅ | sequential / parallel / coordinator / critic_loop / swarm |
| Team RunTurn | ✅ | `teamsNative.RunTurn` |
| Team 成员管理 | ✅ | `team_members` 关联 |
| Team 对话 | ✅ | Chat SSE → `owner_type == "team"` |
| A2A call_agent 工具 | ❌ | `call_agent` 未注入 Agent 工具集 |

---

## 3. 差距与优化

1. **P1（EP-BIZ-05）**：A2A `call_agent` 工具未注入 Agent 工具集（`internal/agent/trpc_build.go` 中无 `call_agent` 注入），Agent 无法在 Team 中调用远程 Agent。
2. **P2**：Team 对话时 `member_message_start/delta/done` SSE 事件未发射，前端无法展示子 Agent 实时流。
3. **P3**：Team 运行结果无结构化汇总（如各成员贡献度、工具调用统计）。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-05）**：注入 `call_agent` 工具到 Agent 工具集
- **Phase 2**：Team 对话发射 member_* SSE 事件
- **Phase 3**：Team 运行结果结构化汇总

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `trpc_build.go`：注入 `call_agent` 工具 | P1 | EP-BIZ-05 |
| 2 | `chat_native.go`：Team turn 中发射 member_* 事件 | P2 | — |
| 3 | TeamRunResult 结构化汇总 API | P3 | — |
| 4 | 单测覆盖 TeamRunner 关键路径 | P1 | EP-TEST-01 |

---

## 6. 验收标准

- [ ] Agent 可在 Team 中通过 `call_agent` 调用其他 Agent
- [ ] Team 对话前端可实时看到子 Agent 增量输出
- [ ] `go test ./internal/agent/... -run TestTeam` 通过

---

## 7. 依赖与风险

- A2A 依赖 M26 a2a-protocol 的 `call_agent` 工具实现
- member_* 事件需与 SSE 管道对齐
