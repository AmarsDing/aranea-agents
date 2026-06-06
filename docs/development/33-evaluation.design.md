# Evaluation 评估模块 — 实现设计文档

> 对应需求：[33 evaluation.md](./33%20evaluation.md)
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)

---

## 一、模块概述

Evaluation 模块对 Agent 输出质量进行结构化评估。采用 Dataset → Case → Run → Result 模型，提供 4 种内置指标、异步评估执行、人工标注与客户端报告导出。

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
| Proto | `api/kratos/evaluation/v1/evaluation.proto` | HTTP + gRPC API 契约 |
| Service | `internal/service/evaluation.go` | proto ↔ biz 映射；RunEvaluation 触发 Runner |
| Service | `internal/service/evaluation_runner.go` | Wire：`NewEvaluationRunner` 装配 AgentRunner + LLMJudge + FrameworkBridge |
| Biz | `internal/biz/evaluation.go` | 领域模型 + EvalRepo 接口 + EvalUsecase |
| Data | `internal/data/evaluation.go` | Raw SQL 持久化 + EnsureEvalSchema |
| Runner | `internal/evaluation/runner.go` | 异步调度、Prometheus |
| Metrics | `internal/evaluation/metrics.go` | 指标解析与 legacy 路径计分（SRP） |
| Framework | `internal/evaluation/framework.go` | trpc AgentEvaluator 桥接 + UserSimulator 注入 |
| FrameworkMetrics | `internal/evaluation/framework_metrics.go` | 扩展指标注册（JSON/XML/ROUGE/ToolTrajectory） |
| Scores | `internal/evaluation/scores.go` | `scores_json` 映射与 run 聚合 |
| UserSim | `internal/evaluation/scripted_simulator.go` / `llm_simulator.go` | 脚本 vs LLM UserSimulation |
| EvalLLM | `internal/evaluation/eval_llm_resolve.go` | env + system_settings 模型解析 |
| Adapter | `internal/evaluation/evalset_adapter.go` | biz EvalCase → trpc EvalSet |
| EvalTools | `internal/evaluation/evalset_tools.go` | expected_tool_calls → Invocation.Tools |
| Chat | `internal/evaluation/chat_runner.go` | ChatService → runner.Runner 适配 |
| Judge | `internal/evaluation/llm_judge.go` | LLM-as-Judge |

---

## 三、Proto 层

### 3.1 消息定义

**EvalDataset**：评估数据集（名称、描述、用例数、工作区）。

**EvalCase**：评估用例（输入、期望输出、metadata_json）。

**EvalRun**：评估运行（状态、进度、4 种指标汇总分数、`scores_json` 扩展分、`pass_at_k`/`pass_hat_k`、错误信息）。

**EvalCaseResult**：逐用例结果（实际输出、各指标得分/判定、人工标注字段）。

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

### 3.3 RunEvaluation 请求

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| dataset_id | string | 是 | 评估数据集 ID |
| agent_id | string | 是 | 被评估的 Agent ID |
| metrics | string | 否 | 逗号分隔指标键名，空值运行 4 种核心指标；扩展见 §6.6 |
| num_runs | int32 | 否 | 每用例重复次数（AgentEvaluator MultiRun，默认 1） |
| use_user_simulation | bool | 否 | 启用 UserSimulation（脚本或 LLM） |

---

## 四、Biz 层

### 4.1 领域模型

**EvalDataset** / **EvalCase** / **EvalRun** / **EvalCaseResult** / **EvalCaseResultAnnotation** — 见 `internal/biz/evaluation.go`。

人工标注字段：`HumanPass`、`HumanScore`、`HumanComment`、`AnnotatedAt`、`AnnotatedBy`。

### 4.2 EvalRepo 接口

```
Dataset：Create / Get / List / Update / Delete / UpdateCaseCount
Case：Insert / List
Run：Create / Get / Update / Delete / List
CaseResult：Insert / List / Get / UpdateAnnotation
```

### 4.3 EvalUsecase

- Dataset CRUD + UploadCases（JSON 数组解析）
- Run 创建（初始 status=`pending`）+ 查询 + DeleteRun
- CaseResult 列表 + AnnotateCaseResult（EVAL-02）

