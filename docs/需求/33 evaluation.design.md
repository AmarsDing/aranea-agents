# Evaluation 评估模块 — 实现设计文档

> 对应需求：`33 evaluation.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 自动化评估：评估集管理、评估运行、通过判定。对标 trpc-agent-go `evaluation` 包。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
service EvaluationService {
  rpc ListEvalSets(ListEvalSetsRequest) returns (ListEvalSetsResponse) {
    option (google.api.http) = { get: "/v1/evaluations/sets" };
  }
  rpc CreateEvalSet(CreateEvalSetRequest) returns (EvalSet) {
    option (google.api.http) = { post: "/v1/evaluations/sets" body: "*" };
  }
  rpc DeleteEvalSet(DeleteEvalSetRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/evaluations/sets/{id}" };
  }
  rpc RunEvaluation(RunEvaluationRequest) returns (EvaluationRun) {
    option (google.api.http) = { post: "/v1/evaluations/runs" body: "*" };
  }
  rpc GetEvaluationRun(GetEvaluationRunRequest) returns (EvaluationRun) {
    option (google.api.http) = { get: "/v1/evaluations/runs/{id}" };
  }
  rpc ListEvaluationRuns(ListEvaluationRunsRequest) returns (ListEvaluationRunsResponse) {
    option (google.api.http) = { get: "/v1/evaluations/runs" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type EvalSet struct {
    ID          string
    Name        string
    Description string
    AgentID     string
    CasesJSON   string  // 评估用例列表
    PassCriteria string // 通过标准
    CreatedAt   string
    UpdatedAt   string
}

type EvalCase struct {
    Input       string
    ExpectedOutput string
    ExpectedTools  []string
    MaxTurns       int32
}

type EvaluationRun struct {
    ID         string
    EvalSetID  string
    AgentID    string
    Status     string  // "running"/"completed"/"failed"
    TotalCases int32
    PassedCases int32
    Results    []EvalCaseResult
    StartedAt  string
    EndedAt    string
}

type EvalCaseResult struct {
    CaseIndex   int32
    ActualOutput string
    ToolsUsed    []string
    Passed       bool
    Score        float64
    Reason       string
}
```

### 3.2 Usecase

```go
func (uc *EvaluationUsecase) CreateEvalSet(ctx, es EvalSet) (EvalSet, error)
func (uc *EvaluationUsecase) RunEvaluation(ctx, evalSetID, agentID string) (EvaluationRun, error)
func (uc *EvaluationUsecase) GetRun(ctx, id string) (EvaluationRun, error)
```

### 3.3 AgentEvaluator 接口

```go
type AgentEvaluator interface {
    Evaluate(ctx, agent Agent, cases []EvalCase) ([]EvalCaseResult, error)
    Close() error
}
```

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/eval_set.go`
- `internal/data/ent/schema/evaluation_run.go`

---

## 五、Service 层

```go
func (s *EvaluationService) CreateEvalSet(ctx, req) (*EvalSet, error)
func (s *EvaluationService) RunEvaluation(ctx, req) (*EvaluationRun, error)
func (s *EvaluationService) GetEvaluationRun(ctx, req) (*EvaluationRun, error)
func (s *EvaluationService) ListEvaluationRuns(ctx, req) (*ListEvaluationRunsResponse, error)
```

---

## 六、Wire 注入

待新增：
```
data.ProviderSet → NewEvalRepo
biz.ProviderSet → NewEvaluationUsecase
service.ProviderSet → NewEvaluationService
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/evaluation/
├── api.ts
├── types.ts
└── components/
    ├── EvalSetListPage.vue
    ├── EvalSetEditorDialog.vue
    ├── EvalRunListPage.vue
    └── EvalRunDetailPage.vue
```

### 7.2 组件设计

**EvalSetEditorDialog.vue**：评估集编辑（用例列表 + 预期输出）

**EvalRunDetailPage.vue**：运行结果详情（通过率 + 各用例结果）

### 7.3 API

```typescript
export async function listEvalSets(query: EvalSetQuery): Promise<EvalSetListResult>
export async function createEvalSet(req: CreateEvalSetRequest): Promise<EvalSet>
export async function runEvaluation(evalSetId: string, agentId: string): Promise<EvaluationRun>
export async function getEvaluationRun(id: string): Promise<EvaluationRun>
export async function listEvaluationRuns(query: EvalRunQuery): Promise<EvalRunListResult>
```
