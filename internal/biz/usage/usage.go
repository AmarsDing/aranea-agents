// Package usage implements token usage tracking, quota management, budget alerts, and cost analysis.
package usage

import (
	"context"
	stderrors "errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
)

// UsecaseRef is a late-binding cell for *Usecase (P1-2, 2026-08-19).
// Aux-usage recorders (session-title generator, subagent service, evolution
// review model) sit UPSTREAM of Usecase in the wire DI graph, so constructor
// injection of *Usecase creates dependency cycles. The ref has zero
// dependencies: wire builds it first, provideUsageUsecase populates it, and
// recorders resolve the usecase lazily at record time (always post-startup).
type UsecaseRef struct {
	p atomic.Pointer[Usecase]
}

// NewUsecaseRef returns an empty ref (Get returns nil until Set is called).
func NewUsecaseRef() *UsecaseRef { return &UsecaseRef{} }

// Set publishes the usecase; safe for concurrent use, last-writer-wins.
func (r *UsecaseRef) Set(u *Usecase) {
	if r != nil {
		r.p.Store(u)
	}
}

// Get returns the current usecase, or nil before Set is called.
func (r *UsecaseRef) Get() *Usecase {
	if r == nil {
		return nil
	}
	return r.p.Load()
}

// ── Constants ─────────────────────────────────────────────────────────────────

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

// ── Models ────────────────────────────────────────────────────────────────────

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

// ── Repo interfaces ───────────────────────────────────────────────────────────

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

// ── Narrow interfaces for cross-usecase dependencies ──────────────────────────

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

// ── Usecase ───────────────────────────────────────────────────────────────────

// Usecase implements usage tracking and quota management workflows.
type Usecase struct {
	repo          Repo
	now           func() time.Time
	alertNotifier AlertNotifier
	alertFired    map[string]time.Time
	alertFiredMu  sync.Mutex
	teamReader    TeamQuotaReader
	sessAccum     SessionMetricsAccumulator
	completion    CompletionUsageLinker
	envelopePub   UsageEnvelopePublisher
	// noPricingLogged dedups the P2-2 no-pricing Warn per provider|model so
	// high-frequency unpriced calls (e.g. local ollama) don't flood the log.
	noPricingLogged sync.Map
	lg              loggateway.Logger
}

// NewUsecase constructs a UsageUsecase.
func NewUsecase(repo Repo, lg loggateway.Logger) *Usecase {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &Usecase{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
		lg:   lg,
	}
}

// SetTeamReader wires the optional team dependency for member quota checks.
func (u *Usecase) SetTeamReader(tr TeamQuotaReader) {
	u.teamReader = tr
}

// SetSessionMetricsAccumulator wires the optional session metrics accumulator.
func (u *Usecase) SetSessionMetricsAccumulator(sa SessionMetricsAccumulator) {
	u.sessAccum = sa
}

// SetCompletionUsageLinker wires the optional runner completion linker.
func (u *Usecase) SetCompletionUsageLinker(cl CompletionUsageLinker) {
	u.completion = cl
}

// SetUsageEnvelopePublisher wires the optional event bus publisher.
func (u *Usecase) SetUsageEnvelopePublisher(pub UsageEnvelopePublisher) {
	u.envelopePub = pub
}

// SetAlertNotifier wires optional budget alert delivery.
func (u *Usecase) SetAlertNotifier(n AlertNotifier) {
	u.alertNotifier = n
}

func dateKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func withLimit(query Query, limit int) Query {
	query.Limit = limit
	return query
}

func (u *Usecase) normalizeQuery(query Query, now time.Time) Query {
	if query.StartDate != "" && query.EndDate != "" {
		return query
	}
	end := dateKey(now)
	start := now.AddDate(0, 0, -(defaultQueryRangeDays - 1))
	switch query.Range {
	case "today":
		start = now
	case "7d":
		start = now.AddDate(0, 0, -6)
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	case "30d", "":
		start = now.AddDate(0, 0, -(defaultQueryRangeDays - 1))
	default:
		start = now.AddDate(0, 0, -(defaultQueryRangeDays - 1))
	}
	if query.StartDate == "" {
		query.StartDate = dateKey(start)
	}
	if query.EndDate == "" {
		query.EndDate = end
	}
	return query
}

func MapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, shared.ErrUsageScopeRequired) {
		return apierror.BadRequest("USAGE", "scope_type and scope_id are required")
	}
	if stderrors.Is(err, shared.ErrBudgetAlertNotFound) {
		return apierror.NotFound("USAGE_ALERT", "budget alert not found")
	}
	return err
}

// dailySupported returns false when the query uses filters not supported by the
// daily rollup table. The daily table (model_token_usage_daily) has no team_id
// column, so TeamID-filtered queries must use the real-time events path.
func (u *Usecase) dailySupported(query Query) bool {
	return query.TeamID == ""
}

// Overview returns the full usage dashboard data.
func (u *Usecase) Overview(ctx context.Context, query Query) (Overview, error) {
	now := u.now()
	todayKey := dateKey(now)
	rangeQuery := u.normalizeQuery(query, now)
	todayQuery := query
	todayQuery.StartDate = dateKey(now)
	todayQuery.EndDate = dateKey(now)
	yesterdayQuery := query
	yesterday := now.AddDate(0, 0, -1)
	yesterdayQuery.StartDate = dateKey(yesterday)
	yesterdayQuery.EndDate = dateKey(yesterday)
	monthQuery := query
	monthQuery.StartDate = dateKey(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC))
	monthQuery.EndDate = dateKey(now)

	// Daily rollup table has no team_id column; fall back to real-time queries
	// when TeamID filter is set to ensure correct filtering.
	useDaily := u.dailySupported(query)

	var (
		today            Summary
		yesterdaySummary Summary
		month            Summary
		rangeSummary     Summary
		trends           []TrendPoint
		topModels        []BreakdownRow
		topAgents        []BreakdownRow
		anomalies        []TokenUsageEvent
		quotaDash        QuotaDashboard
		inefficient      []ModelInsight
	)
	anomalyQuery := withLimit(rangeQuery, anomalyQueryLimit)
	anomalyQuery.Status = "abnormal"

	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error { var e error; today, e = u.repo.GetModelUsageSummary(egCtx, todayQuery); return e })
	eg.Go(func() error {
		var e error
		if useDaily {
			yesterdaySummary, e = u.repo.GetModelUsageSummaryFromDaily(egCtx, yesterdayQuery)
		} else {
			yesterdaySummary, e = u.repo.GetModelUsageSummary(egCtx, yesterdayQuery)
		}
		return e
	})
	eg.Go(func() error {
		var e error
		if useDaily && monthQuery.EndDate < todayKey {
			month, e = u.repo.GetModelUsageSummaryFromDaily(egCtx, monthQuery)
		} else {
			month, e = u.repo.GetModelUsageSummary(egCtx, monthQuery)
		}
		return e
	})
	eg.Go(func() error {
		var e error
		if useDaily && rangeQuery.EndDate < todayKey {
			rangeSummary, e = u.repo.GetModelUsageSummaryFromDaily(egCtx, rangeQuery)
		} else {
			rangeSummary, e = u.repo.GetModelUsageSummary(egCtx, rangeQuery)
		}
		return e
	})
	eg.Go(func() error { var e error; trends, e = u.Trends(egCtx, rangeQuery); return e })
	eg.Go(func() error {
		var e error
		if useDaily && rangeQuery.EndDate < todayKey {
			topModels, e = u.repo.ListTopModelUsageFromDaily(egCtx, withLimit(rangeQuery, 8))
		} else {
			topModels, e = u.repo.ListTopModelUsage(egCtx, withLimit(rangeQuery, 8))
		}
		return e
	})
	eg.Go(func() error {
		var e error
		if useDaily && rangeQuery.EndDate < todayKey {
			topAgents, e = u.repo.ListTopAgentUsageFromDaily(egCtx, withLimit(rangeQuery, 8))
		} else {
			topAgents, e = u.repo.ListTopAgentUsage(egCtx, withLimit(rangeQuery, 8))
		}
		return e
	})
	eg.Go(func() error { var e error; anomalies, e = u.repo.ListModelUsageEvents(egCtx, anomalyQuery); return e })

	var quotaErr, ineffErr error
	eg.Go(func() error { quotaDash, quotaErr = u.QuotaDashboard(egCtx); return nil })
	eg.Go(func() error { inefficient, ineffErr = u.InefficientModels(egCtx, rangeQuery); return nil })

	if err := eg.Wait(); err != nil {
		return Overview{}, err
	}
	if quotaErr != nil {
		u.lg.Warn("overview.quota_dashboard_failed", loggateway.Err(quotaErr))
		quotaDash = QuotaDashboard{}
	}
	if ineffErr != nil {
		u.lg.Warn("overview.inefficient_models_failed", loggateway.Err(ineffErr))
		inefficient = nil
	}

	return Overview{
		Today:             today,
		Yesterday:         yesterdaySummary,
		Month:             month,
		Range:             rangeSummary,
		Trends:            trends,
		TopModels:         topModels,
		TopAgents:         topAgents,
		Anomalies:         anomalies,
		QuotaDashboard:    quotaDash,
		InefficientModels: inefficient,
	}, nil
}

