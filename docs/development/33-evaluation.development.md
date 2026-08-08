# Evaluation 评估 — 开发计划

> **版本**：2026-06-06 | **状态**：🟢 全部完成（所有 Phase 已交付，差距已关闭）
> **需求**：[33 evaluation.md](./33%20evaluation.md) · **设计**：[33 evaluation.design.md](./33-evaluation.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-DATA-01 ✅, EP-RT-08 ✅, EP-BIZ-04 ✅, EVAL-02 ✅

---

## 1. 模块定位

Evaluation 评估：对 Agent 输出质量进行结构化评估，支持自动评估（含 LLM-as-Judge）、人工标注与客户端报告导出。

**代码锚点**：
- `api/kratos/evaluation/v1/evaluation.proto` — Evaluation HTTP+gRPC API（20 RPC + 39 Message）
- `internal/service/evaluation.go` — EvaluationService（传输桥点，20 方法）
- `internal/service/evaluation_runner.go` — NewEvaluationRunner（Wire 装配）
- `internal/service/evaluation_after_turn.go` — NewEvaluationAfterTurnTrigger
- `internal/biz/evaluation.go` — 类型重导出（子包 `evaluation/` 的别名）
- `internal/biz/evaluation/evaluation.go` — 领域模型 + EvalRepo 接口（19 方法）+ EvalUsecase（17 方法）
- `internal/data/evaluation.go` — EvalRepo + EnsureEvalSchema（4 表 + 11 条 ALTER 迁移）
- `internal/data/ent/schema/eval_*.go` — 4 个 Ent Schema（EvalDataset/EvalCase/EvalRun/EvalCaseResult）
- `internal/evaluation/runner.go` — 异步调度
- `internal/evaluation/runner_legacy.go` — Legacy 回退路径
- `internal/evaluation/metrics.go` — 指标解析与 legacy 计分
- `internal/evaluation/framework.go` — trpc AgentEvaluator 桥接
- `internal/evaluation/framework_metrics.go` — 扩展指标（JSON/XML/ROUGE/ToolTrajectory）
- `internal/evaluation/evaluator_registry.go` — 9 种内置评估器注册
- `internal/evaluation/multirun.go` — MultiRun 配置
- `internal/evaluation/chat_runner.go` — ChatService → runner.Runner
- `internal/evaluation/llm_judge.go` — LLM-as-Judge
- `internal/evaluation/judge_runner.go` — judge 执行器（中立 system instruction + verdict 提取，2026-08-08 修复 judge 解析链）
- `internal/evaluation/llm_simulator.go` — LLM UserSimulation + simRunner
- `internal/evaluation/scripted_simulator.go` — 脚本化 UserSimulation
- `internal/evaluation/eval_llm_resolve.go` — env + system_settings 模型解析
- `internal/evaluation/scores.go` — scores_json 映射
- `internal/evaluation/evalset_adapter.go` — biz EvalCase → trpc EvalSet
- `internal/evaluation/evalset_tools.go` — expected_tool_calls → ToolTrajectory
- `internal/evaluation/case_metadata.go` — 用例元数据解析
- `internal/evaluation/pass_metrics.go` — pass@k / pass^k
- `internal/evaluation/after_turn.go` — AfterTurn 自动评估（含 sample_rate 采样判定，P2-2）
- `internal/evaluation/gate.go` — 发布质量门禁 PublishGate（P2-1）
- `internal/biz/evaluation/governance.go` + `internal/data/evaluation_governance.go` — 门禁配置 / 失败归组 / Pairwise 偏好（P2-1/P2-3/P3-3）
- `internal/evaluation/drop_alert.go` — 在线分数连跌告警 ScoreDropAlerter（P2-2）
- `internal/evaluation/redteam.go` + `internal/evaluation/testdata/` — 红队攻击成功率指标 + 预制对抗用例集（P3-4）
- `internal/evaluation/dataset_hash.go` — 数据集版本快照 hash（P3-5）
- `internal/data/ent/schema/eval_gate_config.go` / `eval_run_preference.go` — P2/P3 新增 Ent Schema
- `web/src/components/evaluation/EvaluationGateDialog.vue` — 发布门禁配置对话框（P2-1）
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
| Proto 14 RPC + 26 Message | ✅ | evaluation.proto：14 个 RPC + 26 个 Message |
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
| Ent Schema 4 表 | ✅ | `eval_dataset.go` / `eval_case.go` / `eval_run.go` / `eval_case_result.go` |

### 已交付能力总览

| 能力 | 状态 | 实现位置 |
|------|------|----------|
| AgentEvaluator 集成 | ✅ | `framework.go` FrameworkBridge |
| EvalSet 多轮 Invocation | ✅ | `evalset_adapter.go` + `case_metadata.go` |
| UserSimulation | ✅ | `scripted_simulator.go` + `llm_simulator.go` |
| pass@k / pass^k | ✅ | `pass_metrics.go` + `multirun.go` |
| AfterTurn 自动评估 | ✅ | `after_turn.go` + `evaluation_after_turn.go` |
| 趋势 / A/B API + 前端 | ✅ | `GetAgentEvalTrend` / `CompareEvalRuns` / `EvaluationAnalyticsPanel` |
| FinalResponse ROUGE/XML/JSON | ✅ | `framework_metrics.go` opt-in metrics |
| ToolTrajectory 完整序列 | ✅ | `tool_trajectory` + `evalset_tools.go` |
| Eval LLM 系统配置 | ✅ | `eval_llm_resolve.go` + `system_settings` + Settings 页 |
| 人工标注 | ✅ | `AnnotateCaseResult` API + `EvaluationResultsDialog` |
| 客户端报告导出 | ✅ | `exportRunResults.ts` CSV/JSON |
| 内置评估器注册 | ✅ | `evaluator_registry.go` 9 种评估器 |
| Judge 分歧统计（P1-3） | ✅ | `divergence.go` + `GetJudgeDivergence` API + AnalyticsPanel 分歧卡片 |
| 对话→用例一键转化（P1-1） | ✅ | `useAddToEvalDataset` + `AddEvalCaseDialog`（TaskCard 入口，source 溯源元数据） |
| 负反馈采集→转用例（P1-2） | ✅ | TaskCard 👍/👎 → `SubmitMessageFeedback(context_json)` → monitor_events → `EvaluationFeedbackPanel` |
| 用例级 rubric（P3-2 提前） | ✅ | `CaseMetadata.Rubric` → `evalset_adapter.go` 映射框架 `EvalCase.Rubrics`；对话框可录入 |

### 用户故事实现状态映射

| 用户故事 | 状态 | 实现证据 |
|---------|------|----------|
| US-5 自动评估触发 | ✅ | `NativeTurnAfterHook` + `config_json.evaluation.auto_after_turn`（min_interval_sec 限流） |
| US-6 人工评估标注 | ✅ | `AnnotateCaseResult` API + Results 对话框（EVAL-02） |
| US-7 评估报告与趋势 | ✅ | 客户端 CSV/JSON 导出；`GetAgentEvalTrend` + `CompareEvalRuns` API；前端 `EvaluationAnalyticsPanel` 趋势表 + Run 多选 A/B 对比 |
| US-8 高级评估能力 | ✅ | 多轮 turns、脚本化/LLM UserSimulation、MultiRun + pass@k、ROUGE/XML/JSON FinalResponse、完整 ToolTrajectory（`tool_trajectory`）、趋势/A/B API 与前端 |
| US-9 对话沉淀为用例 | ✅ | TaskCard「加入评估」→ `useAddToEvalDataset` + `AddEvalCaseDialog` → UploadCases（source 溯源元数据） |
| US-10 负反馈采集与转化 | ✅ | TaskCard 👍/👎 → `SubmitMessageFeedback(context_json)` → monitor_events(warning) → `EvaluationFeedbackPanel` 一键转用例 |
| US-11 Judge 校准（分歧统计） | ✅ | `GetJudgeDivergence`（`divergence.go`）+ AnalyticsPanel 分歧卡片 |
| US-12 用例级 rubric | ✅ | `CaseMetadata.Rubric` → 框架 `EvalCase.Rubrics`；AddEvalCaseDialog 可录入 |
| US-13 发布质量门禁 | ✅ | `gate.go` PublishGate + `eval_gate_config` 单例表 + `ProvidePublishGate` 注入 skill 发布/pack 安装 + `EvaluationGateDialog.vue` |
| US-14 在线评估看板 | ✅ | `AgentEvalAutoConfig.SampleRate` 采样（限流前判定）+ `drop_alert.go` 连跌 SystemNoticeEvent + 趋势按 trigger_source 拆分 |
| US-15 失败归组 | ✅ | `ListFailureGroups`（GROUP BY error_message）+ `GetFailureGroups` API + AnalyticsPanel 失败归组面板 |
| US-16 Pairwise 偏好裁决 | ✅ | `eval_run_preference` 表 + Submit/ListRunPreferences API（winner 校验）+ 对比行「更优」按钮 |
| US-17 红队用例包 | ✅ | `testdata/` 预制对抗用例集 + `redteam.go` `redteam_attack_success_rate`（legacy/framework 双路径） |
| US-18 数据集版本快照 | ✅ | `dataset_hash.go` + `eval_runs.dataset_hash` + 对比响应携带 hash + 变更 q-banner 提示 |

### 未完成项（已知短板）

| 项 | 说明 |
|----|------|
| Service 层单元测试 | `internal/service/evaluation*_test.go` 不存在 |
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
| P2 | Runner 核心组件测试 | after_turn / framework / llm_judge / chat_runner 无测试（runner_test.go 已补） |
| P3 | 前端数据集编辑入口 | 后端 API 已就绪，前端未暴露 |

---

## 4. 开发阶段

- **Phase 1**：✅ Schema 启动 + LLM-as-Judge + FrameworkBridge + DeleteRun/UpdateDataset
- **Phase 2**：✅ 前端评估页面（数据集 / 运行 / 结果）
- **Phase 3**：✅ 报告导出 + AfterTurn 自动评估
- **Phase 4**：✅ 人工评估标注
- **Phase 5**：✅ 多轮/UserSim/pass@k + 扩展指标 + LLM UserSim + tool_trajectory + 趋势对比前端 + Eval LLM 系统配置
- **Phase 6（评估飞轮，2026-08-08）**：✅ P1-1 对话→用例一键转化 + P1-2 负反馈采集与待审查转用例 + P1-3 judge 分歧统计 + P3-2 用例级 rubric（提前）+ judge 解析链修复（中立 instruction / llm_rubric_response / verdict 日志）+ 移动端缺口修复（MobileTasksPage 事件绑定 + MobileLayout 渲染 AddEvalCaseDialog）。**退出验证门 3/3 已运行时实证通过（2026-08-08 13:20 终验）**，方案与实证证据见 `test/dogfood-eval/report.md`
- **Phase 7（质量守护 + 深度能力，即路线图 Phase 2/3，2026-08-08）**：✅ P2-1 发布质量门禁 + P2-2 在线评估看板（采样率/连跌告警/趋势拆分）+ P2-3 失败归组（SQL 版）+ P3-3 Pairwise 偏好裁决 + P3-4 红队用例包 + P3-5 数据集版本快照；P3-1 RAG 指标 spike 完成（结论：可行，推荐 Aranea 侧后处理路径，实施未排期，详见设计文档 §6.16）。**运行时实证（退出验证门）待补**，实施要点见 `test/dogfood-eval/report.md`「Phase 2/3 实施记录」

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
| 14 | P1-1 对话→用例一键转化（composable + 对话框 + TaskCard 入口） | P1 | ✅ |
| 15 | P1-2 负反馈采集（context_json 快照 + monitor_events + 待审查面板转用例） | P1 | ✅ |
| 16 | P1-3 judge 分歧统计（ListJudgeAnnotatedResults + GetJudgeDivergence + 分歧卡片） | P1 | ✅ |
| 17 | P3-2 用例级 rubric（CaseMetadata.Rubric + 适配器映射 + 对话框录入） | P1 | ✅（自 P3 提前） |
| 18 | judge 解析链修复（judge_runner 中立 instruction + llm_rubric_response 切换 + judgeReasonFromRuns verdict 日志） | P0 | ✅ |
| 19 | 移动端缺口修复（MobileTasksPage @add-to-eval/@feedback 绑定 + MobileLayout 渲染 AddEvalCaseDialog） | P1 | ✅ |
| 20 | P2-1 发布质量门禁（`gate.go` + `ent/schema/eval_gate_config.go` + `chat_wire.go` ProvidePublishGate + `service/skill.go`/`service/pack.go` 消费 + `EvaluationGateDialog.vue`） | P2 | ✅ |
| 21 | P2-2 采样率（`biz/agent_eval_config.go` SampleRate + `after_turn.go` 限流前采样判定） | P2 | ✅ |
| 22 | P2-2 连跌告警（`drop_alert.go` ScoreDropAlerter + `chat_wire.go` evalAgentConfigReader 装配，nil-safe）+ 趋势 trigger_source 拆分 | P2 | ✅ |
| 23 | P2-3 失败归组（`data/evaluation_governance.go` ListFailureGroups + proto GetFailureGroups + AnalyticsPanel 失败归组面板） | P2 | ✅ |
| 24 | P3-3 Pairwise 偏好（`ent/schema/eval_run_preference.go` + `biz/evaluation/governance.go` + 前端「更优」按钮与偏好列表） | P3 | ✅ |
| 25 | P3-4 红队用例包（`redteam.go` mergeAttackSuccessRate + `internal/evaluation/testdata/` 预制用例集） | P3 | ✅ |
| 26 | P3-5 数据集版本快照（`dataset_hash.go` + `ent/schema/eval_run.go` dataset_hash + DDL 迁移注册 + 对比变更提示） | P3 | ✅ |
| 27 | P3-1 RAG 指标 spike（结论见设计文档 §6.16：可行，推荐 Aranea 侧后处理路径） | P3 | ✅ spike 完成（实施未排期） |

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
- [x] `go test ./internal/evaluation/...` 通过（6 个测试文件）
- [x] `go test ./internal/data/... -run Eval` 通过（2 个测试文件）
- [x] `go test ./internal/biz/evaluation/...` 通过（4 个测试文件）
- [x] 对话可一键转为用例（TaskCard → AddEvalCaseDialog → UploadCases，metadata_json 含 source 溯源）
- [x] 点踩反馈落 monitor_events（chat.user_feedback / warning）且评估页可审查、可转用例
- [x] judge 分歧统计 API 返回一致率与分歧明细，AnalyticsPanel 分歧卡片展示
- [x] 用例 metadata_json.rubric 经适配层注入框架 judge 准则（未勾选 llm_as_judge 时自动剥离）
- [x] 退出验证门 3/3 运行时实证通过（2026-08-08 终验：≥5 条真实对话用例跑通 + 标注→分歧面板一致 + rubric 评语体现评分标准，证据见 `test/dogfood-eval/report.md`）
- [ ] Service 层单元测试（缺失）
- [ ] Runner 核心组件测试（after_turn / framework / llm_judge / chat_runner 缺失）

---

## 7. 依赖与风险

- **LLM Token 成本**：LLM-as-Judge 每条用例一次 LLM 调用；可通过 `metrics` 参数按需禁用
- **自动评估与 Chat 耦合**：AfterTurn Hook 须确保评估失败不影响正常对话
- **Framework 与 Legacy 双路径**：Wire 默认走 Framework；Legacy 仅 framework nil 时启用，需保持 metrics 语义一致
- **Judge/Sim 模型来源**：env > system_settings > 目录；Settings 默认 Sim 为 openai/gpt-4o-mini
- **Schema 迁移**：`scores_json`、`eval_*` 于 `system_settings.eval_*` 均通过 ALTER 兼容旧库

---

## 8. 后续迭代（2026-08-08 终验新发现，非阻塞）

| # | 严重度 | 问题 | 建议 |
|---|--------|------|------|
| N1 | medium | Judge 残余抖动：judge 响应偶发缺结构化负载（"structured output payload is missing"），终验 1/6 命中 | judge 解析失败时单次重试（同 prompt 重放），仍失败再记用例错误 |
| N2 | medium | 团队会话转用例无法 replay：团队任务转录的 expected_output 是团队状态文本而非答案文本，连续运行 actual 为空且无 error | ① 转化入口识别 team 任务并提示「团队任务不适用单 Agent 评估」或自动提取 deliverable 摘要作 expected；② 评估 inference 对空输出用例记 error 而非静默 0 分 |
| N3 | low | 趋势面板只统计 completed 运行 → 全 failed 数据集「暂无已完成运行记录」；A/B 对比列表混入 failed 运行 | 对比列表过滤 failed；趋势可选择展示 failed 点（带标记） |
| N4 | low | exact_match 对对话式输出恒 0（句末标点/Markdown 加粗导致不精确相等）——指标语义严格而非 bug | 结果/启动对话框对 exact_match 加适用性提示（短答案场景），或默认勾选 contains_match |
