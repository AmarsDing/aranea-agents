# Evaluation 评估模块 — 实现设计文档

> 对应需求：[33 evaluation.md](./33%20evaluation.md)
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)

---

## 一、模块概述

Evaluation 模块对 Agent 输出质量进行结构化评估。采用 Dataset → Case → Run → Result 模型，提供 4 种核心指标 + 4 种扩展指标、异步评估执行、人工标注与客户端报告导出。

**核心流程**：创建数据集 → 上传用例 → 启动评估运行 → 异步执行推理 + 评分 → 查看汇总分数和逐用例结果 → 可选人工标注 / 导出报告

**运行时路径**：默认经 `FrameworkBridge` 调用 trpc-agent-go `AgentEvaluator`（含 MultiRun）；`FrameworkBridge` 不可用时回退 legacy 逐用例路径。

---

## 二、架构

```
EvaluationService (Kratos HTTP/gRPC)
       │
EvalUsecase (biz)          ← 数据集/运行 CRUD、用例上传、人工标注
       │
EvalRepo (data) ─── SQLite (Raw SQL, EnsureEvalSchema @ NewData)
       │
evaluation.Runner (async goroutine)
   ├── FrameworkBridge ── trpc AgentEvaluator (默认)
   │      ├── exact_match / contains_match (FinalResponse text)
   │      ├── json_match / xml_match / rouge_l (FinalResponse criterion)
   │      ├── tool_call_accuracy / tool_trajectory (ToolTrajectory)
   │      └── llm_as_judge (Aranea LLMJudge hook, 框架外补算)
   └── executeLegacy (FrameworkBridge 不可用时的回退)
```

**分层职责**：

| 层 | 文件 | 职责 |
|----|------|------|
| Proto | `api/kratos/evaluation/v1/evaluation.proto` | HTTP + gRPC API 契约（14 RPC + 26 Message） |
| Service | `internal/service/evaluation.go` | proto ↔ biz 映射；RunEvaluation 触发 Runner |
| Service | `internal/service/evaluation_runner.go` | Wire：`NewEvaluationRunner` 装配 AgentRunner + LLMJudge + FrameworkBridge |
| Service | `internal/service/evaluation_after_turn.go` | Wire：`NewEvaluationAfterTurnTrigger` 创建 AfterTurn 触发器 |
| Biz | `internal/biz/evaluation.go` | 类型重导出（子包 `evaluation/` 的别名） |
| Biz | `internal/biz/evaluation/evaluation.go` | 领域模型 + EvalRepo 接口（19 方法）+ EvalUsecase（17 方法） |
| Data | `internal/data/evaluation.go` | Raw SQL 持久化 + EnsureEvalSchema（4 表 + 11 条 ALTER 迁移） |
| Runner | `internal/evaluation/runner.go` | 异步调度、Prometheus |
| Legacy | `internal/evaluation/runner_legacy.go` | FrameworkBridge 不可用时的降级执行路径 |
| Metrics | `internal/evaluation/metrics.go` | 指标解析与 legacy 路径计分（SRP） |
| Framework | `internal/evaluation/framework.go` | trpc AgentEvaluator 桥接 + UserSimulator 注入 |
| FrameworkMetrics | `internal/evaluation/framework_metrics.go` | 扩展指标注册（JSON/XML/ROUGE/ToolTrajectory） |
| Scores | `internal/evaluation/scores.go` | `scores_json` 映射与 run 聚合 |
| Registry | `internal/evaluation/evaluator_registry.go` | 注册 9 种 trpc-agent-go 内置评估器 |
| MultiRun | `internal/evaluation/multirun.go` | 多次运行配置 |
| PassMetrics | `internal/evaluation/pass_metrics.go` | pass@k / pass^k 计算 |
| UserSim | `internal/evaluation/scripted_simulator.go` / `llm_simulator.go` | 脚本 vs LLM UserSimulation |
| EvalLLM | `internal/evaluation/eval_llm_resolve.go` | env + system_settings 模型解析 |
| Adapter | `internal/evaluation/evalset_adapter.go` | biz EvalCase → trpc EvalSet |
| EvalTools | `internal/evaluation/evalset_tools.go` | expected_tool_calls → Invocation.Tools |
| CaseMeta | `internal/evaluation/case_metadata.go` | 用例元数据解析（turns / user_simulation / expected_tools） |
| Chat | `internal/evaluation/chat_runner.go` | ChatService → runner.Runner 适配 |
| Judge | `internal/evaluation/llm_judge.go` | LLM-as-Judge |
| AfterTurn | `internal/evaluation/after_turn.go` | AfterTurn 自动评估触发（含限流） |

