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
  rpc GetUsageQuota(GetUsageQuotaRequest) returns (UsageQuota) {
    option (google.api.http) = { get: "/v1/usage/quotas/{scope_type}/{scope_id}" };
  }
  rpc SetUsageQuota(SetUsageQuotaRequest) returns (UsageQuota) {
    option (google.api.http) = { put: "/v1/usage/quotas/{scope_type}/{scope_id}" body: "*" };
  }
  rpc CheckUsageQuota(CheckUsageQuotaRequest) returns (CheckUsageQuotaResponse) {
    option (google.api.http) = { get: "/v1/usage/quotas/{scope_type}/{scope_id}/check" };
  }
  rpc ListBudgetAlerts(ListBudgetAlertsRequest) returns (ListBudgetAlertsResponse) {
    option (google.api.http) = { get: "/v1/usage/budget-alerts" };
  }
  rpc SetBudgetAlert(SetBudgetAlertRequest) returns (BudgetAlert) {
    option (google.api.http) = { post: "/v1/usage/budget-alerts" body: "*" };
  }
  rpc ExportUsageEvents(UsageQuery) returns (ExportUsageEventsResponse) {
    option (google.api.http) = { get: "/v1/usage/events/export" };
  }
  rpc PurgeUsageEvents(PurgeUsageEventsRequest) returns (PurgeUsageEventsResponse) {
    option (google.api.http) = { post: "/v1/usage/events/purge" body: "*" };
  }
}
```

### 2.2 核心 Message

| Message | 说明 |
|---------|------|
| `UsageQuery` | 查询参数：range / start_date / end_date / provider_code / model_api_id / agent_id / team_id / usage_kind / status / limit / granularity |
| `UsageSummary` | 汇总：call_count / request_count / success/failed/cancelled / input/output/total_tokens / total_cost_micro_usd / avg_latency_ms / avg_tokens_per_second / success_rate |
| `UsageTrendPoint` | 趋势点：date_key + UsageSummary 核心字段 |
| `UsageBreakdownRow` | 占比行：provider_code / model_api_id / model_display_name / agent_id / agent_key + 汇总字段 + success_rate |
| `TokenUsageEvent` | 明细事件：53 个字段，包含时间/归属/模型/Token/价格快照/费用/性能/状态/上下文 + 显示名（`agent_name` / `session_title` / `team_name`，仅列表查询填充） |
| `UsageOverview` | 概览：today / yesterday / month / range_summary / trends / top_models / top_agents / anomalies / quota_dashboard / inefficient_models |
| `UsageQuota` | 限额：id / scope_type / scope_id / monthly_micro_usd / period_start / period_end / created_at / updated_at |
| `BudgetAlert` | 告警：id / scope_type / scope_id / alert_ratio / enabled / last_fired_at / created_at / updated_at |
| `UsageQuotaDashboard` | 配额仪表盘：configured_count / total_cap_micro_usd / total_spent_micro_usd / max_utilization_ratio |
| `UsageModelInsight` | 低性价比模型：provider_code / model_api_id / model_display_name / call_count / total_tokens / total_cost_micro_usd / avg_latency_ms / avg_tokens_per_second / success_rate / flags |
| `ExportUsageEventsResponse` | CSV 导出响应：csv 字符串 |
| `PurgeUsageEventsRequest` | 清理请求：retain_days |
| `PurgeUsageEventsResponse` | 清理响应：deleted_count |

---

## 三、Biz 层

包路径：`internal/biz/usage`（`internal/biz/usage.go` 提供类型别名透传到 `biz` 包）

### 3.1 领域模型

```go
type Query struct {
    Range        string
    StartDate    string
    EndDate      string
    ProviderCode string
    ModelAPIID   string
    AgentID      string
    TeamID       string
    UsageKind    string
    Status       string
    Limit        int
    Granularity  string
}

