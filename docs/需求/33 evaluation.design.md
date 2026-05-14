# Evaluation 评估模块 — 实现设计文档

> 对应需求：`33 evaluation.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 自动化评估：评估集管理、评估运行（推理 + 评分）、LLM-as-Judge、用户模拟、多次运行聚合、pass@k/pass^k 指标、可视化评估平台。对标 trpc-agent-go `evaluation` 包。

核心流程：**EvalSet 定义 → Inference 推理 → Evaluate 评分 → Result 聚合 → 前端可视化**

---

## 二、Proto 层

### 2.1 新增文件

`api/kratos/evaluation/v1/evaluation.proto`

```protobuf
syntax = "proto3";

package kratos.evaluation.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";
import "google/protobuf/struct.proto";

option go_package = "aranea-agents/api/kratos/evaluation/v1;v1";
option java_multiple_files = true;
option java_package = "api.kratos.evaluation.v1";

// ──────────────── EvalSet 评估集 ────────────────

message EvalSet {
  string id = 1;
  string name = 2;
  string description = 3;
  string agent_id = 4;
  repeated EvalCase cases = 5;
  string created_at = 6;
  string updated_at = 7;
}

message EvalCase {
  string eval_id = 1;
  string eval_mode = 2; // "" | "trace"
  bool expected_runner_enabled = 3;
  repeated EvalInvocation conversation = 4;
  EvalConversationScenario conversation_scenario = 5;
  EvalSessionInput session_input = 6;
  string created_at = 7;
}

message EvalInvocation {
  string invocation_id = 1;
  EvalMessage user_content = 2;
  EvalMessage final_response = 3;
  repeated EvalTool tools = 4;
  string created_at = 5;
}

message EvalMessage {
  string role = 1;
  string content = 2;
}

message EvalTool {
  string id = 1;
  string name = 2;
  string arguments_json = 3;
  string result_json = 4;
}

message EvalConversationScenario {
  string driver = 1; // "actual" | "expected"
  string starting_prompt = 2;
  string conversation_plan = 3;
  string stop_signal = 4;
  int32 max_allowed_invocations = 5;
}

message EvalSessionInput {
  string app_name = 1;
  string user_id = 2;
  google.protobuf.Struct state = 3;
}

// ──────────────── EvalMetric 评估指标 ────────────────

message EvalMetric {
  string metric_name = 1;
  string evaluator_name = 2;
  double threshold = 3;
  EvalCriterion criterion = 4;
}

message EvalCriterion {
  EvalToolTrajectoryCriterion tool_trajectory = 1;
  EvalFinalResponseCriterion final_response = 2;
  EvalLLMJudgeCriterion llm_judge = 3;
}

message EvalToolTrajectoryCriterion {
  EvalToolTrajectoryStrategy default_strategy = 1;
  google.protobuf.Struct tool_strategy = 2; // map<string, ToolTrajectoryStrategy>
  bool order_sensitive = 3;
  bool subset_matching = 4;
}

message EvalToolTrajectoryStrategy {
  EvalTextCriterion name = 1;
  EvalJSONCriterion arguments = 2;
  EvalJSONCriterion result = 3;
}

message EvalFinalResponseCriterion {
  EvalTextCriterion text = 1;
  EvalJSONCriterion json = 2;
  EvalRougeCriterion rouge = 3;
  EvalXMLCriterion xml = 4;
}

message EvalTextCriterion {
  bool ignore = 1;
  bool case_sensitive = 2;
  bool trim_whitespace = 3;
  bool normalize_whitespace = 4;
}

message EvalJSONCriterion {
  bool ignore = 1;
  repeated string ignore_paths = 2;
  bool array_order_insensitive = 3;
}

message EvalRougeCriterion {
  bool ignore = 1;
  double rouge_1_threshold = 2;
  double rouge_2_threshold = 3;
  double rouge_l_threshold = 4;
}

message EvalXMLCriterion {
  bool ignore = 1;
  bool ignore_order = 2;
  bool ignore_whitespace = 3;
}

message EvalLLMJudgeCriterion {
  repeated EvalRubric rubrics = 1;
  EvalJudgeModel judge_model = 2;
  EvalJudgeTemplate template = 3;
}

message EvalRubric {
  string id = 1;
  string content = 2;
  string description = 3;
  string type = 4;
}

message EvalJudgeModel {
  string provider_name = 1;
  string model_name = 2;
  string variant = 3;
  string base_url = 4;
  string api_key = 5;
  int32 num_samples = 6;
  google.protobuf.Struct generation_config = 7;
}

message EvalJudgeTemplate {
  string prompt = 1;
  string response_scorer_name = 2;
  repeated EvalTemplateVariableBinding variable_bindings = 3;
  string sample_aggregator_name = 4;
  string invocation_aggregator_name = 5;
}

message EvalTemplateVariableBinding {
  string template_variable = 1;
  string scope = 2; // "actual" | "expected"
  string field = 3; // "userContent" | "finalResponse"
}

// ──────────────── EvaluationRun 评估运行 ────────────────

message EvaluationRun {
  string id = 1;
  string eval_set_id = 2;
  string agent_id = 3;
  string status = 4; // "running" | "completed" | "failed"
  int32 num_runs = 5;
  bool parallel_enabled = 6;
  int32 total_cases = 7;
  int32 passed_cases = 8;
  int32 failed_cases = 9;
  int32 not_evaluated_cases = 10;
  double overall_score = 11;
  string execution_time_ms = 12;
  repeated EvalCaseResult case_results = 13;
  string started_at = 14;
  string ended_at = 15;
  string error_message = 16;
  string created_at = 17;
}

message EvalCaseResult {
  string eval_id = 1;
  string final_eval_status = 2; // "passed" | "failed" | "not_evaluated" | "unknown"
  string error_message = 3;
  repeated EvalMetricResult overall_metric_results = 4;
  repeated EvalMetricResultPerInvocation metric_result_per_invocation = 5;
  string session_id = 6;
  string user_id = 7;
  repeated EvalRunDetail run_details = 8;
}

message EvalMetricResult {
  string metric_name = 1;
  double score = 2;
  string eval_status = 3;
  double threshold = 4;
  string reason = 5;
  repeated EvalRubricScore rubric_scores = 6;
}

message EvalMetricResultPerInvocation {
  EvalInvocation actual_invocation = 1;
  EvalInvocation expected_invocation = 2;
  repeated EvalMetricResult metric_results = 3;
}

message EvalRubricScore {
  string id = 1;
  string reason = 2;
  double score = 3;
}

message EvalRunDetail {
  int32 run_id = 1;
  string session_id = 2;
  string user_id = 3;
  string status = 4;
  string error_message = 5;
}

// ──────────────── Request / Response ────────────────

message ListEvalSetsRequest {
  string agent_id = 1;
  int32 limit = 2;
  int32 offset = 3;
}

message ListEvalSetsResponse {
  repeated EvalSet items = 1;
  int32 total = 2;
}

message GetEvalSetRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message CreateEvalSetRequest {
  string name = 1 [(google.api.field_behavior) = REQUIRED];
  string description = 2;
  string agent_id = 3 [(google.api.field_behavior) = REQUIRED];
  repeated EvalCase cases = 4;
}

message UpdateEvalSetRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  EvalSet eval_set = 2;
}

message DeleteEvalSetRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListEvalMetricsRequest {
  string eval_set_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListEvalMetricsResponse {
  repeated EvalMetric items = 1;
}

message CreateEvalMetricRequest {
  string eval_set_id = 1 [(google.api.field_behavior) = REQUIRED];
  EvalMetric metric = 2;
}

message DeleteEvalMetricRequest {
  string eval_set_id = 1 [(google.api.field_behavior) = REQUIRED];
  string metric_name = 2 [(google.api.field_behavior) = REQUIRED];
}

message RunEvaluationRequest {
  string eval_set_id = 1 [(google.api.field_behavior) = REQUIRED];
  string agent_id = 2 [(google.api.field_behavior) = REQUIRED];
  int32 num_runs = 3; // 默认 1
  bool parallel_enabled = 4;
  bool run_details_enabled = 5;
  repeated string eval_case_ids = 6; // 空=全部用例
  int32 eval_case_parallelism = 7;
  bool eval_case_parallel_inference_enabled = 8;
  bool eval_case_parallel_evaluation_enabled = 9;
  string judge_provider = 10; // LLM-as-Judge 用的 Provider
  string judge_model = 11; // LLM-as-Judge 用的 Model
}

message GetEvaluationRunRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListEvaluationRunsRequest {
  string eval_set_id = 1;
  string agent_id = 2;
  string status = 3;
  int32 limit = 4;
  int32 offset = 5;
}

message ListEvaluationRunsResponse {
  repeated EvaluationRun items = 1;
  int32 total = 2;
}

message DeleteEvaluationRunRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

// ──────────────── Service ────────────────

service EvaluationService {
  // EvalSet CRUD
  rpc ListEvalSets(ListEvalSetsRequest) returns (ListEvalSetsResponse) {
    option (google.api.http) = { get: "/v1/evaluations/sets" };
  }
  rpc GetEvalSet(GetEvalSetRequest) returns (EvalSet) {
    option (google.api.http) = { get: "/v1/evaluations/sets/{id}" };
  }
  rpc CreateEvalSet(CreateEvalSetRequest) returns (EvalSet) {
    option (google.api.http) = { post: "/v1/evaluations/sets" body: "*" };
  }
  rpc UpdateEvalSet(UpdateEvalSetRequest) returns (EvalSet) {
    option (google.api.http) = { patch: "/v1/evaluations/sets/{id}" body: "eval_set" };
  }
  rpc DeleteEvalSet(DeleteEvalSetRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/evaluations/sets/{id}" };
  }

  // EvalMetric CRUD
  rpc ListEvalMetrics(ListEvalMetricsRequest) returns (ListEvalMetricsResponse) {
    option (google.api.http) = { get: "/v1/evaluations/sets/{eval_set_id}/metrics" };
  }
  rpc CreateEvalMetric(CreateEvalMetricRequest) returns (EvalMetric) {
    option (google.api.http) = { post: "/v1/evaluations/sets/{eval_set_id}/metrics" body: "*" };
  }
  rpc DeleteEvalMetric(DeleteEvalMetricRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/evaluations/sets/{eval_set_id}/metrics/{metric_name}" };
  }

  // Evaluation Run
  rpc RunEvaluation(RunEvaluationRequest) returns (EvaluationRun) {
    option (google.api.http) = { post: "/v1/evaluations/runs" body: "*" };
  }
  rpc GetEvaluationRun(GetEvaluationRunRequest) returns (EvaluationRun) {
    option (google.api.http) = { get: "/v1/evaluations/runs/{id}" };
  }
  rpc ListEvaluationRuns(ListEvaluationRunsRequest) returns (ListEvaluationRunsResponse) {
    option (google.api.http) = { get: "/v1/evaluations/runs" };
  }
  rpc DeleteEvaluationRun(DeleteEvaluationRunRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/evaluations/runs/{id}" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
// internal/biz/evaluation.go

type EvalSet struct {
    ID          string
    Name        string
    Description string
    AgentID     string
    CasesJSON   string
    CreatedAt   string
    UpdatedAt   string
}

type EvalCaseEntry struct {
    EvalID                string                    `json:"evalId"`
    EvalMode              string                    `json:"evalMode"`
    ExpectedRunnerEnabled bool                      `json:"expectedRunnerEnabled"`
    Conversation          []*EvalInvocationEntry    `json:"conversation"`
    ConversationScenario  *EvalConversationScenario `json:"conversationScenario"`
    SessionInput          *EvalSessionInputEntry    `json:"sessionInput"`
}

type EvalInvocationEntry struct {
    InvocationID    string              `json:"invocationId"`
    UserContent     *EvalMessageEntry   `json:"userContent"`
    FinalResponse   *EvalMessageEntry   `json:"finalResponse"`
    Tools           []*EvalToolEntry    `json:"tools"`
}

type EvalMessageEntry struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type EvalToolEntry struct {
    ID           string `json:"id"`
    Name         string `json:"name"`
    ArgumentsJSON string `json:"arguments"`
    ResultJSON   string `json:"result"`
}

type EvalConversationScenario struct {
    Driver                string `json:"driver"`
    StartingPrompt        string `json:"startingPrompt"`
    ConversationPlan      string `json:"conversationPlan"`
    StopSignal            string `json:"stopSignal"`
    MaxAllowedInvocations *int   `json:"maxAllowedInvocations"`
}

type EvalSessionInputEntry struct {
    AppName string         `json:"appName"`
    UserID  string         `json:"userId"`
    State   map[string]any `json:"state"`
}

type EvalMetricEntry struct {
    MetricName    string              `json:"metricName"`
    EvaluatorName string              `json:"evaluatorName"`
    Threshold     float64             `json:"threshold"`
    Criterion     *EvalCriterionEntry `json:"criterion"`
}

type EvalCriterionEntry struct {
    ToolTrajectory *EvalToolTrajectoryCriterionEntry `json:"toolTrajectory"`
    FinalResponse  *EvalFinalResponseCriterionEntry  `json:"finalResponse"`
    LLMJudge       *EvalLLMJudgeCriterionEntry       `json:"llmJudge"`
}

type EvalToolTrajectoryCriterionEntry struct {
    DefaultStrategy *EvalToolTrajectoryStrategyEntry `json:"defaultStrategy"`
    ToolStrategy    map[string]*EvalToolTrajectoryStrategyEntry `json:"toolStrategy"`
    OrderSensitive  bool `json:"orderSensitive"`
    SubsetMatching  bool `json:"subsetMatching"`
}

type EvalToolTrajectoryStrategyEntry struct {
    Name      *EvalTextCriterionEntry `json:"name"`
    Arguments *EvalJSONCriterionEntry `json:"arguments"`
    Result    *EvalJSONCriterionEntry `json:"result"`
}

type EvalFinalResponseCriterionEntry struct {
    Text  *EvalTextCriterionEntry  `json:"text"`
    JSON  *EvalJSONCriterionEntry  `json:"json"`
    Rouge *EvalRougeCriterionEntry `json:"rouge"`
    XML   *EvalXMLCriterionEntry   `json:"xml"`
}

type EvalTextCriterionEntry struct {
    Ignore             bool `json:"ignore"`
    CaseSensitive      bool `json:"caseSensitive"`
    TrimWhitespace     bool `json:"trimWhitespace"`
    NormalizeWhitespace bool `json:"normalizeWhitespace"`
}

type EvalJSONCriterionEntry struct {
    Ignore                  bool     `json:"ignore"`
    IgnorePaths             []string `json:"ignorePaths"`
    ArrayOrderInsensitive   bool     `json:"arrayOrderInsensitive"`
}

type EvalRougeCriterionEntry struct {
    Ignore          bool    `json:"ignore"`
    Rouge1Threshold float64 `json:"rouge1Threshold"`
    Rouge2Threshold float64 `json:"rouge2Threshold"`
    RougeLThreshold float64 `json:"rougeLThreshold"`
}

type EvalXMLCriterionEntry struct {
    Ignore          bool `json:"ignore"`
    IgnoreOrder     bool `json:"ignoreOrder"`
    IgnoreWhitespace bool `json:"ignoreWhitespace"`
}

type EvalLLMJudgeCriterionEntry struct {
    Rubrics    []*EvalRubricEntry          `json:"rubrics"`
    JudgeModel *EvalJudgeModelEntry        `json:"judgeModel"`
    Template   *EvalJudgeTemplateEntry     `json:"template"`
}

type EvalRubricEntry struct {
    ID          string `json:"id"`
    Content     string `json:"content"`
    Description string `json:"description"`
    Type        string `json:"type"`
}

type EvalJudgeModelEntry struct {
    ProviderName     string         `json:"providerName"`
    ModelName        string         `json:"modelName"`
    Variant          string         `json:"variant"`
    BaseURL          string         `json:"baseURL"`
    APIKey           string         `json:"apiKey"`
    NumSamples       int            `json:"numSamples"`
    GenerationConfig map[string]any `json:"generationConfig"`
}

type EvalJudgeTemplateEntry struct {
    Prompt                   string                          `json:"prompt"`
    ResponseScorerName       string                          `json:"responseScorerName"`
    VariableBindings         []*EvalTemplateVariableBinding  `json:"variableBindings"`
    SampleAggregatorName     string                          `json:"sampleAggregatorName"`
    InvocationAggregatorName string                          `json:"invocationAggregatorName"`
}

type EvalTemplateVariableBinding struct {
    TemplateVariable string `json:"templateVariable"`
    Scope            string `json:"scope"`
    Field            string `json:"field"`
}

type EvaluationRun struct {
    ID               string
    EvalSetID        string
    AgentID          string
    Status           string // "running" | "completed" | "failed"
    NumRuns          int
    ParallelEnabled  bool
    TotalCases       int
    PassedCases      int
    FailedCases      int
    NotEvaluatedCases int
    OverallScore     float64
    ExecutionTimeMs  string
    ResultsJSON      string
    ErrorMessage     string
    StartedAt        string
    EndedAt          string
    CreatedAt        string
}

type EvalCaseResultEntry struct {
    EvalID                   string                          `json:"evalId"`
    FinalEvalStatus          string                          `json:"finalEvalStatus"`
    ErrorMessage             string                          `json:"errorMessage"`
    OverallMetricResults     []*EvalMetricResultEntry        `json:"overallMetricResults"`
    MetricResultPerInvocation []*EvalMetricResultPerInvocationEntry `json:"metricResultPerInvocation"`
    SessionID                string                          `json:"sessionId"`
    UserID                   string                          `json:"userId"`
    RunDetails               []*EvalRunDetailEntry           `json:"runDetails"`
}

type EvalMetricResultEntry struct {
    MetricName  string              `json:"metricName"`
    Score       float64             `json:"score"`
    EvalStatus  string              `json:"evalStatus"`
    Threshold   float64             `json:"threshold"`
    Reason      string              `json:"reason"`
    RubricScores []*EvalRubricScoreEntry `json:"rubricScores"`
}

type EvalMetricResultPerInvocationEntry struct {
    ActualInvocation  *EvalInvocationEntry    `json:"actualInvocation"`
    ExpectedInvocation *EvalInvocationEntry   `json:"expectedInvocation"`
    MetricResults     []*EvalMetricResultEntry `json:"metricResults"`
}

type EvalRubricScoreEntry struct {
    ID     string  `json:"id"`
    Reason string  `json:"reason"`
    Score  float64 `json:"score"`
}

type EvalRunDetailEntry struct {
    RunID        int    `json:"runId"`
    SessionID    string `json:"sessionId"`
    UserID       string `json:"userId"`
    Status       string `json:"status"`
    ErrorMessage string `json:"errorMessage"`
}

type EvalSetQuery struct {
    AgentID string
    Limit   int
    Offset  int
}

type EvalRunQuery struct {
    EvalSetID string
    AgentID   string
    Status    string
    Limit     int
    Offset    int
}

type RunEvaluationParams struct {
    EvalSetID                       string
    AgentID                         string
    NumRuns                         int
    ParallelEnabled                 bool
    RunDetailsEnabled               bool
    EvalCaseIDs                     []string
    EvalCaseParallelism             int
    EvalCaseParallelInferenceEnabled  bool
    EvalCaseParallelEvaluationEnabled bool
    JudgeProvider                   string
    JudgeModel                      string
}
```

### 3.2 Repo 接口

```go
type EvaluationRepo interface {
    // EvalSet
    ListEvalSets(ctx context.Context, q EvalSetQuery) ([]EvalSet, int, error)
    GetEvalSet(ctx context.Context, id string) (EvalSet, error)
    CreateEvalSet(ctx context.Context, es EvalSet) (EvalSet, error)
    UpdateEvalSet(ctx context.Context, es EvalSet) (EvalSet, error)
    DeleteEvalSet(ctx context.Context, id string) error

    // EvalMetric
    ListEvalMetrics(ctx context.Context, evalSetID string) ([]EvalMetricEntry, error)
    CreateEvalMetric(ctx context.Context, evalSetID string, m EvalMetricEntry) (EvalMetricEntry, error)
    DeleteEvalMetric(ctx context.Context, evalSetID, metricName string) error

    // EvaluationRun
    CreateRun(ctx context.Context, run EvaluationRun) (EvaluationRun, error)
    UpdateRun(ctx context.Context, run EvaluationRun) error
    GetRun(ctx context.Context, id string) (EvaluationRun, error)
    ListRuns(ctx context.Context, q EvalRunQuery) ([]EvaluationRun, int, error)
    DeleteRun(ctx context.Context, id string) error
}
```

### 3.3 Usecase

```go
type EvaluationUsecase struct {
    repo    EvaluationRepo
    agents  AgentRepository
}

func NewEvaluationUsecase(repo EvaluationRepo, agents AgentRepository) *EvaluationUsecase

func (uc *EvaluationUsecase) ListEvalSets(ctx context.Context, q EvalSetQuery) ([]EvalSet, int, error)
func (uc *EvaluationUsecase) GetEvalSet(ctx context.Context, id string) (EvalSet, error)
func (uc *EvaluationUsecase) CreateEvalSet(ctx context.Context, es EvalSet) (EvalSet, error)
func (uc *EvaluationUsecase) UpdateEvalSet(ctx context.Context, es EvalSet) (EvalSet, error)
func (uc *EvaluationUsecase) DeleteEvalSet(ctx context.Context, id string) error

func (uc *EvaluationUsecase) ListEvalMetrics(ctx context.Context, evalSetID string) ([]EvalMetricEntry, error)
func (uc *EvaluationUsecase) CreateEvalMetric(ctx context.Context, evalSetID string, m EvalMetricEntry) (EvalMetricEntry, error)
func (uc *EvaluationUsecase) DeleteEvalMetric(ctx context.Context, evalSetID, metricName string) error

func (uc *EvaluationUsecase) RunEvaluation(ctx context.Context, p RunEvaluationParams) (EvaluationRun, error)
func (uc *EvaluationUsecase) GetRun(ctx context.Context, id string) (EvaluationRun, error)
func (uc *EvaluationUsecase) ListRuns(ctx context.Context, q EvalRunQuery) ([]EvaluationRun, int, error)
func (uc *EvaluationUsecase) DeleteRun(ctx context.Context, id string) error
```

### 3.4 RunEvaluation 核心流程

```go
func (uc *EvaluationUsecase) RunEvaluation(ctx context.Context, p RunEvaluationParams) (EvaluationRun, error) {
    // 1. 获取 EvalSet
    evalSet, err := uc.repo.GetEvalSet(ctx, p.EvalSetID)
    if err != nil {
        return EvaluationRun{}, err
    }

    // 2. 创建运行记录
    run := EvaluationRun{
        ID:              newEvalRunID(),
        EvalSetID:       p.EvalSetID,
        AgentID:         p.AgentID,
        Status:          "running",
        NumRuns:         p.NumRuns,
        ParallelEnabled: p.ParallelEnabled,
        StartedAt:       time.Now().Format(time.RFC3339),
        CreatedAt:       time.Now().Format(time.RFC3339),
    }
    run, err = uc.repo.CreateRun(ctx, run)
    if err != nil {
        return EvaluationRun{}, err
    }

    // 3. 异步执行评估（通过 internal/evaluation 包桥接 trpc-agent-go）
    go uc.executeEvaluation(context.Background(), run, evalSet, p)

    return run, nil
}

func (uc *EvaluationUsecase) executeEvaluation(ctx context.Context, run EvaluationRun, evalSet EvalSet, p RunEvaluationParams) {
    // 通过 internal/evaluation/trpc/evaluator.go 桥接 trpc-agent-go 评估框架
    // 1. 构建 evalset.Manager 从数据库加载 EvalSet
    // 2. 构建 metric.Manager 从数据库加载 Metrics
    // 3. 构建 AgentEvaluator 并执行 Evaluate
    // 4. 聚合结果写入 EvaluationRun
    // 5. 更新 run 状态为 completed/failed
}
```

---

## 四、Data 层

### 4.1 Ent Schema

**`internal/data/ent/schema/eval_set.go`**

```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/dialect/entsql"
    "entgo.io/ent/schema"
    "entgo.io/ent/schema/field"
)

type EvalSet struct {
    ent.Schema
}

func (EvalSet) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "eval_set"},
    }
}