// Trends returns usage trend data points.
func (u *Usecase) Trends(ctx context.Context, query Query) ([]TrendPoint, error) {
	q := u.normalizeQuery(query, u.now())
	if strings.EqualFold(strings.TrimSpace(q.Granularity), "hour") {
		return u.repo.ListModelUsageHourlyTrends(ctx, q)
	}
	todayKey := dateKey(u.now())
	// Daily rollup table has no team_id column; fall back to real-time when TeamID is set.
	if u.dailySupported(q) && q.EndDate < todayKey {
		return u.repo.ListModelUsageDailyTrends(ctx, q)
	}
	return u.repo.ListModelUsageTrends(ctx, q)
}

// TopModels returns top model usage breakdown.
func (u *Usecase) TopModels(ctx context.Context, query Query) ([]BreakdownRow, error) {
	return u.repo.ListTopModelUsage(ctx, u.normalizeQuery(query, u.now()))
}

// TopAgents returns top agent usage breakdown.
func (u *Usecase) TopAgents(ctx context.Context, query Query) ([]BreakdownRow, error) {
	return u.repo.ListTopAgentUsage(ctx, u.normalizeQuery(query, u.now()))
}

// AllModelsBreakdown 返回全模型消耗分页明细（用于「全模型消耗总览表」）。
// 与 TopModels 不同：支持服务端分页、LIKE 搜索、动态字段排序，且返回 total 用于前端分页 UI。
// 排序字段与方向由 data 层做白名单校验，避免 SQL 注入。
// 数据源策略与 Overview 对齐：daily rollup 为事件总线异步消费，range 含今天时 daily
// 表必然滞后于 events 表，此时改走实时聚合，保证总览页摘要/Top 榜/总览表三块同源。
func (u *Usecase) AllModelsBreakdown(ctx context.Context, query BreakdownQuery) (BreakdownResult, error) {
	q := u.normalizeBreakdownQuery(query, u.now())
	if q.EndDate < dateKey(u.now()) {
		return u.repo.ListAllModelsBreakdown(ctx, q)
	}
	return u.repo.ListAllModelsBreakdownRealtime(ctx, q)
}

// normalizeBreakdownQuery 将 BreakdownQuery.Range 解析为 StartDate/EndDate，
// 并归一化分页参数（page≥1, page_size 默认 20，最大 100）。
// 与 normalizeQuery 不同：BreakdownQuery 不复用 Query 结构（语义不同）。
func (u *Usecase) normalizeBreakdownQuery(query BreakdownQuery, now time.Time) BreakdownQuery {
	if query.StartDate == "" || query.EndDate == "" {
		end := dateKey(now)
		var start time.Time
		switch query.Range {
		case "today":
			start = now
		case "7d":
			start = now.AddDate(0, 0, -6)
		case "month":
			start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		case "30d", "":
			start = now.AddDate(0, 0, -(defaultQueryRangeDays - 1))
		default:
			start = now.AddDate(0, 0, -(defaultQueryRangeDays - 1))
		}
		if query.StartDate == "" {
			query.StartDate = dateKey(start)
		}
		if query.EndDate == "" {
			query.EndDate = end
		}
	}
	return query
}

