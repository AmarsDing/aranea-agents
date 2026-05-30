// Package usage implements token usage tracking, quota management, budget alerts, and cost analysis.
package usage

import (
	"context"
	stderrors "errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/event"
	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/safego"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	KindChatTurn   = "chat_turn"
	KindTeamMember = "team_member"
	KindTeamTurn   = "team_turn"
)

// Platform-wide quota scope identifiers.
const (
	QuotaScopeGlobal   = "global"
	GlobalQuotaScopeID = "global"
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
	Granularity  string
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
	InputPriceUSDPer1M             float64
	OutputPriceUSDPer1M            float64
	CacheReadPriceUSDPer1M         float64
	CacheWritePriceUSDPer1M        float64
	ReasoningPriceUSDPer1M         float64
	EmbeddingPriceUSDPer1M         float64
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
	Reason            string
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

// QuotaDashboard summarizes agent quota utilization for the overview page.
type QuotaDashboard struct {
	ConfiguredCount    int
	TotalCapMicroUSD   int64
	TotalSpentMicroUSD int64
	MaxUtilization     float64
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
	ListModelUsageHourlyTrends(ctx context.Context, query Query) ([]TrendPoint, error)
	GetModelUsageSummaryFromDaily(ctx context.Context, query Query) (Summary, error)
	ListModelUsageDailyTrends(ctx context.Context, query Query) ([]TrendPoint, error)
	ListTopModelUsageFromDaily(ctx context.Context, query Query) ([]BreakdownRow, error)
	ListTopAgentUsageFromDaily(ctx context.Context, query Query) ([]BreakdownRow, error)
}

// WriteRepo persists usage events and resolves pricing.
type WriteRepo interface {
	RecordTokenUsageEvent(ctx context.Context, event TokenUsageEvent) (TokenUsageEvent, error)
	GetActiveModelPricing(ctx context.Context, providerCode, modelAPIID string) (ModelPricingSnapshot, bool, error)
	PurgeUsageEventsOlderThan(ctx context.Context, retainDays int) (int64, error)
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

// ── Usecase ───────────────────────────────────────────────────────────────────

var alertCooldown = 60 * time.Minute

// Usecase implements usage tracking and quota management workflows.
type Usecase struct {
	repo          Repo
	now           func() time.Time
	alertNotifier AlertNotifier
	alertFired    map[string]time.Time
	alertFiredMu  sync.Mutex
}

// NewUsecase constructs a UsageUsecase.
func NewUsecase(repo Repo) *Usecase {
	return &Usecase{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
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
	start := now.AddDate(0, 0, -29)
	switch query.Range {
	case "today":
		start = now
	case "7d":
		start = now.AddDate(0, 0, -6)
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	case "30d", "":
		start = now.AddDate(0, 0, -29)
	default:
		start = now.AddDate(0, 0, -29)
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
		return errors.BadRequest("USAGE", "scope_type and scope_id are required")
	}
	if stderrors.Is(err, shared.ErrBudgetAlertNotFound) {
		return errors.NotFound("USAGE_ALERT", "budget alert not found")
	}
	return err
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
	anomalyQuery := withLimit(rangeQuery, 12)
	anomalyQuery.Status = "abnormal"

	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error { var e error; today, e = u.repo.GetModelUsageSummary(egCtx, todayQuery); return e })
	eg.Go(func() error {
		var e error
		yesterdaySummary, e = u.repo.GetModelUsageSummaryFromDaily(egCtx, yesterdayQuery)
		return e
	})
	eg.Go(func() error {
		var e error
		if monthQuery.EndDate < todayKey {
			month, e = u.repo.GetModelUsageSummaryFromDaily(egCtx, monthQuery)
		} else {
			month, e = u.repo.GetModelUsageSummary(egCtx, monthQuery)
		}
		return e
	})
	eg.Go(func() error {
		var e error
		if rangeQuery.EndDate < todayKey {
			rangeSummary, e = u.repo.GetModelUsageSummaryFromDaily(egCtx, rangeQuery)
		} else {
			rangeSummary, e = u.repo.GetModelUsageSummary(egCtx, rangeQuery)
		}
		return e
	})
	eg.Go(func() error { var e error; trends, e = u.Trends(egCtx, rangeQuery); return e })
	eg.Go(func() error {
		var e error
		if rangeQuery.EndDate < todayKey {
			topModels, e = u.repo.ListTopModelUsageFromDaily(egCtx, withLimit(rangeQuery, 8))
		} else {
			topModels, e = u.repo.ListTopModelUsage(egCtx, withLimit(rangeQuery, 8))
		}
		return e
	})
	eg.Go(func() error {
		var e error
		if rangeQuery.EndDate < todayKey {
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
		quotaDash = QuotaDashboard{}
	}
	if ineffErr != nil {
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
	if q.EndDate < todayKey {
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

// Events returns raw usage events.
func (u *Usecase) Events(ctx context.Context, query Query) ([]TokenUsageEvent, error) {
	return u.repo.ListModelUsageEvents(ctx, u.normalizeQuery(query, u.now()))
}

// PurgeEvents deletes usage events older than retainDays and returns the count of deleted rows.
func (u *Usecase) PurgeEvents(ctx context.Context, retainDays int) (int64, error) {
	if retainDays < 1 {
		return 0, errors.BadRequest("USAGE", "retain_days must be >= 1")
	}
	return u.repo.PurgeUsageEventsOlderThan(ctx, retainDays)
}

// RecordTokenUsageEvent inserts one usage row, updates session aggregates, and upserts daily rollup.
func (u *Usecase) RecordTokenUsageEvent(ctx context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
	if strings.TrimSpace(e.ID) == "" {
		return TokenUsageEvent{}, errors.BadRequest("USAGE", "id is required")
	}
	e = normalizeTokenUsageEventForInsert(e, u.now())
	u.enrichPricing(ctx, &e)
	out, err := u.repo.RecordTokenUsageEvent(ctx, e)
	if err == nil {
		u.scheduleBudgetAlerts(ctx, out)
	}
	return out, err
}

func normalizeTokenUsageEventForInsert(e TokenUsageEvent, now time.Time) TokenUsageEvent {
	if strings.TrimSpace(e.OccurredAt) != "" {
		if t, err := time.Parse(time.RFC3339, e.OccurredAt); err == nil {
			now = t.UTC()
		}
	}
	if strings.TrimSpace(e.OccurredAt) == "" {
		e.OccurredAt = now.Format(time.RFC3339)
	}
	if strings.TrimSpace(e.CreatedAt) == "" {
		e.CreatedAt = e.OccurredAt
	}
	if strings.TrimSpace(e.DateKey) == "" && len(e.OccurredAt) >= 10 {
		e.DateKey = e.OccurredAt[:10]
	}
	if strings.TrimSpace(e.HourKey) == "" && len(e.OccurredAt) >= 13 {
		e.HourKey = e.OccurredAt[:13] + ":00"
	}
	if strings.TrimSpace(e.UsageKind) == "" {
		e.UsageKind = "chat"
	}
	if e.CallCount <= 0 {
		e.CallCount = 1
	}
	e.Status = NormalizeStatus(e.Status)
	if e.Status == "" {
		e.Status = "success"
	}
	if strings.TrimSpace(e.ModelCategoryJSON) == "" {
		e.ModelCategoryJSON = "[]"
	}
	if strings.TrimSpace(e.MetadataJSON) == "" {
		e.MetadataJSON = "{}"
	}
	return e
}

func (u *Usecase) enrichPricing(ctx context.Context, e *TokenUsageEvent) {
	if e == nil {
		return
	}
	prov := strings.TrimSpace(e.ProviderCode)
	mod := strings.TrimSpace(e.ModelAPIID)
	if prov == "" || mod == "" {
		ApplyTokenUsageCosts(e)
		return
	}
	if e.InputPriceUSDPer1M == 0 && e.OutputPriceUSDPer1M == 0 &&
		e.InputPriceMicroUSDPer1K == 0 && e.OutputPriceMicroUSDPer1K == 0 {
		if snap, ok, err := u.repo.GetActiveModelPricing(ctx, prov, mod); err == nil && ok {
			applyPricingUSDToEvent(e, snap)
		}
	}
	ApplyTokenUsageCosts(e)
}

func applyPricingUSDToEvent(e *TokenUsageEvent, snap ModelPricingSnapshot) {
	if e == nil {
		return
	}
	e.InputPriceUSDPer1M = snap.InputPriceUSDPer1M
	e.OutputPriceUSDPer1M = snap.OutputPriceUSDPer1M
	e.CacheReadPriceUSDPer1M = snap.CacheReadPriceUSDPer1M
	e.CacheWritePriceUSDPer1M = snap.CacheWritePriceUSDPer1M
	e.ReasoningPriceUSDPer1M = snap.ReasoningPriceUSDPer1M
	e.EmbeddingPriceUSDPer1M = snap.EmbeddingPriceUSDPer1M
	// Micro columns remain for DB persistence; derived from USD/1M when available.
	if snap.InputPriceUSDPer1M > 0 {
		e.InputPriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.InputPriceUSDPer1M)
	} else if snap.InputPriceMicroUSDPer1K > 0 {
		e.InputPriceMicroUSDPer1K = snap.InputPriceMicroUSDPer1K
	}
	if snap.OutputPriceUSDPer1M > 0 {
		e.OutputPriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.OutputPriceUSDPer1M)
	} else if snap.OutputPriceMicroUSDPer1K > 0 {
		e.OutputPriceMicroUSDPer1K = snap.OutputPriceMicroUSDPer1K
	}
	if snap.CacheReadPriceUSDPer1M > 0 {
		e.CachedInputPriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.CacheReadPriceUSDPer1M)
	} else if snap.CachedInputPriceMicroUSDPer1K > 0 {
		e.CachedInputPriceMicroUSDPer1K = snap.CachedInputPriceMicroUSDPer1K
	}
	if snap.CacheWritePriceUSDPer1M > 0 {
		e.CacheWritePriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.CacheWritePriceUSDPer1M)
	} else if snap.CacheWritePriceMicroUSDPer1K > 0 {
		e.CacheWritePriceMicroUSDPer1K = snap.CacheWritePriceMicroUSDPer1K
	}
	if snap.ReasoningPriceUSDPer1M > 0 {
		e.ReasoningPriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.ReasoningPriceUSDPer1M)
	} else if snap.ReasoningPriceMicroUSDPer1K > 0 {
		e.ReasoningPriceMicroUSDPer1K = snap.ReasoningPriceMicroUSDPer1K
	}
	if snap.EmbeddingPriceUSDPer1M > 0 {
		e.EmbeddingPriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.EmbeddingPriceUSDPer1M)
	} else if snap.EmbeddingPriceMicroUSDPer1K > 0 {
		e.EmbeddingPriceMicroUSDPer1K = snap.EmbeddingPriceMicroUSDPer1K
	}
}

// ── Status normalization ──────────────────────────────────────────────────────

// NormalizeStatus maps legacy writer values to DB canonical status.
func NormalizeStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "", "success", "ok":
		return "success"
	case "failed", "fail", "error":
		return "failed"
	case "timeout", "timed_out":
		return "timeout"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return strings.TrimSpace(status)
	}
}