func (EvalSet) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("name").MaxLen(1024),
        field.Text("description").Default(""),
        field.String("agent_id").MaxLen(256),
        field.Text("cases_json").Default("[]"),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}
```

**`internal/data/ent/schema/eval_metric.go`**

```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/dialect/entsql"
    "entgo.io/ent/schema"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type EvalMetric struct {
    ent.Schema
}

func (EvalMetric) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "eval_metric"},
    }
}

func (EvalMetric) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("eval_set_id").MaxLen(256),
        field.String("metric_name").MaxLen(512),
        field.String("evaluator_name").MaxLen(512).Default(""),
        field.Float("threshold").Default(0.5),
        field.Text("criterion_json").Default("{}"),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}

func (EvalMetric) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("eval_set_id", "metric_name").Unique(),
    }
}
```

**`internal/data/ent/schema/evaluation_run.go`**

```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/dialect/entsql"
    "entgo.io/ent/schema"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type EvaluationRun struct {
    ent.Schema
}

func (EvaluationRun) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "evaluation_run"},
    }
}

func (EvaluationRun) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("eval_set_id").MaxLen(256),
        field.String("agent_id").MaxLen(256),
        field.String("status").Default("running").MaxLen(64),
        field.Int("num_runs").Default(1),
        field.Bool("parallel_enabled").Default(false),
        field.Int("total_cases").Default(0),
        field.Int("passed_cases").Default(0),
        field.Int("failed_cases").Default(0),
        field.Int("not_evaluated_cases").Default(0),
        field.Float("overall_score").Default(0),
        field.String("execution_time_ms").Default("0"),
        field.Text("results_json").Default("[]"),
        field.Text("error_message").Default(""),
        field.String("started_at").Default(""),
        field.String("ended_at").Default(""),
        field.String("created_at").Default(""),
    }
}