---

## 三、Proto 层

### 3.1 消息定义

**EvalDataset**：评估数据集（名称、描述、用例数、工作区）。

**EvalCase**：评估用例（输入、期望输出、metadata_json）。

**EvalRun**：评估运行（状态、进度、4 种指标汇总分数、`scores_json` 扩展分、`pass_at_k`/`pass_hat_k`、错误信息）。

**EvalCaseResult**：逐用例结果（实际输出、各指标得分/判定、人工标注字段）。

**EvalTrendPoint** / **EvalRunComparison**：趋势点与 A/B 对比结果。

**JudgeDivergence** / **JudgeDivergenceCase**（P1-3）：judge 与人工标注的一致性统计（一致率 / false_pass / false_fail / 分歧用例明细）。

### 3.2 API 端点

| 方法 | 路径 | RPC | 说明 |
|------|------|-----|------|
| POST | `/v1/evaluation/datasets` | CreateDataset | 创建数据集 |
| GET | `/v1/evaluation/datasets/{id}` | GetDataset | 获取数据集 |
| GET | `/v1/evaluation/datasets` | ListDatasets | 列出数据集 |
| PATCH | `/v1/evaluation/datasets/{id}` | UpdateDataset | 更新名称/描述 |
| DELETE | `/v1/evaluation/datasets/{id}` | DeleteDataset | 删除数据集（级联 cases） |
| POST | `/v1/evaluation/datasets/{dataset_id}/cases` | UploadCases | 上传用例（JSON 数组） |
| POST | `/v1/evaluation/runs` | RunEvaluation | 启动异步评估运行 |
| GET | `/v1/evaluation/runs/{id}` | GetRun | 获取运行 + 分数 |
| DELETE | `/v1/evaluation/runs/{id}` | DeleteRun | 删除运行（级联 results） |
| GET | `/v1/evaluation/runs` | ListRuns | 列出运行 |
| GET | `/v1/evaluation/runs/{run_id}/results` | GetRunResults | 逐用例结果 |
| PATCH | `/v1/evaluation/runs/{run_id}/results/{result_id}/annotation` | AnnotateCaseResult | 人工标注 |
| GET | `/v1/evaluation/agents/{agent_id}/trend` | GetAgentEvalTrend | Agent 分数趋势 |
| POST | `/v1/evaluation/runs/compare` | CompareEvalRuns | A/B 或多 run 对比 |
| GET | `/v1/evaluation/datasets/{dataset_id}/judge-divergence` | GetJudgeDivergence | judge/人工分歧统计（P1-3，query：agent_id/threshold/limit） |

### 3.3 RunEvaluation 请求

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| dataset_id | string | 是 | 评估数据集 ID |
| agent_id | string | 是 | 被评估的 Agent ID |
| metrics | string | 否 | 逗号分隔指标键名，空值运行 4 种核心指标；扩展见 §6.5 |
| num_runs | int32 | 否 | 每用例重复次数（AgentEvaluator MultiRun，默认 1） |
| use_user_simulation | bool | 否 | 启用 UserSimulation（脚本或 LLM） |

---

## 四、Biz 层

### 4.1 领域模型

**Dataset** / **Case** / **Run** / **CaseResult** / **CaseResultAnnotation** / **TrendPoint** / **RunComparison** — 见 `internal/biz/evaluation/evaluation.go`。

人工标注字段：`HumanPass`、`HumanScore`、`HumanComment`、`AnnotatedAt`、`AnnotatedBy`。

辅助类型：`CaseUpload`（用例上传行）、`Scores`（指标键→分数 map）、`LLMSetting`（Eval LLM 平台默认配置）。

### 4.2 EvalRepo 接口

```
Dataset：CreateDataset / GetDataset / ListDatasets / DeleteDataset / UpdateDataset / UpdateDatasetCaseCount  (6)
Case：InsertCases / ListCases  (2)
Run：CreateRun / GetRun / UpdateRun / DeleteRun / ListRuns  (5)
CaseResult：InsertCaseResult / ListCaseResults / GetCaseResult / UpdateCaseResultAnnotation  (4)
Trend/Compare：ListTrendPoints / GetRunsByIDs  (2)
Divergence：ListJudgeAnnotatedResults  (1)
```

合计 20 方法。

### 4.3 EvalUsecase