// ApplyTokenUsageCosts fills per-kind costs and total from token counts and USD/1M prices (micro fallback for legacy rows).
func ApplyTokenUsageCosts(e *TokenUsageEvent) {
	if e == nil {
		return
	}
	if e.InputCostMicroUSD == 0 && e.InputTokens > 0 {
		e.InputCostMicroUSD = usageCostMicro(e.InputTokens, e.InputPriceMicroUSDPer1K, e.InputPriceUSDPer1M)
	}
	if e.OutputCostMicroUSD == 0 && e.OutputTokens > 0 {
		e.OutputCostMicroUSD = usageCostMicro(e.OutputTokens, e.OutputPriceMicroUSDPer1K, e.OutputPriceUSDPer1M)
	}
	if e.CachedInputCostMicroUSD == 0 && e.CachedInputTokens > 0 {
		e.CachedInputCostMicroUSD = usageCostMicro(e.CachedInputTokens, e.CachedInputPriceMicroUSDPer1K, e.CacheReadPriceUSDPer1M)
	}
	if e.CacheWriteCostMicroUSD == 0 && e.CacheWriteTokens > 0 {
		e.CacheWriteCostMicroUSD = usageCostMicro(e.CacheWriteTokens, e.CacheWritePriceMicroUSDPer1K, e.CacheWritePriceUSDPer1M)
	}
	if e.ReasoningCostMicroUSD == 0 && e.ReasoningTokens > 0 {
		e.ReasoningCostMicroUSD = usageCostMicro(e.ReasoningTokens, e.ReasoningPriceMicroUSDPer1K, e.ReasoningPriceUSDPer1M)
	}
	if e.EmbeddingCostMicroUSD == 0 && e.EmbeddingTokens > 0 {
		e.EmbeddingCostMicroUSD = usageCostMicro(e.EmbeddingTokens, e.EmbeddingPriceMicroUSDPer1K, e.EmbeddingPriceUSDPer1M)
	}
	if e.TotalCostMicroUSD == 0 {
		e.TotalCostMicroUSD = e.InputCostMicroUSD + e.OutputCostMicroUSD +
			e.CachedInputCostMicroUSD + e.CacheWriteCostMicroUSD + e.ReasoningCostMicroUSD + e.EmbeddingCostMicroUSD
	}
}

