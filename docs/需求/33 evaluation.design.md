# Evaluation 评估模块 — 实现设计文档

> 对应需求：[33 evaluation.md](./33%20evaluation.md)
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> **版本**：2026-05-19 | 反映代码实际状态

---

## 一、模块概述

Evaluation 模块对 Agent 输出质量进行结构化评估。当前实现采用简化的 Dataset→Case→Run→Result 模型，提供 4 种内置指标和异步评估执行。

**核心流程**：创建数据集 → 上传用例 → 启动评估运行 → 异步执行推理+评分 → 查看汇总分数和逐用例结果

**演进方向**：对标 `pkg/trpc-agent-go/evaluation` 完整能力（EvalSet/Metric/Criterion/UserSimulation/MultiRun 等），详见 §八。

---

## 二、架构

```
EvaluationService (Kratos HTTP/gRPC)
       │
EvalUsecase (biz)
       │
EvalRepo (data) ─── SQLite (Raw SQL)
       │
evaluation.Runner (async goroutine)
   ├── exact_match
   ├── contains_match
   ├── llm_as_judge   (optional LLMJudge hook)
   └── tool_call_accuracy
```

**分层职责**：

| 层 | 文件 | 职责 |
|----|------|------|
| Proto | `api/kratos/evaluation/v1/evaluation.proto` | HTTP + gRPC API 契约 |
| Service | `internal/service/evaluation.go` | proto ↔ biz 映射 + Runner 装配 |
| Biz | `internal/biz/evaluation.go` | 领域模型 + EvalRepo 接口 + EvalUsecase |
| Data | `internal/data/evaluation.go` | Raw SQL 持久化 + EnsureEvalSchema |
| Runner | `internal/evaluation/runner.go` | 异步执行 + 指标计算 + Prometheus |
| Wire | `internal/service/wire_providers.go` | NewEvaluationRunner 注入 |

---

## 三、Proto 层

### 3.1 消息定义

**EvalDataset**：评估数据集，包含名称、描述、用例数、工作区。

**EvalCase**：评估用例，包含输入、期望输出、元数据 JSON。

**EvalRun**：评估运行，包含状态、进度、4 种指标分数、错误信息。

**EvalCaseResult**：逐用例结果，包含实际输出、各指标得分/判定。

### 3.2 API 端点

| 方法 | 路径 | RPC | 说明 |
|------|------|-----|------|
| POST | `/v1/evaluation/datasets` | CreateDataset | 创建数据集 |
| GET | `/v1/evaluation/datasets/{id}` | GetDataset | 获取数据集 |
| GET | `/v1/evaluation/datasets` | ListDatasets | 列出数据集 |
| DELETE | `/v1/evaluation/datasets/{id}` | DeleteDataset | 删除数据集 |
| POST | `/v1/evaluation/datasets/{dataset_id}/cases` | UploadCases | 上传用例（JSON 数组） |
| POST | `/v1/evaluation/runs` | RunEvaluation | 启动异步评估运行 |
| GET | `/v1/evaluation/runs/{id}` | GetRun | 获取运行 + 分数 |
| GET | `/v1/evaluation/runs` | ListRuns | 列出运行 |
| GET | `/v1/evaluation/runs/{run_id}/results` | GetRunResults | 逐用例结果 |

### 3.3 RunEvaluation 请求

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| dataset_id | string | 是 | 评估数据集 ID |
| agent_id | string | 是 | 被评估的 Agent ID |
| metrics | string | 否 | 逗号分隔的指标键名，空值运行全部 4 种 |

---

## 四、Biz 层

### 4.1 领域模型

**EvalDataset**：评估数据集，字段包括 ID、Name、Description、CaseCount、Workspace、CreatedAt、UpdatedAt。

**EvalCase**：评估用例，字段包括 ID、DatasetID、Input、ExpectedOutput、MetadataJSON。

**EvalRun**：评估运行，字段包括 ID、DatasetID、AgentID、Status、TotalCases、CompletedCases、ExactMatchScore、ContainsMatchScore、LLMJudgeScore、ToolCallAccuracy、ErrorMessage、StartedAt、FinishedAt、CreatedAt。

**EvalCaseResult**：逐用例结果，字段包括 ID、RunID、CaseID、ActualOutput、ExactMatch、ContainsMatch、LLMJudgeScore、ToolCallAccuracy、ErrorMessage、CreatedAt。

### 4.2 EvalRepo 接口

```
EvalSet 侧：
  CreateDataset / GetDataset / ListDatasets / DeleteDataset / UpdateDatasetCaseCount

EvalCase 侧：
  InsertCases / ListCases

EvalRun 侧：
  CreateRun / GetRun / UpdateRun / ListRuns

EvalCaseResult 侧：
  InsertCaseResult / ListCaseResults
```

### 4.3 EvalUsecase

