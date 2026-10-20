package usage

import (
	"context"
)

const (
	KindChatTurn   = "chat_turn"
	KindTeamMember = "team_member"
	KindTeamTurn   = "team_turn"
	// Aux kinds: auxiliary (non-chat-turn) LLM calls that previously went
	// unrecorded (P1-1, 2026-08-19). All are billable consumption and must
	// participate in aggregates/quota (the billable filter only excludes
	// team_turn, so aux kinds are included by default).
	KindAuxSubagent  = "aux_subagent"  // subagent tool runs (runner.Run child sessions)
	KindAuxTitle     = "aux_title"     // session-title generation
	KindAuxIntent    = "aux_intent"    // intent pass classification
	KindAuxEvolution = "aux_evolution" // evolution draft generation
	// KindAuxMemoryExtract marks sleep-time memory consolidation extraction
	// calls (MemoryLLMExtractor). Previously unrecorded — every after-turn
	// consolidation burned tokens invisibly (P2-D, 2026-08-20).
	KindAuxMemoryExtract = "aux_memory_extract"
	// KindAuxEmbedding marks embedding API calls (knowledge ingest / retrieval
	// query vectorization / memory fact embedding). Tokens live in the
	// EmbeddingTokens column and are priced via EmbeddingPrice (P1-3,
	// 2026-08-19).
	KindAuxEmbedding = "aux_embedding"
	// KindAuxEvalJudge marks evaluation LLM-as-judge / faithfulness /
	// context_precision calls (previously unrecorded).
	KindAuxEvalJudge = "aux_eval_judge"
	// KindAuxPlannerDecompose marks spirit planner decompose LLM calls
	// (decomposeTask / decomposeTaskStream). Previously unrecorded — every
	// team-task decomposition burned tokens invisibly (M83 LBG-6).
	KindAuxPlannerDecompose = "aux_planner_decompose"
	// KindAuxAllocatorMatch marks spirit allocator LLM cold-start matching
	// calls (per-subtask / whole-plan). Previously unrecorded (M83 LBG-6).
	KindAuxAllocatorMatch = "aux_allocator_match"
	// KindUnknown is the fallback for persisted events whose caller did not
	// classify the usage. Previously defaulted to the non-enum literal "chat"
	// (valid enum is chat_turn), which the UI source filter could not map —
	// mislabeling unknown consumption as chat traffic (P0-3, 2026-10-20).
	KindUnknown = "unknown"
)

// Platform-wide quota scope identifiers.
const (
	QuotaScopeGlobal   = "global"
	GlobalQuotaScopeID = "global"
)

// floatCompareEpsilon is used for floating-point comparisons to avoid
// false negatives due to rounding.
const floatCompareEpsilon = 1e-9

// Usage query and operational constants.
const (
	defaultQueryRangeDays  = 30 // default date range for usage queries
	anomalyQueryLimit      = 12 // max anomaly events per overview query
	usageRecordTimeoutSec  = 45 // timeout for recording usage events
	usageLinkTimeoutSec    = 10 // timeout for linking runner completion usage
	csvExportMaxRows       = 5000
	inefficientModelLimit  = 32 // max inefficient models to analyze
	inefficientResultLimit = 8  // max inefficient model insights to return
	usageCostMicroDivisor  = 1000
)

// Query mirrors legacy model-usage GET params.
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
	Offset       int
	Granularity  string
	// SessionID filters events to one session (79-runtime-governance 1.5:
	// per-session cache-hit ratio drill-down; consumed by EventsPage/Events).
	SessionID string
	// Effort filters events to rows whose metadata_json["effort"] equals this
	// value (M83 LBG-6; off/low/medium/high/max). Empty = no effort filter.
	Effort string
	// WorkspaceID filters events by workspace. Empty = no filter (system caller
	// sees all workspaces); non-empty = restrict to that workspace only.
	// Service layer injects this from ctx; clients cannot forge it.
	WorkspaceID string
}