func usageCostMicro(tokens int, microPer1K int64, usdPer1M float64) int64 {
	if tokens <= 0 {
		return 0
	}
	if usdPer1M > 0 && !math.IsNaN(usdPer1M) {
		return modelregistry.CostMicroUSDFromUSDPer1M(tokens, usdPer1M)
	}
	if microPer1K > 0 {
		return int64(tokens) * microPer1K / 1000
	}
	return 0
}

// ── Quota management ──────────────────────────────────────────────────────────

// GetQuota returns the quota for a scope.
func (u *Usecase) GetQuota(ctx context.Context, scopeType, scopeID string) (Quota, error) {
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil {
		if stderrors.Is(err, shared.ErrQuotaNotFound) {
			return Quota{}, errors.NotFound("USAGE_QUOTA", "quota not configured")
		}
		return Quota{}, MapRepoErr(err)
	}
	return q, nil
}

// SetQuota creates or updates a quota.
func (u *Usecase) SetQuota(ctx context.Context, quota Quota) (Quota, error) {
	if strings.TrimSpace(quota.ScopeType) == "" || strings.TrimSpace(quota.ScopeID) == "" {
		return Quota{}, errors.BadRequest("USAGE_QUOTA", "scope_type and scope_id are required")
	}
	if quota.MonthlyMicroUSD < 0 {
		return Quota{}, errors.BadRequest("USAGE_QUOTA", "monthly_micro_usd must be >= 0")
	}
	q, err := u.repo.SetQuota(ctx, quota)
	return q, MapRepoErr(err)
}

