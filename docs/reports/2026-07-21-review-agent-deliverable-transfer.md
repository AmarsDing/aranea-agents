# Review: Agent 间交付物传递机制（P0 + O-4 数据源修复）

> **日期**：2026-07-21 | **类型**：收尾评审 | **结论**：通过，无遗留阻断问题
> **关联**：[67-organization-redesign.design.md](../development/67-organization-redesign.design.md) / [67-organization-redesign.development.md](../development/67-organization-redesign.development.md)（XC-03b / XC-03d）

---

## 1. 评审范围

Agent 间消息链结构化交付的 P0 实施：精灵（Spirit）按计划派出团队执行，团队间指令输入与执行结果沿 DAG 执行流程结构化交付。

**本批次变更集**（O-4 修复，未提交工作区，10 文件，+275/-14）：

| 文件 | 变更 |
|------|------|
| `internal/biz/spirit_team_usecase.go` | 新增 `SpiritStepReader` 窄接口（Stability:evolving）；`ExtractTeamOutput` 主源切换为步骤读取 |
| `internal/biz/repo_ports_v2.go` | `StepV2Reader` 新增 `ListStepsBySessionID`（精确 session_id 语义） |
| `internal/data/step_v2_repo.go` | 实现 `ListStepsBySessionID`；澄清 `ListStepsBySession` 为树级语义（前端历史加载依赖） |
| `internal/data/step_v2_repo_test.go` | 精确语义测试用例（+38） |
| `internal/biz/spirit_team_deliverable_test.go` | 5 个新行为用例（+135） |
| `internal/service/spirit_team.go` | `recordTeamCompletion` 调用点（completed 时先于下游调度） |
| `cmd/admin/wire.go` / `wire_gen.go` | `WithSpiritStepReader` DI 接线 |
| `docs/development/67-*.design.md` / `*.development.md` | O-4 数据源设计补充 + XC-03d 任务记录 |

**前序已提交**（P0）：`teams.deliverables_output_json` 专用列（TECH-DEBT #B-03）、`WriteDeliverablesToSession` 内联进 `RecordTeamCompletion`、`InjectUpstreamDeliverables` 双路径接线（v2 `Orchestrate` 注入 turnContent + v1 `DependentTeamAction.TaskDescription`）、`Orchestrate` 透传 `step.DependsOn`。

## 2. 迭代评审发现与处置（全部闭环）

| # | 发现 | 严重度 | 处置 |
|---|------|--------|------|
| O-1 | DAG 下游 `taskDesc` 未注入上游交付物；生产主路径 `RealTeamOrchestrator.Orchestrate` 未透传依赖 | 高 | 双路径接线注入，已修复 |
| O-2 | 交付物存储超载 `ParallelConfigJSON`（字段语义错误，且 Update 白名单不透传、从未真正持久化） | 高 | 新增 `deliverables_output_json` 专用列，已修复 |
| O-3 | 交付物缓存 JSON 损坏时写入失败 | 中 | 解析失败告警并重建缓存，已修复 |
| O-4 | `ExtractTeamOutput` 读取 `ListStepsBySession`（spirit_session_id 树级过滤）取不到团队主会话步骤，交付物实际为空 | 高 | 拆分语义：新增 `ListStepsBySessionID` 精确查询 + `SpiritStepReader` 窄接口注入；团队主会话按 `SessionType=team` 识别（成员会话共享 team_id 且 Search 无序）；无 reply step 回退 `ListMessagesRecent` |

## 3. 架构合规检查

| 检查项 | 结果 |
|--------|------|
| 接口窄化（biz port ≤5 方法） | ✅ `SpiritStepReader` 仅 1 方法；`StepV2Reader` 扩至 5 方法仍达上限 |
| 接口稳定性标注（AS-STA-01） | ✅ `SpiritStepReader` 标注 `Stability:evolving` |
| 依赖方向（biz 不依赖 trpc-agent-go） | ✅ 无新增违规 import |
| DB 红线 | ✅ 读路径走 `RW().Read(ctx)`（DB-R6）；错误经 `entErrToBizErr` 翻译（DB-R5）；无新连接（DB-R1） |
| 日志红线 #16 | ✅ 全部 `loggateway.Logger` + 结构化字段，错误用 `loggateway.Err` |
| 文档同步（DOC-SYNC-1/5/6） | ✅ design.md 补充 O-4 数据源设计；development.md XC-03d 标记 ✅ 并附验证证据 |
| 语义兼容性 | ✅ `ListStepsBySession` 树级语义保留，前端历史加载（fetchSessionHistory）不受影响 |

## 4. 验证证据

| 层级 | 证据 | 结果 |
|------|------|------|
| 编译 | `go build ./internal/team/... ./internal/biz/... ./internal/data/... ./internal/service/...` | ✅ 通过 |
| 静态检查 | `go vet ./internal/biz/... ./internal/data/... ./internal/service/...` | ✅ 通过 |
| 单元测试 | deliverable 测试 5 个新行为用例（stepReader 优先 / 无 reply 回退消息 / 多 session 按 SessionType 识别 / 无 stepReader 回退 / running reply 不采用）+ step_v2_repo 精确语义用例 | ✅ 绿 |
| 全量回归 | `go test ./internal/biz/... ./internal/data/... ./internal/service/... -count=1` | ✅ biz 13.1s / data 20.8s / service 10.8s，无 FAIL/panic |
| 端到端运行时 | 两步串行 DAG（调研→写作）：上游交付物落库 `teams.deliverables_output_json` 与 reply step 内容一致；下游回复完全基于上游专有内容；`write_deliverables` 日志双向确认 | ✅ 通过 |

## 5. 遗留事项（非阻断，按 roadmap 推进）

| 项 | 说明 | 归属 |
|----|------|------|
| 形式契约填充 | `Orchestrate` 已透传 `step.DependsOn`；`DeliverableContract` 字段由 Planner schema 输出属 P1 | P1 planner 扩展 |
| 产物引用化 | 交付物当前为文本摘要注入；大产物引用（artifact URI）传递属 P2 架构演进 | P2 |
| Graph StateFields 注入 | 交付物注入 Graph StateFields 替代 User Message 前缀 | 后续迭代 |

**注**：工作区另存在知识库模块（`internal/tools/knowledge` US-14 等）的独立改动与测试失败，属其他工作流，不在本评审范围。
