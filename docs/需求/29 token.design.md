# Token 消耗统计模块 — 实现设计文档

> 对应需求：`29 token.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

模型 Token 消耗与费用统计：按 Agent / Session / User / Model 维度分析 Token 用量、费用、趋势，支持预算管控和异常检测。

---

## 二、Proto 层

### 2.1 Proto 定义

文件：`api/kratos/usage/v1/usage.proto`

```protobuf
service UsageService {
  rpc GetUsageOverview(UsageQuery) returns (UsageOverview) {
    option (google.api.http) = { get: "/v1/usage/overview" };
  }
  rpc ListUsageTrends(UsageQuery) returns (ListUsageTrendsResponse) {
    option (google.api.http) = { get: "/v1/usage/trends" };
  }
  rpc ListTopModels(UsageQuery) returns (ListBreakdownResponse) {
    option (google.api.http) = { get: "/v1/usage/top-models" };
  }
  rpc ListTopAgents(UsageQuery) returns (ListBreakdownResponse) {
    option (google.api.http) = { get: "/v1/usage/top-agents" };
  }
  rpc ListUsageEvents(UsageQuery) returns (ListUsageEventsResponse) {
    option (google.api.http) = { get: "/v1/usage/events" };
  }
  rpc RecordTokenUsageEvent(TokenUsageEvent) returns (TokenUsageEvent) {
    option (google.api.http) = { post: "/v1/usage/token-events" body: "*" };
  }
}
```

### 2.2 核心 Message

| Message | 说明 |
|---------|------|
| `UsageQuery` | 查询参数：range / start_date / end_date / provider_code / model_api_id / agent_id / status / limit |
| `UsageSummary` | 汇总：call_count / request_count / success/failed/cancelled / input/output/total_tokens / total_cost_micro_usd / avg_latency_ms / avg_tokens_per_second / success_rate |
| `UsageTrendPoint` | 趋势点：date_key + UsageSummary 核心字段 |
| `UsageBreakdownRow` | 占比行：provider_code / model_api_id / model_display_name / agent_id / agent_key + 汇总字段 + success_rate |
| `TokenUsageEvent` | 明细事件：50 个字段，包含时间/归属/模型/Token/价格快照/费用/性能/状态/上下文 |
| `UsageOverview` | 概览：today / yesterday / month / range_summary / trends / top_models / top_agents / anomalies |

### 2.3 待新增（P2：限额 + P3：告警）

```protobuf
service UsageService {
  // ... 现有 6 个 RPC ...

  // P2: 用量限额
  rpc GetUsageQuota(GetUsageQuotaRequest) returns (UsageQuota) {
    option (google.api.http) = { get: "/v1/usage/quotas/{scope_type}/{scope_id}" };
  }
  rpc SetUsageQuota(SetUsageQuotaRequest) returns (UsageQuota) {
    option (google.api.http) = { put: "/v1/usage/quotas/{scope_type}/{scope_id}" body: "*" };
  }
  rpc CheckUsageQuota(CheckUsageQuotaRequest) returns (CheckUsageQuotaResponse) {
    option (google.api.http) = { get: "/v1/usage/quotas/{scope_type}/{scope_id}/check" };
  }

  // P3: 用量告警
  rpc ListBudgetAlerts(ListBudgetAlertsRequest) returns (ListBudgetAlertsResponse) {
    option (google.api.http) = { get: "/v1/usage/budget-alerts" };
  }
  rpc SetBudgetAlert(SetBudgetAlertRequest) returns (BudgetAlert) {
    option (google.api.http) = { post: "/v1/usage/budget-alerts" body: "*" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

#### 已实现

```go
type UsageQuery struct {
    Range        string
    StartDate    string
    EndDate      string
    ProviderCode string
    ModelAPIID   string
    AgentID      string
    Status       string
    Limit        int
}

type UsageSummary struct {
    CallCount          int
    RequestCount       int
    SuccessCount       int
    FailedCount        int
    CancelledCount     int
    InputTokens        int
    OutputTokens       int
    TotalTokens        int
    TotalCostMicroUSD  int64
    AvgLatencyMS       float64
    AvgTokensPerSecond float64
    SuccessRate        float64
}

type UsageTrendPoint struct {
    DateKey            string
    CallCount          int
    InputTokens        int
    OutputTokens       int
    TotalTokens        int
    TotalCostMicroUSD  int64
    SuccessCount       int
    FailedCount        int
    CancelledCount     int
    AvgLatencyMS       float64
    AvgTokensPerSecond float64
}

type UsageBreakdownRow struct {
    ProviderCode       string
    ModelAPIID         string
    ModelDisplayName   string
    AgentID            string
    AgentKey           string
    CallCount          int
    InputTokens        int
    OutputTokens       int
    TotalTokens        int
    TotalCostMicroUSD  int64
    AvgLatencyMS       float64
    AvgTokensPerSecond float64
    SuccessRate        float64
}

type TokenUsageEvent struct {
    ID                            string
    OccurredAt                    string
    DateKey                       string
    HourKey                       string
    WorkspaceID                   string
    UserID                        string
    TeamID                        string
    AgentID                       string
    AgentKey                      string
    SessionID                     string
    MessageID                     string
    RequestID                     string
    ProviderCode                  string
    ProviderType                  string
    ProviderDisplayName           string
    ModelAPIID                    string
    ModelDisplayName              string
    ModelCategoryJSON             string
    UsageKind                     string
    CallCount                     int
    InputTokens                   int
    OutputTokens                  int
    CachedInputTokens             int
    ReasoningTokens               int
    EmbeddingTokens               int
    TotalTokens                   int
    InputPriceMicroUSDPer1K       int64
    OutputPriceMicroUSDPer1K      int64
    CachedInputPriceMicroUSDPer1K int64
    ReasoningPriceMicroUSDPer1K   int64
    EmbeddingPriceMicroUSDPer1K   int64
    InputCostMicroUSD             int64
    OutputCostMicroUSD            int64
    CachedInputCostMicroUSD       int64
    ReasoningCostMicroUSD         int64
    EmbeddingCostMicroUSD         int64
    TotalCostMicroUSD             int64
    LatencyMS                     int
    TimeToFirstTokenMS            int
    TokensPerSecond               float64
    Status                        string
    ErrorCode                     string
    ErrorMessage                  string
    RetryCount                    int
    PromptMode                    string
    MaxOutputTokens               int
    ContextWindowK                int
    StreamEnabled                 bool
    MetadataJSON                  string
    CreatedAt                     string
}

type UsageOverview struct {
    Today     UsageSummary
    Yesterday UsageSummary
    Month     UsageSummary
    Range     UsageSummary
    Trends    []UsageTrendPoint
    TopModels []UsageBreakdownRow
    TopAgents []UsageBreakdownRow
    Anomalies []TokenUsageEvent
}
```

#### 待新增（P2/P3）

```go
type UsageQuota struct {
    ID              string
    ScopeType       string  // "agent" / "user" / "global"
    ScopeID         string
    MonthlyMicroUSD int64
    PeriodStart     string
    PeriodEnd       string
    CreatedAt       string
    UpdatedAt       string
}

type BudgetAlert struct {
    ID          string
    ScopeType   string  // "agent" / "user" / "global"
    ScopeID     string
    AlertRatio  float64
    Enabled     bool
    CreatedAt   string
    UpdatedAt   string
}
```

### 3.2 Repo 接口

#### 已实现

```go
type UsageRepo interface {
    GetModelUsageSummary(ctx context.Context, query UsageQuery) (UsageSummary, error)
    ListModelUsageTrends(ctx context.Context, query UsageQuery) ([]UsageTrendPoint, error)
    ListTopModelUsage(ctx context.Context, query UsageQuery) ([]UsageBreakdownRow, error)
    ListTopAgentUsage(ctx context.Context, query UsageQuery) ([]UsageBreakdownRow, error)
    ListModelUsageEvents(ctx context.Context, query UsageQuery) ([]TokenUsageEvent, error)
    RecordTokenUsageEvent(ctx context.Context, event TokenUsageEvent) (TokenUsageEvent, error)
}
```

#### 待新增（P2/P3）

```go
type UsageQuotaRepo interface {
    GetQuota(ctx context.Context, scopeType, scopeID string) (UsageQuota, error)
    SetQuota(ctx context.Context, quota UsageQuota) (UsageQuota, error)
    CheckQuota(ctx context.Context, scopeType, scopeID string) (UsageQuota, bool, error)
}

type BudgetAlertRepo interface {
    ListAlerts(ctx context.Context, scopeType, scopeID string) ([]BudgetAlert, error)
    SetAlert(ctx context.Context, alert BudgetAlert) (BudgetAlert, error)
}
```

### 3.3 Usecase

#### 已实现

```go
type UsageUsecase struct {
    repo UsageRepo
    now  func() time.Time
}

func (u *UsageUsecase) Overview(ctx, query) (UsageOverview, error)
func (u *UsageUsecase) Trends(ctx, query) ([]UsageTrendPoint, error)
func (u *UsageUsecase) TopModels(ctx, query) ([]UsageBreakdownRow, error)
func (u *UsageUsecase) TopAgents(ctx, query) ([]UsageBreakdownRow, error)
func (u *UsageUsecase) Events(ctx, query) ([]TokenUsageEvent, error)
func (u *UsageUsecase) RecordTokenUsageEvent(ctx, event) (TokenUsageEvent, error)
```

#### 待新增（P2/P3）

```go
func (u *UsageUsecase) GetQuota(ctx, scopeType, scopeID string) (UsageQuota, error)
func (u *UsageUsecase) SetQuota(ctx, quota UsageQuota) (UsageQuota, error)
func (u *UsageUsecase) CheckQuota(ctx, scopeType, scopeID string) (UsageQuota, bool, error)
func (u *UsageUsecase) ListBudgetAlerts(ctx, scopeType, scopeID string) ([]BudgetAlert, error)
func (u *UsageUsecase) SetBudgetAlert(ctx, alert BudgetAlert) (BudgetAlert, error)
```

---

## 四、Data 层

### 4.1 数据表

#### 已实现：`model_token_usage_events`

明细流水表，每次模型调用产生一条不可变记录。

| 字段分组 | 字段 |
|----------|------|
| 时间维度 | id / occurred_at / date_key / hour_key |
| 归属维度 | workspace_id / user_id / team_id / agent_id / agent_key / session_id / message_id / request_id |
| 模型维度 | provider_code / provider_type / provider_display_name / model_api_id / model_display_name / model_category_json |
| 调用类型 | usage_kind / call_count |
| Token 明细 | input_tokens / output_tokens / cached_input_tokens / reasoning_tokens / embedding_tokens / total_tokens |
| 价格快照 | input_price_micro_usd_per_1k / output_price_micro_usd_per_1k / cached_input_price_micro_usd_per_1k / reasoning_price_micro_usd_per_1k / embedding_price_micro_usd_per_1k |
| 费用结果 | input_cost_micro_usd / output_cost_micro_usd / cached_input_cost_micro_usd / reasoning_cost_micro_usd / embedding_cost_micro_usd / total_cost_micro_usd |
| 性能与状态 | latency_ms / time_to_first_token_ms / tokens_per_second / status / error_code / error_message / retry_count |
| 请求上下文 | prompt_mode / max_output_tokens / context_window_k / stream_enabled |
| 扩展 | metadata_json / created_at |

索引：

```sql
idx_usage_events_time       ON (occurred_at)
idx_usage_events_date_model ON (date_key, provider_code, model_api_id)
idx_usage_events_agent_time ON (agent_id, occurred_at)
idx_usage_events_session    ON (session_id)
idx_usage_events_status     ON (status, occurred_at)
```

#### 已实现：`model_token_usage_daily`

日聚合表，用于趋势图和占比分析。写入明细时自动 upsert。

| 字段分组 | 字段 |
|----------|------|
| 维度 | id / date_key / workspace_id / agent_id / agent_key / provider_code / model_api_id / usage_kind |
| 计数 | call_count / request_count / success_count / failed_count / cancelled_count |
| Token | input_tokens / output_tokens / cached_input_tokens / reasoning_tokens / embedding_tokens / total_tokens |
| 费用 | total_cost_micro_usd |
| 性能 | avg_latency_ms / avg_tokens_per_second |
| 时间 | created_at / updated_at |

UNIQUE 约束：`(date_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind)`

#### 已实现：`model_pricing_rules`

模型价格规则表，维护当前和历史价格。

| 字段 | 说明 |
|------|------|
| provider_code / model_api_id | 模型标识 |
| currency | 货币（默认 USD） |
| input/output/cached_input/reasoning/embedding_price_micro_usd_per_1k | 各类 Token 单价 |
| effective_from / effective_to | 生效时间范围 |
| is_active | 是否当前生效 |
| source | 来源（manual / auto_sync） |

Ent Schema：`internal/data/ent/schema/model_pricing_rule.go`

#### 待新增（P2）：`usage_quotas`

```sql
CREATE TABLE IF NOT EXISTS usage_quotas (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,   -- "agent" / "user" / "global"
  scope_id TEXT NOT NULL,     -- agent_id / user_id / "global"
  monthly_micro_usd INTEGER NOT NULL DEFAULT 0,
  period_start TEXT NOT NULL,
  period_end TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(scope_type, scope_id)
);
```

#### 待新增（P3）：`budget_alerts`

```sql
CREATE TABLE IF NOT EXISTS budget_alerts (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  alert_ratio REAL NOT NULL DEFAULT 0.8,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(scope_type, scope_id, alert_ratio)
);
```

### 4.2 写入流程

`RecordTokenUsageEvent` 在一个事务中完成：

1. INSERT INTO `model_token_usage_events`
2. UPDATE `sessions` 聚合计数器（model_call_count / input_tokens / output_tokens / total_tokens / total_cost_micro_usd）
3. UPSERT `model_token_usage_daily`（ON CONFLICT DO UPDATE 累加）

### 4.3 查询构建

`usageWhere(query)` 根据 `UsageQuery` 动态构建 WHERE 子句：

- `start_date` / `end_date` → `date_key >= ? AND date_key <= ?`
- `provider_code` → `provider_code = ?`
- `model_api_id` → `model_api_id = ?`
- `agent_id` → `agent_id = ?`
- `status` → `status = ?`（特殊值 `abnormal` 映射为 `status <> 'success'`）

`usageLimit(limit)` 限制范围 [1, 200]，默认 10。

### 4.4 费用计算

所有费用以 micro USD 存储：

```
input_cost_micro_usd = input_tokens * input_price_micro_usd_per_1k / 1000
output_cost_micro_usd = output_tokens * output_price_micro_usd_per_1k / 1000
total_cost_micro_usd = input_cost + output_cost + cached_input_cost + reasoning_cost + embedding_cost
```

---

## 五、Service 层

### 5.1 已实现

文件：`internal/service/usage.go`

```go
type UsageService struct {
    v1.UnimplementedUsageServiceServer
    uc *biz.UsageUsecase
}

func NewUsageService(uc *biz.UsageUsecase) *UsageService
func (s *UsageService) GetUsageOverview(ctx, *v1.UsageQuery) (*v1.UsageOverview, error)
func (s *UsageService) ListUsageTrends(ctx, *v1.UsageQuery) (*v1.ListUsageTrendsResponse, error)
func (s *UsageService) ListTopModels(ctx, *v1.UsageQuery) (*v1.ListBreakdownResponse, error)
func (s *UsageService) ListTopAgents(ctx, *v1.UsageQuery) (*v1.ListBreakdownResponse, error)
func (s *UsageService) ListUsageEvents(ctx, *v1.UsageQuery) (*v1.ListUsageEventsResponse, error)
func (s *UsageService) RecordTokenUsageEvent(ctx, *v1.TokenUsageEvent) (*v1.TokenUsageEvent, error)
```

### 5.2 用量记录入口

文件：`internal/service/chat_usage_ingress.go`

`recordChatIngressUsage` 在 native chat turn 完成后自动调用，写入一条 `model_token_usage_events` 记录：

- 从 chat response 的 `am` map 提取 token_in / token_out / latency / status 等
- 当 API 未返回 token 数时，使用 `roughTokenEstimateFromText` 估算
- 通过 `CHAT_RECORD_USAGE_INGRESS` 环境变量控制开关（默认开启）
- 使用独立 45s 超时 context，避免因记录失败影响 chat 响应

### 5.3 待新增（P2/P3）

```go
func (s *UsageService) GetUsageQuota(ctx, *v1.GetUsageQuotaRequest) (*v1.UsageQuota, error)
func (s *UsageService) SetUsageQuota(ctx, *v1.SetUsageQuotaRequest) (*v1.UsageQuota, error)
func (s *UsageService) CheckUsageQuota(ctx, *v1.CheckUsageQuotaRequest) (*v1.CheckUsageQuotaResponse, error)
func (s *UsageService) ListBudgetAlerts(ctx, *v1.ListBudgetAlertsRequest) (*v1.ListBudgetAlertsResponse, error)
func (s *UsageService) SetBudgetAlert(ctx, *v1.SetBudgetAlertRequest) (*v1.BudgetAlert, error)
```

P2 quota 检查需在 Chat turn 前执行（`ChatService.RunNativeTurnUnary` 入口处），增加约 1 次 DB 查询延迟。

---

## 六、Wire 注入

### 6.1 已实现

```
data.ProviderSet  → NewUsageRepo
biz.ProviderSet   → NewUsageUsecase
service.ProviderSet → NewUsageService
```

### 6.2 待新增（P2/P3）

```
data.ProviderSet  → NewUsageQuotaRepo / NewBudgetAlertRepo
biz.ProviderSet   → NewUsageQuotaUsecase / (扩展 UsageUsecase)
service.ProviderSet → (扩展 UsageService)
```

---

## 七、Web 前端设计

### 7.1 已实现

#### 文件结构

```
web/src/features/usage/
├── api.ts          ← API 调用 + snake_case ↔ camelCase 转换
└── types.ts        ← TypeScript 类型定义

web/src/components/usage/
├── UsageMetricCards.vue    ← 核心指标卡片
├── UsageTrendPanel.vue     ← 趋势图
├── UsageTopModels.vue      ← Top 模型排行
├── UsageTopAgents.vue      ← Top Agent 排行
└── UsageAnomalyList.vue    ← 异常请求列表
```

#### API 函数

```typescript
export async function getModelUsageOverview(query?: ModelUsageQuery): Promise<ModelUsageOverview>
export async function listModelUsageTrends(query?: ModelUsageQuery): Promise<ModelUsageTrendPoint[]>
export async function listModelUsageEvents(query?: ModelUsageQuery): Promise<ModelTokenUsageEvent[]>
export async function recordModelTokenUsageEvent(e: ModelTokenUsageEvent): Promise<ModelTokenUsageEvent>
```

#### 类型定义

| 类型 | 说明 |
|------|------|
| `ModelUsageQuery` | 查询参数 |
| `ModelUsageSummary` | 汇总数据 |
| `ModelUsageTrendPoint` | 趋势点 |
| `ModelUsageBreakdownRow` | 占比行 |
| `ModelTokenUsageEvent` | 明细事件 |
| `ModelUsageOverview` | 概览（today/yesterday/month/range/trends/top_models/top_agents/anomalies） |

### 7.2 待新增（P2/P3）

```
web/src/features/usage/
├── quotaApi.ts             ← 限额 API
├── budgetAlertApi.ts       ← 告警 API
└── components/
    ├── UsageQuotaEditor.vue    ← 限额配置
    └── BudgetAlertEditor.vue   ← 告警配置
```

#### 限额编辑器

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QSelect` 范围 | `scopeType` | agent / user / global |
| `QInput` 月预算 | `monthlyMicroUSD` | micro USD |
| `QToggle` 启用 | `enabled` | — |

#### 告警编辑器

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QSelect` 范围 | `scopeType` | agent / user / global |
| `QSlider` 告警比例 | `alertRatio` | 0.5–1.0 |
| `QToggle` 启用 | `enabled` | — |

---

## 八、典型分析查询

### 8.1 最近 30 天消耗趋势

```sql
SELECT date_key,
  SUM(call_count) AS call_count,
  SUM(total_tokens) AS total_tokens,
  SUM(total_cost_micro_usd) AS total_cost_micro_usd
FROM model_token_usage_daily
WHERE date_key >= date('now', '-30 day')
GROUP BY date_key
ORDER BY date_key ASC;
```

### 8.2 模型费用占比

```sql
SELECT provider_code, model_api_id,
  SUM(total_cost_micro_usd) AS cost,
  SUM(total_tokens) AS tokens,
  SUM(call_count) AS call_count
FROM model_token_usage_events
WHERE occurred_at >= datetime('now', '-30 day') AND status = 'success'
GROUP BY provider_code, model_api_id
ORDER BY cost DESC;
```

### 8.3 Agent 成本排行

```sql
SELECT agent_id, agent_key,
  SUM(total_cost_micro_usd) AS cost,
  SUM(total_tokens) AS tokens,
  SUM(call_count) AS call_count
FROM model_token_usage_events
WHERE date_key >= date('now', '-7 day')
GROUP BY agent_id, agent_key
ORDER BY cost DESC;
```

### 8.4 高成本异常请求

```sql
SELECT occurred_at, agent_key, provider_code, model_api_id,
  input_tokens, output_tokens, total_cost_micro_usd, latency_ms
FROM model_token_usage_events
WHERE total_cost_micro_usd > 100000
ORDER BY total_cost_micro_usd DESC
LIMIT 50;
```