---

## 五、Data 层

### 5.1 数据库 Schema

由 `data.EnsureEvalSchema(ctx, db)` 创建，4 张表：`eval_datasets`、`eval_cases`、`eval_runs`、`eval_case_results`。

`eval_case_results` 含人工标注列与 `scores_json`；`eval_runs` 含 `pass_at_k`/`pass_hat_k`/`trigger_source`/`num_runs`/`scores_json`，通过 ALTER 迁移兼容旧库。

### 5.2 Schema 初始化

`NewData()` 启动期调用 `EnsureEvalSchema`（EP-DATA-01 ✅）。

### 5.3 级联删除

- `DeleteDataset`：事务内先删 `eval_cases`，再删 `eval_datasets`
- `DeleteRun`：事务内先删 `eval_case_results`，再删 `eval_runs`

### 5.4 用例上传格式

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

---

## 六、Runner 与 FrameworkBridge

### 6.1 AgentRunner / LLMJudge

**AgentRunner**：`func(ctx, agentID, input) (string, error)` — 经 `ChatService.RunNativeTurnUnary` 执行，每条用例创建临时 Session。

**LLMJudge**：`NewLLMJudge(catalog, rt, sys)` — 模型解析见 §6.7；env 优先于 `system_settings`。

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

### 6.6 Eval LLM 系统配置

持久化于 `system_settings`（ent + `docs/sql/00_system_setting_eval_llm.sql`）：

| 列 | 说明 |
|----|------|
| `eval_sim_provider` / `eval_sim_model` | UserSim 默认模型 |
| `eval_judge_provider` / `eval_judge_model` | Judge 专用（空则回退 Sim） |

Proto：`SystemSettings.eval_llm`；前端 **系统设置** `/settings` 表单（默认 Sim：`openai` / `gpt-4o-mini`）。

**运行时优先级**（Judge / Sim 各自链路）：

1. 对应 env `KRATOS_EVAL_*`
2. DB `eval_judge_*` 或 `eval_sim_*`
3. 交叉 env / DB 回退（Judge↔Sim）
4. Provider 目录首 mini/flash 模型

### 6.7 Prometheus

| 指标 | 类型 | 说明 |
|------|------|------|
| `aranea_eval_runs_total{status}` | Counter | started / completed / error |
| `aranea_eval_case_duration_seconds` | Histogram | 逐用例执行耗时 |

---

## 七、前端

| 路径 | 组件 | 说明 |
|------|------|------|
| `/evaluation` | `EvaluationPage.vue` | 数据集 + 运行 + 结果 + **趋势/A/B**（`EvaluationAnalyticsPanel`） |
| — | `EvaluationResultsDialog.vue` | 逐用例详情 + 人工标注 + CSV/JSON 导出 |
| — | `features/evaluation/` | api / types / `getAgentEvalTrend` / `compareEvalRuns` / exportRunResults |
| `/settings` | `SystemSettingsPage.vue` | Eval LLM（UserSim/Judge）Provider/Model 持久化 |

---

## 八、演进方向（对标 trpc-agent-go evaluation）

| 能力 | 当前状态 |
|------|----------|
| AgentEvaluator 集成 | ✅ FrameworkBridge |
| EvalSet 多轮 Invocation | ✅ metadata_json.turns |
| UserSimulation | ✅ 脚本 + LLM（simRunner） |
| pass@k / pass^k | ✅ num_runs>1 |
| AfterTurn 自动评估 | ✅ NativeTurnAfterHook |
| 趋势 / A/B API + 前端 | ✅ GetAgentEvalTrend / CompareEvalRuns / AnalyticsPanel |
| FinalResponse ROUGE/XML/JSON | ✅ opt-in metrics |
| ToolTrajectory 完整序列 | ✅ tool_trajectory + expected_tool_calls |
| Eval LLM 系统配置 | ✅ system_settings + Settings 页 |

框架目录结构见 `pkg/trpc-agent-go/evaluation/`；Phase 演进路径见 [33-evaluation-development.md](./33-evaluation-development.md) Phase 5。
