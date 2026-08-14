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
EvalStores (data, ISP) ─── SQLite (Raw SQL, EnsureEvalSchema @ NewData)
       │   DatasetStore / CaseStore / RunStore / RunQueryStore / ResultStore / GovernanceStore
       │   （同一 evalRepo 一身多口；宽 Repo 已 Deprecated）
       │
evaluation.Runner (async goroutine)
   ├── FrameworkBridge ── trpc AgentEvaluator (默认)
   │      ├── exact_match / contains_match (FinalResponse text)
   │      ├── json_match / xml_match / rouge_l (FinalResponse criterion)
   │      ├── tool_call_accuracy / tool_trajectory (ToolTrajectory)
   │      └── llm_as_judge (框架 llm_rubric_response 评估器 + JudgeRunner 注入)
   └── executeLegacy (FrameworkBridge 不可用时的回退)
```

**分层职责**：

| 层 | 文件 | 职责 |
|----|------|------|
| Proto | `api/kratos/evaluation/v1/evaluation.proto` | HTTP + gRPC API 契约（20 RPC + 39 Message） |
| Service | `internal/service/evaluation.go` | proto ↔ biz 映射；RunEvaluation 触发 Runner |
| Service | `internal/service/evaluation_runner.go` | Wire：`NewEvaluationRunner` 装配 AgentRunner + LLMJudge + FrameworkBridge |
| Service | `internal/service/evaluation_after_turn.go` | Wire：`NewEvaluationAfterTurnTrigger` 创建 AfterTurn 触发器 |
| Biz | `internal/biz/evaluation.go` | 类型重导出（子包 `evaluation/` 的别名） |
| Biz | `internal/biz/evaluation/evaluation.go` | 领域模型 + EvalUsecase（构造注入 `Stores`，端口字段未导出） |
| Biz | `internal/biz/evaluation/ports.go` | 持久化端口：`Stores` DTO + 6 个窄接口（ISP）；宽 `Repo` Deprecated |
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

**EvalRun**：评估运行（状态、进度、4 种指标汇总分数、`scores_json` 扩展分、`pass_at_k`/`pass_hat_k`、错误信息、`dataset_hash` 数据集快照（P3-5））。

**EvalCaseResult**：逐用例结果（实际输出、各指标得分/判定、人工标注字段）。

**EvalTrendPoint** / **EvalRunComparison**：趋势点与 A/B 对比结果。

**JudgeDivergence** / **JudgeDivergenceCase**（P1-3）：judge 与人工标注的一致性统计（一致率 / false_pass / false_fail / 分歧用例明细）。

**EvalFailureGroup** / **GetFailureGroupsResponse**（P2-3）：失败归组（`error_message` 聚合：`count` / `run_count` / `latest_at`，外加 `total_failed`）。

**EvalRunPreference**（P3-3）：Pairwise 偏好裁决记录（`dataset_id` / `run_id_a` / `run_id_b` / `winner_run_id` / `comment` / `created_by` / `created_at`）。

**EvalGateConfig**（P2-1）：发布质量门禁单例配置（`enabled` / `agent_id` / `dataset_id` / `metric` / `min_score` / `max_drop` / `updated_at`）。

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
| PATCH | `/v1/evaluation/runs/{run_id}/results/{result_id}/annotation` | AnnotateCaseResult | 人工标注（Phase 8 新增 `clear_human_pass`/`clear_human_score` 显式清除位——proto3 optional 不接收 JSON null，B2） |
| GET | `/v1/evaluation/agents/{agent_id}/trend` | GetAgentEvalTrend | Agent 分数趋势 |
| POST | `/v1/evaluation/runs/compare` | CompareEvalRuns | A/B 或多 run 对比 |
| GET | `/v1/evaluation/datasets/{dataset_id}/judge-divergence` | GetJudgeDivergence | judge/人工分歧统计（P1-3，query：agent_id/threshold/limit） |
| GET | `/v1/evaluation/datasets/{dataset_id}/failure-groups` | GetFailureGroups | 失败用例按 error_message 归组（P2-3，query：limit） |
| POST | `/v1/evaluation/preferences` | SubmitRunPreference | 提交 Pairwise 偏好裁决（P3-3） |
| GET | `/v1/evaluation/preferences` | ListRunPreferences | 偏好记录列表（P3-3，query：dataset_id/limit） |
| GET | `/v1/evaluation/gate` | GetEvalGate | 读取发布质量门禁单例配置（P2-1） |
| PUT | `/v1/evaluation/gate` | UpdateEvalGate | 更新发布质量门禁单例配置（P2-1） |

### 3.3 RunEvaluation 请求

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| dataset_id | string | 是 | 评估数据集 ID |
| agent_id | string | 是 | 被评估的 Agent ID |
| metrics | string | 否 | 逗号分隔指标键名，空值运行 4 种核心指标；扩展见 §6.5 |
| num_runs | int32 | 否 | 每用例重复次数（AgentEvaluator MultiRun，默认 1，上限 `EvalMaxNumRuns=20`，Phase 8 B1：API 值直传框架，原默认值覆盖缺陷已修复） |
| use_user_simulation | bool | 否 | 启用 UserSimulation（脚本或 LLM） |

---

## 四、Biz 层

### 4.1 领域模型

**Dataset** / **Case** / **Run** / **CaseResult** / **CaseResultAnnotation** / **TrendPoint** / **RunComparison** — 见 `internal/biz/evaluation/evaluation.go`。

人工标注字段：`HumanPass`、`HumanScore`、`HumanComment`、`AnnotatedAt`、`AnnotatedBy`。

辅助类型：`CaseUpload`（用例上传行）、`Scores`（指标键→分数 map）、`LLMSetting`（Eval LLM 平台默认配置）。

### 4.2 持久化端口（ISP）

生产路径（Wire / `Usecase`）注入 `Stores` DTO，**不**依赖宽 `Repo`。同一 `data.evalRepo` 实现全部窄口（一身多口）。`Repo` 保留为 Deprecated 嵌入组合，供测试 mock 与 `var _ Repo = (*evalRepo)(nil)` 编译期检查（对齐 Memory `SessionAdminStore` / `MemoryLayerPorts`）。

| 接口 | 方法 | 数 |
|------|------|----|
| `DatasetStore` | CreateDataset / GetDataset / ListDatasets / DeleteDataset / UpdateDataset | 5 |
| `CaseStore` | InsertCasesWithCountUpdate / ListCases | 2 |
| `RunStore` | CreateRun / GetRun / UpdateRun / DeleteRun / ListRuns / FailStaleRuns | 6 |
| `RunQueryStore` | ListTrendPoints / GetRunsByIDs | 2 |
| `ResultStore` | InsertCaseResult / ListCaseResults / GetCaseResult / UpdateCaseResultAnnotation / ListJudgeAnnotatedResults / ListFailureGroups | 6 |
| `GovernanceStore` | InsertRunPreference / ListRunPreferences / GetGateConfig / UpsertGateConfig | 4 |

`RunStore` / `ResultStore` 各略超 ≤5（FailStale 属 run 生命周期；judge/failure 聚合属 case-result 表），均明显窄于原 25 方法上帝接口。

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

P2/P3 新增 2 张表 + 1 个列：

- `eval_gate_config`（P2-1）：发布门禁单例配置表（单行）；`enabled` 以 INTEGER 0/1 存储（与既有 eval 表 raw SQL INTEGER bool 惯例一致）
- `eval_run_preference`（P3-3）：Pairwise 偏好记录（`run_id_a` / `run_id_b` / `winner_run_id` / `comment` / `created_by`）
- `eval_runs.dataset_hash`（P3-5）：运行启动时写入的数据集内容 hash（随 eval 域既有双轨机制落地：Ent Schema L1 自动迁移 + `EnsureEvalSchema` 幂等 ALTER 兜底存量库）

### 5.2 Ent Schema

6 个 Ent Schema（`internal/data/ent/schema/eval_*.go`）显式映射表名（`entsql.Annotation{Table: ...}`）：

| Schema | 表名 | Edge |
|--------|------|------|
| `EvalDataset` | `eval_datasets` | → cases / runs |
| `EvalCase` | `eval_cases` | ← dataset / → results |
| `EvalRun` | `eval_runs` | ← dataset / → results（P3-5 新增 `dataset_hash` 列） |
| `EvalCaseResult` | `eval_case_results` | ← run / ← case |
| `EvalGateConfig` | `eval_gate_config`（P2-1，单例行） | — |
| `EvalRunPreference` | `eval_run_preference`（P3-3 建 `dataset_id` 索引；Phase 8 补 `run_id_a`/`run_id_b` 索引，Y9） | — |

> Ent Schema 与 Raw SQL `EnsureEvalSchema` 并存：Ent Schema 作为类型映射真相源，运行期建表由 `EnsureEvalSchema` 完成（含 ALTER 兼容旧库）。

### 5.3 Schema 初始化

`NewData()` 启动期调用 `EnsureEvalSchema`（EP-DATA-01）。

### 5.4 级联删除

- `DeleteDataset`：事务内依次删 `eval_run_preferences`（按 dataset_id，Y11）→ `eval_case_results`（按 run 子查询）→ `eval_runs` → `eval_cases` → `eval_datasets`
- `DeleteRun`：事务内先删 `eval_run_preferences`（按 run_id_a/run_id_b 两侧，Y11）→ `eval_case_results` → `eval_runs`

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

**judge 解析链（2026-08-08 as-built 修复）**：

- `llm_as_judge` 走框架 `llm_rubric_response` 评估器（`framework_metrics.go`）：其 prompt 渲染用例 rubrics 并要求按 rubric 输出 JSON 分值，judge 评语始终体现用例评分标准；无自定义 rubric 的用例由 evalset 适配层合成默认 rubric（该评估器硬要求每用例至少一条 rubric）
- JudgeRunner 的 system instruction 保持中立（`judge_runner.go`：不强制任何输出格式）——各框架评估器 prompt 自定义输出格式（llm_final_response 两行 verdict / llm_rubric_response JSON rubricScores），在 system 层强加格式会与 user prompt 冲突导致间歇解析失败（"no final response blocks found"）
- `judgeReasonFromRuns`（`framework.go`）从 per-run 结果提取 judge 评语写入进程日志——框架聚合计分时丢弃 Details，需从单次运行结果回填
- `session_user_id` / `session_state`：覆盖 SessionInput 默认值（UserID 默认 `"eval"`，State 默认空）
- `source` / `task_id` / `session_id`：溯源元数据（P1-1 对话→用例一键转化写入 `source="chat"` + 来源任务/会话 ID；不参与执行语义，仅供回溯）

> **框架版本说明（2026-08-08）**：`trpc-agent-go/evaluation` 模块经 `go.mod` replace 到 vendored 副本（`pkg/trpc-agent-go/evaluation`）——发布版 v1.9.0 的 `EvalCase` 无用例级 `Rubrics` 字段。切换带来一处 API 迁移：`promptiter/engine.RunRequest.TrainEvalSetIDs/ValidationEvalSetIDs` → `Train/Validation []EvalSetInput`（~~`internal/agent/prompt_iter_adapter.go`~~，已删除 2026-08-14：TEST_ONLY 僵尸实现，生产走 `biz.PromptRefiner`）。

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

- **桌面端链路**：TaskCard → SessionPanel/ChatMessagePanel 逐层 `@add-to-eval` 转发 → ChatPage 渲染对话框
- **移动端链路（2026-08-08 补）**：MobileTasksPage 绑定 `@add-to-eval`/`@feedback` → 共享 workspace（`useChatWorkspace` 在 MobileLayout 布局层创建并 provide）→ **对话框在 MobileLayout 布局层渲染**（workspace 状态在布局层，若只在页面内渲染则事件只改状态无 UI）

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

### 6.10 发布质量门禁（P2-1；Phase 8 重构为异步 advisory）

- **配置**：`eval_gate_config` 单例表（单行）；biz `GateConfig`（`internal/biz/evaluation/governance.go`），data 实现 `internal/data/evaluation_governance.go`；API `GET/PUT /v1/evaluation/gate`；`PUT` 校验指标名白名单（Y12）
- **判定流程（2026-08-14 Y2/Y12 重构）**：`PublishGate.Check(ctx, trigger)`（`internal/evaluation/gate.go`）——加载单例配置（未启用直接放行）→ in-flight 去重（已有 pending/running 的 gate run 则直接放行，防发布风暴扇出 N 份全量评估的 LLM 成本）→ **后台异步启动回归运行**（不再随发布请求同步执行——旧实现请求被取消后写成「执行失败」假拦截）→ 阈值越界（低于 `min_score` / 较基线跌幅超 `max_drop`）仅以通知事件 advisory 透出，**不阻断发布**
- **唯一硬阻断（Y12）**：配置了 `max_drop` 但无任何 completed 基线运行时，返回 Conflict「无可用基线」——drop 检查否则被静默跳过；同时后台已启动的 gate run 即成为重试时的基线
- **注入点**：`internal/service/chat_wire.go` `ProvidePublishGate` 装配；消费方为 skill 发布（`internal/service/skill.go`）与 pack 安装（`internal/service/pack.go`）——均 nil-safe，门禁不可用时不阻断主流程
- **前端**：`EvaluationGateDialog.vue` 配置对话框（开关 / Agent / 数据集 / 指标 / min_score / max_drop）+ 评估页入口

### 6.11 在线评估：采样率与连跌告警（P2-2）

- **采样率**：`AgentEvalAutoConfig.SampleRate`（`internal/biz/agent_eval_config.go`，`config_json.evaluation.sample_rate`）——(0,1] 生效，越界归一化为 1.0（向后兼容既有配置）；采样判定位于 after_turn 触发链路的限流判定之前（`internal/evaluation/after_turn.go`）
- **连跌告警**：`ScoreDropAlerter.CheckAfterRun`（`internal/evaluation/drop_alert.go`）——after_turn run 完成后读 agent 配置 `alert_consecutive_drops`（N，0 禁用）与 `alert_metric`（默认 llm_as_judge），取最近 N 条 online run，该指标分数严格连跌则发布 SystemNoticeEvent 通知
- **装配**：`chat_wire.go` `ProvideEvaluationRunner` 内 `runner.WithDropAlerter(evaluation.NewScoreDropAlerter(...))`；agents/bus 任一为 nil 则不挂（nil-safe）
- **趋势拆分**：趋势面板按 `trigger_source`（manual / after_turn / gate）筛选，拆分在线/离线序列

### 6.12 失败归组（P2-3）

- **API**：`GET /v1/evaluation/datasets/{dataset_id}/failure-groups` → `{total_failed, groups[]{error_message, count, run_count, latest_at}}`
- **实现**：data 层 `ListFailureGroups`（`internal/data/evaluation_governance.go`）纯 SQL：`eval_case_results ⨝ eval_runs`，仅失败用例，`GROUP BY error_message`，按 count 降序 + limit
- **前端**：`EvaluationAnalyticsPanel.vue` 失败归组面板；运行记录全部终态后自动刷新

### 6.13 Pairwise 偏好裁决（P3-3）

- **表**：`eval_run_preference`（Ent schema `eval_run_preference.go`）：`dataset_id` / `run_id_a` / `run_id_b` / `winner_run_id` / `comment` / `created_by` / `created_at`
- **校验**：`winner_run_id` 必须等于 `run_id_a` 或 `run_id_b`（service/biz 校验，违反返回 Conflict/BadRequest）
- **API**：`POST /v1/evaluation/preferences` 提交；`GET /v1/evaluation/preferences`（query dataset_id/limit）列表
- **前端**：A/B 对比结果行加「更优」按钮（`emit('prefer', row)` 由页面提交）；偏好列表展示

### 6.14 红队用例包（P3-4）

- **交付形态**：预制对抗用例集 JSON（prompt injection / 越权 / 数据泄漏），存放 `internal/evaluation/testdata/`，经既有 UploadCases JSON 导入通道入库（不绑 pack 机制）；用例 metadata 携带攻击类别标记
- **专用指标**：`redteam_attack_success_rate`——`internal/evaluation/redteam.go` `mergeAttackSuccessRate` 按用例 metadata 攻击标记 + 结果判定攻击成功数/总数，合并进 `run.ScoresJSON`
- **覆盖**：legacy（exact/contains）与 framework（judge）两条执行路径均计算

### 6.15 数据集版本快照（P3-5）

- **算法**：`internal/evaluation/dataset_hash.go` `hashEvalCases`——cases 按 ID 排序后 SHA-256（字段间 NUL 分隔），取 hex 前 16 字符
- **写入时机**：`Runner.execute` 在 run 启动时写入 `eval_runs.dataset_hash`（Ent schema `eval_run.go` L1 自动迁移 + `EnsureEvalSchema` 幂等 ALTER 兜底存量库，与 eval 域既有双轨机制一致）
- **对比透出**：`CompareEvalRuns` 对比响应每行携带 `dataset_hash`（proto `EvalRun` 字段 20、对比消息字段 15）
- **前端**：参与对比的 runs 间 hash 不一致时 q-banner 提示「数据集已变更，分数不可直接比较」

### 6.17 Phase 8 可靠性与隔离加固（2026-08-14）

**结论：可行。推荐 Aranea 侧后处理路径**（不改 vendored 框架，与 `redteam.go` 同模式）：executeFramework 提取 knowledge_search chunks → 存 EvalCaseResult 扩展字段 → faithfulness 用既有 judge runner 打分合并进 run scores。

**证据链**：

- 框架 inference 捕获工具调用与结果——`pkg/trpc-agent-go/evaluation/service/internal/inference/inference.go` L227-245：`IsToolCallResponse`→convertTools 捕获 name+args；`IsToolResultResponse`→mergeToolResultResponse 将工具返回 JSON 反序列化进 `Tool.Result`
- `evalset.Invocation.Tools []*Tool{ID,Name,Arguments,Result}`（evalset/evalcase.go L96-119）
- knowledge_search 工具返回 chunks 列表（id/content/score/doc_id，`internal/tools/knowledge/tool.go`）
- Aranea 挂接点：`internal/evaluation/framework.go` executeFramework 已访问 `rd.Inference.Inferences[0]` 取 FinalResponse（L157-168），同位置可读 `.Tools`

**注意点**：

1. 评估 agent 需绑定知识库才会产生检索调用
2. 无检索调用的用例应记 N/A 并在聚合时排除（避免 0 分拉低均分）
3. context_precision 需 chunk 级相关性判定，judge prompt 设计是主要工作量
4. 自定义框架 evaluator 路径（实现 `evaluator.Evaluator` 接口 + `mgr.Add` 注册，类比 tooltrajectory）亦可行，但需动框架层

**排期建议**：faithfulness 先行（工作量中），context_precision 后置。

### 6.18 第三轮深扫加固（2026-08-14）

**后端**：

1. **执行 panic 守卫（高）**：`Runner.execute` 顶部 `defer recover`——框架/agent 运行时 panic 时 safego 只恢复 goroutine，run 行曾永远卡 `running`（前端 3s 轮询不停、趋势/门禁基线读到假活跃行，直到下次重启 Y10 清扫）。现转换为 `failed` 终态落库 + 哨兵错误返回，一处覆盖 manual/after_turn/gate 全部执行路径；回归测试 `TestRunnerPanicMarksRunFailed`。
2. **门禁基线时间过滤**：`scanRuns(notAfter, excludeID...)` 排除创建于 gate run 之后的 run（更新的 run 测的是更新的代码，不能作为发布前基线），RFC3339 UTC 字典序比较；同秒平局由 excludeID 兜底；回归测试 `TestScanRunsExcludesRunsYoungerThanGateRun`。
3. **空数据集拒跑**：`RunEvaluation` 在创建 run 前校验用例数——空数据集产生的零分 "completed" 行会污染趋势序列并被门禁选为基线。
4. **进度持久化节流**：每用例 `UpdateRun` 节流为 1 次/秒（仅为轮询 UI 保鲜），终态写始终落库；`InsertCaseResult` 失败计入 case 错误使 run 失败而非静默丢行。
5. judge/user-sim 超时装饰器内 2 处裸 `go func()` 改为 `safego.Go`（K7 约定）。

**前端**：

6. **陈旧响应守卫**（loadRuns 之外补齐同类竞态）：`loadTrend`/`loadDivergence`/`loadFailureGroups`/`loadPreferences`/`loadCaseResults` 增加单调递增序列号，快速切换数据集/Agent/结果对话框时旧响应不再覆盖新状态。
7. 删除当前数据集时同步清零 `runsTotal`（分页总数残留）；结果导出超过 5000 条上限时 warning 提示截断（`evaluationPage.exportTruncated`）。

**记录不修**：连跌告警对持续新低重复触发（设计意图，频率受 min_interval 约束）；趋势指标 0 与"未计算"不可区分（需 nullable score，超出本轮范围）。

---

## 七、前端

| 路径 | 组件/模块 | 说明 |
|------|----------|------|
| `/evaluation` | `EvaluationPage.vue` | 数据集 + 运行 + 结果 + **趋势/A/B**（`EvaluationAnalyticsPanel`） |
| — | `EvaluationDatasetList.vue` | 左侧数据集列表，高亮选中项 |
| — | `EvaluationCreateDialog.vue` | 新建数据集弹窗（名称 + 描述） |
| — | `EvaluationRunDialog.vue` | 启动评估弹窗（Agent 选择 + 指标 + MultiRun 次数） |
| — | `EvaluationResultsDialog.vue` | 逐用例详情 + 人工标注 + CSV/JSON 导出 |
| — | `EvaluationAnalyticsPanel.vue` | 趋势表 + A/B Run 多选对比 + judge 分歧卡片（P1-3）+ trigger_source 趋势拆分（P2-2）+ 失败归组面板（P2-3）+ pairwise「更优」按钮与偏好列表（P3-3）+ 数据集变更 q-banner 提示（P3-5） |
| — | `EvaluationGateDialog.vue` | 发布质量门禁配置对话框（P2-1：开关/Agent/数据集/指标/min_score/max_drop） |
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