// Events returns raw usage events (limit/offset honored by the repo).
func (u *Usecase) Events(ctx context.Context, query Query) ([]TokenUsageEvent, error) {
	return u.repo.ListModelUsageEvents(ctx, u.normalizeQuery(query, u.now()))
}

// EventsPage returns a page of usage events and the matching total for pagination UI.
func (u *Usecase) EventsPage(ctx context.Context, query Query) (EventsResult, error) {
	q := u.normalizeQuery(query, u.now())
	total, err := u.repo.CountModelUsageEvents(ctx, q)
	if err != nil {
		return EventsResult{}, err
	}
	items, err := u.repo.ListModelUsageEvents(ctx, q)
	if err != nil {
		return EventsResult{}, err
	}
	return EventsResult{Items: items, Total: total}, nil
}

// PurgeEvents deletes usage events older than retainDays and returns the count of deleted rows.
func (u *Usecase) PurgeEvents(ctx context.Context, retainDays int) (int64, error) {
	if retainDays < 1 {
		return 0, apierror.BadRequest("USAGE", "retain_days must be >= 1")
	}
	return u.repo.PurgeUsageEventsOlderThan(ctx, retainDays)
}

// RecordTokenUsageEvent inserts one usage row (events INSERT only; session aggregate and daily/hourly rollup are handled separately).
//
// Envelope publishing happens here so both the internal path (RecordTurnUsage /
// RecordAuxLLMUsage) and the external API path (UsageService.RecordTokenUsageEvent)
// emit exactly one event — no double-publish, no omission (P1-4, 2026-08-19).
func (u *Usecase) RecordTokenUsageEvent(ctx context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
	if strings.TrimSpace(e.ID) == "" {
		return TokenUsageEvent{}, apierror.BadRequest("USAGE", "id is required")
	}
	e = normalizeTokenUsageEventForInsert(e, u.now())
	priced := u.enrichPricing(ctx, &e)
	u.markNoPricing(&e, priced)
	out, err := u.repo.RecordTokenUsageEvent(ctx, e)
	if err != nil {
		return out, err
	}
	u.scheduleBudgetAlerts(ctx, out)
	if u.envelopePub != nil {
		u.envelopePub.PublishTokenUsageEnvelope(ctx, out)
	}
	return out, nil
}

func (u *Usecase) RollupDailyHourly(ctx context.Context, e TokenUsageEvent) error {
	return u.repo.RollupDailyHourly(ctx, e)
}

// ── Status normalization ──────────────────────────────────────────────────────

// ── Quota management ──────────────────────────────────────────────────────────

// TurnUsageInput captures the data needed to record usage for a single chat turn.
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
	// CachedTok is the cache-hit portion of PromptTok (billed at cache-read price).
	CachedTok    int
	Latency      time.Duration
	ErrMsg       string
	MetadataJSON string
	TraceID      string
	// TimeToFirstTokenMS is stream-observed TTFT; 0 means unobserved.
	TimeToFirstTokenMS int
	// WaitMS is HITL confirmation wait accumulated on the turn ctx.
	WaitMS int
}

