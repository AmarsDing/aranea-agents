package subagent

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"

	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

const (
	runtimeStateSubagentRun      = "openclaw.subagent.run"
	runtimeStateSubagentRunID    = "openclaw.subagent.run_id"
	runtimeStateSubagentParentID = "openclaw.subagent.parent_session_id"

	subagentSessionPrefix = "subagent:"
	subagentRequestPrefix = "subagent:"
	subagentDirName       = "subagents"
	subagentRunsFileName  = "runs.json"

	defaultStoredResultRunes  = 4000
	defaultStoredSummaryRunes = 240

	defaultMaxConcurrentSubAgents = 5

	// envSubagentMaxConcurrency is the environment variable used to override
	// the default subagent concurrency limit. When set to a positive integer,
	// it replaces defaultMaxConcurrentSubAgents. P1 fix (2026-06-18).
	envSubagentMaxConcurrency = "ARANEA_SUBAGENT_MAX_CONCURRENCY"

	// 包C C4-②（2026-08-28，S07 subagent 治理空白补齐）：subagent token
	// 预算闸。此前 subagent 路径无任何 token 预算（team runner 的
	// accumulateRunTokenBudget 只覆盖 team run），runaway run 只能靠
	// timeout 兜底。两级闸与 team 路径口径对等：
	//   - run 级：单 subagent run 累计 input token 中流跳闸（默认 50 万，
	//     健康重型 run 约 27-32K 的 ~15 倍余量，只拦失控）；
	//   - 父会话级：同一父会话全部 spawn 合计 input 预算（默认 150 万，
	//     对齐 DefaultTeamRunInputTokenBudget——spawn 集合即 team run 的
	//     类比物），Spawn 时拒绝。
	// env 覆盖语义同 team.TokenBudgetInputTokens：>0 覆盖默认、<0 禁用、
	// 0/未设/非法用默认。
	defaultRunTokenBudgetInputTokens    int64 = 500_000
	defaultParentTokenBudgetInputTokens int64 = 1_500_000
	envSubagentRunTokenBudget                 = "ARANEA_SUBAGENT_RUN_TOKEN_BUDGET"
	envSubagentParentTokenBudget              = "ARANEA_SUBAGENT_PARENT_TOKEN_BUDGET"

	// BudgetScopeRun / BudgetScopeParentAggregate 标识预算跳闸作用域。
	BudgetScopeRun             = "run"
	BudgetScopeParentAggregate = "parent_aggregate"

	// Notification limits for outbound completion messages.
	notifyMaxSummaryRunes = 200
	notifyTimeoutSec      = 5

	toolSubagentsSpawn  = "subagents_spawn"
	toolSubagentsList   = "subagents_list"
	toolSubagentsGet    = "subagents_get"
	toolSubagentsCancel = "subagents_cancel"

	argTask           = "task"
	argID             = "id"
	argTimeoutSeconds = "timeout_seconds"
	argKind           = "kind"
	argSubagentType   = "subagent_type"
	argBlockUntilMS   = "block_until_ms"
	argFullResult     = "full_result"

	schemaTypeObject  = "object"
	schemaTypeString  = "string"
	schemaTypeInteger = "integer"
	schemaTypeBoolean = "boolean"

	subagentRunPrompt = "You are running as a background " +
		"subagent. Complete the delegated task once. The parent " +
		"chat will receive your final result automatically. Keep " +
		"the result concise and action-oriented. Do not return " +
		"only a statement of what you will do; complete the " +
		"task and report the result or exact blocker. Do not " +
		"spawn more subagents from inside this subagent."
)

type SpawnRequest struct {
	OwnerUserID     string
	ParentSessionID string
	Task            string
	TimeoutSeconds  int
	// Kind selects the injected system prompt: explore | verify | general.
	Kind string
	// ResultRunes is the max rune count for stored subagent results.
	// When <= 0, defaultStoredResultRunes (4000) is used.
	ResultRunes int
	// SummaryRunes is the max rune count for stored subagent summaries.
	// When <= 0, defaultStoredSummaryRunes (240) is used.
	SummaryRunes int
}

// ModeBStartedInfo is emitted when a background subagent child session starts
// (Mode B agent-card: orphan MemberSession with empty TeamRunID).
type ModeBStartedInfo struct {
	RunID           string
	ParentSessionID string
	ChildSessionID  string
	Task            string
}

// ModeBFinishedInfo is emitted when a Mode B subagent run reaches a terminal status.
type ModeBFinishedInfo struct {
	RunID          string
	ChildSessionID string
	Status         string // completed | failed | cancelled
	Error          string
}

// ModeBStartedHook is invoked after a subagent child session is allocated.
// Implementations must be non-blocking / best-effort.
type ModeBStartedHook func(ctx context.Context, info ModeBStartedInfo)

// ModeBFinishedHook is invoked after a subagent run finishes.
type ModeBFinishedHook func(ctx context.Context, info ModeBFinishedInfo)