// CheckQuota returns whether another chat turn is allowed under the configured cap.
func (u *Usecase) CheckQuota(ctx context.Context, scopeType, scopeID string) (QuotaCheck, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return QuotaCheck{}, errors.BadRequest("USAGE_QUOTA", "scope_type and scope_id are required")
	}
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil {
		if stderrors.Is(err, shared.ErrQuotaNotFound) {
			return QuotaCheck{Allowed: true, Reason: "no quota configured"}, nil
		}
		return QuotaCheck{}, err
	}
	if q.MonthlyMicroUSD <= 0 {
		return QuotaCheck{Quota: q, Allowed: true, Reason: "quota disabled"}, nil
	}
	spent, err := u.quotaSpent(ctx, scopeType, scopeID, q)
	if err != nil {
		return QuotaCheck{}, err
	}
	remaining := q.MonthlyMicroUSD - spent
	if remaining < 0 {
		remaining = 0
	}
	check := QuotaCheck{
		Quota:             q,
		SpentMicroUSD:     spent,
		RemainingMicroUSD: remaining,
	}
	if spent >= q.MonthlyMicroUSD {
		check.Allowed = false
		check.Reason = fmt.Sprintf("monthly quota exceeded: spent %d >= cap %d micro-USD", spent, q.MonthlyMicroUSD)
		return check, nil
	}
	check.Allowed = true
	check.Reason = "within quota"
	return check, nil
}

func (u *Usecase) quotaSpent(ctx context.Context, scopeType, scopeID string, q Quota) (int64, error) {
	switch scopeType {
	case "agent", "user", "global":
		spent, err := u.repo.SumScopeCostInPeriod(ctx, scopeType, scopeID, q.PeriodStart, q.PeriodEnd)
		return spent, MapRepoErr(err)
	default:
		return 0, shared.ErrQuotaUnsupportedScope
	}
}

// ── Budget alerts ─────────────────────────────────────────────────────────────