func (EvaluationRun) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("eval_set_id"),
        index.Fields("agent_id"),
        index.Fields("status"),
    }
}
```

### 4.2 Repo 实现

**`internal/data/evaluation.go`**

```go
package data

type evaluationRepo struct {
    data *Data
}

func NewEvaluationRepo(d *Data) biz.EvaluationRepo {
    return &evaluationRepo{data: d}
}

func entToBizEvalSet(e *ent.EvalSet) biz.EvalSet {
    return biz.EvalSet{
        ID:          e.ID,
        Name:        e.Name,
        Description: e.Description,
        AgentID:     e.AgentID,
        CasesJSON:   e.CasesJSON,
        CreatedAt:   e.CreatedAt,
        UpdatedAt:   e.UpdatedAt,
    }
}

func entToBizEvaluationRun(e *ent.EvaluationRun) biz.EvaluationRun {
    return biz.EvaluationRun{
        ID:                e.ID,
        EvalSetID:         e.EvalSetID,
        AgentID:           e.AgentID,
        Status:            e.Status,
        NumRuns:           e.NumRuns,
        ParallelEnabled:   e.ParallelEnabled,
        TotalCases:        e.TotalCases,
        PassedCases:       e.PassedCases,
        FailedCases:       e.FailedCases,
        NotEvaluatedCases: e.NotEvaluatedCases,
        OverallScore:      e.OverallScore,
        ExecutionTimeMs:   e.ExecutionTimeMs,
        ResultsJSON:       e.ResultsJSON,
        ErrorMessage:      e.ErrorMessage,
        StartedAt:         e.StartedAt,
        EndedAt:           e.EndedAt,
        CreatedAt:         e.CreatedAt,
    }
}

func (r *evaluationRepo) ListEvalSets(ctx context.Context, q biz.EvalSetQuery) ([]biz.EvalSet, int, error) {
    query := r.data.db.EvalSet.Query()
    if q.AgentID != "" {
        query = query.Where(evalset.AgentID(q.AgentID))
    }
    total, err := query.Count(ctx)
    if err != nil {
        return nil, 0, err
    }
    limit := q.Limit
    if limit <= 0 {
        limit = 50
    }
    items, err := query.Limit(limit).Offset(q.Offset).Order(ent.Asc(evalset.FieldCreatedAt)).All(ctx)
    if err != nil {
        return nil, 0, err
    }
    result := make([]biz.EvalSet, len(items))
    for i, e := range items {
        result[i] = entToBizEvalSet(e)
    }
    return result, total, nil
}

func (r *evaluationRepo) GetEvalSet(ctx context.Context, id string) (biz.EvalSet, error) {
    e, err := r.data.db.EvalSet.Get(ctx, id)
    if err != nil {
        return biz.EvalSet{}, err
    }
    return entToBizEvalSet(e), nil
}

func (r *evaluationRepo) CreateEvalSet(ctx context.Context, es biz.EvalSet) (biz.EvalSet, error) {
    e, err := r.data.db.EvalSet.Create().
        SetID(es.ID).
        SetName(es.Name).
        SetDescription(es.Description).
        SetAgentID(es.AgentID).
        SetCasesJSON(es.CasesJSON).
        SetCreatedAt(es.CreatedAt).
        SetUpdatedAt(es.UpdatedAt).
        Save(ctx)
    if err != nil {
        return biz.EvalSet{}, err
    }
    return entToBizEvalSet(e), nil
}

func (r *evaluationRepo) UpdateEvalSet(ctx context.Context, es biz.EvalSet) (biz.EvalSet, error) {
    e, err := r.data.db.EvalSet.UpdateOneID(es.ID).
        SetName(es.Name).
        SetDescription(es.Description).
        SetAgentID(es.AgentID).
        SetCasesJSON(es.CasesJSON).
        SetUpdatedAt(es.UpdatedAt).
        Save(ctx)
    if err != nil {
        return biz.EvalSet{}, err
    }
    return entToBizEvalSet(e), nil
}

func (r *evaluationRepo) DeleteEvalSet(ctx context.Context, id string) error {
    return r.data.db.EvalSet.DeleteOneID(id).Exec(ctx)
}

func (r *evaluationRepo) ListEvalMetrics(ctx context.Context, evalSetID string) ([]biz.EvalMetricEntry, error) {
    items, err := r.data.db.EvalMetric.Query().
        Where(evalmetric.EvalSetID(evalSetID)).
        All(ctx)
    if err != nil {
        return nil, err
    }
    result := make([]biz.EvalMetricEntry, len(items))
    for i, e := range items {
        var m biz.EvalMetricEntry
        json.Unmarshal([]byte(e.CriterionJSON), &m)
        m.MetricName = e.MetricName
        m.EvaluatorName = e.EvaluatorName
        m.Threshold = e.Threshold
        result[i] = m
    }
    return result, nil
}

func (r *evaluationRepo) CreateEvalMetric(ctx context.Context, evalSetID string, m biz.EvalMetricEntry) (biz.EvalMetricEntry, error) {
    criterionJSON, _ := json.Marshal(m)
    id := newEvalMetricID()
    _, err := r.data.db.EvalMetric.Create().
        SetID(id).
        SetEvalSetID(evalSetID).
        SetMetricName(m.MetricName).
        SetEvaluatorName(m.EvaluatorName).
        SetThreshold(m.Threshold).
        SetCriterionJSON(string(criterionJSON)).
        SetCreatedAt(time.Now().Format(time.RFC3339)).
        SetUpdatedAt(time.Now().Format(time.RFC3339)).
        Save(ctx)
    if err != nil {
        return biz.EvalMetricEntry{}, err
    }
    return m, nil
}

func (r *evaluationRepo) DeleteEvalMetric(ctx context.Context, evalSetID, metricName string) error {
    _, err := r.data.db.EvalMetric.Delete().
        Where(evalmetric.EvalSetID(evalSetID), evalmetric.MetricName(metricName)).
        Exec(ctx)
    return err
}

func (r *evaluationRepo) CreateRun(ctx context.Context, run biz.EvaluationRun) (biz.EvaluationRun, error) {
    e, err := r.data.db.EvaluationRun.Create().
        SetID(run.ID).
        SetEvalSetID(run.EvalSetID).
        SetAgentID(run.AgentID).
        SetStatus(run.Status).
        SetNumRuns(run.NumRuns).
        SetParallelEnabled(run.ParallelEnabled).
        SetStartedAt(run.StartedAt).
        SetCreatedAt(run.CreatedAt).
        Save(ctx)
    if err != nil {
        return biz.EvaluationRun{}, err
    }
    return entToBizEvaluationRun(e), nil
}

func (r *evaluationRepo) UpdateRun(ctx context.Context, run biz.EvaluationRun) error {
    return r.data.db.EvaluationRun.UpdateOneID(run.ID).
        SetStatus(run.Status).
        SetTotalCases(run.TotalCases).
        SetPassedCases(run.PassedCases).
        SetFailedCases(run.FailedCases).
        SetNotEvaluatedCases(run.NotEvaluatedCases).
        SetOverallScore(run.OverallScore).
        SetExecutionTimeMs(run.ExecutionTimeMs).
        SetResultsJSON(run.ResultsJSON).
        SetErrorMessage(run.ErrorMessage).
        SetEndedAt(run.EndedAt).
        Exec(ctx)
}

