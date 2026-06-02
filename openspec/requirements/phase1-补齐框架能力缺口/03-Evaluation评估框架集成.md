# Evaluation 评估框架集成

## 一、需求文档

### 1.1 背景

trpc-agent-go 框架提供了完整的评估体系（EvalSet/Metric/Judge/UserSimulation/MultiRun），当前项目 `internal/evaluation/` 已有基础框架（`FrameworkBridge`、`Runner`、`LLMJudge`、`scriptedSimulator`），前端有 `EvaluationPage`，但尚未完整集成框架的评估能力。主要缺口包括：EvalSet 持久化、Metric 注册体系、LLM Judge 评估器、UserSimulation 对话模拟、MultiRun 并行评估。

### 1.2 目标

- 完整集成框架的 `evaluation.Service` 接口（Inference + Evaluate 两阶段）
- 集成框架的 Metric 体系（text/rouge/json/xml/tool_trajectory/final_response/LLM criterion）
- 集成框架的 UserSimulation 对话模拟（LLM 驱动 + 脚本驱动）
- 支持 MultiRun 并行评估和 pass@k / pass^k 指标
- 前端 EvaluationPage 展示完整评估结果

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | 框架 evaluation.Service 集成 | P0 | Inference + Evaluate 两阶段评估流程 |
| F2 | EvalSet 持久化（SQLite） | P0 | 当前使用 inmemory，需替换为 Ent 持久化 |
| F3 | Metric 注册体系 | P0 | 注册框架内置的 text/rouge/json/xml/tool_trajectory 等 Metric |
| F4 | LLM Judge 评估器集成 | P1 | 框架的 `evaluator/llm` 包提供多种 LLM 评估策略 |
| F5 | UserSimulation LLM 驱动 | P1 | 框架的 `usersimulation.Simulator` 接口集成 |
| F6 | MultiRun 并行评估 | P1 | 多次运行取 pass@k 指标 |
| F7 | 评估结果持久化 | P0 | EvalResult 存储（当前 inmemory） |
| F8 | 前端评估结果展示增强 | P2 | 展示 pass@k、详细评分、运行详情 |

### 1.4 非功能需求

- 评估运行不得阻塞主服务（异步执行）
- 单次评估超时由 ctx 控制
- MultiRun 并行度可配置
- 评估结果支持分页查询
- 日志走 `internal/event` FlowLog

### 1.5 验收标准

- 可创建 EvalSet 并添加 EvalCase
- 可配置 Metric 并运行评估
- LLM Judge 评估器正确评分
- UserSimulation 多轮对话模拟正常工作
- MultiRun 生成 pass@k / pass^k 指标
- 评估结果持久化到 SQLite
- 前端可查看完整评估报告

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go）

**核心包路径**：

| 子能力 | 包路径 |
|--------|--------|
| 顶层入口 | `pkg/trpc-agent-go/evaluation/options.go` |
| Service 接口 | `pkg/trpc-agent-go/evaluation/service/service.go` |
| Metric 管理 | `pkg/trpc-agent-go/evaluation/metric/metric.go` |
| Metric 注册 | `pkg/trpc-agent-go/evaluation/metric/registry/registry.go` |
| Evaluator 注册 | `pkg/trpc-agent-go/evaluation/evaluator/registry/registry.go` |
| UserSimulation | `pkg/trpc-agent-go/evaluation/usersimulation/usersimulation.go` |
| Pass@K 计算 | `pkg/trpc-agent-go/evaluation/pass.go` |
| EvalSet 管理 | `pkg/trpc-agent-go/evaluation/evalset/` |
| EvalResult 管理 | `pkg/trpc-agent-go/evaluation/evalresult/` |
| Criterion 体系 | `pkg/trpc-agent-go/evaluation/metric/criterion/` |

**核心类型和函数**：

```go
// evaluation.Service — 评估服务接口
type Service interface {
    Inference(ctx context.Context, request *InferenceRequest, opt ...Option) ([]*InferenceResult, error)
    Evaluate(ctx context.Context, request *EvaluateRequest, opt ...Option) (*EvalSetRunResult, error)
    Close() error
}

// evaluation.Option
func WithEvalSetManager(m evalset.Manager) Option
func WithEvalResultManager(m evalresult.Manager) Option
func WithMetricManager(m metric.Manager) Option
func WithRegistry(r registry.Registry) Option
func WithMetricRegistry(r metricregistry.Registry) Option
func WithEvaluationService(s service.Service) Option
func WithUserSimulator(sim usersimulation.Simulator) Option
func WithJudgeRunner(judge runner.Runner) Option
func WithNumRuns(numRuns int) Option
func WithNumRunsParallelEnabled(enabled bool) Option
func WithEvalCaseParallelism(parallelism int) Option

// metric.Manager — 指标管理接口
type Manager interface {
    List(ctx context.Context, appName, evalSetID string) ([]string, error)
    Get(ctx context.Context, appName, evalSetID, metricName string) (*EvalMetric, error)
    Add(ctx context.Context, appName, evalSetID string, metric *EvalMetric) error
    Delete(ctx context.Context, appName, evalSetID, metricName string) error
    Update(ctx context.Context, appName, evalSetID string, metric *EvalMetric) error
    Close() error
}

// metric.EvalMetric — 评估指标
type EvalMetric struct {
    MetricName    string               `json:"metricName,omitempty"`
    EvaluatorName string               `json:"evaluatorName,omitempty"`
    Threshold     float64              `json:"threshold,omitempty"`
    Criterion     *criterion.Criterion `json:"criterion,omitempty"`
}

// usersimulation.Simulator — 用户模拟器接口
type Simulator interface {
    Start(ctx context.Context, req *StartRequest) (Conversation, error)
}

// usersimulation.Conversation — 对话接口
type Conversation interface {
    Next(ctx context.Context, req *TurnRequest) (*Decision, error)
    Close() error
}

// PassAtK / PassHatK — 评估指标计算
func PassAtK(n, c, k int) (float64, error)
func PassHatK(n, c, k int) (float64, error)
```

