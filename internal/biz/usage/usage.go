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
