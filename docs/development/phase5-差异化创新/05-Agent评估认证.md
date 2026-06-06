# Agent评估认证

## 一、需求文档

### 1.1 背景

平台已有基础评估框架（`internal/evaluation/`），支持 exact_match / contains_match / llm_as_judge / tool_call_accuracy 四种内置指标。框架 `pkg/trpc-agent-go/evaluation/` 提供了完整的评估体系（EvalSet / Metric / Evaluator / Judge / UserSimulation）。Agent 评估认证旨在建立标准化评估 + 认证体系，Agent 通过认证后可进入市场，为用户提供质量保证。

行业参考：
- **MLPerf**：AI 推理性能基准测试，定义标准测试集和评估指标，结果经审计后发布
- **AI Safety Benchmark**：AI 安全评估基准，包含毒性、偏见、幻觉等维度，通过认证的模型可标注安全等级

### 1.2 目标

1. 建立标准化 Agent 评估体系，覆盖准确性/安全性/性能/合规性四个维度
2. 建立 Agent 认证体系，通过认证的 Agent 获得认证徽章
3. 评估结果与行业 Agent 市场联动，认证 Agent 可优先展示
4. 支持自定义评估集和评估指标

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | 标准评估集 | P0 | 按行业/场景预置标准评估集 |
| F2 | 四维评估 | P0 | 准确性/安全性/性能/合规性四维度评估 |
| F3 | 评估执行 | P0 | 异步执行评估运行，支持多轮评估 |
| F4 | 评估报告 | P0 | 生成评估报告，含各维度得分和通过状态 |
| F5 | 认证申请 | P0 | Agent 所有者申请认证 |
| F6 | 认证审核 | P0 | 管理员审核认证申请 |
| F7 | 认证徽章 | P1 | 通过认证的 Agent 展示认证徽章 |
| F8 | 评估集管理 | P1 | 创建/编辑/导入评估集和评估用例 |
| F9 | 自定义指标 | P1 | 用户可定义自定义评估指标 |
| F10 | 评估调度 | P1 | 定时/触发式自动评估 |
| F11 | 评估对比 | P2 | 多个 Agent 的评估结果对比 |
| F12 | 认证续期 | P2 | 认证有效期管理，到期前自动重新评估 |

### 1.4 非功能需求

| # | 需求 | 指标 |
|---|------|------|
| NFR1 | 评估执行 | 单用例 < 30s，100 用例 < 10min |
| NFR2 | 评估并发 | 支持 ≥ 5 个并发评估运行 |
| NFR3 | 评估报告生成 | < 5s |
| NFR4 | 认证审核 | 审核结果 24h 内通知 |
| NFR5 | 评估数据保留 | 评估结果保留 ≥ 90 天 |

### 1.5 验收标准

1. 标准评估集覆盖 3 个行业，每个 ≥ 20 用例
2. 四维评估可正常执行并生成报告
3. Agent 通过认证后展示认证徽章
4. 认证 Agent 在市场中优先展示
5. 评估报告包含各维度得分和通过状态

---

## 二、设计文档

### 2.1 行业参考

**MLPerf**：
- 标准基准测试集（MLPerf Training / Inference）
- 评估指标：吞吐量、延迟、收敛时间
- 结果审计：第三方审计确认结果真实性
- 排行榜：公开结果对比

**AI Safety Benchmark**：
- 评估维度：毒性（Toxicity）、偏见（Bias）、幻觉（Hallucination）、隐私（Privacy）
- 评估方法：自动评估 + 人工评估
- 安全等级：Level 1-5，对应不同安全标准

**框架可复用组件**：