**框架内置 Criterion 类型**（`evaluation/metric/criterion/`）：
- `text` — 文本匹配
- `rouge` — ROUGE-L 指标
- `json` — JSON 结构匹配
- `xml` — XML 结构匹配
- `finalresponse` — 最终响应匹配
- `tooltrajectory` — 工具调用轨迹匹配
- `llm` — LLM 作为评判
- `length` — 长度检查

### 2.2 当前项目现状

| 位置 | 现状 |
|------|------|
| `internal/evaluation/framework.go` | `FrameworkBridge` 已实现，使用 `trpceval.New` 构建评估器 |
| `internal/evaluation/runner.go` | `Runner` 异步执行评估，支持 legacy 和 framework 两种模式 |
| `internal/evaluation/llm_judge.go` | `LLMJudge` 函数已实现，直接调用 LLM 评分 |
| `internal/evaluation/llm_simulator.go` | LLM 用户模拟器已实现 |
| `internal/evaluation/scripted_simulator.go` | 脚本驱动模拟器已实现 |
| `internal/evaluation/evalset_adapter.go` | `BizCasesToEvalSet` 转换函数已实现 |
| `internal/evaluation/framework_metrics.go` | 框架 Metric 注册已部分实现 |
| `internal/evaluation/chat_runner.go` | `chatRunnerAdapter` 适配 `runner.Runner` 接口 |
| 前端 `EvaluationPage` | 已有评估页面 UI |

**主要缺口**：
1. EvalSet/EvalResult/Metric 使用 inmemory 存储，无持久化
2. 框架的 `evaluator/llm` 评估器未注册
3. MultiRun 并行评估未启用
4. pass@k / pass^k 指标未在前端展示
5. 框架的 `service.Service` 接口未直接暴露

### 2.3 架构设计

**模块在四层架构中的位置**：

```
api/**/*.proto          ← 新增评估相关 API
        ↓
internal/service        ← 评估 Service 暴露 HTTP/gRPC 接口
        ↓
internal/biz            ← EvalUsecase 扩展，对接框架 Service
        ↓
internal/data           ← EvalSet/EvalResult/Metric 持久化（Ent ORM）
```

**新增/修改的文件清单**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/evaluation/persistent_evalset.go` | 新增 | 实现 `evalset.Manager` 接口，基于 Ent 持久化 |
| `internal/evaluation/persistent_evalresult.go` | 新增 | 实现 `evalresult.Manager` 接口，基于 Ent 持久化 |
| `internal/evaluation/persistent_metric.go` | 新增 | 实现 `metric.Manager` 接口，基于 Ent 持久化 |
| `internal/evaluation/evaluator_registry.go` | 新增 | 注册框架内置评估器（LLM/ToolTrajectory 等） |
| `internal/evaluation/multirun.go` | 新增 | MultiRun 并行评估配置和执行 |
| `internal/evaluation/framework.go` | 修改 | 替换 inmemory 为持久化 Manager |
| `internal/biz/evaluation.go` | 修改 | EvalUsecase 扩展 MultiRun 和 pass@k |
| `internal/data/ent/schema/` | 修改 | 新增 EvalSet/EvalResult/Metric 表 |
| `api/admin/v1/evaluation.proto` | 修改 | 新增评估 API |

**接口设计**：

```go
// internal/evaluation/persistent_evalset.go

type PersistentEvalSetManager struct {
    data *data.Data
}

func NewPersistentEvalSetManager(data *data.Data) evalset.Manager

// internal/evaluation/persistent_evalresult.go

type PersistentEvalResultManager struct {
    data *data.Data
}

func NewPersistentEvalResultManager(data *data.Data) evalresult.Manager

// internal/evaluation/persistent_metric.go

type PersistentMetricManager struct {
    data *data.Data
}

func NewPersistentMetricManager(data *data.Data) metric.Manager

// internal/evaluation/evaluator_registry.go

func RegisterBuiltinEvaluators(reg registry.Registry, judgeRunner runner.Runner)