func (r *evaluationRepo) GetRun(ctx context.Context, id string) (biz.EvaluationRun, error) {
    e, err := r.data.db.EvaluationRun.Get(ctx, id)
    if err != nil {
        return biz.EvaluationRun{}, err
    }
    return entToBizEvaluationRun(e), nil
}

func (r *evaluationRepo) ListRuns(ctx context.Context, q biz.EvalRunQuery) ([]biz.EvaluationRun, int, error) {
    query := r.data.db.EvaluationRun.Query()
    if q.EvalSetID != "" {
        query = query.Where(evaluationrun.EvalSetID(q.EvalSetID))
    }
    if q.AgentID != "" {
        query = query.Where(evaluationrun.AgentID(q.AgentID))
    }
    if q.Status != "" {
        query = query.Where(evaluationrun.Status(q.Status))
    }
    total, err := query.Count(ctx)
    if err != nil {
        return nil, 0, err
    }
    limit := q.Limit
    if limit <= 0 {
        limit = 50
    }
    items, err := query.Limit(limit).Offset(q.Offset).Order(ent.Desc(evaluationrun.FieldCreatedAt)).All(ctx)
    if err != nil {
        return nil, 0, err
    }
    result := make([]biz.EvaluationRun, len(items))
    for i, e := range items {
        result[i] = entToBizEvaluationRun(e)
    }
    return result, total, nil
}

func (r *evaluationRepo) DeleteRun(ctx context.Context, id string) error {
    return r.data.db.EvaluationRun.DeleteOneID(id).Exec(ctx)
}
```

---

## 五、框架桥接层

### 5.1 AgentEvaluator 适配器

**`internal/evaluation/trpc/evaluator.go`**

将 trpc-agent-go 的 `evaluation.AgentEvaluator` 适配为项目可用组件，负责：
- 从数据库加载 EvalSet → 转换为 trpc `evalset.EvalSet`
- 从数据库加载 Metrics → 转换为 trpc `metric.EvalMetric`
- 构建 `evaluation.New` 所需的 Runner 和选项
- 执行评估并转换结果回 Biz 模型

```go
package trpc

type EvaluatorAdapter struct {
    appName string
    repo    biz.EvaluationRepo
    agents  biz.AgentRepository
}

func NewEvaluatorAdapter(appName string, repo biz.EvaluationRepo, agents biz.AgentRepository) *EvaluatorAdapter

func (a *EvaluatorAdapter) Evaluate(ctx context.Context, run *biz.EvaluationRun, evalSet biz.EvalSet, params biz.RunEvaluationParams) error {
    // 1. 构建 DB-backed evalset.Manager
    evalSetMgr := NewDBEvalSetManager(a.repo, evalSet)

    // 2. 构建 DB-backed metric.Manager
    metricMgr := NewDBMetricManager(a.repo, evalSet.ID)

    // 3. 构建 evalresult.Manager（内存）
    resultMgr := evalresultinmemory.New()

    // 4. 构建 Agent Runner
    agent, err := a.agents.GetAgent(ctx, params.AgentID)
    r, err := agent.BuildRunner(ctx, ...)

    // 5. 构建 Judge Runner（如果配置了 LLM-as-Judge）
    var judgeRunner runner.Runner
    if params.JudgeProvider != "" && params.JudgeModel != "" {
        judgeRunner, err = buildJudgeRunner(ctx, params.JudgeProvider, params.JudgeModel)
    }

    // 6. 构建 AgentEvaluator
    opts := []evaluation.Option{
        evaluation.WithEvalSetManager(evalSetMgr),
        evaluation.WithEvalResultManager(resultMgr),
        evaluation.WithMetricManager(metricMgr),
        evaluation.WithNumRuns(params.NumRuns),
        evaluation.WithRunDetailsEnabled(params.RunDetailsEnabled),
    }
    if judgeRunner != nil {
        opts = append(opts, evaluation.WithJudgeRunner(judgeRunner))
    }
    if params.ParallelEnabled {
        opts = append(opts, evaluation.WithNumRunsParallelEnabled(true))
    }
    if len(params.EvalCaseIDs) > 0 {
        opts = append(opts, evaluation.WithEvalCaseIDs(params.EvalCaseIDs...))
    }
    if params.EvalCaseParallelism > 0 {
        opts = append(opts, evaluation.WithEvalCaseParallelism(params.EvalCaseParallelism))
    }

    evaluator, err := evaluation.New(a.appName, r, opts...)
    if err != nil {
        return err
    }
    defer evaluator.Close()

    // 7. 执行评估
    result, err := evaluator.Evaluate(ctx, evalSet.ID)
    if err != nil {
        return err
    }

    // 8. 转换结果并更新运行记录
    a.updateRunFromResult(run, result)
    return nil
}
```

### 5.2 DB-backed EvalSet Manager

**`internal/evaluation/trpc/evalset_manager.go`**

```go
package trpc

type DBEvalSetManager struct {
    repo    biz.EvaluationRepo
    evalSet biz.EvalSet
    cached  *evalset.EvalSet
}

func NewDBEvalSetManager(repo biz.EvaluationRepo, evalSet biz.EvalSet) *DBEvalSetManager

func (m *DBEvalSetManager) Get(ctx context.Context, appName, evalSetID string) (*evalset.EvalSet, error) {
    // 从 biz.EvalSet 的 CasesJSON 解析为 trpc evalset.EvalSet
    if m.cached != nil && m.cached.EvalSetID == evalSetID {
        return m.cached, nil
    }
    var cases []*evalset.EvalCase
    json.Unmarshal([]byte(m.evalSet.CasesJSON), &cases)
    m.cached = &evalset.EvalSet{
        EvalSetID: m.evalSet.ID,
        Name:      m.evalSet.Name,
        EvalCases: cases,
    }
    return m.cached, nil
}

func (m *DBEvalSetManager) List(ctx context.Context, appName string) ([]string, error)
func (m *DBEvalSetManager) Create(ctx context.Context, appName, evalSetID string) (*evalset.EvalSet, error)
func (m *DBEvalSetManager) Delete(ctx context.Context, appName, evalSetID string) error
func (m *DBEvalSetManager) GetCase(ctx context.Context, appName, evalSetID, evalCaseID string) (*evalset.EvalCase, error)
func (m *DBEvalSetManager) AddCase(ctx context.Context, appName, evalSetID string, evalCase *evalset.EvalCase) error
func (m *DBEvalSetManager) UpdateCase(ctx context.Context, appName, evalSetID string, evalCase *evalset.EvalCase) error
func (m *DBEvalSetManager) DeleteCase(ctx context.Context, appName, evalSetID, evalCaseID string) error
func (m *DBEvalSetManager) Close() error
```

### 5.3 DB-backed Metric Manager

**`internal/evaluation/trpc/metric_manager.go`**

```go
package trpc

type DBMetricManager struct {
    repo      biz.EvaluationRepo
    evalSetID string
    cached    map[string]*metric.EvalMetric
}

func NewDBMetricManager(repo biz.EvaluationRepo, evalSetID string) *DBMetricManager

func (m *DBMetricManager) List(ctx context.Context, appName, evalSetID string) ([]string, error)
func (m *DBMetricManager) Get(ctx context.Context, appName, evalSetID, metricName string) (*metric.EvalMetric, error)
func (m *DBMetricManager) Add(ctx context.Context, appName, evalSetID string, metric *metric.EvalMetric) error
func (m *DBMetricManager) Delete(ctx context.Context, appName, evalSetID, metricName string) error
func (m *DBMetricManager) Update(ctx context.Context, appName, evalSetID string, metric *metric.EvalMetric) error
func (m *DBMetricManager) Close() error
```

### 5.4 UserSimulation 适配器

**`internal/evaluation/trpc/user_simulator.go`**

```go
package trpc