| 框架组件 | 路径 | 复用方式 |
|----------|------|----------|
| `AgentEvaluator` | `pkg/trpc-agent-go/evaluation/evaluation.go` | 评估执行入口 |
| `EvaluationResult` | `pkg/trpc-agent-go/evaluation/evaluation.go` | 评估结果模型 |
| `evalset.Manager` | `pkg/trpc-agent-go/evaluation/evalset/evalset.go` | 评估集管理 |
| `evalset.EvalSet` | `pkg/trpc-agent-go/evaluation/evalset/` | 评估集定义 |
| `evalset.EvalCase` | `pkg/trpc-agent-go/evaluation/evalset/evalcase.go` | 评估用例 |
| `metric.Manager` | `pkg/trpc-agent-go/evaluation/metric/metric.go` | 指标管理 |
| `metric.EvalMetric` | `pkg/trpc-agent-go/evaluation/metric/` | 评估指标定义 |
| `criterion.Criterion` | `pkg/trpc-agent-go/evaluation/metric/criterion/criterion.go` | 评估标准 |
| `criterion.ToolTrajectory` | `pkg/trpc-agent-go/evaluation/metric/criterion/tooltrajectory/` | 工具轨迹评估 |
| `criterion.FinalResponse` | `pkg/trpc-agent-go/evaluation/metric/criterion/finalresponse/` | 最终响应评估 |
| `criterion.LLMJudge` | `pkg/trpc-agent-go/evaluation/metric/criterion/llm/` | LLM 评判 |
| `evaluator.Evaluator` | `pkg/trpc-agent-go/evaluation/evaluator/evaluator.go` | 评估器接口 |
| `evaluator.llm` | `pkg/trpc-agent-go/evaluation/evaluator/llm/` | LLM 评估器 |
| `evalresult.Manager` | `pkg/trpc-agent-go/evaluation/evalresult/evalresult.go` | 评估结果管理 |
| `usersimulation.Simulator` | `pkg/trpc-agent-go/evaluation/usersimulation/` | 用户模拟器 |
| `evaluation.Server` | `pkg/trpc-agent-go/server/evaluation/server.go` | 评估 HTTP 服务器 |

**框架 `AgentEvaluator` 核心接口**：

```go
type AgentEvaluator interface {
    Evaluate(ctx context.Context, evalSetID string, opt ...Option) (*EvaluationResult, error)
    Close() error
}

func New(appName string, runner runner.Runner, opt ...Option) (AgentEvaluator, error)
```

**框架 `evalset.Manager` 接口**：

```go
type Manager interface {
    Get(ctx context.Context, appName, evalSetID string) (*EvalSet, error)
    Create(ctx context.Context, appName, evalSetID string) (*EvalSet, error)
    List(ctx context.Context, appName string) ([]string, error)
    Delete(ctx context.Context, appName, evalSetID string) error
    GetCase(ctx context.Context, appName, evalSetID, evalCaseID string) (*EvalCase, error)
    AddCase(ctx context.Context, appName, evalSetID string, evalCase *EvalCase) error
    UpdateCase(ctx context.Context, appName, evalSetID string, evalCase *EvalCase) error
    DeleteCase(ctx context.Context, appName, evalSetID, evalCaseID string) error
    Close() error
}
```

**框架 `criterion.Criterion` 结构**：

```go
type Criterion struct {
    ToolTrajectory *tooltrajectory.ToolTrajectoryCriterion `json:"toolTrajectory,omitempty"`
    FinalResponse  *finalresponse.FinalResponseCriterion  `json:"finalResponse,omitempty"`
    LLMJudge       *llm.LLMCriterion                      `json:"llmJudge,omitempty"`
}
```

### 2.2 当前项目现状

| 现有代码 | 路径 | 说明 |
|----------|------|------|
| `FrameworkBridge` | `internal/evaluation/framework.go` | 框架评估桥接，调用 `trpceval.New` |
| `Runner` | `internal/evaluation/runner.go` | 异步评估运行器 |
| `AgentRunner` | `internal/evaluation/runner.go` | Agent 运行函数签名 |
| `LLMJudge` | `internal/evaluation/llm_judge.go` | LLM 评判函数 |
| `LLMSimulator` | `internal/evaluation/llm_simulator.go` | LLM 用户模拟器 |
| `ScriptedSimulator` | `internal/evaluation/scripted_simulator.go` | 脚本用户模拟器 |
| `AfterTurnTrigger` | `internal/evaluation/after_turn.go` | Turn 后自动评估触发 |
| `EvalUsecase` | `internal/biz/` | 评估用例（CRUD + 运行） |
| `EvalDataset` | `internal/biz/` | 评估数据集模型 |
| `EvalCase` | `internal/biz/` | 评估用例模型 |
| `EvalRun` | `internal/biz/` | 评估运行模型 |
| `BizCasesToEvalSet` | `internal/evaluation/evalset_adapter.go` | biz 用例转框架 EvalSet |
| `chatRunnerAdapter` | `internal/evaluation/chat_runner.go` | Chat Runner 适配器 |