// RecordTurnUsage records token usage for a completed chat turn.
// It persists the usage event, accumulates session metrics, publishes an
// envelope, and links the runner completion row.
func (u *Usecase) RecordTurnUsage(ctx context.Context, in TurnUsageInput) error {
	if u == nil {
		return nil
	}
	now := u.now()
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	meta = MergeWaitMSMetadata(meta, in.WaitMS, int(in.Latency.Milliseconds()))
	usageID := newUsageID()
	latencyMS := int(in.Latency.Milliseconds())
	ev := TokenUsageEvent{
		ID:                 usageID,
		SessionID:          in.SessionID,
		AgentKey:           in.AgentKey,
		AgentID:            in.AgentID,
		ModelAPIID:         in.Model,
		ModelDisplayName:   in.Model,
		ProviderCode:       in.Provider,
		InputTokens:        in.PromptTok,
		OutputTokens:       in.CompletionTok,
		CachedInputTokens:  in.CachedTok,
		TotalTokens:        in.PromptTok + in.CompletionTok,
		LatencyMS:          latencyMS,
		TimeToFirstTokenMS: in.TimeToFirstTokenMS,
		WaitMS:             in.WaitMS,
		ModelLatencyMS:     ModelLatencyMS(latencyMS, in.WaitMS),
		Status:             in.Status,
		UsageKind:          KindChatTurn,
		MetadataJSON:       meta,
		OccurredAt:         now.Format(time.RFC3339),
		DateKey:            now.Format("2006-01-02"),
		HourKey:            now.Format("2006-01-02T15"),
		ErrorMessage:       in.ErrMsg,
	}
	if in.RunID != "" {
		ev.MessageID = in.RunID
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(usageRecordTimeoutSec)*time.Second)
	defer cancel()
	// recEv carries the normalized row (CallCount=1 etc.); accumulate/publish
	// from it, not from the pre-normalization ev (P1-1 fix: ev.CallCount was 0,
	// so session_metrics.model_call_count never incremented).
	recEv, err := u.RecordTokenUsageEvent(recCtx, ev)
	if err != nil {
		return err
	}
	if u.sessAccum != nil && strings.TrimSpace(in.SessionID) != "" {
		u.sessAccum.AccumulateMetricsDelta(SessionMetricsDelta{
			SessionID:         in.SessionID,
			ModelCallCount:    recEv.CallCount,
			InputTokens:       int64(recEv.InputTokens),
			OutputTokens:      int64(recEv.OutputTokens),
			TotalTokens:       int64(recEv.TotalTokens),
			TotalCostMicroUsd: recEv.TotalCostMicroUSD,
		})
	}
	// Envelope publishing is handled inside RecordTokenUsageEvent (P1-4).
	if u.completion != nil && strings.TrimSpace(in.SessionID) != "" && strings.TrimSpace(in.RunID) != "" {
		linkCtx, linkCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(usageLinkTimeoutSec)*time.Second)
		defer linkCancel()
		if err := u.completion.LinkRunnerCompletionUsage(linkCtx, in.SessionID, in.RunID, usageID, in.TraceID); err != nil {
			u.lg.Warn("link runner completion usage failed", loggateway.Err(err), loggateway.Str("session_id", in.SessionID), loggateway.Str("run_id", in.RunID))
		}
	}
	return nil
}

// newUsageID returns a new UUID for usage events.
func newUsageID() string {
	return uuid.NewString()
}

// UsageSourceResponse marks provider-reported usage read from a non-streaming
// HTTP response body (intent pass / title generation / evolution calls).
// Complements the stream-path sources defined in internal/agent/turn_helpers.go.
const UsageSourceResponse = "response"

