# Evaluation 评估 — 开发计划

> **版本**：2026-05-20（修订）| **状态**：🟡 基础评估 + 人工标注 + 报告导出（客户端）可用；DeleteRun/AfterTurn/框架 Evaluator 待补
> **需求**：[33 evaluation.md](./33%20evaluation.md) · **设计**：[33 evaluation.design.md](./33%20evaluation.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-DATA-01, EP-RT-08, EP-BIZ-04

---

## 1. 模块定位

Evaluation 评估：对 Agent 的输出质量进行结构化评估，支持自动评估（含 LLM-as-Judge）和人工评估，评估结果用于 Agent 迭代优化。

**代码锚点**：
- `api/kratos/evaluation/v1/evaluation.proto` — Evaluation HTTP+gRPC API
- `internal/service/evaluation.go` — EvaluationService（proto ↔ biz 映射）
- `internal/biz/evaluation.go` — EvalUsecase + EvalRepo 接口
- `internal/data/evaluation.go` — EvalRepo 实现 + EnsureEvalSchema
- `internal/evaluation/runner.go` — Runner（异步执行 + 4 种指标）
- `internal/service/wire_providers.go` — NewEvaluationRunner 注入
- `internal/server/http.go` / `grpc.go` — HTTP+gRPC 注册

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Dataset CRUD | ✅ | Create/Get/List/Delete + UploadCases |
| EvalRun 创建/查询 | ✅ | CreateRun/GetRun/ListRuns/GetRunResults |
| 异步 Runner | ✅ | `runner.go`（goroutine + safego） |
| 4 种内置指标 | ✅ | exact_match / contains_match / llm_as_judge / tool_call_accuracy |
| AgentRunner 注入 | ✅ | `NewEvaluationRunner` 经 ChatService.RunNativeTurnUnary（EP-BIZ-04 ✅） |
| HTTP+gRPC 注册 | ✅ | `http.go` / `grpc.go` |
| Prometheus 指标 | ✅ | eval_runs_total / eval_case_duration_seconds |
| EnsureEvalSchema 启动调用 | ❌ | 函数已定义，未在 NewData() 中调用（EP-DATA-01） |
| LLM-as-Judge 实现 | ❌ | LLMJudge hook 注入为 nil，静默跳过 |
| DeleteRun API | ❌ | Proto 和 Service 均无 DeleteRun |
| UpdateDataset API | ❌ | 无更新数据集接口 |
| 自动评估触发 | ❌ | 无 AfterTurn Hook 机制 |
| 人工评估标注 | ✅ | `AnnotateCaseResult` + Results 对话框 |
| 评估报告导出 | ✅ | `exportRunResults.ts` CSV/JSON（客户端，迭代 7） |
| 评估报告服务端生成 | ❌ | 无聚合 PDF/定时报告 API |
| 前端页面 | ✅ | EvaluationPage.vue；路由 /evaluation 已注册；features/evaluation/ api/types/mapper/store 均已实现 |
| trpc AgentEvaluator 集成 | ❌ | 使用自建 Runner，未集成框架评估器 |
| MultiRun / UserSimulation | ❌ | 未实现 |
| ToolTrajectory / FinalResponse 多维度 | ❌ | 仅有简化版 exact/contains/tool_call_accuracy |

---

## 3. 差距与优化

1. **P0**：`EnsureEvalSchema` 未在 `NewData()` 启动期调用，无表时首跑失败（EP-DATA-01）
2. **P1**：LLM-as-Judge 未实现，llm_as_judge 指标永远跳过
3. ~~**P1**~~：前端 Evaluation 页面已实现（EvaluationPage.vue）✅
4. **P2**：无自动评估触发机制，评估需手动触发
5. **P2**：无 DeleteRun / UpdateDataset API，管理能力不完整
6. ~~**P2**~~：评估报告导出（客户端 CSV/JSON）✅ 迭代 7
7. ~~**P3**~~：人工评估标注 ✅
8. **P3**：未集成 trpc AgentEvaluator，无法使用框架高级能力（MultiRun/UserSimulation/ToolTrajectory 等）

---

## 4. 开发阶段

- **Phase 1**：基础补全（Schema 启动调用 + LLM-as-Judge + API 补全）
- **Phase 2**：前端评估页面（数据集管理 + 运行评估 + 结果查看）
- **Phase 3**：自动评估触发 + 评估报告
- **Phase 4**：人工评估标注
- **Phase 5**：trpc AgentEvaluator 集成（高级评估能力）

---

## 5. 任务清单

| # | 任务 | 优先级 | EP | 阶段 |
|---|------|--------|-----|------|
| 1 | `NewData()` 启动期调用 `EnsureEvalSchema` | P0 | EP-DATA-01 | Phase 1 |
| 2 | 实现 LLM-as-Judge（接入 Provider/Model 配置，注入 LLMJudge hook） | P1 | — | Phase 1 |
| 3 | 补全 DeleteRun API（Proto + Biz + Data + Service） | P2 | — | Phase 1 |
| 4 | 补全 UpdateDataset API（名称/描述更新） | P2 | — | Phase 1 |
| 5 | 前端：评估数据集管理页（列表/创建/删除/上传用例） | P1 | — | Phase 2 | ✅ EvaluationPage.vue 已集成 |
| 6 | 前端：运行评估页（选择数据集+Agent → 启动 → 查看进度） | P1 | — | Phase 2 | ✅ EvaluationPage.vue 已集成 |
| 7 | 前端：评估结果页（汇总分数 + 逐用例详情） | P1 | — | Phase 2 | ✅ EvaluationPage.vue 已集成 |
| 8 | 自动评估触发（AfterTurn Hook + 关联数据集配置） | P2 | — | Phase 3 |
| 9 | 评估报告导出（JSON/CSV，客户端） | P2 | ✅ | Phase 3 · 迭代 7 |
| 10 | 人工评估标注（标注字段 + API + 前端） | P3 | ✅ | Phase 4 |
| 11 | 集成 trpc AgentEvaluator（替换自建 Runner） | P3 | — | Phase 5 |
| 12 | 引入 EvalSet 完整模型 + ToolTrajectory/FinalResponse 评估 | P3 | — | Phase 5 |
| 13 | MultiRun + UserSimulation + pass@k | P3 | — | Phase 5 |

---

## 6. 验收标准

- [ ] `NewData()` 启动后 eval_* 表自动创建，无需手动建表
- [ ] LLM-as-Judge 可配置 Judge 模型并返回有效分数
- [ ] 可通过 API 删除评估运行、更新数据集
- [x] 前端可管理评估数据集、运行评估、查看结果（EvaluationPage.vue ✅）
- [ ] Agent 运行后可自动触发评估
- [ ] 评估结果可人工标注
- [x] 可导出评估报告（CSV/JSON，结果对话框）
- [ ] `go test ./internal/evaluation/...` 通过
- [ ] `go test ./internal/biz/... -run Eval` 通过
- [ ] `go test ./internal/data/... -run Eval` 通过

---

## 7. 依赖与风险

- **LLM Token 成本**：LLM-as-Judge 每条用例消耗一次 LLM 调用，100 用例 ≈ 100 次 Judge 调用
- **自动评估与 Chat 流程耦合**：AfterTurn Hook 需确保评估失败不影响正常对话
- **trpc 框架集成复杂度**：AgentEvaluator 依赖 trpc Runner 接口，需适配当前 Kratos+ChatService 架构
- **Schema 迁移**：Phase 5 引入 EvalSet 完整模型时需考虑现有 eval_* 表的数据迁移
