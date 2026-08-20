// Package usage implements token usage tracking, quota management, budget alerts, and cost analysis.
package usage

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
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
)

// MetadataKeyUsageSource is the metadata_json key recording how a usage row's
// token counts were obtained: "streaming" / "runner_completion" (provider-reported)
// or "estimated" (filled from text by EstimateTokensIfMissing). Lets DB queries
// distinguish estimated rows from authoritative ones (P0-2, 2026-08-19).
const MetadataKeyUsageSource = "usage_source"

// MergeUsageSourceMetadata sets metadata_json[usage_source], preserving existing
// keys. Passthrough when usageSource is empty (usage never observed — nothing to
// claim) or metaJSON is unparseable (caller keeps its original payload).
func MergeUsageSourceMetadata(metaJSON, usageSource string) string {
	usageSource = strings.TrimSpace(usageSource)
	if usageSource == "" {
		return metaJSON
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload[MetadataKeyUsageSource] = usageSource
	raw, err := json.Marshal(payload)
	if err != nil {
		return metaJSON
	}
	return string(raw)
}

// MetadataKeyLLMRounds is the metadata_json key recording how many LLM rounds
// (billable API calls) a chat_turn usage row sums over. Tool-call loops bill
// each round's full prompt separately, so without the round count a summed
// row is unexplainable (P1-C, 2026-08-20).
const MetadataKeyLLMRounds = "llm_rounds"

// MergeLLMRoundsMetadata sets metadata_json[llm_rounds], preserving existing
// keys. Passthrough when rounds <= 0 (no stream observation — nothing to
// claim) or metaJSON is unparseable.
func MergeLLMRoundsMetadata(metaJSON string, rounds int) string {
	if rounds <= 0 {
		return metaJSON
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload[MetadataKeyLLMRounds] = rounds
	raw, err := json.Marshal(payload)
	if err != nil {
		return metaJSON
	}
	return string(raw)
}

// MetadataKeyUsageAttribution is the metadata_json key recording how a usage
// row's tokens are attributed. UsageAttributionRunLevelAnchorFallback marks
// team_member rows that carry RUN-LEVEL totals attributed to the anchor member
// (anchor-fallback path when graph watch produced no per-member steps) — such
// rows duplicate the team_turn row's totals and must NOT additionally
// accumulate session metrics (P2-1 双计根治, 2026-08-19).
const MetadataKeyUsageAttribution = "usage_attribution"

// UsageAttributionRunLevelAnchorFallback see MetadataKeyUsageAttribution.
const UsageAttributionRunLevelAnchorFallback = "run_level_anchor_fallback"

// UsageAttributionMemberLevelStream marks team_member rows persisted from the
// stream's per-member MemberUsage on the completion path (P2-1b, 2026-08-19):
// graph watch writes member steps with zero tokens, so without these rows a
// watch-healthy team run is invisible to billable aggregates. Like the
// anchor-fallback row, these duplicate the team_turn row's totals and must NOT
// additionally accumulate session metrics.
const UsageAttributionMemberLevelStream = "member_level_stream"

// UsageAttributionStreamAnchorRemainder marks the team_member row carrying the
// run-total REMAINDER (run totals − Σ MemberUsage) attributed to the anchor
// member: consumption authored by the team root (non-member author) is inside
// the run totals but outside every MemberUsage entry. Same session-metrics
// rule as UsageAttributionMemberLevelStream.
const UsageAttributionStreamAnchorRemainder = "stream_anchor_remainder"

// MergeUsageAttributionMetadata sets metadata_json[usage_attribution],
// preserving existing keys. Same passthrough rules as MergeUsageSourceMetadata.
func MergeUsageAttributionMetadata(metaJSON, attribution string) string {
	attribution = strings.TrimSpace(attribution)
	if attribution == "" {
		return metaJSON
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload[MetadataKeyUsageAttribution] = attribution
	raw, err := json.Marshal(payload)
	if err != nil {
		return metaJSON
	}
	return string(raw)
}

// MetadataKeyCostStatus is the metadata_json key flagging rows whose cost could
// not be priced (P2-2, 2026-08-19). CostStatusNoPricing marks events that carry
// billable tokens but zero computed cost AND no price fields at all — without
// the flag, missing pricing config is indistinguishable from legitimately-free
// consumption in aggregates.
const MetadataKeyCostStatus = "cost_status"

// CostStatusNoPricing see MetadataKeyCostStatus.
const CostStatusNoPricing = "no_pricing"

// MergeCostStatusMetadata sets metadata_json[cost_status], preserving existing
// keys. Same passthrough rules as MergeUsageSourceMetadata.
func MergeCostStatusMetadata(metaJSON, status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return metaJSON
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload[MetadataKeyCostStatus] = status
	raw, err := json.Marshal(payload)
	if err != nil {
		return metaJSON
	}
	return string(raw)
}

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

// Stable quota-check reason codes (API contract; UI maps them to localized text).
const (
	QuotaCheckReasonNoQuota  = "no_quota"
	QuotaCheckReasonDisabled = "quota_disabled"
	QuotaCheckReasonExceeded = "quota_exceeded"
	QuotaCheckReasonWithin   = "within_quota"
)

// QuotaExceededMessage builds the localized user-facing message for a blocked turn.
func QuotaExceededMessage(check QuotaCheck) string {
	return fmt.Sprintf("本月用量已达配额上限（已消耗 $%.2f / 上限 $%.2f），新对话已被拦截；请调整配额或等待下月自动重置",
		float64(check.SpentMicroUSD)/1e6, float64(check.Quota.MonthlyMicroUSD)/1e6)
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

var alertCooldown = 60 * time.Minute

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
func (u *Usecase) AllModelsBreakdown(ctx context.Context, query BreakdownQuery) (BreakdownResult, error) {
	return u.repo.ListAllModelsBreakdown(ctx, u.normalizeBreakdownQuery(query, u.now()))
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

// enrichPricing resolves prices for the event (caller-supplied prices win;
// otherwise the active pricing snapshot is applied) and recomputes costs.
//
// The return value reports whether the event is considered PRICED: true when a
// pricing snapshot matched (even an explicitly all-zero free tier) or any
// price field was supplied — in both cases the operator made a deliberate
// pricing decision. false means no pricing source exists at all, which
// markNoPricing treats as a config gap (P2-2 review fix, 2026-08-19).
func (u *Usecase) enrichPricing(ctx context.Context, e *TokenUsageEvent) bool {
	if e == nil {
		return true
	}
	prov := strings.TrimSpace(e.ProviderCode)
	mod := strings.TrimSpace(e.ModelAPIID)
	if prov != "" && mod != "" &&
		e.InputPriceUSDPer1M == 0 && e.OutputPriceUSDPer1M == 0 &&
		e.InputPriceMicroUSDPer1K == 0 && e.OutputPriceMicroUSDPer1K == 0 {
		if snap, ok, err := u.repo.GetActiveModelPricing(ctx, prov, mod); err == nil && ok {
			applyPricingUSDToEvent(e, snap)
			ApplyTokenUsageCosts(e)
			return true
		}
	}
	ApplyTokenUsageCosts(e)
	return hasAnyPrice(e)
}

// hasAnyPrice reports whether any price field is non-zero (caller-supplied or
// snapshot-applied). Snapshot hits return early in enrichPricing and never
// reach this check, so a false here reliably means "no pricing source".
func hasAnyPrice(e *TokenUsageEvent) bool {
	return e.InputPriceUSDPer1M != 0 || e.OutputPriceUSDPer1M != 0 ||
		e.CacheReadPriceUSDPer1M != 0 || e.CacheWritePriceUSDPer1M != 0 ||
		e.ReasoningPriceUSDPer1M != 0 || e.EmbeddingPriceUSDPer1M != 0 ||
		e.InputPriceMicroUSDPer1K != 0 || e.OutputPriceMicroUSDPer1K != 0 ||
		e.CachedInputPriceMicroUSDPer1K != 0 || e.CacheWritePriceMicroUSDPer1K != 0 ||
		e.ReasoningPriceMicroUSDPer1K != 0 || e.EmbeddingPriceMicroUSDPer1K != 0
}

// markNoPricing flags events carrying billable tokens whose cost resolved to
// zero with NO pricing source at all (no active pricing snapshot, no
// caller-supplied prices): such rows are indistinguishable from legitimately
// free consumption in aggregates, so they get metadata_json[cost_status]=
// "no_pricing" plus a Warn log deduped per provider|model (P2-2, 2026-08-19).
// priced comes from enrichPricing; an explicitly all-zero snapshot (free tier)
// reports priced=true and is NOT flagged.
func (u *Usecase) markNoPricing(e *TokenUsageEvent, priced bool) {
	if e == nil {
		return
	}
	billable := e.InputTokens + e.OutputTokens + e.CachedInputTokens +
		e.CacheWriteTokens + e.ReasoningTokens + e.EmbeddingTokens
	if billable <= 0 || e.TotalCostMicroUSD > 0 || priced {
		return
	}
	e.MetadataJSON = MergeCostStatusMetadata(e.MetadataJSON, CostStatusNoPricing)
	key := strings.TrimSpace(e.ProviderCode) + "|" + strings.TrimSpace(e.ModelAPIID)
	if _, dup := u.noPricingLogged.LoadOrStore(key, struct{}{}); dup {
		return
	}
	u.lg.Warn("usage event has tokens but no pricing; cost_status=no_pricing",
		loggateway.StepID("usage.no_pricing"),
		loggateway.Str("provider", e.ProviderCode),
		loggateway.Str("model", e.ModelAPIID),
		loggateway.Str("usage_kind", e.UsageKind),
		loggateway.Int("total_tokens", billable))
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

// QuoteTokenUsageCostMicroUSD computes the total cost (micro USD) for the given
// token counts via the same pricing path used when persisting usage events
// (enrichPricing → ApplyTokenUsageCosts). Returns 0 for nil usecase, zero-token
// input, or unpriced provider/model — callers treat 0 as "no cost data", which
// matches the behavior of usage rows recorded without a pricing snapshot.
// Read-only: nothing is persisted (P2-1 TeamRunStep.CostMicroUSD 回填).
func (u *Usecase) QuoteTokenUsageCostMicroUSD(ctx context.Context, prov, mod string, inputTok, outputTok, cachedTok int) int64 {
	if u == nil || (inputTok <= 0 && outputTok <= 0) {
		return 0
	}
	e := &TokenUsageEvent{
		ProviderCode:      strings.TrimSpace(prov),
		ModelAPIID:        strings.TrimSpace(mod),
		InputTokens:       inputTok,
		OutputTokens:      outputTok,
		CachedInputTokens: cachedTok,
	}
	u.enrichPricing(ctx, e)
	return e.TotalCostMicroUSD
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
//
// Cached-input billing semantics (OpenAI/DeepSeek): InputTokens (prompt_tokens)
// already INCLUDES the cache-hit portion. The hit portion must be billed at the
// cache-read price only, never at the full input price. Therefore:
//
//	billableInput = InputTokens - min(CachedInputTokens, InputTokens)
//	InputCost     = billableInput × inputPrice
//	CachedCost    = min(CachedInputTokens, InputTokens) × cacheReadPrice
func ApplyTokenUsageCosts(e *TokenUsageEvent) {
	if e == nil {
		return
	}
	cachedBillable := e.CachedInputTokens
	if cachedBillable > e.InputTokens {
		cachedBillable = e.InputTokens
	}
	if cachedBillable < 0 {
		cachedBillable = 0
	}
	if e.InputCostMicroUSD == 0 && e.InputTokens > 0 {
		e.InputCostMicroUSD = usageCostMicro(e.InputTokens-cachedBillable, e.InputPriceMicroUSDPer1K, e.InputPriceUSDPer1M)
	}
	if e.OutputCostMicroUSD == 0 && e.OutputTokens > 0 {
		e.OutputCostMicroUSD = usageCostMicro(e.OutputTokens, e.OutputPriceMicroUSDPer1K, e.OutputPriceUSDPer1M)
	}
	if e.CachedInputCostMicroUSD == 0 && cachedBillable > 0 {
		e.CachedInputCostMicroUSD = usageCostMicro(cachedBillable, e.CachedInputPriceMicroUSDPer1K, e.CacheReadPriceUSDPer1M)
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
		return int64(tokens) * microPer1K / usageCostMicroDivisor
	}
	return 0
}

// ── Quota management ──────────────────────────────────────────────────────────

// GetQuota returns the quota for a scope.
func (u *Usecase) GetQuota(ctx context.Context, scopeType, scopeID string) (Quota, error) {
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil {
		if stderrors.Is(err, shared.ErrQuotaNotFound) {
			return Quota{}, apierror.NotFound("USAGE_QUOTA", "quota not configured")
		}
		return Quota{}, MapRepoErr(err)
	}
	return q, nil
}

// SetQuota creates or updates a quota.
func (u *Usecase) SetQuota(ctx context.Context, quota Quota) (Quota, error) {
	if strings.TrimSpace(quota.ScopeType) == "" || strings.TrimSpace(quota.ScopeID) == "" {
		return Quota{}, apierror.BadRequest("USAGE_QUOTA", "scope_type and scope_id are required")
	}
	if quota.MonthlyMicroUSD < 0 {
		return Quota{}, apierror.BadRequest("USAGE_QUOTA", "monthly_micro_usd must be >= 0")
	}
	if err := validateQuotaPeriod(quota.PeriodStart, quota.PeriodEnd); err != nil {
		return Quota{}, err
	}
	q, err := u.repo.SetQuota(ctx, quota)
	return q, MapRepoErr(err)
}

// validateQuotaPeriod rejects malformed period bounds. Both empty is allowed
// (the data layer defaults to the current calendar month); exactly one set is
// ambiguous and rejected. When both are set they must be YYYY-MM-DD and
// start <= end.
func validateQuotaPeriod(periodStart, periodEnd string) error {
	start := strings.TrimSpace(periodStart)
	end := strings.TrimSpace(periodEnd)
	if start == "" && end == "" {
		return nil
	}
	if start == "" || end == "" {
		return apierror.BadRequest("USAGE_QUOTA", "period_start and period_end must be set together")
	}
	if _, err := time.Parse("2006-01-02", start); err != nil {
		return apierror.BadRequest("USAGE_QUOTA", "period_start must be YYYY-MM-DD")
	}
	if _, err := time.Parse("2006-01-02", end); err != nil {
		return apierror.BadRequest("USAGE_QUOTA", "period_end must be YYYY-MM-DD")
	}
	if start > end {
		return apierror.BadRequest("USAGE_QUOTA", "period_start must be <= period_end")
	}
	return nil
}

// CheckQuota returns whether another chat turn is allowed under the configured cap.
func (u *Usecase) CheckQuota(ctx context.Context, scopeType, scopeID string) (QuotaCheck, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return QuotaCheck{}, apierror.BadRequest("USAGE_QUOTA", "scope_type and scope_id are required")
	}
	q, err := u.repo.GetQuota(ctx, scopeType, scopeID)
	if err != nil {
		if stderrors.Is(err, shared.ErrQuotaNotFound) {
			return QuotaCheck{Allowed: true, Reason: QuotaCheckReasonNoQuota}, nil
		}
		return QuotaCheck{}, err
	}
	if q.MonthlyMicroUSD <= 0 {
		return QuotaCheck{Quota: q, Allowed: true, Reason: QuotaCheckReasonDisabled}, nil
	}
	q = u.normalizeQuotaPeriod(ctx, q)
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
		check.Reason = QuotaCheckReasonExceeded
		return check, nil
	}
	check.Allowed = true
	check.Reason = QuotaCheckReasonWithin
	return check, nil
}

// normalizeQuotaPeriod lazily rolls an expired monthly period forward to the
// current UTC calendar month (same convention as date_key) and best-effort
// persists it; on persist failure the in-memory period is still used so
// enforcement never silently keeps summing a stale month.
func (u *Usecase) normalizeQuotaPeriod(ctx context.Context, q Quota) Quota {
	if !quotaPeriodExpired(q.PeriodEnd, u.now()) {
		return q
	}
	rolled := q
	rolled.PeriodStart, rolled.PeriodEnd = currentMonthPeriod(u.now())
	if _, err := u.repo.SetQuota(ctx, rolled); err != nil {
		u.lg.Warn("quota period rollover persist failed; enforcing with in-memory period",
			loggateway.Err(err),
			loggateway.Str("scope_type", q.ScopeType),
			loggateway.Str("scope_id", q.ScopeID))
		return rolled
	}
	u.lg.Info("quota period rolled to current month",
		loggateway.Str("scope_type", q.ScopeType),
		loggateway.Str("scope_id", q.ScopeID),
		loggateway.Str("period_start", rolled.PeriodStart),
		loggateway.Str("period_end", rolled.PeriodEnd))
	return rolled
}

// quotaPeriodExpired reports whether the configured period no longer covers
// today (UTC). Empty or malformed periods count as expired so they self-heal.
func quotaPeriodExpired(periodEnd string, now time.Time) bool {
	end := strings.TrimSpace(periodEnd)
	if _, err := time.Parse("2006-01-02", end); err != nil {
		return true
	}
	return end < now.Format("2006-01-02")
}

// currentMonthPeriod returns the first and last day ("YYYY-MM-DD") of the
// calendar month containing now.
func currentMonthPeriod(now time.Time) (string, string) {
	y, m, _ := now.Date()
	start := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, -1)
	return start.Format("2006-01-02"), end.Format("2006-01-02")
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

// CheckTeamMemberQuotas validates that all enabled team members are within quota.
// Returns nil when teamID is empty or no team reader is configured.
// Uses batch queries to avoid N+1 (S3 fix).
func (u *Usecase) CheckTeamMemberQuotas(ctx context.Context, teamID string) error {
	if u == nil || u.teamReader == nil {
		return nil
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil
	}
	agentIDs, err := u.teamReader.EnabledMemberAgentIDs(ctx, teamID)
	if err != nil {
		return err
	}
	if len(agentIDs) == 0 {
		return nil
	}
	// Batch-fetch all active quotas in a single query.
	allQuotas, err := u.repo.ListActiveQuotas(ctx)
	if err != nil {
		return err
	}
	// Filter to agent-scope quotas matching the team members.
	agentIDSet := make(map[string]struct{}, len(agentIDs))
	for _, id := range agentIDs {
		agentIDSet[id] = struct{}{}
	}
	relevant := make([]Quota, 0, len(agentIDs))
	for _, q := range allQuotas {
		if q.ScopeType != "agent" {
			continue
		}
		if _, ok := agentIDSet[q.ScopeID]; !ok {
			continue
		}
		if q.MonthlyMicroUSD <= 0 {
			continue // quota disabled
		}
		relevant = append(relevant, u.normalizeQuotaPeriod(ctx, q))
	}
	if len(relevant) == 0 {
		return nil // no active quotas for these agents
	}
	// Batch-sum spent cost for all relevant quotas in one query per scope type.
	spentMap, err := u.repo.BatchSumScopeCost(ctx, relevant)
	if err != nil {
		return err
	}
	for _, q := range relevant {
		key := q.ScopeType + ":" + q.ScopeID
		spent := spentMap[key]
		if spent >= q.MonthlyMicroUSD {
			return apierror.Forbidden("USAGE_QUOTA",
				"成员 Agent「%s」本月用量已达配额上限（已消耗 $%.2f / 上限 $%.2f），团队运行已被拦截",
				q.ScopeID, float64(spent)/1e6, float64(q.MonthlyMicroUSD)/1e6)
		}
	}
	return nil
}

// enforceQuota blocks when the scope monthly cap is exceeded (no-op if quota unset).
func (u *Usecase) enforceQuota(ctx context.Context, scopeType, scopeID string) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil
	}
	check, err := u.CheckQuota(ctx, scopeType, scopeID)
	if err != nil {
		return err
	}
	if !check.Allowed {
		return apierror.Forbidden("USAGE_QUOTA", QuotaExceededMessage(check))
	}
	return nil
}

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
	usageID := newUsageID()
	ev := TokenUsageEvent{
		ID:                usageID,
		SessionID:         in.SessionID,
		AgentKey:          in.AgentKey,
		AgentID:           in.AgentID,
		ModelAPIID:        in.Model,
		ModelDisplayName:  in.Model,
		ProviderCode:      in.Provider,
		InputTokens:       in.PromptTok,
		OutputTokens:      in.CompletionTok,
		CachedInputTokens: in.CachedTok,
		TotalTokens:       in.PromptTok + in.CompletionTok,
		LatencyMS:         int(in.Latency.Milliseconds()),
		Status:            in.Status,
		UsageKind:         KindChatTurn,
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
	UsageSource  string
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
	q = u.normalizeQuotaPeriod(ctx, q)
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
		if !a.Enabled || a.AlertRatio <= 0 || util+floatCompareEpsilon < a.AlertRatio {
			continue
		}
		if u.alertRecentlyFired(a, now) {
			continue
		}
		if err := u.alertNotifier.NotifyBudgetAlert(ctx, a, spent, q.MonthlyMicroUSD, util); err != nil {
			continue
		}
		if err := u.repo.UpdateBudgetAlertLastFired(ctx, a.ID, now.Format(time.RFC3339)); err != nil {
			u.lg.Warn("update budget alert last fired failed", loggateway.Err(err), loggateway.Str("alert_id", a.ID))
		}
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
		return BudgetAlert{}, apierror.BadRequest("USAGE_ALERT", "scope_type and scope_id are required")
	}
	if alert.AlertRatio <= 0 || alert.AlertRatio > 1 {
		return BudgetAlert{}, apierror.BadRequest("USAGE_ALERT", "alert_ratio must be in (0,1]")
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
		u.lg.Warn("quota_dashboard.batch_failed", loggateway.StepID("usage"), loggateway.Err(batchErr))
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
	q := withLimit(u.normalizeQuery(query, u.now()), inefficientModelLimit)
	var rows []BreakdownRow
	var err error
	// Use daily rollup for historical ranges (consistent with Overview); fall back
	// to real-time when TeamID is set (daily table has no team_id column).
	if u.dailySupported(q) && q.EndDate < dateKey(u.now()) {
		rows, err = u.repo.ListTopModelUsageFromDaily(ctx, q)
	} else {
		rows, err = u.repo.ListTopModelUsage(ctx, q)
	}
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
		if len(out) >= inefficientResultLimit {
			break
		}
	}
	return out, nil
}

// ── CSV export ────────────────────────────────────────────────────────────────

// ExportUsageEventsCSV returns CSV rows for usage events.
func (u *Usecase) ExportUsageEventsCSV(ctx context.Context, query Query) (string, error) {
	if query.Limit <= 0 || query.Limit > csvExportMaxRows {
		query.Limit = csvExportMaxRows
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
