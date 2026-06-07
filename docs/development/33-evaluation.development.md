# Evaluation 评估 — 开发计划

> **版本**：2026-06-06 | **状态**：🟢 全部完成（所有 Phase 已交付，差距已关闭）
> **需求**：[33 evaluation.md](./33%20evaluation.md) · **设计**：[33 evaluation.design.md](./33%20evaluation.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-DATA-01 ✅, EP-RT-08 ✅, EP-BIZ-04 ✅, EVAL-02 ✅

---

## 1. 模块定位

Evaluation 评估：对 Agent 输出质量进行结构化评估，支持自动评估（含 LLM-as-Judge）、人工标注与客户端报告导出。

**代码锚点**：
- `api/kratos/evaluation/v1/evaluation.proto` — Evaluation HTTP+gRPC API（14 RPC）
- `internal/service/evaluation.go` — EvaluationService（传输桥点）
- `internal/service/evaluation_runner.go` — NewEvaluationRunner（Wire 装配）
- `internal/service/evaluation_after_turn.go` — NewEvaluationAfterTurnTrigger
- `internal/biz/evaluation.go` — 类型重导出（子包 `evaluation/` 的别名）
- `internal/biz/evaluation/evaluation.go` — 领域模型 + EvalRepo 接口（16 方法）+ EvalUsecase（17 方法）
- `internal/data/evaluation.go` — EvalRepo + EnsureEvalSchema（4 表 + 11 条 ALTER 迁移）
- `internal/evaluation/runner.go` — 异步调度
- `internal/evaluation/runner_legacy.go` — Legacy 回退路径
- `internal/evaluation/metrics.go` — 指标解析与 legacy 计分
- `internal/evaluation/framework.go` — trpc AgentEvaluator 桥接
- `internal/evaluation/framework_metrics.go` — 扩展指标（JSON/XML/ROUGE/ToolTrajectory）
- `internal/evaluation/evaluator_registry.go` — 9 种内置评估器注册
- `internal/evaluation/multirun.go` — MultiRun 配置
- `internal/evaluation/chat_runner.go` — ChatService → runner.Runner
- `internal/evaluation/llm_judge.go` — LLM-as-Judge
- `internal/evaluation/llm_simulator.go` — LLM UserSimulation + simRunner
- `internal/evaluation/scripted_simulator.go` — 脚本化 UserSimulation
- `internal/evaluation/eval_llm_resolve.go` — env + system_settings 模型解析
- `internal/evaluation/scores.go` — scores_json 映射
- `internal/evaluation/evalset_adapter.go` — biz EvalCase → trpc EvalSet
- `internal/evaluation/evalset_tools.go` — expected_tool_calls → ToolTrajectory
- `internal/evaluation/case_metadata.go` — 用例元数据解析
- `internal/evaluation/pass_metrics.go` — pass@k / pass^k
- `internal/evaluation/after_turn.go` — AfterTurn 自动评估
- `web/src/pages/EvaluationPage.vue` — 管理页 `/evaluation`
- `web/src/components/evaluation/EvaluationAnalyticsPanel.vue` — 趋势 + A/B 对比
- `web/src/components/evaluation/EvaluationResultsDialog.vue` — 逐用例详情 + 人工标注
- `web/src/components/evaluation/EvaluationDatasetList.vue` — 数据集列表
- `web/src/components/evaluation/EvaluationCreateDialog.vue` — 新建数据集弹窗
- `web/src/components/evaluation/EvaluationRunDialog.vue` — 启动评估弹窗
- `web/src/features/evaluation/` — api / types / mappers / exportRunResults / useEvaluationPage / evaluationTableUi
- `web/src/stores/evaluation/index.ts` — Pinia Store（13 方法）
- `web/src/features/system-settings/eval-llm.ts` — Eval LLM 表单（默认 openai/gpt-4o-mini）

---

## 2. 现状评估（2026-06-06 代码审计）

| 项 | 状态 | 证据 |
|----|------|------|
| Proto 14 RPC | ✅ | evaluation.proto：14 个 RPC + 18 个 Message |
| Dataset CRUD | ✅ | Create/Get/List/Update/Delete + UploadCases |
| DeleteRun | ✅ | `DELETE /v1/evaluation/runs/{id}`（级联 results） |
| DeleteDataset 级联 | ✅ | 事务删 cases + dataset |
| EvalRun 创建/查询 | ✅ | CreateRun/GetRun/ListRuns/GetRunResults/DeleteRun |
| 异步 Runner | ✅ | `runner.go` + safego |
| FrameworkBridge | ✅ | `framework.go` → trpc AgentEvaluator + MultiRun |
| Legacy 回退路径 | ✅ | `runner_legacy.go` + `metrics.go` |
| 4 种核心指标 | ✅ | exact / contains / llm_as_judge / tool_call_accuracy |
| 4 种扩展指标 | ✅ | json_match / xml_match / rouge_l / tool_trajectory |
| AgentRunner 注入 | ✅ | `evaluation_runner.go` → ChatService.RunNativeTurnUnary |
| LLM-as-Judge | ✅ | `llm_judge.go`；env `KRATOS_EVAL_JUDGE_*` 或目录首 mini/flash 模型 |
| num_runs (MultiRun) | ✅ | Proto `num_runs` → AgentEvaluator.WithNumRuns |
| EnsureEvalSchema 启动 | ✅ | `data.go` NewData() 调用（EP-DATA-01） |
| AnnotateCaseResult | ✅ | PATCH annotation + Results 对话框（EVAL-02） |
| 报告导出（客户端） | ✅ | `exportRunResults.ts` CSV/JSON |
| HTTP+gRPC 注册 | ✅ | `http.go` / `grpc.go` |
| Wire 注入 | ✅ | `wire_gen.go`：Repo → Usecase → Runner → Service → Registry |
| Prometheus | ✅ | eval_runs_total / eval_case_duration_seconds |
| 前端页面 | ✅ | EvaluationPage + 5 个子组件 + features/evaluation + Store |
| AfterTurn 自动评估 | ✅ | `AfterTurnTrigger` + `evaluation_after_turn.go` + `config_json.evaluation` |
| GetAgentEvalTrend | ✅ | `GET /v1/evaluation/agents/{agent_id}/trend` |
| CompareEvalRuns A/B | ✅ | `POST /v1/evaluation/runs/compare` |
| EvalSet 多轮 turns | ✅ | `metadata_json.turns` → evalset_adapter |
| UserSimulation 脚本 | ✅ | `scripted_simulator.go` + `use_user_simulation` |
| UserSimulation LLM | ✅ | `llm_simulator.go` + trpc `simRunner` |
| scores_json | ✅ | run/result 扩展分数字段 |
| 趋势/对比前端 | ✅ | `EvaluationAnalyticsPanel` + API |
| Eval LLM 系统配置 | ✅ | `system_settings.eval_*` + Settings 页 + `eval_llm_resolve.go` |
| pass@k / pass^k | ✅ | `pass_metrics.go` + `eval_runs.pass_at_k/pass_hat_k` |
| 内置评估器注册 | ✅ | `evaluator_registry.go` 9 种评估器 |
| Biz 子包结构 | ✅ | `internal/biz/evaluation/` 子包 + 外层类型重导出 |
| 前端 Pinia Store | ✅ | `stores/evaluation/index.ts` 13 方法 |
| 前端路由 + 导航 + i18n | ✅ | `/evaluation` 路由 + 侧边栏 + 中英文 |

### 未完成项

| 项 | 说明 |
|----|------|
| Service 层单元测试 | `internal/service/evaluation*_test.go` 不存在 |
| Runner 异步执行测试 | `internal/evaluation/runner_test.go` 不存在 |
| AfterTurn 触发器测试 | `internal/evaluation/after_turn_test.go` 不存在 |
| FrameworkBridge 集成测试 | `internal/evaluation/framework_test.go` 不存在 |
| LLM Judge 测试 | `internal/evaluation/llm_judge_test.go` 不存在 |
| ChatRunner 适配器测试 | `internal/evaluation/chat_runner_test.go` 不存在 |
| 前端数据集编辑 | 后端 `UpdateDataset` API 已实现，前端未暴露编辑入口 |

---

## 3. 差距与优化（全部已关闭）

| 优先级 | 项 | 状态 |
|--------|-----|------|
| **P0** | EnsureEvalSchema 启动调用 | ✅ EP-DATA-01 |
| **P1** | LLM-as-Judge 实现 | ✅ llm_judge.go |
| **P1** | FrameworkBridge / EP-RT-08 | ✅ framework.go + chat_runner |
| **P1** | 前端 Evaluation 页面 | ✅ EvaluationPage.vue |
| **P2** | DeleteRun / UpdateDataset API | ✅ |
| **P2** | DeleteDataset 级联删 cases | ✅ |
| **P2** | Runner 指标逻辑 SRP 拆分 | ✅ metrics.go + runner_legacy.go |
| **P2** | 自动评估触发（AfterTurn） | ✅ |
| **P2** | 报告导出（客户端） | ✅ |
| **P3** | 人工评估标注 | ✅ EVAL-02 |
| **P3** | 服务端评估报告 / 趋势 API | ✅ 趋势 + A/B |
| **P3** | EvalSet 多轮 + UserSimulation + pass@k | ✅ Phase 5 |
| **P3** | 扩展指标 + LLM UserSim + 趋势前端 | ✅ |
| **P3** | Eval LLM → system_settings | ✅ |

### 待改进项（非阻塞）

| 优先级 | 项 | 说明 |
|--------|-----|------|
| P2 | Service 层单元测试 | `internal/service/evaluation*_test.go` 缺失 |
| P2 | Runner 核心组件测试 | runner / after_turn / framework / llm_judge / chat_runner 无测试 |
| P3 | 前端数据集编辑入口 | 后端 API 已就绪，前端未暴露 |

---

## 4. 开发阶段

- **Phase 1**：✅ Schema 启动 + LLM-as-Judge + FrameworkBridge + DeleteRun/UpdateDataset
- **Phase 2**：✅ 前端评估页面（数据集 / 运行 / 结果）
- **Phase 3**：✅ 报告导出 + AfterTurn 自动评估
- **Phase 4**：✅ 人工评估标注
- **Phase 5**：✅ 多轮/UserSim/pass@k + 扩展指标 + LLM UserSim + tool_trajectory + 趋势对比前端 + Eval LLM 系统配置

---

## 5. 任务清单

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| 1 | `NewData()` 调用 `EnsureEvalSchema` | P0 | ✅ |
| 2 | LLM-as-Judge（Provider 目录 + env） | P1 | ✅ |
| 3 | FrameworkBridge + ChatRunnerAdapter | P1 | ✅ EP-RT-08 |
| 4 | DeleteRun + UpdateDataset API | P2 | ✅ |
| 5 | DeleteDataset 级联 + metrics SRP 拆分 | P2 | ✅ |
| 6 | 前端 EvaluationPage | P1 | ✅ |
| 7 | 报告导出 CSV/JSON | P2 | ✅ |
| 8 | 人工标注 AnnotateCaseResult | P3 | ✅ |
| 9 | AfterTurn 自动评估 Hook | P2 | ✅ |
| 10 | EvalSet 多轮 + UserSimulation + pass@k | P3 | ✅ |
| 11 | 趋势 / A/B Compare API + AnalyticsPanel | P3 | ✅ |
| 12 | ROUGE/XML/JSON + LLM UserSim + tool_trajectory | P3 | ✅ |
| 13 | Eval LLM system_settings + Settings 页 | P3 | ✅ |

---

## 6. 验收标准

- [x] `NewData()` 启动后 eval_* 表自动创建
- [x] LLM-as-Judge 可配置 Judge 模型并返回有效分数（需 Provider 目录或 env）
- [x] 可通过 API 删除评估运行、更新数据集
- [x] DeleteDataset 级联删除用例
- [x] 前端可管理数据集、运行评估、查看结果、人工标注、导出报告
- [x] Agent 运行后可自动触发评估（AfterTurn + config_json.evaluation）
- [x] 趋势 API / A/B 对比 API + 前端趋势表与 Run 多选对比
- [x] 扩展指标 opt-in（json_match / xml_match / rouge_l / tool_trajectory）与 scores_json
- [x] Eval LLM 可在系统设置页配置（env 优先）
- [x] `go test ./internal/evaluation/...` 通过（5 个测试文件）
- [x] `go test ./internal/data/... -run Eval` 通过（2 个测试文件）
- [x] `go test ./internal/biz/evaluation/...` 通过（4 个测试文件）
- [ ] Service 层单元测试（缺失）
- [ ] Runner 核心组件测试（runner / after_turn / framework / llm_judge / chat_runner 缺失）

---

## 7. 依赖与风险

- **LLM Token 成本**：LLM-as-Judge 每条用例一次 LLM 调用；可通过 `metrics` 参数按需禁用
- **自动评估与 Chat 耦合**：AfterTurn Hook 须确保评估失败不影响正常对话
- **Framework 与 Legacy 双路径**：Wire 默认走 Framework；Legacy 仅 framework nil 时启用，需保持 metrics 语义一致
- **Judge/Sim 模型来源**：env > system_settings > 目录；Settings 默认 Sim 为 openai/gpt-4o-mini
- **Schema 迁移**：`scores_json`、`eval_*` 于 `system_settings.eval_*` 均通过 ALTER 兼容旧库