提供 Dataset CRUD、Case 上传、Run 创建/查询、CaseResult 查询等业务逻辑。Run 创建时自动生成 ID 并设置初始状态为 `pending`。

---

## 五、Data 层

### 5.1 数据库 Schema

由 `data.EnsureEvalSchema(ctx, db)` 创建，4 张表：

**eval_datasets**：

| 列 | 类型 | 说明 |
|----|------|------|
| id | TEXT PK | 数据集 ID |
| name | TEXT NOT NULL | 数据集名称 |
| description | TEXT NOT NULL DEFAULT '' | 描述 |
| case_count | INTEGER NOT NULL DEFAULT 0 | 用例数 |
| workspace | TEXT NOT NULL DEFAULT '' | 工作区 |
| created_at | TEXT NOT NULL | 创建时间 |
| updated_at | TEXT NOT NULL | 更新时间 |

**eval_cases**：

| 列 | 类型 | 说明 |
|----|------|------|
| id | TEXT PK | 用例 ID |
| dataset_id | TEXT NOT NULL | 所属数据集 |
| input | TEXT NOT NULL | 输入 |
| expected_output | TEXT NOT NULL DEFAULT '' | 期望输出 |
| metadata_json | TEXT NOT NULL DEFAULT '{}' | 元数据（含 expected_tools） |

**eval_runs**：

| 列 | 类型 | 说明 |
|----|------|------|
| id | TEXT PK | 运行 ID |
| dataset_id | TEXT NOT NULL | 数据集 ID |
| agent_id | TEXT NOT NULL | Agent ID |
| status | TEXT NOT NULL DEFAULT 'pending' | pending/running/completed/failed |
| total_cases | INTEGER NOT NULL DEFAULT 0 | 总用例数 |
| completed_cases | INTEGER NOT NULL DEFAULT 0 | 已完成用例数 |
| exact_match_score | REAL NOT NULL DEFAULT 0 | 精确匹配分数 |
| contains_match_score | REAL NOT NULL DEFAULT 0 | 包含匹配分数 |
| llm_judge_score | REAL NOT NULL DEFAULT 0 | LLM 评判分数 |
| tool_call_accuracy | REAL NOT NULL DEFAULT 0 | 工具调用准确率 |
| error_message | TEXT NOT NULL DEFAULT '' | 错误信息 |
| started_at | TEXT NOT NULL DEFAULT '' | 开始时间 |
| finished_at | TEXT NOT NULL DEFAULT '' | 结束时间 |
| created_at | TEXT NOT NULL | 创建时间 |

**eval_case_results**：

| 列 | 类型 | 说明 |
|----|------|------|
| id | TEXT PK | 结果 ID |
| run_id | TEXT NOT NULL | 运行 ID |
| case_id | TEXT NOT NULL | 用例 ID |
| actual_output | TEXT NOT NULL DEFAULT '' | 实际输出 |
| exact_match | INTEGER NOT NULL DEFAULT 0 | 精确匹配（0/1） |
| contains_match | INTEGER NOT NULL DEFAULT 0 | 包含匹配（0/1） |
| llm_judge_score | REAL NOT NULL DEFAULT 0 | LLM 评判分数 |
| tool_call_accuracy | REAL NOT NULL DEFAULT 0 | 工具调用准确率 |
| error_message | TEXT NOT NULL DEFAULT '' | 错误信息 |
| created_at | TEXT NOT NULL | 创建时间 |

### 5.2 Schema 初始化

`EnsureEvalSchema` 已定义但**未在 `NewData()` 启动期调用**（EP-DATA-01），无表时首跑可能失败。需在 `data.go` 的初始化流程中补调。

### 5.3 用例上传格式

`UploadCases` 接受 JSON 数组：

```json
[
  { "input": "What is 2+2?", "expected_output": "4" },
  {
    "input": "Call the weather tool",
    "expected_output": "The weather is sunny",
    "metadata_json": "{\"expected_tools\":[\"get_weather\"]}"
  }
]
```

---

## 六、Runner

### 6.1 核心类型

**AgentRunner**：`func(ctx context.Context, agentID, input string) (string, error)` — 执行单条评估用例的推理函数。

**LLMJudge**：`func(ctx context.Context, input, expected, actual string) (float32, error)` — 可选的 LLM 评判函数，nil 时静默跳过 llm_as_judge 指标。

**Runner**：异步评估执行器，持有 EvalUsecase、AgentRunner、LLMJudge。

### 6.2 执行流程

1. `Start(ctx, run, metrics)` 派发异步 goroutine，立即返回
2. 加载数据集全部用例，更新运行状态为 `running`
3. 逐用例执行：调用 AgentRunner 获取实际输出 → 计算选定指标 → 写入 CaseResult → 更新 CompletedCases
4. 聚合分数：各指标在全部用例上的平均值
5. 更新运行状态为 `completed`（或 `failed`）