func NewUserSimulator(simRunner runner.Runner) (usersimulation.Simulator, error) {
    return usersimulation.New(simRunner,
        usersimulation.WithUserIDSupplier(func(ctx context.Context) string {
            return "eval-sim-user-" + uuid.NewString()[:8]
        }),
        usersimulation.WithSessionIDSupplier(func(ctx context.Context) string {
            return "eval-sim-session-" + uuid.NewString()[:8]
        }),
        usersimulation.WithSystemPromptBuilder(usersimulation.DefaultSystemPromptBuilder),
    )
}
```

---

## 六、Service 层

**`internal/service/evaluation.go`**

```go
type EvaluationService struct {
    v1.UnimplementedEvaluationServiceServer
    uc *biz.EvaluationUsecase
}

func NewEvaluationService(uc *biz.EvaluationUsecase) *EvaluationService {
    return &EvaluationService{uc: uc}
}

func (s *EvaluationService) ListEvalSets(ctx context.Context, req *v1.ListEvalSetsRequest) (*v1.ListEvalSetsResponse, error) {
    q := biz.EvalSetQuery{
        AgentID: req.GetAgentId(),
        Limit:   int(req.GetLimit()),
        Offset:  int(req.GetOffset()),
    }
    items, total, err := s.uc.ListEvalSets(ctx, q)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.ListEvalSetsResponse{
        Items: mapSlice(items, toProtoEvalSet),
        Total: int32(total),
    }, nil
}

func (s *EvaluationService) GetEvalSet(ctx context.Context, req *v1.GetEvalSetRequest) (*v1.EvalSet, error) {
    es, err := s.uc.GetEvalSet(ctx, req.GetId())
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoEvalSet(&es), nil
}

func (s *EvaluationService) CreateEvalSet(ctx context.Context, req *v1.CreateEvalSetRequest) (*v1.EvalSet, error) {
    es := fromProtoCreateEvalSetRequest(req)
    result, err := s.uc.CreateEvalSet(ctx, es)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoEvalSet(&result), nil
}

func (s *EvaluationService) UpdateEvalSet(ctx context.Context, req *v1.UpdateEvalSetRequest) (*v1.EvalSet, error) {
    es := fromProtoUpdateEvalSetRequest(req)
    result, err := s.uc.UpdateEvalSet(ctx, es)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoEvalSet(&result), nil
}

func (s *EvaluationService) DeleteEvalSet(ctx context.Context, req *v1.DeleteEvalSetRequest) (*emptypb.Empty, error) {
    err := s.uc.DeleteEvalSet(ctx, req.GetId())
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &emptypb.Empty{}, nil
}

func (s *EvaluationService) ListEvalMetrics(ctx context.Context, req *v1.ListEvalMetricsRequest) (*v1.ListEvalMetricsResponse, error) {
    items, err := s.uc.ListEvalMetrics(ctx, req.GetEvalSetId())
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.ListEvalMetricsResponse{
        Items: mapSlice(items, toProtoEvalMetric),
    }, nil
}

func (s *EvaluationService) CreateEvalMetric(ctx context.Context, req *v1.CreateEvalMetricRequest) (*v1.EvalMetric, error) {
    m := fromProtoEvalMetric(req.GetMetric())
    result, err := s.uc.CreateEvalMetric(ctx, req.GetEvalSetId(), m)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoEvalMetric(&result), nil
}

func (s *EvaluationService) DeleteEvalMetric(ctx context.Context, req *v1.DeleteEvalMetricRequest) (*emptypb.Empty, error) {
    err := s.uc.DeleteEvalMetric(ctx, req.GetEvalSetId(), req.GetMetricName())
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &emptypb.Empty{}, nil
}

func (s *EvaluationService) RunEvaluation(ctx context.Context, req *v1.RunEvaluationRequest) (*v1.EvaluationRun, error) {
    p := biz.RunEvaluationParams{
        EvalSetID:                       req.GetEvalSetId(),
        AgentID:                         req.GetAgentId(),
        NumRuns:                         int(req.GetNumRuns()),
        ParallelEnabled:                 req.GetParallelEnabled(),
        RunDetailsEnabled:               req.GetRunDetailsEnabled(),
        EvalCaseIDs:                     req.GetEvalCaseIds(),
        EvalCaseParallelism:             int(req.GetEvalCaseParallelism()),
        EvalCaseParallelInferenceEnabled:  req.GetEvalCaseParallelInferenceEnabled(),
        EvalCaseParallelEvaluationEnabled: req.GetEvalCaseParallelEvaluationEnabled(),
        JudgeProvider:                   req.GetJudgeProvider(),
        JudgeModel:                      req.GetJudgeModel(),
    }
    if p.NumRuns <= 0 {
        p.NumRuns = 1
    }
    run, err := s.uc.RunEvaluation(ctx, p)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoEvaluationRun(&run), nil
}

func (s *EvaluationService) GetEvaluationRun(ctx context.Context, req *v1.GetEvaluationRunRequest) (*v1.EvaluationRun, error) {
    run, err := s.uc.GetRun(ctx, req.GetId())
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoEvaluationRun(&run), nil
}

func (s *EvaluationService) ListEvaluationRuns(ctx context.Context, req *v1.ListEvaluationRunsRequest) (*v1.ListEvaluationRunsResponse, error) {
    q := biz.EvalRunQuery{
        EvalSetID: req.GetEvalSetId(),
        AgentID:   req.GetAgentId(),
        Status:    req.GetStatus(),
        Limit:     int(req.GetLimit()),
        Offset:    int(req.GetOffset()),
    }
    items, total, err := s.uc.ListRuns(ctx, q)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.ListEvaluationRunsResponse{
        Items: mapSlice(items, toProtoEvaluationRun),
        Total: int32(total),
    }, nil
}

func (s *EvaluationService) DeleteEvaluationRun(ctx context.Context, req *v1.DeleteEvaluationRunRequest) (*emptypb.Empty, error) {
    err := s.uc.DeleteRun(ctx, req.GetId())
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &emptypb.Empty{}, nil
}
```

### 6.1 类型转换函数

```go
func toProtoEvalSet(es *biz.EvalSet) *v1.EvalSet
func fromProtoCreateEvalSetRequest(req *v1.CreateEvalSetRequest) biz.EvalSet
func fromProtoUpdateEvalSetRequest(req *v1.UpdateEvalSetRequest) biz.EvalSet
func toProtoEvalMetric(m *biz.EvalMetricEntry) *v1.EvalMetric
func fromProtoEvalMetric(m *v1.EvalMetric) biz.EvalMetricEntry
func toProtoEvaluationRun(run *biz.EvaluationRun) *v1.EvaluationRun
func toProtoEvalCaseResult(r *biz.EvalCaseResultEntry) *v1.EvalCaseResult
func toProtoEvalMetricResult(r *biz.EvalMetricResultEntry) *v1.EvalMetricResult
```

---

## 七、Wire 注入

### 7.1 新增 Provider

```go
// internal/data/data.go
var ProviderSet = wire.NewSet(
    // ... 已有
    NewEvaluationRepo,
)

// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    // ... 已有
    NewEvaluationUsecase,
)

// internal/service/service.go
var ProviderSet = wire.NewSet(
    // ... 已有
    NewEvaluationService,
)
```

### 7.2 Server 注册

**`internal/server/register_evaluation.go`**

```go
func RegisterEvaluationService(srv *kratos.Server, svc *service.EvaluationService) {
    v1.RegisterEvaluationServiceServer(srv, svc)
}
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/evaluation/
├── api.ts
├── types.ts
└── components/
    ├── EvalSetListPage.vue
    ├── EvalSetEditorDialog.vue
    ├── EvalCaseEditor.vue
    ├── EvalMetricEditorDialog.vue
    ├── EvalRunListPage.vue
    ├── EvalRunDetailPage.vue
    ├── EvalCaseResultCard.vue
    ├── EvalMetricResultCard.vue
    ├── EvalScoreChart.vue
    └── EvalRunStatusBadge.vue
```

### 8.2 类型定义

**`web/src/features/evaluation/types.ts`**

```typescript
export type EvalStatus = 'passed' | 'failed' | 'not_evaluated' | 'unknown';
export type EvalRunStatus = 'running' | 'completed' | 'failed';
export type EvalMode = '' | 'trace';
export type ConversationScenarioDriver = 'actual' | 'expected';

export type EvalSet = {
  id: string;
  name: string;
  description: string;
  agent_id: string;
  cases: EvalCase[];
  created_at: string;
  updated_at: string;
};