// internal/evaluation/multirun.go

type MultiRunConfig struct {
    NumRuns                int
    ParallelEnabled        bool
    CaseParallelism        int
    ParallelInference      bool
    ParallelEvaluation     bool
    RunDetailsEnabled      bool
}

func (c MultiRunConfig) ToOptions() []trpceval.Option
```

**数据流图**：

```
前端 EvaluationPage
  → API CreateEvalRun
    → service/evaluation.go
      → biz.EvalUsecase.StartRun()
        → evaluation.Runner.Execute()
          → FrameworkBridge.Evaluate()
            → trpceval.New(opts...)
              → service.Service.Inference() → runner.Run()
              → service.Service.Evaluate() → metric/criterion 计算
            → 结果持久化 (PersistentEvalResultManager)
          → pass@k / pass^k 计算
        → WebSocket 推送进度
```

### 2.4 与框架的集成方式

1. **Manager 持久化**：实现框架的 `evalset.Manager`/`evalresult.Manager`/`metric.Manager` 接口，底层使用 Ent ORM 操作 SQLite
2. **Evaluator 注册**：调用框架 `registry.Registry.Register()` 注册内置评估器（LLM Judge、ToolTrajectory 等）
3. **Metric 注册**：调用框架 `metricregistry.Registry.Register()` 注册内置 Metric
4. **Service 构建**：使用 `trpceval.New()` 构建顶层评估器，传入持久化 Manager 和注册的 Evaluator/Metric
5. **UserSimulation**：复用现有 `llm_simulator.go` 和 `scripted_simulator.go`，通过 `usersimulation.Simulator` 接口注入
6. **MultiRun**：通过 `WithNumRuns`/`WithNumRunsParallelEnabled`/`WithEvalCaseParallelism` 配置

### 2.5 错误处理

| 场景 | 处理方式 |
|------|----------|
| Inference 失败 | `InferenceResult.Status = status.EvalStatusFailed`，记录 ErrorMessage |
| Evaluate 失败 | `EvalCaseResult.FinalEvalStatus = status.EvalStatusFailed` |
| LLM Judge 超时 | ctx 超时控制，返回错误评分 |
| 持久化写入失败 | `kerrors.InternalServer("EVALUATION", err.Error())` |
| EvalSet 不存在 | `kerrors.NotFound("EVALUATION", "eval set not found")` |
| Agent Runner 构建失败 | 评估运行标记为 failed |
| 并行评估部分失败 | 单个 case 失败不影响其他 case，汇总时标记 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| EV-01 | `internal/data/ent/schema/`：新增 eval_set / eval_result / eval_metric 表 | 无 | M |
| EV-02 | `go generate` 重新生成 Ent 代码 | EV-01 | S |
| EV-03 | `internal/evaluation/persistent_evalset.go`：实现 `evalset.Manager` | EV-02 | L |
| EV-04 | `internal/evaluation/persistent_evalresult.go`：实现 `evalresult.Manager` | EV-02 | L |
| EV-05 | `internal/evaluation/persistent_metric.go`：实现 `metric.Manager` | EV-02 | M |
| EV-06 | `internal/evaluation/evaluator_registry.go`：注册内置评估器 | 无 | M |
| EV-07 | `internal/evaluation/multirun.go`：MultiRun 配置和执行 | 无 | M |
| EV-08 | `internal/evaluation/framework.go`：替换 inmemory 为持久化 Manager | EV-03, EV-04, EV-05 | M |
| EV-09 | `internal/biz/evaluation.go`：EvalUsecase 扩展 | EV-08 | M |
| EV-10 | `api/admin/v1/evaluation.proto`：新增评估 API | EV-09 | M |
| EV-11 | `make api` 重新生成 proto 代码 | EV-10 | S |
| EV-12 | Service 层评估 API 实现 | EV-11 | M |
| EV-13 | 单元测试：持久化 Manager | EV-03, EV-04, EV-05 | M |
| EV-14 | 集成测试：完整评估流程 | EV-12 | L |
| EV-15 | `make wire` 更新 Wire 注入 | EV-12 | S |

### 3.2 开发顺序

```
EV-01 → EV-02 → EV-03 ─┐
                EV-04 ─┤→ EV-08 → EV-09 → EV-10 → EV-11 → EV-12
                EV-05 ─┘                                      ↓
EV-06 ─────────────────────────────────────────────→ EV-13 → EV-14 → EV-15
EV-07 ──────────────────────────────────────────────────────↑
```

### 3.3 验证方案

| 验证项 | 方法 |
|--------|------|
| 持久化 Manager | `go test ./internal/evaluation/... -run TestPersistent -count=1` |
| Evaluator 注册 | `go test ./internal/evaluation/... -run TestEvaluator -count=1` |
| MultiRun | `go test ./internal/evaluation/... -run TestMultiRun -count=1` |
| 完整评估流程 | `go test ./internal/evaluation/... -run TestFramework -count=1` |
| Proto 生成 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 全量验证 | `make api && make wire && make build && make test` |