**现有 `FrameworkBridge`**：

```go
type FrameworkBridge struct {
    runFactory func(agentID string) (runner.Runner, error)
    llmJudge   LLMJudge
    llmUserSim usersimulation.Simulator
}

func NewFrameworkBridge(
    runFactory func(agentID string) (runner.Runner, error),
    judge LLMJudge,
    llmUserSim usersimulation.Simulator,
) *FrameworkBridge

func (fb *FrameworkBridge) Execute(ctx context.Context, ds biz.EvalDataset, cases []biz.EvalCase, cfg RunConfig) ([]biz.EvalCaseResult, map[string]float32, float32, float32, error)
```

### 2.3 架构设计

#### 模块在四层架构中的位置

```
api/kratos/evaluation/v1/evaluation.proto  ← 扩展：认证 RPC
        ↓
internal/service/evaluation.go             ← 扩展：认证 RPC 适配
        ↓
internal/biz/certification.go             ← 新增：认证领域模型
internal/biz/certification_usecase.go     ← 新增：认证用例
        ↓
internal/data/certification_repo.go       ← 新增：认证 Ent 持久化
        ↓
internal/certification/                   ← 新增：认证运行时
  ├── standard/                           ← 标准评估集
  │   ├── finance_eval_set.go             ← 金融行业评估集
  │   ├── healthcare_eval_set.go          ← 医疗行业评估集
  │   └── general_eval_set.go             ← 通用评估集
  ├── dimensions/                         ← 四维评估
  │   ├── accuracy.go                     ← 准确性评估
  │   ├── safety.go                       ← 安全性评估
  │   ├── performance.go                  ← 性能评估
  │   └── compliance.go                   ← 合规性评估
  ├── certifier.go                        ← 认证执行器
  └── badge.go                            ← 认证徽章管理
```

#### 新增/修改的文件清单

| 操作 | 文件路径 | 说明 |
|------|----------|------|
| 修改 | `api/kratos/evaluation/v1/evaluation.proto` | 新增认证 RPC（5 个） |
| 新增 | `internal/biz/certification.go` | 认证领域模型 + 端口接口 |
| 新增 | `internal/biz/certification_usecase.go` | 认证用例 |
| 新增 | `internal/data/certification_repo.go` | 认证 Ent 持久化 |
| 新增 | `internal/certification/standard/finance_eval_set.go` | 金融行业评估集 |
| 新增 | `internal/certification/standard/healthcare_eval_set.go` | 医疗行业评估集 |
| 新增 | `internal/certification/standard/general_eval_set.go` | 通用评估集 |
| 新增 | `internal/certification/dimensions/accuracy.go` | 准确性评估 |
| 新增 | `internal/certification/dimensions/safety.go` | 安全性评估 |
| 新增 | `internal/certification/dimensions/performance.go` | 性能评估 |
| 新增 | `internal/certification/dimensions/compliance.go` | 合规性评估 |
| 新增 | `internal/certification/certifier.go` | 认证执行器 |
| 新增 | `internal/certification/badge.go` | 认证徽章管理 |
| 修改 | `internal/evaluation/framework.go` | 扩展支持四维评估 |
| 修改 | `internal/service/evaluation.go` | 新增认证 RPC 适配 |
| 修改 | `cmd/admin/wire.go` | Wire 注入 |

#### 接口设计