export type EvalCase = {
  eval_id: string;
  eval_mode: EvalMode;
  expected_runner_enabled: boolean;
  conversation: EvalInvocation[];
  conversation_scenario?: EvalConversationScenario;
  session_input?: EvalSessionInput;
  created_at: string;
};

export type EvalInvocation = {
  invocation_id: string;
  user_content?: EvalMessage;
  final_response?: EvalMessage;
  tools: EvalTool[];
  created_at: string;
};

export type EvalMessage = {
  role: string;
  content: string;
};

export type EvalTool = {
  id: string;
  name: string;
  arguments_json: string;
  result_json: string;
};

export type EvalConversationScenario = {
  driver: ConversationScenarioDriver;
  starting_prompt: string;
  conversation_plan: string;
  stop_signal: string;
  max_allowed_invocations: number;
};

export type EvalSessionInput = {
  app_name: string;
  user_id: string;
  state: Record<string, unknown>;
};

export type EvalMetric = {
  metric_name: string;
  evaluator_name: string;
  threshold: number;
  criterion?: EvalCriterion;
};

export type EvalCriterion = {
  tool_trajectory?: EvalToolTrajectoryCriterion;
  final_response?: EvalFinalResponseCriterion;
  llm_judge?: EvalLLMJudgeCriterion;
};

export type EvalToolTrajectoryCriterion = {
  default_strategy?: EvalToolTrajectoryStrategy;
  tool_strategy?: Record<string, EvalToolTrajectoryStrategy>;
  order_sensitive: boolean;
  subset_matching: boolean;
};

export type EvalToolTrajectoryStrategy = {
  name?: EvalTextCriterion;
  arguments?: EvalJSONCriterion;
  result?: EvalJSONCriterion;
};

export type EvalFinalResponseCriterion = {
  text?: EvalTextCriterion;
  json?: EvalJSONCriterion;
  rouge?: EvalRougeCriterion;
  xml?: EvalXMLCriterion;
};

export type EvalTextCriterion = {
  ignore: boolean;
  case_sensitive: boolean;
  trim_whitespace: boolean;
  normalize_whitespace: boolean;
};

export type EvalJSONCriterion = {
  ignore: boolean;
  ignore_paths: string[];
  array_order_insensitive: boolean;
};

export type EvalRougeCriterion = {
  ignore: boolean;
  rouge_1_threshold: number;
  rouge_2_threshold: number;
  rouge_l_threshold: number;
};

export type EvalXMLCriterion = {
  ignore: boolean;
  ignore_order: boolean;
  ignore_whitespace: boolean;
};

export type EvalLLMJudgeCriterion = {
  rubrics: EvalRubric[];
  judge_model?: EvalJudgeModel;
  template?: EvalJudgeTemplate;
};

export type EvalRubric = {
  id: string;
  content: string;
  description: string;
  type: string;
};

export type EvalJudgeModel = {
  provider_name: string;
  model_name: string;
  variant: string;
  base_url: string;
  num_samples: number;
  generation_config: Record<string, unknown>;
};

export type EvalJudgeTemplate = {
  prompt: string;
  response_scorer_name: string;
  variable_bindings: EvalTemplateVariableBinding[];
  sample_aggregator_name: string;
  invocation_aggregator_name: string;
};

export type EvalTemplateVariableBinding = {
  template_variable: string;
  scope: string;
  field: string;
};

export type EvaluationRun = {
  id: string;
  eval_set_id: string;
  agent_id: string;
  status: EvalRunStatus;
  num_runs: number;
  parallel_enabled: boolean;
  total_cases: number;
  passed_cases: number;
  failed_cases: number;
  not_evaluated_cases: number;
  overall_score: number;
  execution_time_ms: string;
  case_results: EvalCaseResult[];
  started_at: string;
  ended_at: string;
  error_message: string;
  created_at: string;
};

export type EvalCaseResult = {
  eval_id: string;
  final_eval_status: EvalStatus;
  error_message: string;
  overall_metric_results: EvalMetricResult[];
  metric_result_per_invocation: EvalMetricResultPerInvocation[];
  session_id: string;
  user_id: string;
  run_details: EvalRunDetail[];
};

export type EvalMetricResult = {
  metric_name: string;
  score: number;
  eval_status: EvalStatus;
  threshold: number;
  reason: string;
  rubric_scores: EvalRubricScore[];
};

export type EvalMetricResultPerInvocation = {
  actual_invocation?: EvalInvocation;
  expected_invocation?: EvalInvocation;
  metric_results: EvalMetricResult[];
};

export type EvalRubricScore = {
  id: string;
  reason: string;
  score: number;
};

export type EvalRunDetail = {
  run_id: number;
  session_id: string;
  user_id: string;
  status: string;
  error_message: string;
};

export type EvalSetQuery = {
  agent_id?: string;
  limit?: number;
  offset?: number;
};

export type EvalRunQuery = {
  eval_set_id?: string;
  agent_id?: string;
  status?: string;
  limit?: number;
  offset?: number;
};

export type RunEvaluationParams = {
  eval_set_id: string;
  agent_id: string;
  num_runs?: number;
  parallel_enabled?: boolean;
  run_details_enabled?: boolean;
  eval_case_ids?: string[];
  eval_case_parallelism?: number;
  eval_case_parallel_inference_enabled?: boolean;
  eval_case_parallel_evaluation_enabled?: boolean;
  judge_provider?: string;
  judge_model?: string;
};
```

### 8.3 API 层

**`web/src/features/evaluation/api.ts`**

```typescript
import { createEvaluationService } from "../../services";
import type {
  EvalSet,
  EvalSetQuery,
  EvalMetric,
  EvaluationRun,
  EvalRunQuery,
  RunEvaluationParams,
} from "./types";

const evaluation = createEvaluationService();

export async function listEvalSets(query: EvalSetQuery = {}): Promise<{ items: EvalSet[]; total: number }> {
  const res = await evaluation.ListEvalSets({
    agentId: query.agent_id,
    limit: query.limit,
    offset: query.offset,
  });
  return {
    items: (res.items ?? []).map(wireEvalSet),
    total: res.total,
  };
}

export async function getEvalSet(id: string): Promise<EvalSet> {
  const res = await evaluation.GetEvalSet({ id });
  return wireEvalSet(res);
}

export async function createEvalSet(req: {
  name: string;
  description?: string;
  agent_id: string;
  cases?: EvalCase[];
}): Promise<EvalSet> {
  const res = await evaluation.CreateEvalSet({
    name: req.name,
    description: req.description,
    agentId: req.agent_id,
    cases: req.cases,
  });
  return wireEvalSet(res);
}

export async function updateEvalSet(id: string, evalSet: Partial<EvalSet>): Promise<EvalSet> {
  const res = await evaluation.UpdateEvalSet({ id, evalSet });
  return wireEvalSet(res);
}

export async function deleteEvalSet(id: string): Promise<void> {
  await evaluation.DeleteEvalSet({ id });
}

export async function listEvalMetrics(evalSetId: string): Promise<EvalMetric[]> {
  const res = await evaluation.ListEvalMetrics({ evalSetId });
  return (res.items ?? []).map(wireEvalMetric);
}

export async function createEvalMetric(evalSetId: string, metric: EvalMetric): Promise<EvalMetric> {
  const res = await evaluation.CreateEvalMetric({ evalSetId, metric });
  return wireEvalMetric(res);
}

export async function deleteEvalMetric(evalSetId: string, metricName: string): Promise<void> {
  await evaluation.DeleteEvalMetric({ evalSetId, metricName });
}

export async function runEvaluation(params: RunEvaluationParams): Promise<EvaluationRun> {
  const res = await evaluation.RunEvaluation({
    evalSetId: params.eval_set_id,
    agentId: params.agent_id,
    numRuns: params.num_runs,
    parallelEnabled: params.parallel_enabled,
    runDetailsEnabled: params.run_details_enabled,
    evalCaseIds: params.eval_case_ids,
    evalCaseParallelism: params.eval_case_parallelism,
    evalCaseParallelInferenceEnabled: params.eval_case_parallel_inference_enabled,
    evalCaseParallelEvaluationEnabled: params.eval_case_parallel_evaluation_enabled,
    judgeProvider: params.judge_provider,
    judgeModel: params.judge_model,
  });
  return wireEvaluationRun(res);
}

export async function getEvaluationRun(id: string): Promise<EvaluationRun> {
  const res = await evaluation.GetEvaluationRun({ id });
  return wireEvaluationRun(res);
}