- Dataset CRUD + UploadCases（JSON 数组解析）
- Run 创建（初始 status=`pending`）+ 查询 + DeleteRun
- CaseResult 列表 + AnnotateCaseResult（EVAL-02）
- GetAgentEvalTrend（趋势数据）+ CompareEvalRuns（A/B 对比，baseline = 首个 run）
- GetJudgeDivergence（judge/人工一致性统计，`divergence.go`，P1-3）

合计 18 方法。

---

## 五、Data 层

### 5.1 数据库 Schema

由 `data.EnsureEvalSchema(ctx, db)` 创建，4 张表：`eval_datasets`、`eval_cases`、`eval_runs`、`eval_case_results`。

`eval_case_results` 含人工标注列与 `scores_json`；`eval_runs` 含 `pass_at_k`/`pass_hat_k`/`trigger_source`/`num_runs`/`scores_json`，通过 ALTER 迁移兼容旧库。

### 5.2 Ent Schema

4 个 Ent Schema（`internal/data/ent/schema/eval_*.go`）显式映射表名（`entsql.Annotation{Table: ...}`）：

| Schema | 表名 | Edge |
|--------|------|------|
| `EvalDataset` | `eval_datasets` | → cases / runs |
| `EvalCase` | `eval_cases` | ← dataset / → results |
| `EvalRun` | `eval_runs` | ← dataset / → results |
| `EvalCaseResult` | `eval_case_results` | ← run / ← case |

> Ent Schema 与 Raw SQL `EnsureEvalSchema` 并存：Ent Schema 作为类型映射真相源，运行期建表由 `EnsureEvalSchema` 完成（含 ALTER 兼容旧库）。

### 5.3 Schema 初始化

`NewData()` 启动期调用 `EnsureEvalSchema`（EP-DATA-01）。

### 5.4 级联删除

- `DeleteDataset`：事务内先删 `eval_cases`，再删 `eval_datasets`
- `DeleteRun`：事务内先删 `eval_case_results`，再删 `eval_runs`

### 5.5 用例上传格式

```json
[
  { "input": "What is 2+2?", "expected_output": "4" },
  {
    "input": "Call the weather tool",
    "expected_output": "The weather is sunny",
    "metadata_json": "{\"expected_tool_calls\":[{\"name\":\"get_weather\"}],\"turns\":[{\"role\":\"user\",\"content\":\"weather?\"}],\"user_simulation\":{\"use_llm\":true,\"conversation_plan\":\"追问直到得到天气\"}}"
  }
]
```

`metadata_json` 可包含：
- `turns`：多轮对话序列
- `user_simulation`：`{script: "..."}` 脚本驱动 或 `{use_llm: true, conversation_plan: "..."}` LLM 驱动
- `expected_tools` / `expected_tool_calls`：ToolTrajectory 期望工具序列
- `rubric`：用例级评分标准文本（P3-2）。适配层映射为框架 `EvalCase.Rubrics`（绑定 `llm_as_judge` 指标实例），judge 将其合并进评分准则；当运行未勾选 `llm_as_judge` 时框架会因指标未注册而使该用例失败，故 `FrameworkBridge.Execute` 在此场景自动剥离 rubric（`stripCaseRubricsWhenNoJudge`）
- `session_user_id` / `session_state`：覆盖 SessionInput 默认值（UserID 默认 `"eval"`，State 默认空）
- `source` / `task_id` / `session_id`：溯源元数据（P1-1 对话→用例一键转化写入 `source="chat"` + 来源任务/会话 ID；不参与执行语义，仅供回溯）

> **框架版本说明（2026-08-08）**：`trpc-agent-go/evaluation` 模块经 `go.mod` replace 到 vendored 副本（`pkg/trpc-agent-go/evaluation`）——发布版 v1.9.0 的 `EvalCase` 无用例级 `Rubrics` 字段。切换带来一处 API 迁移：`promptiter/engine.RunRequest.TrainEvalSetIDs/ValidationEvalSetIDs` → `Train/Validation []EvalSetInput`（`internal/agent/prompt_iter_adapter.go`）。

---

## 六、Runner 与 FrameworkBridge

### 6.1 AgentRunner / LLMJudge

**AgentRunner**：`func(ctx, agentID, input) (string, error)` — 经 `ChatService.RunNativeTurnUnary` 执行，每条用例创建临时 Session。

**LLMJudge**：`NewLLMJudge(catalog, rt, sys)` — 模型解析见 §6.6；env 优先于 `system_settings`。