// AuxLLMUsageInput captures the data needed to record usage for an auxiliary
// (non-chat-turn) LLM call: subagent runs, session-title generation, intent
// pass classification, evolution drafting, etc. (P1-1, 2026-08-19).
type AuxLLMUsageInput struct {
	// Kind is one of KindAux*; required.
	Kind string
	// SessionID is optional — empty for session-less callers (e.g. evolution).
	// When set, session metrics are accumulated (same as chat turns).
	SessionID string
	// RunID is an optional correlation id (subagent run id / intent run id);
	// stored in MessageID for cross-referencing.
	RunID    string
	AgentID  string
	AgentKey string
	TeamID   string
	UserID   string
	Provider string
	Model    string
	Status   string
	// PromptTok/CompletionTok are provider-reported usage; CachedTok is the
	// cache-hit portion of PromptTok. When the caller only knows the total
	// (e.g. biz.LLMCaller collapses the breakdown), put it in CompletionTok
	// and leave PromptTok=0 — cost is then priced at the output rate, which
	// overestimates slightly; prefer plumbing the real breakdown where possible.
	PromptTok     int
	CompletionTok int
	CachedTok     int
	// EmbeddingTok is the embedding-call token count (KindAuxEmbedding only);
	// persisted into the EmbeddingTokens column and priced at the embedding
	// rate. Mutually exclusive with the Prompt/Completion split: embedding
	// calls have input-only consumption.
	EmbeddingTok int
	// UsageSource records how the token counts were obtained
	// (UsageSourceResponse / "streaming" / "estimated"); merged into
	// metadata_json["usage_source"].
	UsageSource string
	// Effort is the resolved thinking effort actually applied to the call
	// (off/low/medium/high/max; M83 LBG-6); merged into metadata_json["effort"].
	// Empty = call site did not route effort → key omitted.
	Effort       string
	Latency      time.Duration
	ErrMsg       string
	MetadataJSON string
}

// RecordAuxLLMUsage records token usage for an auxiliary LLM call.
// Like RecordTurnUsage it persists the event (with pricing enrichment and
// budget-alert scheduling), accumulates session metrics when SessionID is
// set, and publishes an envelope; unlike it there is no runner-completion
// row to link.
func (u *Usecase) RecordAuxLLMUsage(ctx context.Context, in AuxLLMUsageInput) error {
	if u == nil {
		return nil
	}
	if strings.TrimSpace(in.Kind) == "" {
		return apierror.BadRequest("USAGE", "kind is required")
	}
	now := u.now()
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	meta = MergeUsageSourceMetadata(meta, in.UsageSource)
	meta = MergeEffortMetadata(meta, in.Effort)
	ev := TokenUsageEvent{
		ID:                newUsageID(),
		SessionID:         in.SessionID,
		AgentKey:          in.AgentKey,
		AgentID:           in.AgentID,
		TeamID:            in.TeamID,
		UserID:            in.UserID,
		ModelAPIID:        in.Model,
		ModelDisplayName:  in.Model,
		ProviderCode:      in.Provider,
		InputTokens:       in.PromptTok,
		OutputTokens:      in.CompletionTok,
		CachedInputTokens: in.CachedTok,
		EmbeddingTokens:   in.EmbeddingTok,
		TotalTokens:       in.PromptTok + in.CompletionTok + in.EmbeddingTok,
		LatencyMS:         int(in.Latency.Milliseconds()),
		Status:            in.Status,
		UsageKind:         in.Kind,
		MetadataJSON:      meta,
		OccurredAt:        now.Format(time.RFC3339),
		DateKey:           now.Format("2006-01-02"),
		HourKey:           now.Format("2006-01-02T15"),
		ErrorMessage:      in.ErrMsg,
	}
	if in.RunID != "" {
		ev.MessageID = in.RunID
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(usageRecordTimeoutSec)*time.Second)
	defer cancel()
	recEv, err := u.RecordTokenUsageEvent(recCtx, ev)
	if err != nil {
		return err
	}
	if u.sessAccum != nil && strings.TrimSpace(in.SessionID) != "" {
		u.sessAccum.AccumulateMetricsDelta(SessionMetricsDelta{
			SessionID:         in.SessionID,
			ModelCallCount:    recEv.CallCount,
			InputTokens:       int64(recEv.InputTokens),
			OutputTokens:      int64(recEv.OutputTokens),
			TotalTokens:       int64(recEv.TotalTokens),
			TotalCostMicroUsd: recEv.TotalCostMicroUSD,
		})
	}
	// Envelope publishing is handled inside RecordTokenUsageEvent (P1-4).
	return nil
}

// ── Budget alerts ─────────────────────────────────────────────────────────────

// ── Inefficient models ────────────────────────────────────────────────────────

// ── CSV export ────────────────────────────────────────────────────────────────