// EventsResult is a page of usage events plus the filter-scoped total.
type EventsResult struct {
	Items []TokenUsageEvent
	Total int
}

// Summary aggregates usage statistics.
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

// TrendPoint is one data point on a usage trend chart.
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

// BreakdownRow is one row in a usage breakdown table.
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
	// LastActiveDate is YYYY-MM-DD of the most recent daily usage bucket (optional).
	LastActiveDate string
}

// BreakdownQuery 是 ListAllModelsBreakdown 的查询参数（分页 + 搜索 + 排序）。
// 与 Query（用于 trend/top 等）不同，BreakdownQuery 专为「全模型消耗总览表」设计，
// 因为它需要服务端分页/排序，而 Query 仅支持 limit。
type BreakdownQuery struct {
	Range        string // today/7d/30d/month（输入）
	StartDate    string // 由 Usecase.normalizeBreakdownQuery 解析后填充（data 层使用）
	EndDate      string // 由 Usecase.normalizeBreakdownQuery 解析后填充（data 层使用）
	ProviderCode string // 可选 provider 过滤
	Search       string // 可选 LIKE 搜索（匹配 provider_code 或 model_api_id）
	SortField    string // call_count/total_tokens/total_cost_micro_usd/success_rate/avg_latency_ms
	SortDir      string // asc/desc
	Page         int32  // 1-based
	PageSize     int32  // 默认 20，最大 100
	// WorkspaceID filters breakdown by workspace. Same semantics as Query.WorkspaceID.
	WorkspaceID string
}

// BreakdownResult 是 ListAllModelsBreakdown 的分页结果。
type BreakdownResult struct {
	Items    []BreakdownRow
	Total    int32 // 匹配过滤条件的总行数（用于前端分页 UI）
	Page     int32
	PageSize int32
}

// TokenUsageEvent mirrors SQLite row from model_token_usage_events.
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
	WaitMS                        int
	ModelLatencyMS                int
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
	// Resolved display names (list queries only; empty when the referenced
	// entity was deleted). AgentName falls back to the session's agent.
	AgentName    string
	SessionTitle string
	TeamName     string
}

// Overview aggregates all usage dashboard data.
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

// ModelPricingSnapshot is the active price row for a provider/model pair.
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

// Quota is a monthly spend cap for a scope.
type Quota struct {
	ID              string
	ScopeType       string
	ScopeID         string
	MonthlyMicroUSD int64
	PeriodStart     string
	PeriodEnd       string
	CreatedAt       string
	UpdatedAt       string
}

// QuotaCheck returns whether another chat turn is allowed under the configured cap.
type QuotaCheck struct {
	Quota             Quota
	Allowed           bool
	SpentMicroUSD     int64
	RemainingMicroUSD int64
	// Reason is a stable machine-readable code (QuotaCheckReason*); callers
	// must localize before showing to users.
	Reason string
}

// BudgetAlert is a spend-ratio threshold for a scope.
type BudgetAlert struct {
	ID          string
	ScopeType   string
	ScopeID     string
	AlertRatio  float64
	Enabled     bool
	LastFiredAt string
	CreatedAt   string
	UpdatedAt   string
}

// AlertNotifier delivers budget threshold notifications.
type AlertNotifier interface {
	NotifyBudgetAlert(ctx context.Context, alert BudgetAlert, spentMicroUSD, capMicroUSD int64, utilization float64) error
}

// ModelInsight flags models with high cost and poor efficiency signals.
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