func (u *Usecase) scheduleBudgetAlerts(ctx context.Context, e TokenUsageEvent) {
	if u == nil || u.alertNotifier == nil || e.TotalCostMicroUSD <= 0 {
		return
	}
	if strings.TrimSpace(e.AgentID) == "" && strings.TrimSpace(e.UserID) == "" {
		return
	}
	ev := e
	safego.Go(ctx, "usage.budget_alert", func() {
		u.EvaluateBudgetAlerts(context.WithoutCancel(ctx), ev)
	})
}

// EvaluateBudgetAlerts checks budget thresholds after a usage event.
func (u *Usecase) EvaluateBudgetAlerts(ctx context.Context, e TokenUsageEvent) {
	if u == nil || u.alertNotifier == nil {
		return
	}
	if id := strings.TrimSpace(e.AgentID); id != "" {
		u.evaluateBudgetAlertsForScope(ctx, "agent", id)
	}
	if id := strings.TrimSpace(e.UserID); id != "" {
		u.evaluateBudgetAlertsForScope(ctx, "user", id)
	}
	u.evaluateBudgetAlertsForScope(ctx, QuotaScopeGlobal, GlobalQuotaScopeID)
}

func (u *Usecase) evaluateBudgetAlertsForScope(ctx context.Context, scopeType, scopeID string) {
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil || q.MonthlyMicroUSD <= 0 {
		return
	}
	spent, err := u.repo.SumScopeCostInPeriod(ctx, scopeType, scopeID, q.PeriodStart, q.PeriodEnd)
	if err != nil || spent <= 0 {
		return
	}
	util := float64(spent) / float64(q.MonthlyMicroUSD)
	alerts, err := u.repo.ListBudgetAlerts(ctx, scopeType, scopeID)
	if err != nil {
		return
	}
	now := u.now().UTC()
	for _, a := range alerts {
		if !a.Enabled || a.AlertRatio <= 0 || util+1e-9 < a.AlertRatio {
			continue
		}
		if u.alertRecentlyFired(a, now) {
			continue
		}
		if err := u.alertNotifier.NotifyBudgetAlert(ctx, a, spent, q.MonthlyMicroUSD, util); err != nil {
			continue
		}
		_ = u.repo.UpdateBudgetAlertLastFired(ctx, a.ID, now.Format(time.RFC3339))
		u.markAlertFired(a.ID, now)
	}
}

func (u *Usecase) alertRecentlyFired(a BudgetAlert, now time.Time) bool {
	u.alertFiredMu.Lock()
	defer u.alertFiredMu.Unlock()
	if u.alertFired == nil {
		u.alertFired = make(map[string]time.Time)
	}
	if t, ok := u.alertFired[a.ID]; ok && now.Sub(t) < alertCooldown {
		return true
	}
	if strings.TrimSpace(a.LastFiredAt) != "" {
		if t, err := time.Parse(time.RFC3339, a.LastFiredAt); err == nil && now.Sub(t) < alertCooldown {
			return true
		}
	}
	return false
}

func (u *Usecase) markAlertFired(id string, now time.Time) {
	u.alertFiredMu.Lock()
	defer u.alertFiredMu.Unlock()
	if u.alertFired == nil {
		u.alertFired = make(map[string]time.Time)
	}
	u.alertFired[id] = now
}

// ListBudgetAlerts returns budget alerts for a scope.
func (u *Usecase) ListBudgetAlerts(ctx context.Context, scopeType, scopeID string) ([]BudgetAlert, error) {
	return u.repo.ListBudgetAlerts(ctx, scopeType, scopeID)
}

// SetBudgetAlert creates or updates a budget alert.
func (u *Usecase) SetBudgetAlert(ctx context.Context, alert BudgetAlert) (BudgetAlert, error) {
	if strings.TrimSpace(alert.ScopeType) == "" || strings.TrimSpace(alert.ScopeID) == "" {
		return BudgetAlert{}, errors.BadRequest("USAGE_ALERT", "scope_type and scope_id are required")
	}
	if alert.AlertRatio <= 0 || alert.AlertRatio > 1 {
		return BudgetAlert{}, errors.BadRequest("USAGE_ALERT", "alert_ratio must be in (0,1]")
	}
	a, err := u.repo.SetBudgetAlert(ctx, alert)
	return a, MapRepoErr(err)
}

