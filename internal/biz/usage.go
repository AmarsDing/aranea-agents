package biz

import (
	"context"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
)

// UsageQuery mirrors legacy model-usage GET params.
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

// TokenUsageEvent mirrors SQLite row scan from model_token_usage_events.
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

type UsageRepo interface {
	GetModelUsageSummary(ctx context.Context, query UsageQuery) (UsageSummary, error)
	ListModelUsageTrends(ctx context.Context, query UsageQuery) ([]UsageTrendPoint, error)
	ListTopModelUsage(ctx context.Context, query UsageQuery) ([]UsageBreakdownRow, error)
	ListTopAgentUsage(ctx context.Context, query UsageQuery) ([]UsageBreakdownRow, error)
	ListModelUsageEvents(ctx context.Context, query UsageQuery) ([]TokenUsageEvent, error)
	RecordTokenUsageEvent(ctx context.Context, event TokenUsageEvent) (TokenUsageEvent, error)
	GetQuota(ctx context.Context, scopeType, scopeID string) (UsageQuota, error)
	SetQuota(ctx context.Context, quota UsageQuota) (UsageQuota, error)
	SumAgentCostInPeriod(ctx context.Context, agentID, periodStart, periodEnd string) (int64, error)
}

type UsageUsecase struct {
	repo UsageRepo
	now  func() time.Time
}

func NewUsageUsecase(repo UsageRepo) *UsageUsecase {
	return &UsageUsecase{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func dateKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func withUsageLimit(query UsageQuery, limit int) UsageQuery {
	query.Limit = limit
	return query
}

func (u *UsageUsecase) normalizeQuery(query UsageQuery, now time.Time) UsageQuery {
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

func (u *UsageUsecase) Overview(ctx context.Context, query UsageQuery) (UsageOverview, error) {
	now := u.now()
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

	today, err := u.repo.GetModelUsageSummary(ctx, todayQuery)
	if err != nil {
		return UsageOverview{}, err
	}
	yesterdaySummary, err := u.repo.GetModelUsageSummary(ctx, yesterdayQuery)
	if err != nil {
		return UsageOverview{}, err
	}
	month, err := u.repo.GetModelUsageSummary(ctx, monthQuery)
	if err != nil {
		return UsageOverview{}, err
	}
	rangeSummary, err := u.repo.GetModelUsageSummary(ctx, rangeQuery)
	if err != nil {
		return UsageOverview{}, err
	}
	trends, err := u.repo.ListModelUsageTrends(ctx, rangeQuery)
	if err != nil {
		return UsageOverview{}, err
	}
	topModels, err := u.repo.ListTopModelUsage(ctx, withUsageLimit(rangeQuery, 8))
	if err != nil {
		return UsageOverview{}, err
	}
	topAgents, err := u.repo.ListTopAgentUsage(ctx, withUsageLimit(rangeQuery, 8))
	if err != nil {
		return UsageOverview{}, err
	}
	anomalyQuery := withUsageLimit(rangeQuery, 12)
	anomalyQuery.Status = "abnormal"
	anomalies, err := u.repo.ListModelUsageEvents(ctx, anomalyQuery)
	if err != nil {
		return UsageOverview{}, err
	}

	return UsageOverview{
		Today:     today,
		Yesterday: yesterdaySummary,
		Month:     month,
		Range:     rangeSummary,
		Trends:    trends,
		TopModels: topModels,
		TopAgents: topAgents,
		Anomalies: anomalies,
	}, nil
}

func (u *UsageUsecase) Trends(ctx context.Context, query UsageQuery) ([]UsageTrendPoint, error) {
	return u.repo.ListModelUsageTrends(ctx, u.normalizeQuery(query, u.now()))
}

func (u *UsageUsecase) TopModels(ctx context.Context, query UsageQuery) ([]UsageBreakdownRow, error) {
	return u.repo.ListTopModelUsage(ctx, u.normalizeQuery(query, u.now()))
}

func (u *UsageUsecase) TopAgents(ctx context.Context, query UsageQuery) ([]UsageBreakdownRow, error) {
	return u.repo.ListTopAgentUsage(ctx, u.normalizeQuery(query, u.now()))
}

func (u *UsageUsecase) Events(ctx context.Context, query UsageQuery) ([]TokenUsageEvent, error) {
	return u.repo.ListModelUsageEvents(ctx, u.normalizeQuery(query, u.now()))
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
	if strings.TrimSpace(e.Status) == "" {
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

// RecordTokenUsageEvent inserts one usage row, updates session aggregates, and upserts daily rollup (parity with pkg/backend).
func (u *UsageUsecase) RecordTokenUsageEvent(ctx context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
	if strings.TrimSpace(e.ID) == "" {
		return TokenUsageEvent{}, errors.BadRequest("USAGE", "id is required")
	}
	e = normalizeTokenUsageEventForInsert(e, u.now())
	return u.repo.RecordTokenUsageEvent(ctx, e)
}