// BudgetTripInfo 是 subagent token 预算跳闸事件（包C C4-②）。Scope 取
// BudgetScopeRun（单 run 中流跳闸，run 被取消）或
// BudgetScopeParentAggregate（父会话 spawn 合计预算，Spawn 被拒绝）。
type BudgetTripInfo struct {
	RunID             string
	ParentSessionID   string
	Scope             string
	UsedInputTokens   int64
	BudgetInputTokens int64
}

// BudgetTripHook is invoked once per budget trip (best-effort, non-blocking).
// Wired at the service layer to emit decision-records gate events, mirroring
// the team runner's TriggerTokenBudgetTripped double-write.
type BudgetTripHook func(ctx context.Context, info BudgetTripInfo)

// UsageRecorder records auxiliary (non-chat-turn) LLM usage. Implemented by
// *biz.UsageUsecase; injected via SetUsageRecorder (P1-2, 2026-08-19).
type UsageRecorder interface {
	RecordAuxLLMUsage(ctx context.Context, in biz.AuxLLMUsageInput) error
}

// UsageRecorderFunc adapts a function to UsageRecorder.
type UsageRecorderFunc func(ctx context.Context, in biz.AuxLLMUsageInput) error

// RecordAuxLLMUsage implements UsageRecorder.
func (f UsageRecorderFunc) RecordAuxLLMUsage(ctx context.Context, in biz.AuxLLMUsageInput) error {
	return f(ctx, in)
}

// runAttribution snapshots the provider/model/agent identity of the parent
// turn at Spawn time, so the (asynchronous) subagent run's token usage is
// billed to the right model even after another session's turn has rotated
// the service's current runner/attribution.
type runAttribution struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	AgentID  string `json:"agent_id,omitempty"`
	AgentKey string `json:"agent_key,omitempty"`
}

// turnRunnerContext binds the spawning turn's immutable runner + billing
// attribution to one parent session (E2E-H4). Written wholesale under s.mu
// by SetTurnContext at the orchestrator's buildTurn; snapshotted into each
// runRecord at Spawn. Never mutated in place — per-turn injection replaces
// the old service-wide last-writer-wins singleton (data race + wrong-agent
// identity + billing attribution divergence).
type turnRunnerContext struct {
	runner      trpcrunner.Runner
	attribution runAttribution
}

// runesManager manages per-session rune limits for subagent results.
// Extracted from Service to reduce cognitive complexity (AS-COG-02).
type runesManager struct {
	store sync.Map // map[string]runesConfig
}

func newRunesManager() *runesManager {
	return &runesManager{}
}

func (rm *runesManager) Set(sessionID string, resultRunes, summaryRunes int) {
	if sessionID != "" {
		rm.store.Store(sessionID, runesConfig{Result: resultRunes, Summary: summaryRunes})
	}
}

func (rm *runesManager) Remove(sessionID string) {
	if sessionID != "" {
		rm.store.Delete(sessionID)
	}
}

func (rm *runesManager) Get(sessionID string) (int, int) {
	if sessionID == "" {
		return 0, 0
	}
	if v, ok := rm.store.Load(sessionID); ok {
		cfg := v.(runesConfig)
		return cfg.Result, cfg.Summary
	}
	return 0, 0
}

type runningRun struct {
	cancel          context.CancelFunc
	skipNotify      bool
	cancelRequested bool
	// cancelReason 区分取消来源（用户取消 vs 预算跳闸）：预算跳闸时
	// 写入跳闸说明，finishRun 用它生成 Summary（C4-②）。
	cancelReason string
	childSession string
	requestID    string
	startedAt    time.Time
}

type runRecord struct {
	trpcsubagent.Run
	OwnerUserID string `json:"owner_user_id,omitempty"`
	// resultRunes is the max rune count for stored subagent results.
	// When <= 0, defaultStoredResultRunes is used.
	ResultRunes int `json:"result_runes,omitempty"`
	// summaryRunes is the max rune count for stored subagent summaries.
	// When <= 0, defaultStoredSummaryRunes is used.
	SummaryRunes int `json:"summary_runes,omitempty"`
	// attribution snapshots the parent turn's provider/model/agent at Spawn
	// time, for aux usage billing of the asynchronous run (P1-2).
	Attribution runAttribution `json:"attribution,omitempty"`
	// runner is the SPAWNING turn's runner, captured at Spawn under s.mu and
	// immutable for the run's lifetime (E2E-H4). In-memory only: reloaded
	// records have nil, but normalizeLoadedRuns already fails all
	// non-terminal runs on boot, so no loaded record ever reaches runChild.
	runner trpcrunner.Runner `json:"-"`
}

type storeFile struct {
	Version int         `json:"version"`
	Runs    []runRecord `json:"runs,omitempty"`
}

// runesConfig holds per-session rune limits for subagent results.
type runesConfig struct {
	Result  int
	Summary int
}
