# 11 Team / Multi-Agent Review

> **评分**：83 / 100 | **风险等级**：P1  
> **文档**：[11-multi-agent-development.md](../需求/11-multi-agent-development.md)  
> **代码锚点**：`internal/team/` · `internal/service/team.go` · `internal/biz/team_usecase.go` · `internal/biz/team_summary.go`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | 编排 + RunTest + Summary + WS P3 闭环完成；RunTest UI / step_started / Summary RPC 仍待补 |
| 架构一致性 | 22 | 25 | `internal/team` 独立层正确；与 `internal/agent` 同级；共用 RunGateway 和 TurnDeps ✅ |
| 后端实现质量 | 18 | 20 | 5 种模式（chain/parallel/coordinator/critic_loop/swarm）已实现；`team_summary` Envelope + `BuildTeamRunSummary` ✅ |
| 前端实现质量 | 14 | 15 | Team 卡片 + 编辑对话框 + RunsDialog ✅；WS `member_*` / `team_summary` 联动 ✅；RunTest UI 待补 |
| 测试与验证 | 6 | 10 | `team_summary_parity_test.go` ✅；RunTeamTest 集成测试缺失 |
| 文档一致性 | 6 | 10 | 开发计划 P3 闭环同步良好；11-multi-agent-development.md 已更新 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| Team CRUD + 5 种模式 | ✅ |
| Team Runner 装配（BuildTRPCTeam） | ✅ |
| `RunTeamTest` RPC | ✅ |
| `CancelTeamRun` | ✅ |
| `member_*` WS Envelope | ✅ |
| `team_summary` Envelope + `BuildTeamRunSummary` | ✅ P3 |
| `EnvelopeTypeTeamSummary` | ✅ |
| `PluginsForAgent` scope 过滤 | ✅ |
| Team 成员分栏 UX | ✅ |
| Team RunsDialog | ✅ |
| RunTest UI（`TeamTestDialog`） | ✅ |
| `team_step_started` Envelope | ✅ `runner_helpers.go` |
| `GetTeamRunSummary` RPC + 前端展示 | ✅ |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| TEAM-P1-01 | Team `team_summary` 仅用内存数据聚合；跨进程重启后历史 summary 丢失 | 将 summary 持久化到 `team_run_steps` 表 |
| TEAM-P1-02 | 无端到端集成测试覆盖五种模式的完整 Turn 流程 | 补各模式 E2E 集成测试 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| TEAM-P2-01 | 5 种模式差异化测试缺失（尤其 swarm / critic_loop） | 补各模式单测 |
| TEAM-P2-02 | 跨 Team 编排（一个 Team 调用另一个 Team）可观测性弱 | 规划跨 Team trace 关联 |

---

## Team 5 种运行模式

| 模式 | 描述 | 状态 |
|------|------|------|
| `chain` | 顺序传递输出到下一 Agent | ✅ |
| `parallel` | 并行调用所有成员 Agent | ✅ |
| `coordinator` | 协调者决定下一步由谁处理 | ✅ |
| `critic_loop` | 生成者 + 评估者反复迭代 | ✅ |
| `swarm` | 动态传递，成员自主决定移交 | ✅ |

---

## 建议优化路径

1. 持久化 team_summary 数据到 `team_run_steps` 表（P1）。
2. 补五种模式的端到端集成测试（P1）。
3. 规划跨 Team 编排可观测性（P2）。