### 6.3 Wire 注入

`NewEvaluationRunner(uc, chat)` 在 `wire_providers.go` 中注入：
- AgentRunner 通过 `ChatService.RunNativeTurnUnary` 执行，为每条用例创建临时 Session
- LLMJudge 当前注入为 `nil`，llm_as_judge 指标静默跳过

### 6.4 Prometheus 指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `aranea_eval_runs_total{status}` | Counter | 按状态的运行次数（started / completed / error） |
| `aranea_eval_case_duration_seconds` | Histogram | 逐用例执行时间 |

---

## 七、Service 层

`EvaluationService` 实现 `v1.EvaluationServiceServer`，负责 proto ↔ biz 类型转换和参数校验。

- Dataset CRUD：Create/Get/List/Delete
- Case 上传：UploadCases（解析 JSON 数组，批量插入）
- Run 管理：RunEvaluation（创建 Run + 启动 Runner）、GetRun、ListRuns、GetRunResults

HTTP 和 gRPC 均已注册（`http.go` / `grpc.go`）。

---

## 八、演进方向：对标 trpc-agent-go evaluation

当前实现为简化模型，`pkg/trpc-agent-go/evaluation` 提供了更完整的能力。以下为框架能力与当前实现的差距：

### 8.1 trpc-agent-go evaluation 核心概念

| 概念 | 说明 | 当前状态 |
|------|------|----------|
| AgentEvaluator | 评估器主接口（Evaluate/Close） | ❌ 未集成，使用自建 Runner |
| EvalSet | 评估集（含多轮对话 Invocation） | 🟡 简化为 Dataset+Case |
| EvalMetric | 评估指标（含 Criterion 配置） | 🟡 硬编码 4 种，无配置化 |
| ToolTrajectory | 工具调用轨迹评估 | ❌ 仅有 tool_call_accuracy 简化版 |
| FinalResponse | 终态响应多维度评估（Text/JSON/ROUGE/XML） | ❌ 仅有 exact/contains |
| LLM-as-Judge | LLM 评判（含 Rubric/Template/VariableBinding） | 🟡 仅简单打分 hook |
| UserSimulation | 用户模拟交互 | ❌ 未实现 |
| MultiRun | 多次运行取平均 | ❌ 未实现 |
| pass@k / pass^k | 通过率指标 | ❌ 未实现 |
| EvalResult 持久化 | 评估结果存储/查询 | 🟡 简化版已实现 |
| PromptIter | Prompt 迭代优化 | ❌ 未实现 |

### 8.2 框架目录结构

```
pkg/trpc-agent-go/evaluation/
├── evaluation.go          # AgentEvaluator 接口
├── options.go             # 评估选项
├── pass.go                # pass@k / pass^k
├── evalset/               # 评估集管理
│   ├── evalset.go         # EvalSet 接口 + EvalCase 定义
│   ├── evalcase.go        # EvalCase（含 Invocation/Conversation）
│   ├── local/             # 本地文件存储
│   ├── inmemory/          # 内存存储
│   └── mysql/             # MySQL 存储
├── metric/                # 指标管理
│   ├── metric.go          # Metric 接口
│   ├── criterion/         # 评估准则
│   │   ├── llm/           # LLM-as-Judge 配置
│   │   ├── finalresponse/ # 终态响应准则
│   │   ├── tooltrajectory/# 工具轨迹准则
│   │   ├── text/          # 文本匹配准则
│   │   ├── json/          # JSON 匹配准则
│   │   └── xml/           # XML 匹配准则
│   └── registry/          # 指标注册表
├── evaluator/             # 评估器
│   ├── finalresponse/     # 终态响应评估器
│   ├── tooltrajectory/    # 工具轨迹评估器
│   ├── llm/               # LLM 评估器
│   │   ├── finalresponse/ # LLM 终态响应评分
│   │   ├── rubricresponse/# Rubric 响应评分
│   │   └── rubricknowledgerecall/ # Rubric 知识召回评分
│   └── registry/          # 评估器注册表
├── service/               # 评估服务
│   └── local/             # 本地评估服务
├── evalresult/            # 评估结果
│   ├── inmemory/          # 内存存储
│   ├── local/             # 本地文件存储
│   └── mysql/             # MySQL 存储
├── status/                # 评估状态枚举
├── usersimulation/        # 用户模拟
└── workflow/promptiter/   # Prompt 迭代优化
```

### 8.3 演进路径

1. **Phase A**：集成 trpc `AgentEvaluator`，替换自建 Runner
2. **Phase B**：引入 EvalSet 完整模型（多轮对话 Invocation），升级 Proto
3. **Phase C**：引入 Metric/Criterion 配置化，支持 ToolTrajectory/FinalResponse 多维度评估
4. **Phase D**：引入 UserSimulation + MultiRun + pass@k
5. **Phase E**：Prompt 迭代优化闭环