**LLM UserSimulator**：`NewLLMUserSimulator(catalog, rt, sys)` — trpc `usersimulation.New(simRunner)`；`resolveUserSimulator` 脚本优先于 LLM。

### 6.2 执行流程

1. `Start(ctx, run, metrics, numRuns, useUserSimulation)` 派发 async goroutine
2. 加载数据集用例，status → `running`
3. **Framework 路径**（默认）：`FrameworkBridge.Execute` → trpc `AgentEvaluator.Evaluate`；`num_runs>1` 时计算 pass@k/pass^k
4. **Legacy 路径**（framework nil）：逐用例 AgentRunner + `metrics.go` 计分
5. 写入 CaseResult，更新 CompletedCases
6. 聚合分数，status → `completed` / `failed`

### 6.3 AfterTurn 自动评估（US-5）

| 层 | 文件 | 职责 |
|----|------|------|
| Biz | `native_turn_after.go` | `NativeTurnAfterHook` 接口 |
| Biz | `agent_eval_config.go` | 解析 `config_json.evaluation` |
| Eval | `after_turn.go` | 限流 + 异步 `CreateRun` + `Runner.Start` |
| Service | `chat_native.go` | `notifyNativeTurnHooks`（Chat 成功后调用） |
| Runtime | `deps.go` | `TurnDeps.AfterTurn` 注入 |

配置示例（Agent `config_json`）：

```json
{
  "evaluation": {
    "auto_after_turn": true,
    "dataset_id": "ds-abc",
    "metrics": "exact_match,contains_match",
    "num_runs": 1,
    "min_interval_sec": 300
  }
}
```

### 6.4 多轮 / UserSimulation / pass@k（Phase 5）

- **多轮**：`metadata_json.turns` → `evalset_adapter.invocationsFromTurns`
- **UserSimulation 脚本**：`user_simulation.script` + `scripted_simulator.go`
- **UserSimulation LLM**：`use_llm` 或 `conversation_plan`（无 script）+ `llm_simulator.go` + trpc `simRunner`
- **pass@k**：`pass_metrics.go` → `eval_runs.pass_at_k/pass_hat_k`

### 6.5 扩展指标与 scores_json

| 键 | 实现 |
|----|------|
| `json_match` / `xml_match` / `rouge_l` | `framework_metrics.go` → trpc FinalResponse criterion |
| `tool_trajectory` | 顺序敏感 ToolTrajectory；`evalset_tools.go` 填充期望工具 |
| `scores_json` | run/result 扩展分 map；legacy 列仍写入 |

扩展分数字段：`eval_runs.scores_json` / `eval_case_results.scores_json`（键 → 分数 map）；legacy 四列仍同步写入。

### 6.6 Eval LLM 系统配置

持久化于 `system_settings`（ent + `docs/sql/00_system_setting_eval_llm.sql`）：

| 列 | 说明 |
|----|------|
| `eval_sim_provider` / `eval_sim_model` | UserSim 默认模型 |
| `eval_judge_provider` / `eval_judge_model` | Judge 专用（空则回退 Sim） |

Proto：`SystemSettings.eval_llm`；前端 **系统设置** `/settings` 表单（默认 Sim：`openai` / `gpt-4o-mini`）。

**运行时优先级**（Judge / Sim 各自链路）：

1. 对应 env `KRATOS_EVAL_SIM_PROVIDER` / `KRATOS_EVAL_SIM_MODEL`（UserSim）或 `KRATOS_EVAL_JUDGE_PROVIDER` / `KRATOS_EVAL_JUDGE_MODEL`（Judge）
2. DB `eval_judge_*` 或 `eval_sim_*`
3. 交叉 env / DB 回退（Judge↔Sim）
4. Provider 目录首 mini/flash 模型

### 6.7 Prometheus

| 指标 | 类型 | 说明 |
|------|------|------|
| `aranea_eval_runs_total{status}` | Counter | started / completed / error |
| `aranea_eval_case_duration_seconds` | Histogram | 逐用例执行耗时 |

### 6.8 Judge 分歧统计（P1-3）

Judge 校准闭环（对标 T6）的入口：人工标注与 llm_as_judge 分数已同表并存于 `eval_case_results`，本节聚合两者一致性，使 judge 失败模式（过宽/过严）可见。