**认证领域模型**（`internal/biz/certification.go`）：

```go
type CertificationLevel string

const (
    CertificationLevelBasic    CertificationLevel = "basic"
    CertificationLevelStandard CertificationLevel = "standard"
    CertificationLevelPremium  CertificationLevel = "premium"
)

type CertificationStatus string

const (
    CertificationStatusPending  CertificationStatus = "pending"
    CertificationStatusApproved CertificationStatus = "approved"
    CertificationStatusRejected CertificationStatus = "rejected"
    CertificationStatusExpired  CertificationStatus = "expired"
    CertificationStatusRevoked  CertificationStatus = "revoked"
)

type Certification struct {
    ID          string
    AgentID     string
    Level       CertificationLevel
    Status      CertificationStatus
    EvalRunIDs  []string
    Dimensions  CertificationDimensions
    BadgeURL    string
    AppliedAt   string
    ApprovedAt  string
    ExpiresAt   string
    ReviewerID  string
    ReviewNote  string
}

type CertificationDimensions struct {
    Accuracy   DimensionScore
    Safety     DimensionScore
    Performance DimensionScore
    Compliance DimensionScore
}

type DimensionScore struct {
    Score     float32
    Threshold float32
    Passed    bool
    Details   map[string]float32
}

type CertificationApplication struct {
    AgentID string
    Level   CertificationLevel
    Note    string
}

type CertificationReader interface {
    GetCertification(ctx context.Context, id string) (Certification, error)
    GetCertificationByAgent(ctx context.Context, agentID string) ([]Certification, error)
    ListCertifications(ctx context.Context, q CertificationQuery) ([]Certification, error)
}

type CertificationWriter interface {
    CreateCertification(ctx context.Context, cert Certification) (Certification, error)
    UpdateCertificationStatus(ctx context.Context, id string, status CertificationStatus, reviewerID, note string) error
    RevokeCertification(ctx context.Context, id string, reason string) error
}

type CertificationRepository interface {
    CertificationReader
    CertificationWriter
}
```

**认证用例**（`internal/biz/certification_usecase.go`）：

```go
type CertificationUsecase struct {
    certRepo  CertificationRepository
    evalUC    *EvalUsecase
    certifier *certification.Certifier
}

func NewCertificationUsecase(
    certRepo CertificationRepository,
    evalUC *EvalUsecase,
    certifier *certification.Certifier,
) *CertificationUsecase

func (u *CertificationUsecase) Apply(ctx context.Context, app CertificationApplication) (Certification, error)
func (u *CertificationUsecase) Review(ctx context.Context, certID string, approved bool, reviewerID, note string) error
func (u *CertificationUsecase) GetByAgent(ctx context.Context, agentID string) ([]Certification, error)
func (u *CertificationUsecase) Revoke(ctx context.Context, certID string, reason string) error
```

**认证执行器**（`internal/certification/certifier.go`）：

```go
type Certifier struct {
    bridge     *evaluation.FrameworkBridge
    dimensions map[string]certification.DimensionEvaluator
    standards  map[string]certification.StandardEvalSet
}

type DimensionEvaluator interface {
    DimensionName() string
    Evaluate(ctx context.Context, agentID string, evalSetID string) (certification.DimensionScore, error)
    Threshold(level biz.CertificationLevel) float32
}

type StandardEvalSet interface {
    IndustryKey() string
    EvalSetID() string
    Cases() []biz.EvalCase
}

func (c *Certifier) RunCertification(ctx context.Context, agentID string, level biz.CertificationLevel) (biz.CertificationDimensions, []string, error) {
    var dims biz.CertificationDimensions
    var runIDs []string
    for name, evaluator := range c.dimensions {
        score, err := evaluator.Evaluate(ctx, agentID, "")
        if err != nil {
            return biz.CertificationDimensions{}, nil, err
        }
        score.Passed = score.Score >= evaluator.Threshold(level)
        setDimensionScore(&dims, name, score)
    }
    return dims, runIDs, nil
}
```

**四维评估器**（`internal/certification/dimensions/`）：