export async function listEvaluationRuns(query: EvalRunQuery = {}): Promise<{ items: EvaluationRun[]; total: number }> {
  const res = await evaluation.ListEvaluationRuns({
    evalSetId: query.eval_set_id,
    agentId: query.agent_id,
    status: query.status,
    limit: query.limit,
    offset: query.offset,
  });
  return {
    items: (res.items ?? []).map(wireEvaluationRun),
    total: res.total,
  };
}

export async function deleteEvaluationRun(id: string): Promise<void> {
  await evaluation.DeleteEvaluationRun({ id });
}

function wireEvalSet(t: any): EvalSet { /* proto → types 映射 */ }
function wireEvalMetric(t: any): EvalMetric { /* proto → types 映射 */ }
function wireEvaluationRun(t: any): EvaluationRun { /* proto → types 映射 */ }
```

### 8.4 组件设计

#### EvalSetListPage.vue

评估集列表页，展示所有评估集，支持筛选、创建、编辑、删除。

| 区域 | 组件 | 功能 |
|------|------|------|
| 顶部 | `QBtn` | 创建评估集 |
| 筛选 | `QSelect` | 按 Agent 筛选 |
| 列表 | `QTable` | 展示评估集（名称、Agent、用例数、创建时间、操作） |
| 行操作 | `QBtn` | 编辑、删除、运行评估 |

#### EvalSetEditorDialog.vue

评估集编辑对话框，支持编辑评估用例和指标。

| 区域 | 组件 | 功能 |
|------|------|------|
| 基本信息 | `QInput` | 名称、描述 |
| Agent 选择 | `QSelect` | 选择关联 Agent |
| 用例列表 | `EvalCaseEditor` | 编辑评估用例（对话、场景、预期输出） |
| 指标管理 | `EvalMetricEditorDialog` | 管理评估指标 |

#### EvalCaseEditor.vue

评估用例编辑器，支持对话模式和场景模式。

| 区域 | 组件 | 功能 |
|------|------|------|
| 模式选择 | `QToggle` | 对话模式 / 场景模式 |
| 对话编辑 | `QCard` 列表 | 编辑每轮对话（用户输入 + 预期响应 + 工具调用） |
| 场景编辑 | `QInput`/`QTextarea` | 对话计划、起始提示、停止信号、最大轮次 |
| 会话输入 | `QInput` | AppName、UserID、初始状态 |

#### EvalMetricEditorDialog.vue

评估指标编辑对话框，支持三种评估准则。

| 区域 | 组件 | 功能 |
|------|------|------|
| 基本信息 | `QInput` | 指标名称、评估器名称、阈值 |
| 准则类型 | `QTab` | 工具轨迹 / 最终响应 / LLM 评判 |
| 工具轨迹 | `QToggle` | 顺序敏感、子集匹配；策略编辑 |
| 最终响应 | `QToggle` | 文本/JSON/ROUGE/XML 准则开关及配置 |
| LLM 评判 | `QSelect` + `QInput` | Judge 模型选择、Rubric 编辑、模板配置 |

#### EvalRunListPage.vue

评估运行列表页，展示历史运行记录。

| 区域 | 组件 | 功能 |
|------|------|------|
| 筛选 | `QSelect` | 按评估集、Agent、状态筛选 |
| 列表 | `QTable` | 展示运行记录（评估集、Agent、状态、通过率、耗时、时间） |
| 状态 | `EvalRunStatusBadge` | 运行状态标签 |
| 行操作 | `QBtn` | 查看详情、删除 |

#### EvalRunDetailPage.vue

评估运行详情页，展示运行结果和各用例评分。

| 区域 | 组件 | 功能 |
|------|------|------|
| 概览 | `QCard` | 运行状态、总用例数、通过/失败数、总体得分、耗时 |
| 得分图表 | `EvalScoreChart` | 各用例得分柱状图/雷达图 |
| 用例结果列表 | `EvalCaseResultCard` | 每个用例的详细结果 |
| 运行详情 | `QExpansionItem` | 多次运行的详细推理信息 |

#### EvalCaseResultCard.vue

评估用例结果卡片。

| 区域 | 组件 | 功能 |
|------|------|------|
| 头部 | `QBadge` | 用例 ID + 通过/失败状态 |
| 指标结果 | `EvalMetricResultCard` | 各指标评分详情 |
| 对话对比 | `QCard` | 实际输出 vs 预期输出对比 |

#### EvalMetricResultCard.vue

评估指标结果卡片。

| 区域 | 组件 | 功能 |
|------|------|------|
| 指标名 | `QLabel` | 指标名称 |
| 得分 | `QLinearProgress` | 得分进度条（颜色反映通过/失败） |
| 阈值 | `QLabel` | 阈值显示 |
| 原因 | `QLabel` | 评分理由 |
| Rubric | `QTable` | LLM 评判 Rubric 得分表 |

#### EvalScoreChart.vue

评估得分图表，使用 Chart.js 或 ECharts。

| 图表类型 | 功能 |
|----------|------|
| 柱状图 | 各用例得分对比 |
| 雷达图 | 多维度指标得分 |
| 趋势图 | 历史运行得分趋势 |

#### EvalRunStatusBadge.vue

运行状态标签组件。

| 状态 | 颜色 |
|------|------|
| running | blue |
| completed | green |
| failed | red |

### 8.5 路由配置

```typescript
// web/src/router/routes.ts 新增
{
  path: '/evaluation',
  component: () => import('layouts/MainLayout.vue'),
  children: [
    { path: 'sets', component: () => import('features/evaluation/components/EvalSetListPage.vue') },
    { path: 'runs', component: () => import('features/evaluation/components/EvalRunListPage.vue') },
    { path: 'runs/:id', component: () => import('features/evaluation/components/EvalRunDetailPage.vue') },
  ],
}
```

---

## 九、实现阶段

### Phase 1：基础 CRUD（MVP）

1. 创建 Proto 文件并生成代码
2. 实现 Ent Schema（eval_set、eval_metric、evaluation_run）
3. 实现 Biz 层（EvalSet/Metric/Run 的 CRUD）
4. 实现 Data 层 Repo
5. 实现 Service 层
6. 前端 EvalSetListPage + EvalSetEditorDialog

### Phase 2：评估执行

1. 实现 `internal/evaluation/trpc/evaluator.go` 适配器
2. 实现 DB-backed EvalSet Manager 和 Metric Manager
3. 实现 RunEvaluation 异步执行流程
4. 前端 EvalRunListPage + EvalRunDetailPage

### Phase 3：高级评估

1. 实现 LLM-as-Judge 集成
2. 实现 UserSimulation 适配器
3. 实现 pass@k / pass^k 计算
4. 前端 EvalScoreChart + A/B 对比

### Phase 4：超越层

1. 评估结果仪表盘（分数分布、趋势图）
2. 回归检测（新版本是否退化）
3. 评估报告导出（PDF/JSON）
4. 评估集模板市场

---

## 十、涉及文件总览

| 文件 | 操作 | 说明 |
|------|------|------|
| `api/kratos/evaluation/v1/evaluation.proto` | 新建 | Proto 定义 |
| `internal/biz/evaluation.go` | 新建 | 领域模型 + Usecase |
| `internal/data/ent/schema/eval_set.go` | 新建 | EvalSet Ent Schema |
| `internal/data/ent/schema/eval_metric.go` | 新建 | EvalMetric Ent Schema |
| `internal/data/ent/schema/evaluation_run.go` | 新建 | EvaluationRun Ent Schema |
| `internal/data/evaluation.go` | 新建 | Repo 实现 |
| `internal/evaluation/trpc/evaluator.go` | 新建 | AgentEvaluator 适配器 |
| `internal/evaluation/trpc/evalset_manager.go` | 新建 | DB-backed EvalSet Manager |
| `internal/evaluation/trpc/metric_manager.go` | 新建 | DB-backed Metric Manager |
| `internal/evaluation/trpc/user_simulator.go` | 新建 | UserSimulation 适配器 |
| `internal/service/evaluation.go` | 新建 | Service 层 |
| `internal/server/register_evaluation.go` | 新建 | HTTP 注册 |
| `web/src/features/evaluation/api.ts` | 新建 | 前端 API |
| `web/src/features/evaluation/types.ts` | 新建 | 前端类型 |
| `web/src/features/evaluation/components/*.vue` | 新建 | 前端组件 |
