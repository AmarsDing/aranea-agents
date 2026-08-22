# Evaluation 评估 — 开发计划

> **版本**：2026-08-14 | **状态**：🟢 全部完成（Phase 1-8 已交付；Phase 8 深度评审整改 16 项全绿；第二轮深度评审整改 9 项全绿，见 §10）
> **需求**：[33 evaluation.md](./33%20evaluation.md) · **设计**：[33 evaluation.design.md](./33-evaluation.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-DATA-01 ✅, EP-RT-08 ✅, EP-BIZ-04 ✅, EVAL-02 ✅

---

## 1. 模块定位

Evaluation 评估：对 Agent 输出质量进行结构化评估，支持自动评估（含 LLM-as-Judge）、人工标注与客户端报告导出。

**代码锚点**：
- `api/kratos/evaluation/v1/evaluation.proto` — Evaluation HTTP+gRPC API（24 RPC，含 Case CRUD + CancelRun）
- `internal/service/evaluation.go` — EvaluationService（传输桥点）
- `internal/service/evaluation_runner.go` — NewEvaluationRunner（Wire 装配）
- `internal/service/evaluation_after_turn.go` — NewEvaluationAfterTurnTrigger
- `internal/biz/evaluation.go` — 类型重导出（子包 `evaluation/` 的别名）
- `internal/biz/evaluation/evaluation.go` — 领域模型 + EvalUsecase（构造注入 `Stores`，端口字段未导出）
- `internal/biz/evaluation/ports.go` — `Stores` + Dataset/Case/Run/RunQuery/Result/Governance 窄口；宽 `Repo` Deprecated
- `internal/data/evaluation.go` — evalRepo（一身多口）+ EnsureEvalSchema（4 表 + 11 条 ALTER 迁移）
- `internal/data/ent/schema/eval_*.go` — 4 个 Ent Schema（EvalDataset/EvalCase/EvalRun/EvalCaseResult）
- `internal/evaluation/runner.go` — 异步调度 + Cancel
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
| Wire 注入 | ✅ | `wire_gen.go`：Stores → Usecase → Runner → Service → Registry |
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
| US-13 发布质量门禁 | ✅ | `gate.go` PublishGate + per-agent `eval_gate_config` + `mode` advisory\|blocking + `ProvidePublishGate` + `EvaluationGateDialog.vue` |
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
| 前端数据集编辑 / 用例 CRUD / 取消运行 | P0 运营闭环（2026-08-22）：UpdateDataset/DeleteRun 已接前端；List/Update/DeleteCase + CancelRun 已贯通 |

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
- **Phase 8（全模块深度评审整改，2026-08-14）**：✅ 4B + 12Y 共 16 项修复（详见 §5 任务 28-31 与 §9 整改清单）：
  - **P0 阻断**：B1 `num_runs` API 值被默认值覆盖（framework bridge 直接采用 API 值）+ 上限校验（`EvalMaxNumRuns=20`）；B2 人工标注无法清除（proto3 optional 不收 JSON null → 新增 `clear_human_pass`/`clear_human_score` 显式清除位，proto+service+repo+前端贯通）；B3 前端轮询刷新覆盖用户勾选；B4 workspace 隔离/IDOR（`assertEvalDatasetAccess`/`assertEvalDatasetMutate` + ListDatasets/ListTrendPoints/GetRunsByIDs 工作区过滤）
  - **P1 正确性**：Y1 LLM UserSim 无脚本用例 max_invocations 默认 5（原 `len(script)+1`=1 提前结束多轮）；Y4 混合数据集 `hybridSimulator` 按用例路由脚本/LLM 模拟器；Y5 judge/sim LLM 调用 2min 超时包装（`timeoutRunner`，防 provider 断连致 run 永久 running）；Y6 未知指标名 `ValidateMetricNames` 白名单校验；Y10 启动清扫 `FailStaleRuns`（孤儿 pending/running 标记 failed）；Y7 tool 列确定性（`tool_call_accuracy` 优先于 `tool_trajectory` 写 legacy 列 + 按指标名排序合并）；Y2 门禁异步化（advisory 模式：回归运行后台执行、in-flight 去重、阈值越界仅通知不阻断）；Y12 gate 严格化（配置时校验指标名 + max_drop 无基线时硬阻断并自动启动基线评估）
  - **P2 质量**：Y3/Y13 死代码清除（`scoresFromJSON`/`marshalScoresMap`/`ChatRunnerFactory`/`AgentRunnerFactory`）；Y8 data 层错误翻译（三个评估 repo 文件全部裸 err 经 `entErrToBizErr(err,"EVAL")`，红线 DB-R5）；Y9 `eval_run_preferences` 补 `run_id_a`/`run_id_b` 索引；Y11 DeleteDataset/DeleteRun 级联删除 preferences 孤儿行；Y14 runner 持久化失败补 Error 日志（K2）

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
| 28 | P0 阻断修复（B1 num_runs 生效+上限 / B2 标注清除位 proto+service+repo+前端 / B3 轮询清勾选 / B4 workspace/IDOR） | P0 | ✅ |
| 29 | P1 正确性修复（Y1 UserSim 默认轮次 / Y4 混合模拟器 / Y5 LLM 超时 / Y6 指标名校验 / Y10 启动清扫 / Y7 tool 列确定性 / Y2 门禁异步化 / Y12 gate 严格化） | P1 | ✅ |
| 30 | P2 质量修复（Y3/Y13 死代码 / Y8 错误翻译 / Y9 索引 / Y11 级联 / Y14 Warn 日志） | P2 | ✅ |
| 31 | 门禁异步化测试重写（`gate_test.go`：below-floor/drop advisory + 无基线阻断 + in-flight 去重；`runner_test.go` Y7 回归） | P1 | ✅ |
| 32 | 评估治理 P1：Run FSM、per-agent 门禁 advisory\|blocking、in-flight 部分唯一索引、并发槽、Prometheus、FailStaleRuns grace、趋势曲线 | P1 | ✅ |
| 33 | 评估平台 P2：数据集版本、实验矩阵、faithfulness、失败 Observation、结果跳 Trace、门禁 composable 拆分 | P2 | ✅ |

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

---

## 9. Phase 8 深度评审整改清单（2026-08-14）

> 评审范围：biz / data / service / internal/evaluation / 前端 / Schema 全层。以下编号为评审报告原始编号。
> 验证：`go build ./cmd/... ./internal/... ./api/... ./pkg/...` ✅ · `make wire` ✅ · `go test ./internal/evaluation ./internal/biz/evaluation ./internal/service ./internal/data`（PG 实库）✅ · 前端 lint/test/build ✅

### P0 阻断（4 项）

| # | 问题 | 修复 | 关键文件 |
|---|------|------|---------|
| B1 | `num_runs>1` 被静默忽略（framework bridge 仅在 ≤0 时赋值，默认值 1 覆盖 API 值），pass@k 失真；且无上限可被恶意放大 LLM 成本 | bridge 直接采用 API 值；`biz.EvalMaxNumRuns=20` 上限 + RunEvaluation 入参校验 + agent 自动评估配置 clamp；回归测试 `TestFrameworkBridgeNumRunsOverridesDefault` | `internal/evaluation/framework.go`、`internal/biz/evaluation/`、`internal/service/evaluation.go` |
| B2 | 人工标注无法清除：proto3 optional + JSON null 被 protojson 视为「未设置」，清除语义丢失 | proto 新增 `clear_human_pass`/`clear_human_score` 显式布尔位，service/repo/前端贯通 | `api/kratos/evaluation/v1/evaluation.proto`、`internal/service/evaluation.go`、`internal/data/evaluation.go`、前端 Results 对话框 |
| B3 | 前端轮询刷新运行列表时覆盖用户已勾选的对比选择 | 轮询合并时保留既有勾选状态 | `web/src/features/evaluation/` |
| B4 | workspace 隔离缺失（IDOR）：跨工作区可读/改他人数据集与运行 | service 层 `assertEvalDatasetAccess`/`assertEvalDatasetMutate`（拒绝一律 404 防存在性探测）；data 层 ListDatasets/ListTrendPoints/GetRunsByIDs 加工作区过滤 | `internal/service/evaluation.go`、`internal/data/evaluation.go` |

### P1 正确性（8 项）

| # | 问题 | 修复 | 关键文件 |
|---|------|------|---------|
| Y1 | 无脚本用例 LLM UserSim `max_invocations=len(script)+1=1`，多轮对话首轮即结束 | 改用 `meta.UserSimulationMaxInvocations()`（默认 5） | `internal/evaluation/`（scenario 构建） |
| Y2 | 发布门禁同步跑全量评估，发布请求超时/被取消后续跑写「执行失败」假拦截 | 门禁改异步 advisory：后台启动回归运行 + in-flight 去重 + 阈值越界仅通知不阻断 | `internal/evaluation/gate.go` |
| Y4 | 混合数据集（脚本+LLM 用例）只用脚本模拟器，LLM 用例被静默脚本化 | `hybridSimulator` 按用例 metadata 路由 | `internal/evaluation/scripted_simulator.go` |
| Y5 | judge/simulator LLM 调用无超时，provider 断连致 run 永久 running | `timeoutRunner` 2min 超时包装，超时用例记 failed | `internal/evaluation/`（judge_runner/llm_simulator） |
| Y6 | 未知指标名静默忽略，拼写错误退化为默认指标 | `ValidateMetricNames` 白名单校验，未知指标报错 | `internal/evaluation/metrics.go` |
| Y7 | `tool_call_accuracy` 与 `tool_trajectory` 同写 legacy 列，map 迭代序决定终值（非确定） | 列写入优先级：accuracy 在 scores map 中存在即赢（与顺序无关）；run 合并按指标名排序迭代；回归测试 | `internal/evaluation/scores.go`、`internal/evaluation/runner.go` |
| Y10 | 进程重启后孤儿 run 滞留 pending/running | `FailStaleRuns` 启动清扫（created_at 早于阈值的 pending/running 标记 failed） | `internal/data/evaluation.go`、启动装配 |
| Y12 | gate 配置不校验指标名，发布期才暴露；max_drop 无基线时 drop 检查被静默跳过 | UpsertGateConfig 校验指标名；max_drop 无完成基线 → 硬阻断（Conflict）并后台启动基线评估 | `internal/evaluation/gate.go`、`internal/biz/evaluation/governance.go` |

### P2 质量（5 项）

| # | 问题 | 修复 | 关键文件 |
|---|------|------|---------|
| Y3/Y13 | 死代码：`scoresFromJSON`/`marshalScoresMap`（scores.go）、`ChatRunnerFactory`/`AgentRunnerFactory`（chat_runner.go） | 删除 | `internal/evaluation/scores.go`、`internal/evaluation/chat_runner.go` |
| Y8 | data 层评估 repo 大量裸 err 直返，违反红线 DB-R5（唯一约束等无法翻译为 Conflict） | `evaluation.go`/`evaluation_governance.go`/`evaluation_divergence.go` 全部错误出口经 `entErrToBizErr(err,"EVAL")`（事务方法在 `ExecInTx` 调用点统一包装；scan 辅助函数内部包装；已有 apierror 透传不受影响） | `internal/data/evaluation*.go` |
| Y9 | `eval_run_preferences` 缺 `run_id_a`/`run_id_b` 索引，级联删除全表扫 | Ent Schema 补两个字段索引 | `internal/data/ent/schema/eval_run_preference.go` |
| Y11 | DeleteDataset/DeleteRun 不删 preferences，残留指向已删 run 的孤儿行 | 两个删除事务内补 preferences 级联 DELETE | `internal/data/evaluation.go` |
| Y14 | runner `UpdateRun` 失败静默返回，run 滞留 pending 无迹可查 | 补 Error 日志（step `evaluation.run.persist_running_fail`，K2） | `internal/evaluation/runner.go` |

### 终审残留项（2026-08-14 终审，非阻塞）

| # | 级别 | 问题 | 处置 |
|---|------|------|------|
| R1 | 提示 | `EvaluationAnalyticsPanel.vue` 模板 7 处 slot 变量 `props` 遮蔽 defineProps（vue/no-template-shadow 警告）+ 1 处 prettier 警告；ResultsDialog 已用 `slotProps` 命名可参照 | 下轮前端治理统一改名 |
| R2 | 提示 | `EvaluationFeedbackPanel.vue` 面板组件内直接 `$q.notify`（L132），与监控模块「Notify 归 Page 层」整改先例不一致 | 下轮随反馈面板迭代上移 |
| R3 | 技术债务 | `mappers.ts` 与 `api.ts` 重复定义 mapDataset/mapRun（ISSUES.md #9 已登记） | 统一为单一 mapper 文件时处理 |
| R4 | 技术债务 | 评估模块 8 个前端文件共 68 处硬编码中文由 `i18n-baseline.json` 豁免管控，新增字符串受门禁拦截 | 按基线逐步治理，禁止新增 |
| R5 | 测试缺口 | Service 层单测、Runner 组件测试（after_turn/framework/llm_judge/chat_runner）缺失（§6 验收未勾项） | 纳入后续测试补齐计划 |

### 文档同步（2026-08-14）

- `33-evaluation.md`：US-13 验收标准改写为异步 advisory 语义（Y2/Y12）；US-6 补「可清除标注」（B2）
- `33-evaluation.design.md`：§6.10 门禁重构、级联删除更正、§6.17 Phase 8 可靠性与隔离
- `65-module-cross-reference-full.md`：§1.15 评估卡片更新（核心导出/共享类型/ workspace 隔离与级联删除）

---

## 10. 第二轮深度评审整改清单（2026-08-14）

> 评审范围：biz / data / service / internal/evaluation 全层复查（Phase 8 整改后的二次评审）。
> 编号为评审报告原始编号（EVAL-xx），跳号项为评审阶段合并/判定为非问题项。
> 验证：`go build ./cmd/... ./internal/... ./api/... ./pkg/...` ✅ · `go test ./internal/evaluation ./internal/biz/evaluation` ✅ · `go test ./internal/data -run TestEval`（PG 实库）✅ · `go vet ./internal/data` ✅

### 阻断（2 项）

| # | 问题 | 修复 | 关键文件 |
|---|------|------|---------|
| EVAL-01 | AfterTurn 自动评估丢失 workspace：后台 goroutine 继承的 ctx 未传递工作区，创建的 run `workspace_id` 为空 → 租户列表不可见 + 跨租户统计泄漏 | `context.WithoutCancel(ctx)` 解耦生命周期 + 按 ctx 显式设置 `WorkspaceID` | `internal/evaluation/after_turn.go` |
| EVAL-02 | 评估执行失败与得分不达标混同：`failRun` 只落库不返回错误，门禁/Prometheus 把执行失败误计为低分完成 | `errEvalRunFailed` 哨兵错误，`Start` 中 `errors.Is` 区分 `failed`（执行失败）/`error`（内部错误）/`completed` 标签 | `internal/evaluation/runner.go` |

### 建议（6 项）

| # | 问题 | 修复 | 关键文件 |
|---|------|------|---------|
| EVAL-03 | `ListFailureGroups` 治理查询缺 workspace 过滤（跨租户泄漏失败分组） | 补 `evalRunsWorkspaceFilter` | `internal/data/evaluation_governance.go` |
| EVAL-04 | `UpdateEvalGate` 无鉴权：任意租户可改写平台全局发布门禁 | `assertSystemCaller`（system 主体或 admin） | `internal/service/evaluation.go` |
| EVAL-05 | Legacy 执行路径为生产死代码（旧 runner/LLM judge 双轨残留） | 删除 `runner_legacy.go`、`llm_judge.go` 及 `metrics.go` 遗留计算；测试适配：新增 `echoRunner`/`echoBridge` 替身（framework 路径等价 echo 语义），`TestRunnerLegacyCaseErrorMarksRunFailed` 语义已由 `TestRunnerFrameworkCaseErrorMarksRunFailed` 覆盖故删除 | `internal/evaluation/` |
| EVAL-06 | Repo 死方法 `InsertCases`/`UpdateDatasetCaseCount`：无生产调用方，分离写法破坏 case_count 原子性 | 接口与实现删除，统一 `InsertCasesWithCountUpdate` | `internal/biz/evaluation/evaluation.go`、`internal/data/evaluation.go` |
| EVAL-07 | `EnsureEvalSchema` ALTER 迁移循环全吞错误：非「列已存在」错误（权限/连接/语法）静默丢失，schema 静默不一致直至运行时查询失败 | 签名加 `Dialect` 参数；仅 `AlreadyExistsErr` 视为幂等成功，其余错误经 `entErrToBizErr` 上报阻断启动迁移 | `internal/data/evaluation.go`、`ddl_migration_registry.go` |
| EVAL-08 | 手动 `RunEvaluation` 无 in-flight 去重：并发点击/重试产生重复 run | 创建前扫描近 20 条（`inFlightScanLimit`）pending/running → Conflict | `internal/service/evaluation.go` |

### 提示（1 项）

| # | 问题 | 修复 | 关键文件 |
|---|------|------|---------|
| EVAL-13 | `runner==nil`（未装配）时 `RunEvaluation` 照常创建 run，永久滞留 pending | runner 为 nil 直接返回 Unavailable | `internal/service/evaluation.go` |

### 文档同步（第二轮，2026-08-14）

- `33-evaluation.development.md`：追加本清单（§10）