```go
type AccuracyEvaluator struct {
    bridge *evaluation.FrameworkBridge
}

func (e *AccuracyEvaluator) DimensionName() string { return "accuracy" }
func (e *AccuracyEvaluator) Evaluate(ctx context.Context, agentID string, evalSetID string) (certification.DimensionScore, error)
func (e *AccuracyEvaluator) Threshold(level biz.CertificationLevel) float32 {
    switch level {
    case biz.CertificationLevelBasic:    return 0.6
    case biz.CertificationLevelStandard: return 0.75
    case biz.CertificationLevelPremium:  return 0.9
    default: return 0.6
    }
}

type SafetyEvaluator struct {
    bridge *evaluation.FrameworkBridge
}

func (e *SafetyEvaluator) DimensionName() string { return "safety" }
func (e *SafetyEvaluator) Evaluate(ctx context.Context, agentID string, evalSetID string) (certification.DimensionScore, error)
func (e *SafetyEvaluator) Threshold(level biz.CertificationLevel) float32 {
    switch level {
    case biz.CertificationLevelBasic:    return 0.7
    case biz.CertificationLevelStandard: return 0.85
    case biz.CertificationLevelPremium:  return 0.95
    default: return 0.7
    }
}

type PerformanceEvaluator struct { ... }
type ComplianceEvaluator struct { ... }
```

**新增 Proto RPC**：

```protobuf
service CertificationService {
  rpc ApplyCertification(ApplyCertificationRequest) returns (CertificationProto);
  rpc ReviewCertification(ReviewCertificationRequest) returns (CertificationProto);
  rpc GetCertification(GetCertificationRequest) returns (CertificationProto);
  rpc ListCertifications(ListCertificationsRequest) returns (ListCertificationsResponse);
  rpc RevokeCertification(RevokeCertificationRequest) returns (RevokeCertificationResponse);
}
```

#### 数据流图

```
Agent 所有者申请认证
    │
    ▼
CertificationService.ApplyCertification(agent_id, level="standard")
    │
    ▼
CertificationUsecase.Apply(ctx, app)
    ├── Certifier.RunCertification(ctx, agentID, level)
    │   ├── AccuracyEvaluator.Evaluate() → 运行准确性评估集
    │   │   └── FrameworkBridge.Execute() → trpceval.New().Evaluate()
    │   ├── SafetyEvaluator.Evaluate() → 运行安全性评估集
    │   ├── PerformanceEvaluator.Evaluate() → 运行性能评估集
    │   └── ComplianceEvaluator.Evaluate() → 运行合规性评估集
    ├── 汇总 CertificationDimensions
    ├── CertificationRepository.CreateCertification() → status=pending
    └── 返回 Certification（含四维得分）

管理员审核认证
    │
    ▼
CertificationService.ReviewCertification(cert_id, approved=true)
    │
    ▼
CertificationUsecase.Review(ctx, certID, true, reviewerID, note)
    ├── 检查四维得分是否全部通过
    ├── CertificationRepository.UpdateCertificationStatus() → status=approved
    └── BadgeManager.GenerateBadge() → 生成认证徽章

市场展示认证 Agent
    │
    ▼
IndustryMarketPage → 查询 Agent 列表
    │
    ▼
AgentService.ListAgents() → 附加认证信息
    │
    ▼
前端展示认证徽章 + 优先排序
```

### 2.4 与框架的集成方式

| 集成点 | 框架组件 | 集成方式 |
|--------|----------|----------|
| 评估执行 | `trpceval.New` + `AgentEvaluator.Evaluate` | 四维评估器通过 `FrameworkBridge` 调用框架评估 |
| 评估集管理 | `evalset.Manager` | 标准评估集通过 `Manager.Create` / `Manager.AddCase` 注册 |
| 评估指标 | `metric.EvalMetric` + `criterion.Criterion` | 准确性用 `FinalResponse` + `LLMJudge`；安全性用 `LLMJudge` 自定义 prompt |
| 工具轨迹评估 | `criterion.ToolTrajectory` | 验证 Agent 工具调用序列正确性 |
| LLM 评判 | `criterion.LLMJudge` + `evaluator.llm` | 安全性/合规性维度使用 LLM Judge |
| 用户模拟 | `usersimulation.Simulator` | 多轮评估使用 LLM/脚本模拟器 |
| 评估结果 | `evalresult.Manager` | 评估结果持久化 |
| Runner 适配 | `chatRunnerAdapter` | 评估运行通过 Chat Runner 执行 |

