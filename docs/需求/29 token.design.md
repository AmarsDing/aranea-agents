# Token 消耗统计模块 — 实现设计文档

> 对应需求：`29 token.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

模型 Token 消耗与费用统计：按 Agent/Session/User/Model 维度分析 Token 用量、费用、趋势，支持预算管控和异常检测。

---

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/monitor/v1/monitor.proto` 中已有 `GetMonitorStats`。

### 2.2 待新增

```protobuf
service TokenUsageService {
  rpc GetTokenUsageOverview(GetTokenUsageOverviewRequest) returns (TokenUsageOverview) {
    option (google.api.http) = { get: "/v1/token-usage/overview" };
  }
  rpc GetTokenUsageByAgent(GetTokenUsageByAgentRequest) returns (TokenUsageByAgentResponse) {
    option (google.api.http) = { get: "/v1/token-usage/by-agent" };
  }
  rpc GetTokenUsageByModel(GetTokenUsageByModelRequest) returns (TokenUsageByModelResponse) {
    option (google.api.http) = { get: "/v1/token-usage/by-model" };
  }
  rpc GetTokenUsageTrend(GetTokenUsageTrendRequest) returns (TokenUsageTrendResponse) {
    option (google.api.http) = { get: "/v1/token-usage/trend" };
  }
  rpc GetTokenUsageCost(GetTokenUsageCostRequest) returns (TokenUsageCostResponse) {
    option (google.api.http) = { get: "/v1/token-usage/cost" };
  }
  rpc SetBudgetAlert(SetBudgetAlertRequest) returns (BudgetAlert) {
    option (google.api.http) = { post: "/v1/token-usage/budget-alerts" body: "*" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type ModelTokenUsageEvent struct {
    ID              string
    SessionID       string
    AgentID         string
    UserID          string
    Provider        string
    Model           string
    InputTokens     int64
    OutputTokens    int64
    CacheReadTokens  int64
    CacheWriteTokens int64
    ReasoningTokens  int64
    EmbeddingTokens  int64
    CostCents       int64
    LatencyMs       int32
    Status          string
    CreatedAt       string
}

type TokenUsageOverview struct {
    TotalInputTokens    int64
    TotalOutputTokens   int64
    TotalCostCents      int64
    TotalCalls          int64
    PeriodStart         string
    PeriodEnd           string
    TopAgents           []AgentUsage
    TopModels           []ModelUsage
}

type BudgetAlert struct {
    ID          string
    ScopeType   string  // "agent"/"user"/"global"
    ScopeID     string
    MonthlyCents int64
    AlertRatio  float64
    Enabled     bool
}
```

### 3.2 Usecase

```go
type TokenUsageUsecase struct {
    repo TokenUsageRepository
}

func (uc *TokenUsageUsecase) GetOverview(ctx, period string) (TokenUsageOverview, error)
func (uc *TokenUsageUsecase) GetByAgent(ctx, agentID, period string) ([]ModelTokenUsageEvent, error)
func (uc *TokenUsageUsecase) GetByModel(ctx, provider, model, period string) ([]ModelTokenUsageEvent, error)
func (uc *TokenUsageUsecase) GetTrend(ctx, granularity, period string) ([]TrendPoint, error)
func (uc *TokenUsageUsecase) GetCost(ctx, scopeType, scopeID, period string) (CostSummary, error)
func (uc *TokenUsageUsecase) SetBudgetAlert(ctx, alert BudgetAlert) (BudgetAlert, error)
func (uc *TokenUsageUsecase) RecordUsage(ctx, event ModelTokenUsageEvent) error
```

---

## 四、Data 层

### 4.1 Ent Schema

文件：`internal/data/ent/schema/model_token_usage_event.go`