// QuotaDashboard summarizes agent quota utilization for the overview page.
func (u *Usecase) QuotaDashboard(ctx context.Context) (QuotaDashboard, error) {
	quotas, err := u.repo.ListActiveQuotas(ctx)
	if err != nil {
		return QuotaDashboard{}, err
	}
	var dash QuotaDashboard
	if len(quotas) == 0 {
		return dash, nil
	}
	spentMap, batchErr := u.repo.BatchSumScopeCost(ctx, quotas)
	if batchErr != nil {
		event.SysLogWarn("system.usage", "quota_dashboard.batch_failed", event.P("error", batchErr.Error()))
	}
	var maxUtil float64
	for _, q := range quotas {
		if q.MonthlyMicroUSD <= 0 {
			continue
		}
		key := q.ScopeType + ":" + q.ScopeID
		spent, ok := spentMap[key]
		if !ok && batchErr != nil {
			continue
		}
		dash.ConfiguredCount++
		dash.TotalCapMicroUSD += q.MonthlyMicroUSD
		dash.TotalSpentMicroUSD += spent
		util := float64(spent) / float64(q.MonthlyMicroUSD)
		if util > maxUtil {
			maxUtil = util
		}
	}
	dash.MaxUtilization = maxUtil
	return dash, nil
}

// ── Inefficient models ────────────────────────────────────────────────────────

const (
	inefficientMinCalls       = 3
	inefficientCostMicroFloor = int64(100_000)
	inefficientLowTPS         = 5.0
	inefficientLowSuccess     = 0.85
)

// InefficientModels returns top models in range that match high-cost + (low TPS or low success).
func (u *Usecase) InefficientModels(ctx context.Context, query Query) ([]ModelInsight, error) {
	q := withLimit(u.normalizeQuery(query, u.now()), 32)
	rows, err := u.repo.ListTopModelUsage(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []ModelInsight
	for _, r := range rows {
		if r.CallCount < inefficientMinCalls || r.TotalCostMicroUSD < inefficientCostMicroFloor {
			continue
		}
		var flags []string
		if r.AvgTokensPerSecond > 0 && r.AvgTokensPerSecond < inefficientLowTPS {
			flags = append(flags, "low_tps")
		}
		if r.SuccessRate > 0 && r.SuccessRate < inefficientLowSuccess {
			flags = append(flags, "high_failure")
		}
		if r.TotalCostMicroUSD >= inefficientCostMicroFloor*10 {
			flags = append(flags, "high_cost")
		}
		if len(flags) == 0 {
			continue
		}
		out = append(out, ModelInsight{
			ProviderCode:       r.ProviderCode,
			ModelAPIID:         r.ModelAPIID,
			ModelDisplayName:   strings.TrimSpace(r.ModelDisplayName),
			CallCount:          r.CallCount,
			TotalTokens:        r.TotalTokens,
			TotalCostMicroUSD:  r.TotalCostMicroUSD,
			AvgLatencyMS:       r.AvgLatencyMS,
			AvgTokensPerSecond: r.AvgTokensPerSecond,
			SuccessRate:        r.SuccessRate,
			Flags:              flags,
		})
		if len(out) >= 8 {
			break
		}
	}
	return out, nil
}

// ── CSV export ────────────────────────────────────────────────────────────────

// ExportUsageEventsCSV returns CSV rows for usage events.
func (u *Usecase) ExportUsageEventsCSV(ctx context.Context, query Query) (string, error) {
	query.Limit = 5000
	if query.Limit <= 0 {
		query.Limit = 5000
	}
	events, err := u.Events(ctx, query)
	if err != nil {
		return "", err
	}
	return formatUsageEventsCSV(events), nil
}

func formatUsageEventsCSV(events []TokenUsageEvent) string {
	var b strings.Builder
	b.WriteString("occurred_at,usage_kind,agent_id,provider_code,model_api_id,session_id,team_id,input_tokens,output_tokens,total_tokens,total_cost_micro_usd,latency_ms,status,error_message\n")
	for _, e := range events {
		b.WriteString(fmt.Sprintf("%q,%q,%q,%q,%q,%q,%q,%d,%d,%d,%d,%d,%q,%q\n",
			e.OccurredAt, e.UsageKind, e.AgentID, e.ProviderCode, e.ModelAPIID, e.SessionID, e.TeamID,
			e.InputTokens, e.OutputTokens, e.TotalTokens, e.TotalCostMicroUSD, e.LatencyMS, e.Status, csvEscape(e.ErrorMessage),
		))
	}
	return b.String()
}

func csvEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `"`, `""`), "\n", " ")
}