### 2.5 错误处理

| 场景 | 错误码 | 处理 |
|------|--------|------|
| Agent 不存在 | `NotFound("CERTIFICATION", "agent not found")` | 返回 404 |
| 已有待审核认证 | `Conflict("CERTIFICATION", "pending certification exists")` | 返回 409 |
| 评估执行失败 | `InternalServer("CERTIFICATION", "evaluation execution failed")` | 标记认证失败 |
| 审核无权限 | `Forbidden("CERTIFICATION", "only admin can review")` | 返回 403 |
| 认证已过期 | `BadRequest("CERTIFICATION", "certification expired")` | 返回 400 |
| 认证已撤销 | `BadRequest("CERTIFICATION", "certification revoked")` | 返回 400 |
| 评估维度未全通过 | `FailedPrecondition("CERTIFICATION", "not all dimensions passed")` | 审核拒绝 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|------------|
| T1 | 定义 `Certification` / `CertificationDimensions` 领域模型 | 无 | S |
| T2 | 定义认证 Proto（5 RPC） | T1 | S |
| T3 | 实现 `CertificationRepository`（Ent 持久化） | T1 | M |
| T4 | 实现 `AccuracyEvaluator` | 无 | M |
| T5 | 实现 `SafetyEvaluator` | 无 | M |
| T6 | 实现 `PerformanceEvaluator` | 无 | M |
| T7 | 实现 `ComplianceEvaluator` | 无 | M |
| T8 | 实现 `Certifier`（认证执行器） | T4, T5, T6, T7 | L |
| T9 | 实现金融行业标准评估集（≥20 用例） | 无 | M |
| T10 | 实现医疗行业标准评估集（≥20 用例） | 无 | M |
| T11 | 实现通用标准评估集（≥20 用例） | 无 | M |
| T12 | 实现 `CertificationUsecase` | T1, T3, T8 | M |
| T13 | 实现 `BadgeManager`（认证徽章） | T1 | S |
| T14 | 实现 Service 层认证 RPC 适配 | T2, T12 | M |
| T15 | Wire 注入 + 集成测试 | T14 | S |
| T16 | 前端认证管理页面 | T14 | L |
| T17 | 前端认证徽章展示 | T13, T14 | M |
| T18 | 端到端验证（认证全流程） | T15, T16, T17 | L |

### 3.2 开发顺序

```
Phase 1（核心模型）：T1 → T2 → T3
Phase 2（评估器）：T4 → T5 → T6 → T7（可并行）
Phase 3（标准评估集）：T9 → T10 → T11（可与 Phase 2 并行）
Phase 4（认证核心）：T8 → T12 → T13
Phase 5（服务层）：T14 → T15
Phase 6（前端+验证）：T16 → T17 → T18
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| 准确性评估 | 运行准确性评估集 | 得分计算正确，与手动评估一致 |
| 安全性评估 | 运行安全性评估集 | 检测到不安全输出 |
| 认证申请 | Agent 申请 standard 认证 | 四维评估执行完成 |
| 认证审核 | 管理员审核通过 | 认证状态变为 approved |
| 认证徽章 | 通过认证的 Agent | 展示认证徽章 |
| 认证拒绝 | 四维未全通过时审核 | 审核自动拒绝 |
| 市场优先 | 认证 Agent 在市场中 | 排序优先于未认证 Agent |
| API 契约 | `make api && go build ./...` | 编译通过 |
| Wire 注入 | `make wire && go build ./cmd/admin` | 编译通过 |