```go
func (ModelTokenUsageEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("session_id"),
        field.String("agent_id"),
        field.String("user_id"),
        field.String("provider"),
        field.String("model"),
        field.Int64("input_tokens").Default(0),
        field.Int64("output_tokens").Default(0),
        field.Int64("cache_read_tokens").Default(0),
        field.Int64("cache_write_tokens").Default(0),
        field.Int64("reasoning_tokens").Default(0),
        field.Int64("embedding_tokens").Default(0),
        field.Int64("cost_cents").Default(0),
        field.Int32("latency_ms").Default(0),
        field.String("status"),
        field.String("created_at").Default(time.NowString),
    }
}

func (ModelTokenUsageEvent) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("agent_id", "created_at"),
        index.Fields("provider", "model", "created_at"),
        index.Fields("session_id"),
        index.Fields("user_id", "created_at"),
    }
}
```

### 4.2 预算表

文件：`internal/data/ent/schema/budget_alert.go`

### 4.3 费用计算

```go
// internal/data/token_pricing.go
type PriceEntry struct {
    Provider       string
    Model          string
    InputPer1M     int64  // 美分/百万 Token
    OutputPer1M    int64
    CacheReadPer1M int64
    CacheWritePer1M int64
    EffectiveFrom  string
}

func CalculateCost(p PriceEntry, e ModelTokenUsageEvent) int64
```

---

## 五、Service 层

```go
func (s *TokenUsageService) GetTokenUsageOverview(ctx, req) (*TokenUsageOverview, error)
func (s *TokenUsageService) GetTokenUsageByAgent(ctx, req) (*TokenUsageByAgentResponse, error)
func (s *TokenUsageService) GetTokenUsageByModel(ctx, req) (*TokenUsageByModelResponse, error)
func (s *TokenUsageService) GetTokenUsageTrend(ctx, req) (*TokenUsageTrendResponse, error)
func (s *TokenUsageService) GetTokenUsageCost(ctx, req) (*TokenUsageCostResponse, error)
func (s *TokenUsageService) SetBudgetAlert(ctx, req) (*BudgetAlert, error)
```

---

## 六、Wire 注入

待新增：
```
data.ProviderSet → NewTokenUsageRepo
biz.ProviderSet → NewTokenUsageUsecase
service.ProviderSet → NewTokenUsageService
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/token-usage/
├── api.ts
├── types.ts
├── utils.ts
├── wireNormalize.ts
├── tokenUsageEndpoints.ts
└── components/
    ├── TokenUsageOverviewPage.vue
    ├── TokenUsageByAgentPanel.vue
    ├── TokenUsageByModelPanel.vue
    ├── TokenUsageTrendChart.vue
    ├── TokenUsageCostTable.vue
    └── BudgetAlertEditor.vue
```

### 7.2 组件设计

**TokenUsageOverviewPage.vue**：

| 区域 | 组件 | 说明 |
|------|------|------|
| 概览卡片 | 4×`QCard` | 总输入/总输出/总费用/总调用 |
| 趋势图 | `TokenUsageTrendChart` | 按小时/天/周/月 |
| Agent Top | `TokenUsageByAgentPanel` | Top 10 Agent 用量 |
| Model 占比 | `TokenUsageByModelPanel` | 饼图 |
| 费用明细 | `TokenUsageCostTable` | 表格 + 导出 |

**BudgetAlertEditor.vue**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QSelect` 范围 | `scopeType` | agent/user/global |
| `QInput` 月预算 | `monthlyCents` | 美分 |
| `QSlider` 告警比例 | `alertRatio` | 0.5-1.0 |
| `QToggle` 启用 | `enabled` | — |

### 7.3 API

```typescript
export async function getTokenUsageOverview(period: string): Promise<TokenUsageOverview>
export async function getTokenUsageByAgent(agentId: string, period: string): Promise<AgentUsage[]>
export async function getTokenUsageByModel(provider: string, model: string, period: string): Promise<ModelUsage[]>
export async function getTokenUsageTrend(granularity: string, period: string): Promise<TrendPoint[]>
export async function getTokenUsageCost(scopeType: string, scopeId: string, period: string): Promise<CostSummary>
export async function setBudgetAlert(req: SetBudgetAlertRequest): Promise<BudgetAlert>
```