type Summary struct {
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

type TrendPoint struct {
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

type BreakdownRow struct {
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
    CanonicalProviderCode         string
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
    CacheWriteTokens              int
    ReasoningTokens               int
    EmbeddingTokens               int
    TotalTokens                   int
    InputPriceMicroUSDPer1K       int64
    OutputPriceMicroUSDPer1K      int64
    CachedInputPriceMicroUSDPer1K int64
    CacheWritePriceMicroUSDPer1K  int64
    ReasoningPriceMicroUSDPer1K   int64
    EmbeddingPriceMicroUSDPer1K   int64
    InputPriceUSDPer1M            float64
    OutputPriceUSDPer1M           float64
    CacheReadPriceUSDPer1M        float64
    CacheWritePriceUSDPer1M       float64
    ReasoningPriceUSDPer1M        float64
    EmbeddingPriceUSDPer1M        float64
    InputCostMicroUSD             int64
    OutputCostMicroUSD            int64
    CachedInputCostMicroUSD       int64
    CacheWriteCostMicroUSD        int64
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
    // 显示名（仅 ListModelUsageEvents 经标量子查询填充；实体已删除时为空，前端回退显示 ID）
    AgentName                     string
    SessionTitle                  string
    TeamName                      string
}

type Overview struct {
    Today             Summary
    Yesterday         Summary
    Month             Summary
    Range             Summary
    Trends            []TrendPoint
    TopModels         []BreakdownRow
    TopAgents         []BreakdownRow
    Anomalies         []TokenUsageEvent
    QuotaDashboard    QuotaDashboard
    InefficientModels []ModelInsight
}

type ModelPricingSnapshot struct {
    InputPriceMicroUSDPer1K       int64
    OutputPriceMicroUSDPer1K      int64
    CachedInputPriceMicroUSDPer1K int64
    CacheWritePriceMicroUSDPer1K  int64
    ReasoningPriceMicroUSDPer1K   int64
    EmbeddingPriceMicroUSDPer1K   int64
    InputPriceUSDPer1M            float64
    OutputPriceUSDPer1M           float64
    CacheReadPriceUSDPer1M        float64
    CacheWritePriceUSDPer1M       float64
    ReasoningPriceUSDPer1M        float64
    EmbeddingPriceUSDPer1M        float64
}

type Quota struct {
    ID              string
    ScopeType       string  // "agent" / "user" / "global"
    ScopeID         string
    MonthlyMicroUSD int64
    PeriodStart     string
    PeriodEnd       string
    CreatedAt       string
    UpdatedAt       string
}

type QuotaCheck struct {
    Quota             Quota
    Allowed           bool
    SpentMicroUSD     int64
    RemainingMicroUSD int64
    Reason            string
}

type BudgetAlert struct {
    ID          string
    ScopeType   string  // "agent" / "user" / "global"
    ScopeID     string
    AlertRatio  float64
    Enabled     bool
    LastFiredAt string
    CreatedAt   string
    UpdatedAt   string
}

type QuotaDashboard struct {
    ConfiguredCount    int
    TotalCapMicroUSD   int64
    TotalSpentMicroUSD int64
    MaxUtilization     float64
}

type ModelInsight struct {
    ProviderCode       string
    ModelAPIID         string
    ModelDisplayName   string
    CallCount          int
    TotalTokens        int
    TotalCostMicroUSD  int64
    AvgLatencyMS       float64
    AvgTokensPerSecond float64
    SuccessRate        float64
    Flags              []string
}

type TurnUsageInput struct {
    SessionID     string
    RunID         string
    AgentKey      string
    AgentID       string
    Provider      string
    Model         string
    Status        string
    PromptTok     int
    CompletionTok int
    Latency       time.Duration
    ErrMsg        string
    MetadataJSON  string
    TraceID       string
}
```

### 3.2 Repo 接口

```go
type AnalyticsRepo interface {
    GetModelUsageSummary(ctx context.Context, query Query) (Summary, error)
    ListModelUsageTrends(ctx context.Context, query Query) ([]TrendPoint, error)
    ListTopModelUsage(ctx context.Context, query Query) ([]BreakdownRow, error)
    ListTopAgentUsage(ctx context.Context, query Query) ([]BreakdownRow, error)
    ListModelUsageEvents(ctx context.Context, query Query) ([]TokenUsageEvent, error)
    ListModelUsageHourlyTrends(ctx context.Context, query Query) ([]TrendPoint, error)
    GetModelUsageSummaryFromDaily(ctx context.Context, query Query) (Summary, error)
    ListModelUsageDailyTrends(ctx context.Context, query Query) ([]TrendPoint, error)
    ListTopModelUsageFromDaily(ctx context.Context, query Query) ([]BreakdownRow, error)
    ListTopAgentUsageFromDaily(ctx context.Context, query Query) ([]BreakdownRow, error)
}

type WriteRepo interface {
    RecordTokenUsageEvent(ctx context.Context, event TokenUsageEvent) (TokenUsageEvent, error)
    GetActiveModelPricing(ctx context.Context, providerCode, modelAPIID string) (ModelPricingSnapshot, bool, error)
    PurgeUsageEventsOlderThan(ctx context.Context, retainDays int) (int64, error)
    RollupDailyHourly(ctx context.Context, event TokenUsageEvent) error
}

type QuotaRepo interface {
    GetQuota(ctx context.Context, scopeType, scopeID string) (Quota, error)
    SetQuota(ctx context.Context, quota Quota) (Quota, error)
    SumScopeCostInPeriod(ctx context.Context, scopeType, scopeID, periodStart, periodEnd string) (int64, error)
    ListActiveQuotas(ctx context.Context) ([]Quota, error)
    BatchSumScopeCost(ctx context.Context, quotas []Quota) (map[string]int64, error)
    ListBudgetAlerts(ctx context.Context, scopeType, scopeID string) ([]BudgetAlert, error)
    SetBudgetAlert(ctx context.Context, alert BudgetAlert) (BudgetAlert, error)
    UpdateBudgetAlertLastFired(ctx context.Context, id, firedAt string) error
}

type Repo interface {
    AnalyticsRepo
    WriteRepo
    QuotaRepo
}

// 窄接口（跨 usecase 依赖）
type TeamQuotaReader interface {
    EnabledMemberAgentIDs(ctx context.Context, teamID string) ([]string, error)
}

type SessionMetricsAccumulator interface {
    AccumulateMetricsDelta(delta SessionMetricsDelta)
}

type CompletionUsageLinker interface {
    LinkRunnerCompletionUsage(ctx context.Context, sessionID, runID, usageEventID, traceID string) error
}

type UsageEnvelopePublisher interface {
    PublishTokenUsageEnvelope(ctx context.Context, e TokenUsageEvent)
}

type AlertNotifier interface {
    NotifyBudgetAlert(ctx context.Context, alert BudgetAlert, spentMicroUSD, capMicroUSD int64, utilization float64) error
}
```

### 3.3 Usecase

```go
type Usecase struct {
    repo UsageRepo
    now  func() time.Time
    // 可选注入（通过 Setter 设置）
    teamReader   TeamQuotaReader
    sessAccum    SessionMetricsAccumulator
    completion   CompletionUsageLinker
    envelopePub  UsageEnvelopePublisher
    alertNotifier AlertNotifier
}

// 查询
func (u *Usecase) Overview(ctx context.Context, query Query) (Overview, error)
func (u *Usecase) Trends(ctx context.Context, query Query) ([]TrendPoint, error)
func (u *Usecase) TopModels(ctx context.Context, query Query) ([]BreakdownRow, error)
func (u *Usecase) TopAgents(ctx context.Context, query Query) ([]BreakdownRow, error)
func (u *Usecase) Events(ctx context.Context, query Query) ([]TokenUsageEvent, error)

// 写入
func (u *Usecase) RecordTokenUsageEvent(ctx context.Context, e TokenUsageEvent) (TokenUsageEvent, error)
func (u *Usecase) RecordTurnUsage(ctx context.Context, in TurnUsageInput) error
func (u *Usecase) RollupDailyHourly(ctx context.Context, e TokenUsageEvent) error

// 限额
func (u *Usecase) GetQuota(ctx context.Context, scopeType, scopeID string) (Quota, error)
func (u *Usecase) SetQuota(ctx context.Context, quota Quota) (Quota, error)
func (u *Usecase) CheckQuota(ctx context.Context, scopeType, scopeID string) (QuotaCheck, error)
func (u *Usecase) QuotaDashboard(ctx context.Context) (QuotaDashboard, error)
func (u *Usecase) CheckTeamMemberQuotas(ctx context.Context, teamID string) error

// 告警
func (u *Usecase) ListBudgetAlerts(ctx context.Context, scopeType, scopeID string) ([]BudgetAlert, error)
func (u *Usecase) SetBudgetAlert(ctx context.Context, alert BudgetAlert) (BudgetAlert, error)
func (u *Usecase) EvaluateBudgetAlerts(ctx context.Context, e TokenUsageEvent)

// 导出 / 清理
func (u *Usecase) ExportUsageEventsCSV(ctx context.Context, query Query) (string, error)
func (u *Usecase) PurgeEvents(ctx context.Context, retainDays int) (int64, error)

// 增强
func (u *Usecase) InefficientModels(ctx context.Context, query Query) ([]ModelInsight, error)

// Setter（由 Wire provideUsageUsecase 注入窄接口适配器）
func (u *Usecase) SetTeamReader(tr TeamQuotaReader)
func (u *Usecase) SetSessionMetricsAccumulator(sa SessionMetricsAccumulator)
func (u *Usecase) SetCompletionUsageLinker(cl CompletionUsageLinker)
func (u *Usecase) SetUsageEnvelopePublisher(pub UsageEnvelopePublisher)
func (u *Usecase) SetAlertNotifier(n AlertNotifier)
```

### 3.4 配额拦截（TurnAdmissionUsecase）

配额拦截由独立的 `TurnAdmissionUsecase`（`internal/biz/turn_admission.go`）承担，依赖 `QuotaEnforcer` 窄接口：

```go
type QuotaEnforcer interface {
    CheckQuota(ctx context.Context, scopeType, scopeID string) (UsageQuotaCheck, error)
    CheckTeamMemberQuotas(ctx context.Context, teamID string) error
}

type TurnAdmissionUsecase struct {
    quota             QuotaEnforcer
    agents            AgentHydrator
    thresholdResolver ContextThresholdResolver
}

func (u *TurnAdmissionUsecase) EnforceChatTurnQuotas(ctx context.Context, agentID, userID string) error
func (u *TurnAdmissionUsecase) EnforceTeamMemberQuotas(ctx context.Context, teamID string) error
```

Service 层通过 `ChatOrchestrator.admission()` 调用：
- `internal/service/chat_orchestrator_turn.go` → `EnforceChatTurnQuotas`（单 Agent）
- `internal/service/team_turn_hooks.go` → `EnforceChatTurnQuotas`（Team 整轮）
- `internal/service/chat_orchestrator_turn_metrics.go` → `checkTeamMemberQuotas` 包装 `EnforceTeamMemberQuotas`

### 3.5 配额检查契约（Reason 机器码 + 周期惰性滚动）

**Reason 机器码**（`internal/biz/usage/usage.go` `QuotaCheckReason*`，API 稳定契约，调用方负责本地化，禁止直出给用户）：

| 码 | 含义 | Allowed |
|----|------|---------|
| `no_quota` | 未配置配额记录 | true |
| `quota_disabled` | 已配置但 `monthly_micro_usd <= 0`（不限制） | true |
| `quota_exceeded` | 当月消耗已达上限 | false |
| `within_quota` | 当月消耗在配额内 | true |

- 拦截文案：超限时由 `QuotaExceededMessage(check)` 生成中文用户文案（含 USD 金额），`TurnAdmissionUsecase.enforceScope` 与 `Usecase.enforceQuota` 统一使用；Team 批量校验（`CheckTeamMemberQuotas`）单独生成含成员 Agent ID 的中文文案。
- 前端映射：`web/src/features/usage/useAgentUsageQuota.ts` `QUOTA_REASON_TEXT`；未配置/未启用时不展示 已消耗/剩余/月上限 数值区（后端不统计，数值恒 0 无意义）。

**周期惰性滚动（每月自动重置）**：`period_start`/`period_end` 为静态存储值，`normalizeQuotaPeriod` 在读取配额后检测 `period_end < today(UTC)`（空/非法视为过期），自动滚动为当自然月（UTC，与 `date_key` 同口径）并 best-effort 回写（回写失败仅 Warn，用内存周期继续校验，保证 enforcement 不静默失效）。三条路径共用：

| 路径 | 位置 |
|------|------|
| 单 scope 校验 | `Usecase.CheckQuota` |
| Team 成员批量校验 | `Usecase.CheckTeamMemberQuotas` |
| 预算告警评估 | `Usecase.evaluateBudgetAlertsForScope` |

- 滚动仅对有效配额（`monthly_micro_usd > 0`）触发；disabled 配额不写库。
- 前端 `loadQuota` 先 `check` 后 `get`，确保表单展示滚动后的最新周期。

---

## 四、Data 层

### 4.1 数据表

#### `model_token_usage_events`（DDL 管理）

明细流水表，每次模型调用产生一条不可变记录。通过 DDL 迁移管理（`internal/data/sql/migrations/20260717_usage_events_schema.sql` + `20260612_pricing_rule_patches.sql`）。

| 字段分组 | 字段 |
|----------|------|
| 时间维度 | id / occurred_at / date_key / hour_key |
| 归属维度 | workspace_id / user_id / team_id / agent_id / agent_key / session_id / message_id / request_id |
| 模型维度 | provider_code / canonical_provider_code / provider_type / provider_display_name / model_api_id / model_display_name / model_category_json |
| 调用类型 | usage_kind / call_count |
| Token 明细 | input_tokens / output_tokens / cached_input_tokens / cache_write_tokens / reasoning_tokens / embedding_tokens / total_tokens |
| 价格快照 | input_price_micro_usd_per_1k / output_price_micro_usd_per_1k / cached_input_price_micro_usd_per_1k / cache_write_price_micro_usd_per_1k / reasoning_price_micro_usd_per_1k / embedding_price_micro_usd_per_1k |
| 费用结果 | input_cost_micro_usd / output_cost_micro_usd / cached_input_cost_micro_usd / cache_write_cost_micro_usd / reasoning_cost_micro_usd / embedding_cost_micro_usd / total_cost_micro_usd |
| 性能与状态 | latency_ms / time_to_first_token_ms / tokens_per_second / status / error_code / error_message / retry_count |
| 请求上下文 | prompt_mode / max_output_tokens / context_window_k / stream_enabled |
| 扩展 | metadata_json / created_at |

索引：

```sql
idx_model_token_usage_events_date_key       ON (date_key)
idx_model_token_usage_events_session_id     ON (session_id)
idx_model_token_usage_events_agent_id       ON (agent_id)
idx_model_token_usage_events_provider_code  ON (provider_code)
```

#### `model_token_usage_daily`（DDL 管理）

日聚合表，用于趋势图和占比分析。写入明细时自动 upsert。通过 DDL 迁移管理。

| 字段分组 | 字段 |
|----------|------|
| 维度 | id / date_key / workspace_id / agent_id / agent_key / provider_code / model_api_id / usage_kind |
| 计数 | call_count / request_count / success_count / failed_count / cancelled_count |
| Token | input_tokens / output_tokens / cached_input_tokens / reasoning_tokens / embedding_tokens / total_tokens |
| 费用 | total_cost_micro_usd |
| 性能 | avg_latency_ms / avg_tokens_per_second |
| 时间 | created_at / updated_at |

UNIQUE 约束：`(date_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind)`

#### `model_token_usage_hourly`（Ent Schema）

Ent Schema：`internal/data/ent/schema/model_token_usage_hourly.go`

小时聚合表，结构同 `model_token_usage_daily`，按 `hour_key` 分组。写入明细时自动 upsert。

UNIQUE 约束：`(hour_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind)`

#### `model_pricing_rules`（Ent Schema）

Ent Schema：`internal/data/ent/schema/model_pricing_rule.go`

模型价格规则表，维护当前和历史价格。

| 字段 | 说明 |
|------|------|
| provider_code / model_api_id | 模型标识 |
| currency | 货币（默认 USD） |
| input/output/cached_input/cache_write/reasoning/embedding_price_micro_usd_per_1k | 各类 Token 单价（micro USD / 1K） |
| input/output/cached_input(cache_read)/cache_write/reasoning/embedding_price_usd_per_1m | 各类 Token 单价（USD / 1M，浮点） |
| effective_from / effective_to | 生效时间范围 |
| is_active | 是否当前生效 |
| source | 来源（manual / auto_sync） |
| metadata_json | 扩展元数据 |

UNIQUE 约束：`(provider_code, model_api_id, effective_from)`

#### `usage_quotas`（Ent Schema）

Ent Schema：`internal/data/ent/schema/usage_quota.go`

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

#### `budget_alerts`（Ent Schema）

Ent Schema：`internal/data/ent/schema/budget_alert.go`

```sql
CREATE TABLE IF NOT EXISTS budget_alerts (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  alert_ratio REAL NOT NULL DEFAULT 0.8,
  enabled INTEGER NOT NULL DEFAULT 1,
  last_fired_at TEXT NOT NULL DEFAULT '',
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
4. UPSERT `model_token_usage_hourly`（同上，按 hour_key）

### 4.3 查询构建

`usageWhere(query, billableOnly)` 根据 `Query` 动态构建 WHERE 子句：

| 参数 | 条件 |
|------|------|
| `start_date` / `end_date` | `date_key >= ? AND date_key <= ?` |
| `provider_code` | `provider_code = ?` |
| `model_api_id` | `model_api_id = ?` |
| `agent_id` | `agent_id = ?` |
| `team_id` | `team_id = ?` |
| `usage_kind` | `usage_kind = ?`（仅明细/导出；与 `billableOnly` 独立） |
| `status` | `status = ?`；`abnormal` → `status <> 'success'` |

`usageLimit(limit)` 限制范围 [1, 200]，默认 10。

### 4.4 显示名解析（sqlUsageEventNames）

`ListModelUsageEvents` 在 SELECT 末尾追加 `sqlUsageEventNames`（`internal/data/usage.go`），用**标量子查询**（而非 JOIN）解析三个显示名列，避免 WHERE 列歧义与 COUNT 查询膨胀：

| 列 | 解析规则 |
|----|----------|
| `agent_name` | `agents.display_name`（`agent_id` 同时匹配 `agents.id` / `agents.agent_key`）；未命中时回退到该事件 `session_id` 关联 Agent 的 display_name（同 monitor traces 的 `sqlMonitorTracesNames` 模式）；最终 `COALESCE ''` |
| `session_title` | `sessions.title`（按 `session_id` 匹配，排除 `deleted_at`） |
| `team_name` | `teams.display_name`（`team_id` 同时匹配 `teams.id` / `teams.team_key`） |

写入侧保证 `agent_id` 列存 Agent 行 ID（`chat_orchestrator_turn.go` 传 `ag.ID`；Team `usage_record.go` 传 `ag.ID`），故前端筛选下拉提交行 ID 即可精确匹配；显示名解析额外兼容历史/边界数据中存 `*_key` 的行。实体已删除时显示名为空串，前端回退显示原始 ID。

### 4.5 费用计算

所有费用以 micro USD 存储。实现：`internal/biz/usage/usage.go` → `ApplyTokenUsageCosts`（幂等：仅当对应 cost 字段为 0 时计算，不覆盖已写入值）：

```
cached_billable = clamp(cached_input_tokens, 0, input_tokens)   // 防 cached 超过 input
input_cost         = (input_tokens − cached_billable) × 输入单价   // 缓存命中部分不再按全价输入计费
cached_input_cost  = cached_billable × 缓存读取单价
output_cost        = output_tokens × 输出单价
total_cost         = input_cost + output_cost + cached_input_cost + cache_write_cost + reasoning_cost + embedding_cost
```

单价取值优先级：`xxx_price_usd_per_1m`（cost = `round(tokens × usd_per_1m)`）> `xxx_price_micro_usd_per_1k`（cost = `tokens × micro_per_1k / 1000`，整除）。事件表仅持久化 micro 单价快照；USD 单价来自定价快照（`model_pricing_rules` 优先）。

### 4.6 统计口径矩阵（可计费 vs 对账）

常量：`internal/data/usage_sql.go` → `sqlUsageBillableKind`

```sql
(usage_kind IS NULL OR usage_kind = '' OR usage_kind <> 'team_turn')
```

| API / 读路径 | `billableOnly` | 说明 |
|--------------|----------------|------|
| `GetModelUsageSummary` / Overview 各段 | `true` | 今日/昨日/本月/range |
| `ListModelUsageTrends` | `true` | 日趋势；`granularity=hour` 时 `model_token_usage_hourly` 同样带 billable 子句 |
| `ListTopModelUsage` / `ListTopAgentUsage` | `true` | 排行 |
| `SumScopeCostInPeriod`（配额已用额） | 固定 billable | `usage_quota.go` |
| `ListModelUsageEvents` / `ExportUsageEvents` | `false` | 明细含 `team_turn`；可用 `usage_kind` / `team_id` 精确筛选 |

**写入 `usage_kind`（`internal/biz/usage/usage.go` 常量）**

| 值 | 写入路径 | 聚合 |
|----|----------|------|
| `chat_turn` | `recordTurnUsage` | 可计费 |
| `team_member` | Team `persistStep` → `recordMemberUsage` | 可计费；Team 总额 = `SUM(...) WHERE team_id=? AND usage_kind='team_member'` |
| `team_turn` | `recordTeamRunUsage` | **排除**默认可计费聚合；明细可见 |

**Team 并行**：`EventStreamResult.MemberUsage` 按 `agent_key` 汇总 → `stepTokensForMember`；无成员级 usage 时 anchor（sortIdx=0）回退整轮 tokens。

---

## 五、Service 层

### 5.1 UsageService

文件：`internal/service/usage.go`

```go
type UsageService struct {
    v1.UnimplementedUsageServiceServer
    uc *biz.UsageUsecase
}

func NewUsageService(uc *biz.UsageUsecase) *UsageService

// 查询
func (s *UsageService) GetUsageOverview(ctx, *v1.UsageQuery) (*v1.UsageOverview, error)
func (s *UsageService) ListUsageTrends(ctx, *v1.UsageQuery) (*v1.ListUsageTrendsResponse, error)
func (s *UsageService) ListTopModels(ctx, *v1.UsageQuery) (*v1.ListBreakdownResponse, error)
func (s *UsageService) ListTopAgents(ctx, *v1.UsageQuery) (*v1.ListBreakdownResponse, error)
func (s *UsageService) ListUsageEvents(ctx, *v1.UsageQuery) (*v1.ListUsageEventsResponse, error)

// 写入
func (s *UsageService) RecordTokenUsageEvent(ctx, *v1.TokenUsageEvent) (*v1.TokenUsageEvent, error)

// 限额
func (s *UsageService) GetUsageQuota(ctx, *v1.GetUsageQuotaRequest) (*v1.UsageQuota, error)
func (s *UsageService) SetUsageQuota(ctx, *v1.SetUsageQuotaRequest) (*v1.UsageQuota, error)
func (s *UsageService) CheckUsageQuota(ctx, *v1.CheckUsageQuotaRequest) (*v1.CheckUsageQuotaResponse, error)

// 告警
func (s *UsageService) ListBudgetAlerts(ctx, *v1.ListBudgetAlertsRequest) (*v1.ListBudgetAlertsResponse, error)
func (s *UsageService) SetBudgetAlert(ctx, *v1.SetBudgetAlertRequest) (*v1.BudgetAlert, error)

// 导出 / 清理
func (s *UsageService) ExportUsageEvents(ctx, *v1.UsageQuery) (*v1.ExportUsageEventsResponse, error)
func (s *UsageService) PurgeUsageEvents(ctx, *v1.PurgeUsageEventsRequest) (*v1.PurgeUsageEventsResponse, error)
```

辅助文件：
- `internal/service/usage_mapper.go` — Proto ↔ Biz 类型映射
- `internal/service/usage_alert_notifier.go` — `AlertNotifier` 实现（写入监控事件 `usage.budget_alert`）

**访问控制**：限额 / 告警 / 清理端点（`GetUsageQuota` / `SetUsageQuota` / `CheckUsageQuota` / `ListBudgetAlerts` / `SetBudgetAlert` / `PurgeUsageEvents`）通过 `assertSystemCaller` 校验调用者身份，允许以下两类调用者：
- **系统工作区**（`workspace.IsSystem`）— 用于 cron / 后台任务
- **管理员**（`auth.FromContext().HasAdminAccess()`）— 用于前端管理后台

### 5.2 用量记录入口

**主路径**：`internal/service/turn_usage.go` → `recordTurnUsage`（`trpc_turn` defer）

- 字段完整：`agent_id` / `provider_code` / `model_api_id` / `usage_kind=chat_turn` / Trace `metadata_json`
- 经 `UsageUsecase.RecordTokenUsageEvent`：`NormalizeUsageStatus` + `enrichTokenUsagePricing` + 事务落库

**可选路径**：

| 入口 | 开关 | 说明 |
|------|------|------|
| `event_bus_runner_handler` | `CHAT_RECORD_RUNNER_USAGE=1` | Runner completion；需带 `id`（uuid） |
| `recordChatIngressUsage` | `CHAT_RECORD_USAGE_INGRESS=1` | 遗留；**不再**由 `SendChatMessage` 自动调用，避免与主路径双写 |

**配额拦截**：由 `TurnAdmissionUsecase`（`internal/biz/turn_admission.go`）承担，Service 层通过 `ChatOrchestrator.admission()` 调用：
- 单 Agent：`chat_orchestrator_turn.go` → `EnforceChatTurnQuotas`
- Team 整轮：`team_turn_hooks.go` → `EnforceChatTurnQuotas`
- Team 成员：`chat_orchestrator_turn_metrics.go` → `checkTeamMemberQuotas` → `EnforceTeamMemberQuotas`

**Team 明细**：

| kind | 路径 | 说明 |
|------|------|------|
| `team_member` | `ConsumeEventStream` → `MemberUsage[agent_key]` → `stepTokensForMember` → `persistStep` → `recordMemberUsage` | parallel/swarm 子 Agent 在事件流带 `Usage` 时按成员落库 |
| `team_turn` | `recordTeamRunUsage` | 整轮 `promptTok`/`completionTok` 聚合一行（anchor Agent） |

落库失败 → `CtxFlowLogWarn`（`team.usage_record_fail`，**禁 slog**）。成员流未带 `Usage` 时仅 anchor 成员（sortIdx=0）继承整轮 tokens。

**定价回退**：`GetActiveModelPricing` → `model_pricing_rules`，否则 `llm_provider_models.config_json`。

**缓存命中捕获**：DeepSeek 系响应的 `prompt_cache_hit_tokens` 不被 OpenAI SDK 解析。`internal/provider/usage_tap_transport.go`（装配点 `trpc_llm.go`）在 HTTP 传输层把响应体改写为标准 `prompt_tokens_details.cached_tokens`（SSE / JSON 均支持），随后经 `turn_helpers.go` 累计进 `CachedInputTokens` 落库，费用按 §4.4 缓存公式计算。

**team_member 事件 provider/model**：`PersistGraphRunStep` 以 anchor Agent 的 `Provider`/`Model` 兜底（`strutil.FirstNonEmpty`），避免空 `model_api_id` 导致定价回退失败、费用为 0。

**告警**：`EvaluateBudgetAlerts` → 监控事件 `usage.budget_alert`（60min 冷却）。

### 5.3 写入路径（真相源）

```
Chat Turn 结束 → service/turn_usage.recordTurnUsage
              → biz.RecordTokenUsageEvent（归一 status + 定价 + 费用）
              → data/usage_write（events + sessions 聚合 + daily/hourly upsert）

Team RunTurn 结束 → agent.ConsumeEventStream（MemberUsage 按 agent_key）
                 → team.persistStep → recordMemberUsage（usage_kind=team_member）
                 → team.recordTeamRunUsage（usage_kind=team_turn，整轮聚合）

（可选）CHAT_RECORD_RUNNER_USAGE=1 → event_bus_runner_handler
（已停用默认）CHAT_RECORD_USAGE_INGRESS=1 → recordChatIngressUsage
```

---

## 六、Wire 注入

文件：`cmd/admin/wire.go`

```
data.ProviderSet  → NewUsageRepo（含 AnalyticsRepo / WriteRepo / QuotaRepo）
biz.ProviderSet   → provideUsageUsecase
service.ProviderSet → NewUsageService

// Wire Bindings
wire.Bind(new(biz.UsageQuotaRepo), new(biz.UsageRepo))
wire.Bind(new(bizusage.AnalyticsRepo), new(biz.UsageRepo))
wire.Bind(new(biz.TeamUsageQuerier), new(*biz.UsageUsecase))

// provideUsageUsecase 注入窄接口适配器
func provideUsageUsecase(repo biz.UsageRepo, mon *biz.MonitorUsecase, teamUC *biz.TeamUsecase,
    sessions *biz.SessionUsecase, bus contract.Bus, lg loggateway.Logger) *biz.UsageUsecase
```

窄接口适配器（`cmd/admin/wire.go`）：
- `sessionMetricsAdapter` — 适配 `SessionMetricsAccumulator`
- `completionLinkerAdapter` — 适配 `CompletionUsageLinker`
- `envelopePublisherAdapter` — 适配 `UsageEnvelopePublisher`
- `service.NewMonitorBudgetAlertNotifier(mon)` — `AlertNotifier` 实现

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/usage/
├── api.ts                    ← API 调用 + snake_case ↔ camelCase 转换
├── quotaApi.ts               ← 限额 + 告警 API（含 listBudgetAlerts / setBudgetAlert）
├── useAgentUsageQuota.ts     ← 限额 + 告警 composable
├── useOverviewPage.ts        ← 概览页 composable
├── useUsageEventsPage.ts     ← 明细页 composable（筛选下拉选项：Provider/模型来自 platformStore.providerModels，Agent/Team 来自 agentsCatalog/teamsStore；Provider↔模型联动）
├── useUsageChart.ts          ← 趋势图 composable
├── useProviderTrendDialog.ts  ← Provider 趋势弹窗
├── usageTrendMetrics.ts      ← 趋势指标定义
├── usageTableUi.ts           ← 明细表格 UI 配置
├── usageEcharts.ts           ← ECharts 配置
├── usageBreakdownSlices.ts   ← 占比切片定义
├── pricingWarning.ts         ← 定价缺失警告
├── moneyFormat.ts            ← 金额格式化
└── types.ts                  ← TypeScript 类型定义

web/src/components/usage/
├── UsageMetricCards.vue          ← 核心指标卡片（含月预算使用率）
├── UsageTrendChart.vue           ← ECharts 趋势（useUsageChart + usageTrendMetrics）
├── UsageBreakdownCharts.vue      ← 模型/Provider 占比饼图
├── UsageTopModels.vue            ← Top 模型排行
├── UsageTopAgents.vue            ← Top Agent 排行
├── UsageAnomalyList.vue          ← 异常请求列表
├── UsageInefficientModels.vue    ← 低性价比模型识别
├── UsageProviderCostPie.vue      ← Provider 费用占比
├── UsageModelCostPie.vue         ← 模型费用占比
├── UsageTokenComposition.vue     ← Token 组成
├── UsageFallbackEvents.vue       ← 降级事件列表
├── TokenTrendDialog.vue          ← Token 趋势弹窗
├── OverviewRunnerMetrics.vue     ← 概览页 Runner 条
├── OverviewProviderHealth.vue    ← 概览页 Provider 健康
├── OverviewPageHero.vue          ← 概览页 Hero
├── OverviewMonitorQuickLinks.vue ← 概览页监控快捷链接
├── CommandCenterHero.vue         ← 命令中心 Hero
├── CommandCenterStatusPanels.vue ← 命令中心状态面板
├── CommandCenterQuickActions.vue ← 命令中心快捷操作
├── StatusPanelAgent.vue          ← Agent 状态面板
├── StatusPanelSession.vue        ← Session 状态面板
├── StatusPanelRunner.vue         ← Runner 状态面板
└── StatusPanelProvider.vue       ← Provider 状态面板

web/src/components/agents/
└── AgentUsageQuotaPanel.vue      ← Agent token配额 Tab 限额 + 告警配置

web/src/pages/
├── OverviewPage.vue              ← 概览页（useOverviewPage）
└── UsageEventsPage.vue           ← 用量明细页（useUsageEventsPage）
```

### 7.2 API 函数

```typescript
// features/usage/api.ts
export async function getModelUsageOverview(query?: ModelUsageQuery): Promise<ModelUsageOverview>
export async function listModelUsageTrends(query?: ModelUsageQuery): Promise<ModelUsageTrendPoint[]>
export async function listModelUsageEvents(query?: ModelUsageQuery): Promise<ModelTokenUsageEvent[]>
export async function exportUsageEventsCsv(query?: ModelUsageQuery): Promise<void>
export async function recordModelTokenUsageEvent(e: ModelTokenUsageEvent): Promise<ModelTokenUsageEvent>
export async function purgeUsageEvents(before: string): Promise<void>

// features/usage/quotaApi.ts
export async function getUsageQuota(scopeType: string, scopeId: string): Promise<UsageQuota>
export async function setUsageQuota(scopeType: string, scopeId: string, quota: UsageQuota): Promise<UsageQuota>
export async function checkUsageQuota(scopeType: string, scopeId: string): Promise<QuotaCheck>
export async function listBudgetAlerts(scopeType: string, scopeId: string): Promise<BudgetAlert[]>
export async function setBudgetAlert(alert: BudgetAlert): Promise<BudgetAlert>
export function microUsdToUsd(micro: number): number
```

### 7.3 类型定义

| 类型 | 说明 |
|------|------|
| `ModelUsageQuery` | 查询参数 |
| `ModelUsageSummary` | 汇总数据 |
| `ModelUsageTrendPoint` | 趋势点 |
| `ModelUsageBreakdownRow` | 占比行 |
| `ModelTokenUsageEvent` | 明细事件 |
| `ModelUsageOverview` | 概览（today/yesterday/month/range/trends/top_models/top_agents/anomalies/quota_dashboard/inefficient_models） |
| `UsageQuota` | 限额配置 |
| `BudgetAlert` | 告警配置 |
| `QuotaCheck` | 配额检查结果 |
| `ModelInsight` | 低性价比模型洞察 |

### 7.4 数据流

- 页面组件通过 composable（`useOverviewPage` / `useUsageEventsPage`）访问 API，禁止直接 `import features/usage/api`
- `AgentUsageQuotaPanel` 通过 `useAgentUsageQuota` composable 访问 `quotaApi`，禁止直接调 API
- 告警 API 合并进 `quotaApi.ts`，不再单独维护 `budgetAlertApi.ts`

### 7.5 明细页可读性设计（2026-07-30）

- **筛选下拉**：Provider / 模型 / Agent / Team 均为 `q-select`（显示名称、提交 code/ID）。Provider 与模型选项派生自 `platformStore.providerModels`（PlatformResource 的 `provider` / `model` / `name`），模型选项随已选 Provider 联动；Agent / Team 选项来自 `agentsCatalog.fetchAgents` / `teamsStore.loadTeams`（label 用 `display_name`，value 用行 ID）
- **名称列**：Agent 列主行显示 `agent_name`（回退 `agent_key` → `agent_id`），有名称时次要行显示原始 ID；Session 列显示 `session_title`（无标题截断 ID），完整 ID 经 `AppRegistryHoverTip` 悬停查看；状态列用 `AppStatusChip`
- **状态标签公共组件**：`components/common/AppStatusChip.vue` + `features/ui/appStatusMeta.ts`——状态枚举（success/completed/error/failed/timeout/cancelled/running/pending/queued/idle/interrupted）→ tone + 图标 + i18n key（`common.status.*`，zh-CN/en-US 双语）集中定义，各页面共用；筛选下拉的 statusOptions 复用同一元数据
- **工具栏紧凑排布**：7 个筛选字段收窄（`min-width:108px / max-width:160px`）+ 操作按钮 `flat dense`，`flex-wrap: wrap` 换行替代横向滚动

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