// AnalyticsRepo reads aggregates and event lists.
type AnalyticsRepo interface {
	GetModelUsageSummary(ctx context.Context, query Query) (Summary, error)
	ListModelUsageTrends(ctx context.Context, query Query) ([]TrendPoint, error)
	ListTopModelUsage(ctx context.Context, query Query) ([]BreakdownRow, error)
	ListTopAgentUsage(ctx context.Context, query Query) ([]BreakdownRow, error)
	ListModelUsageEvents(ctx context.Context, query Query) ([]TokenUsageEvent, error)
	CountModelUsageEvents(ctx context.Context, query Query) (int, error)
	ListModelUsageHourlyTrends(ctx context.Context, query Query) ([]TrendPoint, error)
	GetModelUsageSummaryFromDaily(ctx context.Context, query Query) (Summary, error)
	ListModelUsageDailyTrends(ctx context.Context, query Query) ([]TrendPoint, error)
	ListTopModelUsageFromDaily(ctx context.Context, query Query) ([]BreakdownRow, error)
	ListTopAgentUsageFromDaily(ctx context.Context, query Query) ([]BreakdownRow, error)
	// ListAllModelsBreakdown 返回全模型消耗分页明细（用于「全模型消耗总览表」）。
	// 与 ListTopModelUsageFromDaily 不同：支持服务端分页/搜索/动态排序，且返回 total。
	// TECH-DEBT(CS-B4): AnalyticsRepo 方法数=11，超出 ≤5 上限（DB-DEBT-02）。
	// 后续应拆分为 ModelTrendReader / ModelBreakdownReader / AgentBreakdownReader 子接口。
	ListAllModelsBreakdown(ctx context.Context, query BreakdownQuery) (BreakdownResult, error)
	// ListAllModelsBreakdownRealtime 与 ListAllModelsBreakdown 同签名，但基于 events
	// 表实时聚合。用于 range 含今天时与 Overview 摘要/Top 榜保持同源（daily rollup
	// 为异步消费，含今天必然滞后）。
	ListAllModelsBreakdownRealtime(ctx context.Context, query BreakdownQuery) (BreakdownResult, error)
}

// WriteRepo persists usage events and resolves pricing.
type WriteRepo interface {
	RecordTokenUsageEvent(ctx context.Context, event TokenUsageEvent) (TokenUsageEvent, error)
	GetActiveModelPricing(ctx context.Context, providerCode, modelAPIID string) (ModelPricingSnapshot, bool, error)
	PurgeUsageEventsOlderThan(ctx context.Context, retainDays int) (int64, error)
	RollupDailyHourly(ctx context.Context, event TokenUsageEvent) error
}

// QuotaRepo manages caps, spend sums, and budget alerts.
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

// Repo is the composed persistence contract for Usecase.
type Repo interface {
	AnalyticsRepo
	WriteRepo
	QuotaRepo
}

// TeamQuotaReader provides the minimal team information needed for quota checks.
type TeamQuotaReader interface {
	// EnabledMemberAgentIDs returns the agent IDs of all enabled team members.
	// Returns nil (not error) when the team has no enabled members.
	EnabledMemberAgentIDs(ctx context.Context, teamID string) ([]string, error)
}

// SessionMetricsAccumulator accumulates session-level metrics deltas after a usage event.
type SessionMetricsAccumulator interface {
	AccumulateMetricsDelta(delta SessionMetricsDelta)
}

// SessionMetricsDelta mirrors session.SessionMetricsDelta for the usage package
// to avoid importing the session package directly.
type SessionMetricsDelta struct {
	SessionID         string
	MessageCount      int
	ModelCallCount    int
	ToolCallCount     int
	SkillCallCount    int
	McpCallCount      int
	InputTokens       int64
	OutputTokens      int64
	TotalTokens       int64
	TotalCostMicroUsd int64
}

// CompletionUsageLinker patches runner completion rows with usage_event_id.
type CompletionUsageLinker interface {
	LinkRunnerCompletionUsage(ctx context.Context, sessionID, runID, usageEventID, traceID string) error
}

// UsageEnvelopePublisher publishes token usage events to the event bus.
type UsageEnvelopePublisher interface {
	PublishTokenUsageEnvelope(ctx context.Context, e TokenUsageEvent)
}