- **数据源**：`ListJudgeAnnotatedResults`（data 层三表 join：results ⨝ runs ⨝ cases，过滤 `human_pass IS NOT NULL`）；judge 分数读 `scores_json["llm_as_judge"]`——该键存在是 judge 真实评过分的唯一可靠信号（`llm_judge_score` 列默认 0 无法区分）
- **阈值**：默认 `DefaultJudgePassThreshold = 0.5`（与框架 judge pass cutoff 一致），可经 query `threshold` 覆盖
- **输出**：`agreement_rate` / `false_pass_count`（judge 过宽）/ `false_fail_count`（judge 过严）/ 分歧明细；计数覆盖全量分歧集，明细列表按 `limit` 截断（默认 50）
- **前端**：`EvaluationAnalyticsPanel` 分歧卡片（`loadDivergence`，标注保存后自动刷新）

### 6.9 对话转用例与负反馈采集（P1-1 / P1-2）

评估飞轮的两条生产数据入口：

**P1-1 对话→用例一键转化**：TaskCard 气泡菜单「加入评估」→ `useAddToEvalDataset.openWith`（预填 input=用户消息、expected_output=Agent 最终回复、source 溯源元数据）→ `AddEvalCaseDialog`（选已有数据集或内联新建）→ 复用 `UploadCases` 通道落库。

**P1-2 负反馈采集→待审查→转用例**：

```
TaskCard 👍/👎 → SubmitMessageFeedback（chat.proto，context_json 快照 {task_id,input,output}）
  → service/chat_feedback.go 发 SystemNoticeEvent("user_feedback")
  → biz/event_bus_user_feedback_consumer.go → RecordUserFeedbackMonitor
  → monitor_events（event_key=chat.user_feedback，negative → status=warning）
  → EvaluationFeedbackPanel 读取 warning 列表（快照自包含，任务删除后仍可转化）
  → 「转为用例」复用 AddEvalCaseDialog
```

- 快照写入侧容错：`context_json` 宽容解析，坏 JSON 静默忽略（反馈持久化永不因快照失败）；input/output 快照按 2000 runes 截断（`feedbackContextSnapshotMaxRunes`）
- 复用的 chat.proto 契约定点见 [1-chat.design.md](./1-chat.design.md)（`SubmitMessageFeedback` RPC + `context_json` 字段）

---

## 七、前端

| 路径 | 组件/模块 | 说明 |
|------|----------|------|
| `/evaluation` | `EvaluationPage.vue` | 数据集 + 运行 + 结果 + **趋势/A/B**（`EvaluationAnalyticsPanel`） |
| — | `EvaluationDatasetList.vue` | 左侧数据集列表，高亮选中项 |
| — | `EvaluationCreateDialog.vue` | 新建数据集弹窗（名称 + 描述） |
| — | `EvaluationRunDialog.vue` | 启动评估弹窗（Agent 选择 + 指标 + MultiRun 次数） |
| — | `EvaluationResultsDialog.vue` | 逐用例详情 + 人工标注 + CSV/JSON 导出 |
| — | `EvaluationAnalyticsPanel.vue` | 趋势表 + A/B Run 多选对比 + judge 分歧卡片（P1-3） |
| — | `AddEvalCaseDialog.vue` | 对话/反馈→用例对话框（P1-1/P1-2；含数据集选择/内联新建 + rubric 录入 P3-2） |
| — | `EvaluationFeedbackPanel.vue` | 负反馈待审查列表（P1-2；monitor_events warning → 一键转用例） |
| — | `features/evaluation/useAddToEvalDataset.ts` | 转用例对话框状态 + 提交（metadata_json：source 溯源 + rubric） |
| — | `features/evaluation/useFeedbackReview.ts` | 负反馈列表加载（monitorStore.fetchMonitorEvents 封装） |
| — | `features/evaluation/api.ts` | 13 个 API 函数（含 snake_case/camelCase 双格式映射） |
| — | `features/evaluation/types.ts` | 13 个类型定义 |
| — | `features/evaluation/useEvaluationPage.ts` | 页面级 Composable |
| — | `features/evaluation/evaluationTableUi.ts` | 5 组表格列定义 |
| — | `features/evaluation/mappers.ts` | 轻量版 mapper |
| — | `features/evaluation/exportRunResults.ts` | 客户端 CSV/JSON 导出 |
| — | `stores/evaluation/index.ts` | Pinia Store（13 个方法） |
| — | `features/system-settings/eval-llm.ts` | Eval LLM 表单类型 + 映射 |
| `/settings` | `SystemSettingsPage.vue` | Eval LLM（UserSim/Judge）Provider/Model 持久化 |

> 已交付能力状态与已知短板见 [开发计划文档](./33-evaluation-development.md)
