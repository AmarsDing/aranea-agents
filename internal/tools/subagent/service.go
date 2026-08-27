package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"aranea-agents/pkg/apierror"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/outbound"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
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
	RunID              string
	ParentSessionID    string
	Scope              string
	UsedInputTokens    int64
	BudgetInputTokens  int64
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

type Service struct {
	path           string
	runner         trpcrunner.Runner
	lg             loggateway.Logger
	outboundRouter *outbound.Router // optional: for completion notifications
	usageRecorder  UsageRecorder    // optional: nil skips aux usage recording
	attribution    runAttribution   // latest turn's attribution (set alongside SetRunner)

	runes *runesManager

	clock func() time.Time

	mu      sync.RWMutex
	runs    map[string]*runRecord
	running map[string]*runningRun

	persistMu sync.Mutex

	startOnce sync.Once
	baseCtx   context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// maxConcurrent is the upper bound on simultaneously running subagents.
	// Defaults to defaultMaxConcurrentSubAgents; overridable via
	// ARANEA_SUBAGENT_MAX_CONCURRENCY env var (P1 fix 2026-06-18).
	maxConcurrent int

	// onModeBStarted publishes orphan MemberSession cards for Mode B UI.
	// Optional; nil disables Mode B projection.
	onModeBStarted  ModeBStartedHook
	onModeBFinished ModeBFinishedHook

	// 包C C4-② 预算闸配置与状态。runBudgetInputTokens 是单 run 累计
	// input 预算（中流跳闸）；parentBudgetInputTokens 是父会话 spawn
	// 合计 input 预算（Spawn 拒绝）；<0 各自禁用。
	runBudgetInputTokens    int64
	parentBudgetInputTokens int64
	// parentInputTokens 按父会话累计 spawn 的 input 消耗（流式增量记账）。
	// 与 team runner 的内存预算表同型：进程级、best-effort，重启清零。
	parentInputTokens map[string]int64
	onBudgetTrip      BudgetTripHook
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
}

type storeFile struct {
	Version int         `json:"version"`
	Runs    []runRecord `json:"runs,omitempty"`
}

func NewService(stateDir string, r trpcrunner.Runner, lg loggateway.Logger) (*Service, error) {
	if strings.TrimSpace(stateDir) == "" {
		return nil, apierror.Internal(apierror.DomainSubagent, "empty state dir")
	}

	path := filepath.Join(
		strings.TrimSpace(stateDir),
		subagentDirName,
		subagentRunsFileName,
	)
	runs, err := loadRuns(path, lg)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainSubagent, "load runs: "+err.Error())
	}

	svc := &Service{
		path:                  path,
		runner:                r,
		lg:                    lg,
		runes:                 newRunesManager(),
		clock:                 time.Now,
		runs:                  runs,
		running:               make(map[string]*runningRun),
		maxConcurrent:         resolveSubagentMaxConcurrency(),
		runBudgetInputTokens:  resolveSubagentTokenBudget(envSubagentRunTokenBudget, defaultRunTokenBudgetInputTokens),
		parentBudgetInputTokens: resolveSubagentTokenBudget(envSubagentParentTokenBudget, defaultParentTokenBudgetInputTokens),
		parentInputTokens:     make(map[string]int64),
	}
	if normalizeLoadedRuns(svc.runs, svc.clock()) {
		if err := svc.persist(); err != nil {
			return nil, apierror.Internal(apierror.DomainSubagent, "persist: "+err.Error())
		}
	}
	return svc, nil
}

// SetRunner sets the runner for subagent execution. This allows deferred
// runner initialization when the runner cannot be provided at construction time.
func (s *Service) SetRunner(r trpcrunner.Runner) {
	if s != nil {
		s.mu.Lock()
		s.runner = r
		s.mu.Unlock()
	}
}

// SetUsageRecorder injects the aux-usage recorder (P1-2, 2026-08-19).
// Safe to call before Start; nil disables recording.
func (s *Service) SetUsageRecorder(ur UsageRecorder) {
	if s != nil {
		s.mu.Lock()
		s.usageRecorder = ur
		s.mu.Unlock()
	}
}

// SetAttribution snapshots the current turn's provider/model/agent identity.
// Must be called alongside SetRunner (same turn-build site): subagent runs
// execute asynchronously, so billing attribution is captured at Spawn time
// from this snapshot (last-writer-wins, same semantics as the runner itself).
func (s *Service) SetAttribution(provider, model, agentID, agentKey string) {
	if s != nil {
		s.mu.Lock()
		s.attribution = runAttribution{
			Provider: strings.TrimSpace(provider),
			Model:    strings.TrimSpace(model),
			AgentID:  strings.TrimSpace(agentID),
			AgentKey: strings.TrimSpace(agentKey),
		}
		s.mu.Unlock()
	}
}

// SetModeBStartedHook registers a best-effort callback for Mode B agent-card
// projection (orphan MemberSession). Safe to call before Start.
func (s *Service) SetModeBStartedHook(hook ModeBStartedHook) {
	if s != nil {
		s.mu.Lock()
		s.onModeBStarted = hook
		s.mu.Unlock()
	}
}

// SetModeBFinishedHook registers a best-effort callback when a Mode B run ends.
func (s *Service) SetModeBFinishedHook(hook ModeBFinishedHook) {
	if s != nil {
		s.mu.Lock()
		s.onModeBFinished = hook
		s.mu.Unlock()
	}
}

// SetBudgetTripHook registers the budget-trip callback (C4-② decision-record
// wiring). Safe to call before Start; nil disables emission (the gate still
// cancels/rejects — only the audit double-write is skipped).
func (s *Service) SetBudgetTripHook(hook BudgetTripHook) {
	if s != nil {
		s.mu.Lock()
		s.onBudgetTrip = hook
		s.mu.Unlock()
	}
}

// WithOutboundRouter sets the outbound router for completion notifications.
// Must be called before Start(); not safe for concurrent use after startup.
func (s *Service) WithOutboundRouter(router *outbound.Router) *Service {
	if s != nil {
		s.mu.Lock()
		s.outboundRouter = router
		s.mu.Unlock()
	}
	return s
}

// resolveSubagentMaxConcurrency reads ARANEA_SUBAGENT_MAX_CONCURRENCY and
// returns the effective concurrency limit. Falls back to
// defaultMaxConcurrentSubAgents when unset or invalid. P1 fix (2026-06-18).
func resolveSubagentMaxConcurrency() int {
	v := strings.TrimSpace(os.Getenv(envSubagentMaxConcurrency))
	if v == "" {
		return defaultMaxConcurrentSubAgents
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return defaultMaxConcurrentSubAgents
	}
	return n
}

// resolveSubagentTokenBudget reads an env override for one token-budget gate
// (C4-②). Semantics mirror team.TokenBudgetInputTokens: unset/invalid/0 →
// def; >0 → override; <0 → gate disabled.
func resolveSubagentTokenBudget(env string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(env))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n == 0 {
		return def
	}
	return n
}

// runesConfig holds per-session rune limits for subagent results.
type runesConfig struct {
	Result  int
	Summary int
}

// SetSessionRunes registers per-session rune limits for subagent results.
// This replaces the old SetStoredResultRunes/SetStoredSummaryRunes setters
// which were unsafe on a singleton service (race condition across agents).
func (s *Service) SetSessionRunes(sessionID string, resultRunes, summaryRunes int) {
	if s != nil {
		s.runes.Set(sessionID, resultRunes, summaryRunes)
	}
}

// RemoveSessionRunes removes per-session rune limits for a session.
func (s *Service) RemoveSessionRunes(sessionID string) {
	if s != nil {
		s.runes.Remove(sessionID)
	}
}

// getSessionRunes returns the per-session rune limits for a session.
// Returns (0, 0) if no config is registered for the session.
func (s *Service) getSessionRunes(sessionID string) (int, int) {
	if s == nil {
		return 0, 0
	}
	return s.runes.Get(sessionID)
}

func (s *Service) Start(ctx context.Context) {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		s.baseCtx, s.cancel = context.WithCancel(ctx)
	})
}

func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.stopAllRunning()
	s.wg.Wait()
	return s.persist()
}

func (s *Service) Spawn(ctx context.Context, req SpawnRequest) (trpcsubagent.Run, error) {
	if s == nil {
		return trpcsubagent.Run{}, apierror.Internal(apierror.DomainSubagent, "nil service")
	}
	if s.baseCtx == nil {
		return trpcsubagent.Run{}, apierror.Internal(apierror.DomainSubagent, "not started")
	}

	nested, _ := trpcagent.GetRuntimeStateValueFromContext[bool](ctx, runtimeStateSubagentRun)
	if nested {
		return trpcsubagent.Run{}, apierror.BadRequest(apierror.DomainSubagent, "nested subagent spawn is not allowed")
	}

	// Concurrency limit: check and reserve slot atomically to avoid TOCTOU race.
	s.mu.Lock()
	if s.runner == nil {
		s.mu.Unlock()
		return trpcsubagent.Run{}, apierror.Internal(apierror.DomainSubagent, "runner not configured")
	}
	if len(s.running) >= s.maxConcurrent {
		// WP-2b: surface the active run ids so the caller (LLM) can track them
		// via subagents_get instead of blind-retrying. Sorted for determinism.
		// CodeRateLimit is classified deterministic by retry_reflect (WP-2c):
		// it propagates this error untouched without burning retry budget.
		active := make([]string, 0, len(s.running))
		for id := range s.running {
			active = append(active, id)
		}
		sort.Strings(active)
		s.mu.Unlock()
		return trpcsubagent.Run{}, apierror.RateLimit(apierror.DomainSubagent, fmt.Sprintf("too many concurrent sub-agents (limit: %d); active run ids: [%s] — track them with subagents_get and wait for completion notifications instead of retrying immediately", s.maxConcurrent, strings.Join(active, ", ")))
	}

	if err := validateSpawnRequest(req); err != nil {
		s.mu.Unlock()
		return trpcsubagent.Run{}, err
	}

	// 父会话 spawn 合计预算闸（C4-②）：该会话历史 spawn 的 input 消耗已
	// 达上限时拒绝新 spawn——否则「每次 spawn 都合法、合计失控」的
	// S02/S07 型盲区在 subagent 路径重演。RateLimit 与并发闸同型：
	// retry_reflect 判确定性错误，不烧重试配额。
	if s.parentBudgetInputTokens > 0 {
		parentSID := strings.TrimSpace(req.ParentSessionID)
		if used := s.parentInputTokens[parentSID]; used >= s.parentBudgetInputTokens {
			trip := BudgetTripInfo{
				ParentSessionID:   parentSID,
				Scope:             BudgetScopeParentAggregate,
				UsedInputTokens:   used,
				BudgetInputTokens: s.parentBudgetInputTokens,
			}
			s.mu.Unlock()
			s.emitBudgetTrip(context.Background(), trip)
			return trpcsubagent.Run{}, apierror.RateLimit(apierror.DomainSubagent, fmt.Sprintf("parent session subagent input-token budget exhausted (used %d, budget %d) — do NOT spawn more subagents; compose the answer from runs already available via subagents_list/subagents_get", used, s.parentBudgetInputTokens))
		}
	}

	record := s.newRunRecord(req)
	s.runs[record.ID] = record
	view := record.publicView()
	s.mu.Unlock()

	if err := s.persist(); err != nil {
		s.mu.Lock()
		delete(s.runs, record.ID)
		s.mu.Unlock()
		return trpcsubagent.Run{}, apierror.Internal(apierror.DomainSubagent, "persist: "+err.Error())
	}

	s.wg.Add(1)
	safego.Go(s.baseCtx, "subagent.execute", func() {
		defer s.wg.Done()
		s.execute(s.baseCtx, record.ID, req.TimeoutSeconds)
	})

	return view, nil
}

// validateSpawnRequest checks required fields in a spawn request.
func validateSpawnRequest(req SpawnRequest) error {
	if strings.TrimSpace(req.OwnerUserID) == "" {
		return apierror.BadRequest(apierror.DomainSubagent, "empty owner")
	}
	if strings.TrimSpace(req.ParentSessionID) == "" {
		return apierror.BadRequest(apierror.DomainSubagent, "empty parent session id")
	}
	if strings.TrimSpace(req.Task) == "" {
		return apierror.BadRequest(apierror.DomainSubagent, "empty task")
	}
	return nil
}

// newRunRecord creates a runRecord from a spawn request with resolved rune limits.
func (s *Service) newRunRecord(req SpawnRequest) *runRecord {
	now := s.clock()
	resultRunes := req.ResultRunes
	summaryRunes := req.SummaryRunes
	if resultRunes <= 0 || summaryRunes <= 0 {
		if sr, ssr := s.getSessionRunes(strings.TrimSpace(req.ParentSessionID)); sr > 0 || ssr > 0 {
			if resultRunes <= 0 {
				resultRunes = sr
			}
			if summaryRunes <= 0 {
				summaryRunes = ssr
			}
		}
	}
	if resultRunes <= 0 {
		resultRunes = defaultStoredResultRunes
	}
	if summaryRunes <= 0 {
		summaryRunes = defaultStoredSummaryRunes
	}
	return &runRecord{
		Run: trpcsubagent.Run{
			ID:              uuid.NewString(),
			ParentSessionID: strings.TrimSpace(req.ParentSessionID),
			Task:            strings.TrimSpace(req.Task),
			Kind:            normalizeKind(req.Kind),
			Status:          trpcsubagent.StatusQueued,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		OwnerUserID:  strings.TrimSpace(req.OwnerUserID),
		ResultRunes:  resultRunes,
		SummaryRunes: summaryRunes,
		// P1-2: snapshot billing attribution now — the run executes
		// asynchronously and the service-level attribution may have rotated
		// to another session's turn by then. Caller (Spawn) holds s.mu.
		Attribution: s.attribution,
	}
}

func (s *Service) ListForUser(userID string, filter trpcsubagent.ListFilter) []trpcsubagent.Run {
	if s == nil {
		return nil
	}
	userID = strings.TrimSpace(userID)
	parentSessionID := strings.TrimSpace(filter.ParentSessionID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := make([]trpcsubagent.Run, 0, len(s.runs))
	for _, item := range s.runs {
		if item == nil || item.OwnerUserID != userID {
			continue
		}
		if parentSessionID != "" && item.ParentSessionID != parentSessionID {
			continue
		}
		runs = append(runs, item.publicView())
	}
	sort.Slice(runs, func(i int, j int) bool {
		return runs[i].UpdatedAt.After(runs[j].UpdatedAt)
	})
	return runs
}

func (s *Service) GetForUser(userID string, runID string) (*trpcsubagent.Run, error) {
	record, err := s.runForUser(userID, runID)
	if err != nil {
		return nil, err
	}
	view := record.publicView()
	return &view, nil
}

const subagentWaitPoll = 200 * time.Millisecond

// WaitForUser polls GetForUser until the run is terminal or wait elapses.
func (s *Service) WaitForUser(ctx context.Context, userID, runID string, wait time.Duration) (*trpcsubagent.Run, error) {
	run, err := s.GetForUser(userID, runID)
	if err != nil || run == nil || wait <= 0 || run.Status.IsTerminal() {
		return run, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.Now().Add(wait)
	ticker := time.NewTicker(subagentWaitPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return run, ctx.Err()
		case <-ticker.C:
			run, err = s.GetForUser(userID, runID)
			if err != nil || run == nil || run.Status.IsTerminal() {
				return run, err
			}
			if !time.Now().Before(deadline) {
				return run, nil
			}
		}
	}
}

func enrichRunView(run trpcsubagent.Run, now time.Time) map[string]any {
	b, err := json.Marshal(run)
	if err != nil {
		return map[string]any{"id": run.ID, "status": run.Status, "kind": run.Kind}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{"id": run.ID, "status": run.Status, "kind": run.Kind}
	}
	start := run.CreatedAt
	if run.StartedAt != nil && !run.StartedAt.IsZero() {
		start = *run.StartedAt
	}
	if !start.IsZero() && (run.Status == trpcsubagent.StatusQueued || run.Status == trpcsubagent.StatusRunning) {
		ms := now.Sub(start).Milliseconds()
		if ms < 0 {
			ms = 0
		}
		m["running_for_ms"] = ms
	}
	return m
}

// C4-③ 上行交付物压缩（2026-08-28，S07 570K 根修）：subagent 交付物回灌
// 父 LLM 上下文只经 subagents_get/subagents_list 工具结果——工具结果永久
// 留在父会话历史、后续每轮 LLM 全量重发（二次累积），是 S07 主上下文
// 570K 的主因。与 team 路径 ClipUpwardPayload 对齐（biz.UpwardPipeMaxRunes
// =2000 runes）：
//   - get 默认返回 ≤2KB 截断视图 + 截断标记（告知模型全量可取，防把截断
//     误当完整交付物）；full_result=true 返回存储全量（一次性 deliberate
//     拉取的逃生舱，常量 argFullResult 此前已声明未接线，本次接通）；
//   - list 剥离 result 只留 summary——list 是跟踪视图，取交付物走 get。
// 存储层不动：ResultRunes（默认 4000）保持全保真，压缩只发生在回灌边界。

// upwardTruncatedMarker 告知模型结果被截断及取全量的方式。
const upwardTruncatedMarker = "\n…[truncated: full result %d runes — re-call subagents_get with full_result=true]"

// clipUpwardResultView 把 get 视图的 result 裁剪到 ≤biz.UpwardPipeMaxRunes
// 并附截断标记与元数据（result_truncated/result_full_runes）。
func clipUpwardResultView(view map[string]any) {
	raw, ok := view["result"].(string)
	if !ok || raw == "" {
		return
	}
	total := utf8.RuneCountInString(raw)
	if total <= biz.UpwardPipeMaxRunes {
		return
	}
	view["result"] = biz.ClipUpwardPayload(raw) + fmt.Sprintf(upwardTruncatedMarker, total)
	view["result_truncated"] = true
	view["result_full_runes"] = total
}

func (s *Service) CancelForUser(userID string, runID string) (*trpcsubagent.Run, bool, error) {
	record, err := s.runForUser(userID, runID)
	if err != nil {
		return nil, false, err
	}

	s.mu.Lock()
	current := s.runs[record.ID]
	if current == nil {
		s.mu.Unlock()
		return nil, false, trpcsubagent.ErrRunNotFound
	}
	if current.Status.IsTerminal() {
		view := current.publicView()
		s.mu.Unlock()
		return &view, false, nil
	}

	now := s.clock()
	current.Status = trpcsubagent.StatusCanceled
	current.Error = ""
	current.Summary = summarizeResult("canceled", 0)
	current.UpdatedAt = now
	current.FinishedAt = cloneTime(now)

	if running := s.running[current.ID]; running != nil {
		running.skipNotify = true
		running.cancelRequested = true
		if running.cancel != nil {
			running.cancel()
		}
	}
	view := current.publicView()
	s.mu.Unlock()

	if err := s.persist(); err != nil {
		return nil, false, apierror.Internal(apierror.DomainSubagent, "persist: "+err.Error())
	}
	return &view, true, nil
}

func (s *Service) FrameworkTools() []trpctool.Tool {
	svc := s
	return []trpctool.Tool{
		newSpawnTool(svc),
		newListTool(svc),
		newGetTool(svc),
		newCancelTool(svc),
	}
}

func (s *Service) execute(parent context.Context, runID string, timeoutSeconds int) {
	record, runCtx, started, err := s.markRunning(parent, runID, timeoutSeconds)
	if err != nil {
		return
	}
	if started.cancel != nil {
		defer started.cancel()
	}

	result := replyAccumulator{}
	usage := &usageAccum{}
	guard := &runBudgetGuard{svc: s, runID: record.ID, parentSID: record.ParentSessionID}
	runErr := s.runChild(runCtx, record, started, &result, usage, guard)
	output := sanitizeStoredResult(result.text, record.ResultRunes)
	s.finishRun(runID, output, runErr, record.SummaryRunes)
	s.recordRunUsage(record, started, usage, runErr, guard.tripReason)
}

// runBudgetGuard 是单 run 的流式预算哨兵（C4-②）：每个 usage 事件后
// 核对累计 input token——超阈则置取消原因并取消 run ctx（只触发一次，
// 触发后继续排空事件流，避免生产者阻塞），同时把增量记账到父会话合计
// （供 Spawn 时的 aggregate 闸）。与 team runner
// accumulateRunTokenBudgetFromStream 同型。
type runBudgetGuard struct {
	svc       *Service
	runID     string
	parentSID string

	lastPromptTotal int
	tripped         bool
	tripReason      string
}

func (g *runBudgetGuard) observe(a *usageAccum) {
	if g == nil || g.svc == nil || a == nil {
		return
	}
	prompt, _, _ := a.totals()
	delta := prompt - g.lastPromptTotal
	if delta < 0 {
		delta = 0
	}
	g.lastPromptTotal = prompt

	var cancelFn context.CancelFunc
	var trip *BudgetTripInfo
	g.svc.mu.Lock()
	if delta > 0 && g.parentSID != "" {
		g.svc.parentInputTokens[g.parentSID] += int64(delta)
	}
	if !g.tripped && g.svc.runBudgetInputTokens > 0 && int64(prompt) > g.svc.runBudgetInputTokens {
		g.tripped = true
		g.tripReason = fmt.Sprintf("subagent input-token budget tripped (used %d, budget %d)", prompt, g.svc.runBudgetInputTokens)
		if r := g.svc.running[g.runID]; r != nil {
			r.cancelRequested = true
			r.cancelReason = g.tripReason
			cancelFn = r.cancel
		}
		trip = &BudgetTripInfo{
			RunID:             g.runID,
			ParentSessionID:   g.parentSID,
			Scope:             BudgetScopeRun,
			UsedInputTokens:   int64(prompt),
			BudgetInputTokens: g.svc.runBudgetInputTokens,
		}
	}
	g.svc.mu.Unlock()
	if cancelFn != nil {
		cancelFn()
	}
	if trip != nil {
		g.svc.emitBudgetTrip(context.Background(), *trip)
	}
}

// emitBudgetTrip 是预算跳闸的审计出口（C4-②）：Warn 日志 + onBudgetTrip
// 钩子（service 层接线 decision-records 双写，对齐 team
// tripRunTokenBudget 的 EmitGate 四件套中的决策落账）。best-effort：
// 钩子 nil 时仅日志，闸本身的取消/拒绝不依赖钩子可用性。
func (s *Service) emitBudgetTrip(ctx context.Context, info BudgetTripInfo) {
	if s == nil {
		return
	}
	s.lg.Warn("subagent input-token 预算跳闸",
		loggateway.StepID("subagent.token_budget.tripped"),
		loggateway.Str("run_id", info.RunID),
		loggateway.Str("parent_session_id", info.ParentSessionID),
		loggateway.Str("scope", info.Scope),
		loggateway.Int64("budget_used_input_tokens", info.UsedInputTokens),
		loggateway.Int64("budget_limit_input_tokens", info.BudgetInputTokens),
	)
	s.mu.RLock()
	hook := s.onBudgetTrip
	s.mu.RUnlock()
	if hook != nil {
		hook(ctx, info)
	}
}

// recordRunUsage persists the subagent run's LLM token usage as an
// aux_subagent usage event (P1-2, 2026-08-19). Zero-token runs (spawn
// failures, provider returned no usage) are skipped: nothing observable was
// consumed. Best-effort: failures are Warn-logged, never fatal.
// tripReason 非空时写入 usage 行 ErrMsg（预算跳闸留痕，C4-②）——跳闸
// 路径 runErr=context.Canceled，仅靠 status=cancelled 无法与用户主动
// 取消区分。
func (s *Service) recordRunUsage(record *runRecord, started runningRun, usage *usageAccum, runErr error, tripReason string) {
	if s == nil || record == nil || usage == nil {
		return
	}
	s.mu.RLock()
	rec := s.usageRecorder
	s.mu.RUnlock()
	if rec == nil {
		return
	}
	promptTok, completionTok, cachedTok := usage.totals()
	if promptTok <= 0 && completionTok <= 0 {
		return
	}
	status := "success"
	var errMsg string
	switch {
	case errors.Is(runErr, context.Canceled):
		status = "cancelled"
	case errors.Is(runErr, context.DeadlineExceeded):
		status = "timeout"
	case runErr != nil:
		status = "failed"
		errMsg = runErr.Error()
	}
	if tripReason != "" {
		errMsg = tripReason
	}
	attr := record.Attribution
	// Prefer the provider-reported model id (authoritative for what actually
	// served the run); fall back to the Spawn-time attribution snapshot.
	model := usage.model
	if model == "" {
		model = attr.Model
	}
	latency := time.Duration(0)
	if !started.startedAt.IsZero() {
		latency = s.clock().Sub(started.startedAt)
	}
	if err := rec.RecordAuxLLMUsage(context.Background(), biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxSubagent,
		SessionID:     record.ParentSessionID,
		RunID:         record.ID,
		AgentID:       attr.AgentID,
		AgentKey:      attr.AgentKey,
		UserID:        record.OwnerUserID,
		Provider:      attr.Provider,
		Model:         model,
		Status:        status,
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		CachedTok:     cachedTok,
		UsageSource:   "streaming",
		Latency:       latency,
		ErrMsg:        errMsg,
	}); err != nil {
		s.lg.Warn("subagent aux usage record failed",
			loggateway.StepID("tool.subagent_usage_record"),
			loggateway.Str("run_id", record.ID),
			loggateway.Err(err))
	}
}

// usageAccum accumulates per-round billing totals from the run's event
// stream. Semantics mirror agent.accumulateStreamUsage (2026-08-19): each
// LLM round is a separately billed API call → prompt/cached/completion are
// SUMMED across rounds; within one round streaming usage is cumulative →
// track the max. A usage payload whose prompt differs from the current
// round's prompt marks a new billable round.
type usageAccum struct {
	prevPrompt, prevCompletion, prevCached int
	curPrompt, curCompletion, curCached    int
	// model is the last non-empty provider-reported model id seen.
	model string
}

func (a *usageAccum) consume(evt *trpcevent.Event) {
	if a == nil || evt == nil || evt.Response == nil {
		return
	}
	if m := strings.TrimSpace(evt.Response.Model); m != "" {
		a.model = m
	}
	u := evt.Response.Usage
	if u == nil {
		return
	}
	promptTok := u.PromptTokens
	completionTok := u.CompletionTokens
	cachedTok := u.PromptTokensDetails.CachedTokens
	if promptTok != a.curPrompt {
		a.prevPrompt += a.curPrompt
		a.prevCompletion += a.curCompletion
		a.prevCached += a.curCached
		a.curPrompt = promptTok
		a.curCompletion = completionTok
		a.curCached = cachedTok
	} else {
		if completionTok > a.curCompletion {
			a.curCompletion = completionTok
		}
		if cachedTok > a.curCached {
			a.curCached = cachedTok
		}
	}
}

func (a *usageAccum) totals() (prompt, completion, cached int) {
	if a == nil {
		return 0, 0, 0
	}
	return a.prevPrompt + a.curPrompt,
		a.prevCompletion + a.curCompletion,
		a.prevCached + a.curCached
}

func (s *Service) runChild(
	ctx context.Context,
	record *runRecord,
	started runningRun,
	result *replyAccumulator,
	usage *usageAccum,
	guard *runBudgetGuard,
) error {
	if record == nil {
		return apierror.Internal(apierror.DomainSubagent, "nil run record")
	}
	runtimeState := map[string]any{
		runtimeStateSubagentRun:      true,
		runtimeStateSubagentRunID:    record.ID,
		runtimeStateSubagentParentID: record.ParentSessionID,
	}

	runOpts := []trpcagent.RunOption{
		trpcagent.WithRequestID(started.requestID),
		trpcagent.WithRuntimeState(runtimeState),
		trpcagent.WithInjectedContextMessages([]trpcmodel.Message{
			trpcmodel.NewSystemMessage(kindSystemPrompt(record.Kind)),
		}),
		// 框架 v1.11 修复管线：参数 JSON 修复 + 文本工具调用提取（与
		// chat 主路径对齐）。
		trpcagent.WithToolCallArgumentsJSONRepairEnabled(true),
		trpcagent.WithToolCallTextRepairEnabled(true),
	}

	evts, err := s.runner.Run(
		ctx,
		record.OwnerUserID,
		started.childSession,
		trpcmodel.NewUserMessage(record.Task),
		runOpts...,
	)
	if err != nil {
		return err
	}
	for evt := range evts {
		usage.consume(evt)
		result.consume(evt)
		guard.observe(usage)
	}
	return result.err
}

func (s *Service) markRunning(
	parent context.Context,
	runID string,
	timeoutSeconds int,
) (*runRecord, context.Context, runningRun, error) {
	s.mu.Lock()
	record := s.runs[strings.TrimSpace(runID)]
	if record == nil {
		s.mu.Unlock()
		return nil, nil, runningRun{}, trpcsubagent.ErrRunNotFound
	}
	if record.Status == trpcsubagent.StatusCanceled {
		s.mu.Unlock()
		return nil, nil, runningRun{}, apierror.BadRequest(apierror.DomainSubagent, "run canceled before start")
	}

	now := s.clock()
	started := runningRun{
		startedAt:    now,
		childSession: newChildSessionID(record.ID, now),
		requestID:    newRequestID(record.ID, now),
	}

	runCtx := parent
	if runCtx == nil {
		runCtx = context.Background()
	}
	if timeoutSeconds > 0 {
		timeoutCtx, cancel := context.WithTimeout(
			runCtx,
			time.Duration(timeoutSeconds)*time.Second,
		)
		runCtx = timeoutCtx
		started.cancel = cancel
	} else {
		nextCtx, cancel := context.WithCancel(runCtx)
		runCtx = nextCtx
		started.cancel = cancel
	}

	record.Status = trpcsubagent.StatusRunning
	record.ChildSessionID = started.childSession
	record.UpdatedAt = now
	record.StartedAt = cloneTime(now)
	record.FinishedAt = nil
	record.Error = ""
	record.Summary = ""
	record.Result = ""

	s.running[record.ID] = &runningRun{
		cancel:       started.cancel,
		childSession: started.childSession,
		requestID:    started.requestID,
		startedAt:    started.startedAt,
	}
	clone := record.clone()
	s.mu.Unlock()

	if err := s.persist(); err != nil {
		if started.cancel != nil {
			started.cancel()
		}
		s.mu.Lock()
		delete(s.running, runID)
		if current := s.runs[runID]; current != nil {
			current.Status = trpcsubagent.StatusFailed
			current.Error = err.Error()
			current.Summary = summarizeResult(current.Error, 0)
			current.UpdatedAt = now
			current.FinishedAt = cloneTime(now)
		}
		s.mu.Unlock()
		return nil, nil, runningRun{}, apierror.Internal(apierror.DomainSubagent, "persist: "+err.Error())
	}
	s.emitModeBStarted(parent, ModeBStartedInfo{
		RunID:           clone.ID,
		ParentSessionID: clone.ParentSessionID,
		ChildSessionID:  started.childSession,
		Task:            clone.Task,
	})
	return clone, runCtx, started, nil
}

func (s *Service) emitModeBStarted(ctx context.Context, info ModeBStartedInfo) {
	if s == nil {
		return
	}
	s.mu.RLock()
	hook := s.onModeBStarted
	s.mu.RUnlock()
	if hook == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if r := recover(); r != nil {
			s.lg.Warn("mode B started hook panic",
				loggateway.Str("run_id", info.RunID),
				loggateway.Str("panic", fmt.Sprint(r)),
			)
		}
	}()
	hook(ctx, info)
}

func (s *Service) emitModeBFinished(ctx context.Context, info ModeBFinishedInfo) {
	if s == nil {
		return
	}
	s.mu.RLock()
	hook := s.onModeBFinished
	s.mu.RUnlock()
	if hook == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if r := recover(); r != nil {
			s.lg.Warn("mode B finished hook panic",
				loggateway.Str("run_id", info.RunID),
				loggateway.Str("panic", fmt.Sprint(r)),
			)
		}
	}()
	hook(ctx, info)
}

func (s *Service) finishRun(runID string, output string, runErr error, summaryRunes int) {
	s.mu.Lock()
	record := s.runs[runID]
	if record == nil {
		delete(s.running, runID)
		s.mu.Unlock()
		return
	}
	now := s.clock()
	running := s.running[runID]
	delete(s.running, runID)

	record.Result = output
	record.UpdatedAt = now
	record.FinishedAt = cloneTime(now)

	switch {
	case running != nil && running.cancelRequested:
		record.Status = trpcsubagent.StatusCanceled
		record.Error = ""
		// 预算跳闸的取消带跳闸说明（C4-②），与用户主动取消区分。
		reason := strings.TrimSpace(running.cancelReason)
		if reason == "" {
			reason = "canceled"
		}
		record.Summary = summarizeResult(reason, summaryRunes)
	case errors.Is(runErr, context.Canceled):
		record.Status = trpcsubagent.StatusCanceled
		record.Error = ""
		record.Summary = summarizeResult("canceled", summaryRunes)
	case runErr != nil:
		record.Status = trpcsubagent.StatusFailed
		record.Error = runErr.Error()
		record.Summary = summarizeResult(record.Error, summaryRunes)
	default:
		record.Status = trpcsubagent.StatusCompleted
		record.Error = ""
		record.Summary = summarizeResult(output, summaryRunes)
	}
	s.mu.Unlock()

	if err := s.persist(); err != nil {
		s.lg.Warn("subagent.finishRun",
			loggateway.StepID("tool.subagent_persist_fail"),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
	}

	finishedStatus := "completed"
	switch record.Status {
	case trpcsubagent.StatusFailed:
		finishedStatus = "failed"
	case trpcsubagent.StatusCanceled:
		finishedStatus = "cancelled"
	}
	s.emitModeBFinished(context.Background(), ModeBFinishedInfo{
		RunID:          record.ID,
		ChildSessionID: record.ChildSessionID,
		Status:         finishedStatus,
		Error:          record.Error,
	})

	// Notify via outbound router if configured.
	// Clone the record under lock so doNotifyCompletion reads a
	// consistent snapshot without holding the lock during the
	// (potentially slow) router.SendText call.
	s.mu.Lock()
	router := s.outboundRouter
	cloned := record.clone()
	s.mu.Unlock()
	if router != nil && cloned != nil && cloned.Status == trpcsubagent.StatusCompleted {
		s.doNotifyCompletion(router, cloned)
	}
}

func (s *Service) doNotifyCompletion(router *outbound.Router, record *runRecord) {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeoutSec*time.Second)
	defer cancel()
	summary := record.Summary
	if summary == "" {
		summary = record.Result
	}
	if len(summary) > notifyMaxSummaryRunes {
		summary = truncateRunes(summary, notifyMaxSummaryRunes) + "..."
	}
	text := fmt.Sprintf("子 Agent 任务完成: %s\n摘要: %s", record.Task, summary)
	// Try to resolve target from parent session
	target := outbound.DeliveryTarget{}
	if dt, ok := outbound.ResolveTargetFromSessionID(record.ParentSessionID); ok {
		target = dt
	}
	if target.Channel == "" {
		return // no delivery target, skip notification
	}
	if err := router.SendText(ctx, target, text); err != nil {
		s.lg.Warn("SubAgent completion notification failed",
			loggateway.StepID("subagent.notify.fail"),
			loggateway.Str("run_id", record.ID),
			loggateway.Err(err),
		)
	}
}

func (s *Service) runForUser(userID string, runID string) (*runRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record := s.runs[strings.TrimSpace(runID)]
	if record == nil || record.OwnerUserID != strings.TrimSpace(userID) {
		return nil, trpcsubagent.ErrRunNotFound
	}
	return record.clone(), nil
}

func (s *Service) stopAllRunning() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, running := range s.running {
		if running == nil {
			delete(s.running, id)
			continue
		}
		running.skipNotify = true
		running.cancelRequested = true
		if running.cancel != nil {
			running.cancel()
		}
	}
}

func (s *Service) persist() error {
	if s == nil {
		return nil
	}
	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	s.mu.Lock()
	runs := make(map[string]*runRecord, len(s.runs))
	for id, item := range s.runs {
		runs[id] = item.clone()
	}
	s.mu.Unlock()
	return saveRuns(s.path, runs)
}

func (r *runRecord) clone() *runRecord {
	if r == nil {
		return nil
	}
	out := *r
	if r.StartedAt != nil {
		startedAt := *r.StartedAt
		out.StartedAt = &startedAt
	}
	if r.FinishedAt != nil {
		finishedAt := *r.FinishedAt
		out.FinishedAt = &finishedAt
	}
	return &out
}

func svcNow(s *Service) time.Time {
	if s == nil || s.clock == nil {
		return time.Now()
	}
	return s.clock()
}

func (r *runRecord) publicView() trpcsubagent.Run {
	if r == nil {
		return trpcsubagent.Run{}
	}
	return r.Run
}

func normalizeLoadedRuns(runs map[string]*runRecord, now time.Time) bool {
	changed := false
	for _, r := range runs {
		if r == nil {
			continue
		}
		if !r.Status.IsTerminal() {
			r.Status = trpcsubagent.StatusFailed
			r.Error = "interrupted"
			r.Summary = truncateRunes("interrupted", defaultStoredSummaryRunes)
			r.UpdatedAt = now
			r.FinishedAt = cloneTime(now)
			changed = true
		}
		if r.CreatedAt.IsZero() {
			r.CreatedAt = now
			changed = true
		}
		if r.UpdatedAt.IsZero() {
			r.UpdatedAt = now
			changed = true
		}
	}
	return changed
}

func loadRuns(path string, lg loggateway.Logger) (map[string]*runRecord, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	runs := make(map[string]*runRecord)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return runs, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return runs, nil
	}
	var sf storeFile
	if err := json.Unmarshal(data, &sf); err != nil {
		lg.Warn("failed to unmarshal subagent store file",
			loggateway.StepID("tool.subagent.load_runs"),
			loggateway.Err(err),
		)
		return nil, err
	}
	for i := range sf.Runs {
		r := sf.Runs[i]
		runs[r.ID] = &r
	}
	return runs, nil
}

func saveRuns(path string, runs map[string]*runRecord) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	sf := storeFile{Version: 1}
	for _, r := range runs {
		if r == nil {
			continue
		}
		sf.Runs = append(sf.Runs, *r)
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func newChildSessionID(runID string, now time.Time) string {
	return fmt.Sprintf("%s%s:%d", subagentSessionPrefix, strings.TrimSpace(runID), now.UnixNano())
}

func newRequestID(runID string, now time.Time) string {
	return fmt.Sprintf("%s%s:%d", subagentRequestPrefix, strings.TrimSpace(runID), now.UnixNano())
}

func sanitizeStoredResult(text string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = defaultStoredResultRunes
	}
	return truncateRunes(text, maxRunes)
}

func summarizeResult(text string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = defaultStoredSummaryRunes
	}
	return truncateRunes(text, maxRunes)
}

func truncateRunes(text string, limit int) string {
	trimmed := strings.TrimSpace(text)
	if limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit])
}

func cloneTime(t time.Time) *time.Time {
	return &t
}

type replyAccumulator struct {
	text     string
	builder  strings.Builder
	seenFull bool
	err      error
}

func (a *replyAccumulator) consume(evt *trpcevent.Event) {
	if evt == nil {
		return
	}
	if evt.Error != nil {
		a.err = apierror.Internal(apierror.DomainTool, evt.Error.Message)
		return
	}
	if evt.Response == nil {
		return
	}
	switch evt.Object {
	case trpcmodel.ObjectTypeChatCompletion:
		a.consumeFull(evt.Response)
	case trpcmodel.ObjectTypeChatCompletionChunk:
		a.consumeDelta(evt.Response)
	}
}

func (a *replyAccumulator) consumeFull(rsp *trpcmodel.Response) {
	if rsp == nil || len(rsp.Choices) == 0 {
		return
	}
	content := rsp.Choices[0].Message.Content
	if content == "" {
		return
	}
	a.text = content
	a.seenFull = true
}

func (a *replyAccumulator) consumeDelta(rsp *trpcmodel.Response) {
	if rsp == nil || a.seenFull {
		return
	}
	for _, choice := range rsp.Choices {
		if choice.Delta.Content == "" {
			continue
		}
		a.builder.WriteString(choice.Delta.Content)
	}
	a.text = a.builder.String()
}

type spawnTool struct {
	svc *Service
}

// LongRunning marks subagents_spawn as a long-running (asynchronous) tool.
// The tool returns immediately with a queued Run, while the actual sub-agent
// executes in the background. This flag is propagated to the frontend via
// EnvelopeToolCall.is_long_running so the UI can display an appropriate
// "running in background" indicator instead of a spinner.
func (t *spawnTool) LongRunning() bool { return true }

type listTool struct {
	svc *Service
}

type getTool struct {
	svc *Service
}

type cancelTool struct {
	svc *Service
}

type spawnInput struct {
	Task           string `json:"task"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Kind           string `json:"kind"`
	SubagentType   string `json:"subagent_type"`
}

type getInput struct {
	ID           string `json:"id"`
	BlockUntilMS int    `json:"block_until_ms"`
	// FullResult 是上行压缩的逃生舱（C4-③）：true 时返回存储全量
	// result（≤ResultRunes），默认 false 只回 ≤UpwardPipeMaxRunes 的
	// 截断视图——防轮询路径把完整交付物反复回灌父上下文（S07 570K）。
	FullResult bool `json:"full_result"`
}

type runIDInput struct {
	ID string `json:"id"`
}

type listResult struct {
	Runs []map[string]any `json:"runs,omitempty"`
}

func newSpawnTool(svc *Service) *spawnTool {
	return &spawnTool{svc: svc}
}

func newListTool(svc *Service) *listTool {
	return &listTool{svc: svc}
}

func newGetTool(svc *Service) *getTool {
	return &getTool{svc: svc}
}

func newCancelTool(svc *Service) *cancelTool {
	return &cancelTool{svc: svc}
}

func (t *spawnTool) Declaration() *trpctool.Declaration {
	// WP-2b: state the concurrency limit statically so the model can plan
	// parallelism up-front instead of discovering it via 429 failures
	// (production: 67% of spawn calls failed). maxConcurrent is fixed for the
	// process lifetime (env override at startup), so inlining it keeps the
	// declaration byte-stable within a session.
	limit := defaultMaxConcurrentSubAgents
	if t != nil && t.svc != nil && t.svc.maxConcurrent > 0 {
		limit = t.svc.maxConcurrent
	}
	return &trpctool.Declaration{
		Name: toolSubagentsSpawn,
		Description: fmt.Sprintf("Spawn one background subagent for the current "+
			"session. Use kind=explore for codebase search (avoid writes) or "+
			"kind=verify for tests/builds. Use this for long-running work, "+
			"parallelizable work, or independent verification. It returns "+
			"immediately with a run id. At most %d subagent runs may be "+
			"active concurrently; when the limit is reached the call fails "+
			"with a 429 — do NOT retry immediately; track the active runs "+
			"with subagents_get (optional block_until_ms) and wait for their "+
			"completion notifications before spawning more.", limit),
		InputSchema: &trpctool.Schema{
			Type:     schemaTypeObject,
			Required: []string{argTask},
			Properties: map[string]*trpctool.Schema{
				argTask: {
					Type:        schemaTypeString,
					Description: "Delegated task for the background subagent.",
				},
				argTimeoutSeconds: {
					Type:        schemaTypeInteger,
					Description: "Optional timeout in seconds for the delegated run.",
				},
				argKind: {
					Type:        schemaTypeString,
					Description: "Subagent kind: explore (search/read, avoid writes), verify (tests/builds), or general. Alias: subagent_type.",
				},
				argSubagentType: {
					Type:        schemaTypeString,
					Description: "Alias for kind.",
				},
			},
		},
	}
}

// Call executes the subagents_spawn tool. It returns the queued Run object
// (not nil) so that the LLM receives a tool_result confirming the spawn
// succeeded and containing the Run ID for subsequent subagents_get queries.
// Returning nil would cause the framework to skip tool_result generation,
// leaving an orphaned tool_call in the LLM conversation and violating the
// tool_call/tool_result pairing required by LLM APIs.
func (t *spawnTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.svc == nil {
		return nil, apierror.Internal(apierror.DomainSubagent, "service unavailable")
	}
	if isNestedSubagent(ctx) {
		return nil, apierror.BadRequest(apierror.DomainSubagent, "nested subagent spawn is not supported")
	}

	var in spawnInput
	if err := json.Unmarshal(args, &in); err != nil {
		t.svc.lg.Warn("failed to unmarshal spawn arguments",
			loggateway.StepID("tool.subagent.spawn"),
			loggateway.Err(err),
		)
		return nil, apierror.BadRequest(apierror.DomainSubagent, "invalid arguments: "+err.Error())
	}

	userID, sess, err := currentContext(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve per-session rune limits from the service's session config.
	resultRunes, summaryRunes := t.svc.getSessionRunes(sess.ID)

	run, err := t.svc.Spawn(ctx, SpawnRequest{
		OwnerUserID:     userID,
		ParentSessionID: sess.ID,
		Task:            in.Task,
		TimeoutSeconds:  in.TimeoutSeconds,
		Kind:            firstNonEmpty(in.Kind, in.SubagentType),
		ResultRunes:     resultRunes,
		SummaryRunes:    summaryRunes,
	})
	if err != nil {
		return nil, err
	}
	return enrichRunView(run, svcNow(t.svc)), nil
}

func (t *listTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: toolSubagentsList,
		Description: "List background subagents created from the " +
			"current session. Returns status and short summary per run " +
			"(results are omitted — fetch one with subagents_get).",
		InputSchema: &trpctool.Schema{
			Type: schemaTypeObject,
		},
	}
}

func (t *listTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.svc == nil {
		return nil, apierror.Internal(apierror.DomainSubagent, "service unavailable")
	}
	if len(args) > 0 && strings.TrimSpace(string(args)) != "" &&
		strings.TrimSpace(string(args)) != "{}" {
		var ignored map[string]any
		if err := json.Unmarshal(args, &ignored); err != nil {
			t.svc.lg.Warn("failed to unmarshal list arguments",
				loggateway.StepID("tool.subagent.list"),
				loggateway.Err(err),
			)
			return nil, apierror.BadRequest(apierror.DomainSubagent, "invalid arguments: "+err.Error())
		}
	}

	userID, sess, err := currentContext(ctx)
	if err != nil {
		return nil, err
	}
	now := svcNow(t.svc)
	raw := t.svc.ListForUser(
		userID,
		trpcsubagent.ListFilter{
			ParentSessionID: sess.ID,
		},
	)
	views := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		view := enrichRunView(r, now)
		// C4-③：list 是跟踪视图——剥离 result（可能数千 runes），只留
		// summary（≤240 runes）供状态跟踪；取交付物走 subagents_get。
		delete(view, "result")
		views = append(views, view)
	}
	return listResult{Runs: views}, nil
}

func (t *getTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: toolSubagentsGet,
		Description: "Get the latest status and result for one " +
			"background subagent run. Optional block_until_ms waits until the run is terminal. " +
			"The result is truncated to 2000 runes by default; pass full_result=true once " +
			"if you genuinely need the complete stored result.",
		InputSchema: &trpctool.Schema{
			Type:     schemaTypeObject,
			Required: []string{argID},
			Properties: map[string]*trpctool.Schema{
				argID: {
					Type:        schemaTypeString,
					Description: "Subagent run id returned by spawn.",
				},
				argBlockUntilMS: {
					Type:        schemaTypeInteger,
					Description: "Wait this many milliseconds for a terminal status before returning.",
				},
				argFullResult: {
					Type:        schemaTypeBoolean,
					Description: "Return the complete stored result instead of the default 2000-rune truncated view.",
				},
			},
		},
	}
}

func (t *getTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.svc == nil {
		return nil, apierror.Internal(apierror.DomainSubagent, "service unavailable")
	}
	var in getInput
	if err := json.Unmarshal(args, &in); err != nil {
		t.svc.lg.Warn("failed to unmarshal get arguments",
			loggateway.StepID("tool.subagent.get"),
			loggateway.Err(err),
		)
		return nil, apierror.BadRequest(apierror.DomainSubagent, "invalid arguments: "+err.Error())
	}
	userID, _, err := currentContext(ctx)
	if err != nil {
		return nil, err
	}
	runID := strings.TrimSpace(in.ID)
	if runID == "" {
		return nil, apierror.BadRequest(apierror.DomainSubagent, "empty run id")
	}
	wait := time.Duration(0)
	if in.BlockUntilMS > 0 {
		wait = time.Duration(in.BlockUntilMS) * time.Millisecond
	}
	run, err := t.svc.WaitForUser(ctx, userID, runID, wait)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, trpcsubagent.ErrRunNotFound
	}
	view := enrichRunView(*run, svcNow(t.svc))
	// C4-③：默认截断 result 到 ≤2KB（含截断标记）；full_result=true
	// 时返回存储全量（逃生舱）。
	if !in.FullResult {
		clipUpwardResultView(view)
	}
	return view, nil
}

func (t *cancelTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: toolSubagentsCancel,
		Description: "Cancel one background subagent run. This is " +
			"best-effort.",
		InputSchema: &trpctool.Schema{
			Type:     schemaTypeObject,
			Required: []string{argID},
			Properties: map[string]*trpctool.Schema{
				argID: {
					Type:        schemaTypeString,
					Description: "Subagent run id returned by spawn.",
				},
			},
		},
	}
}

func (t *cancelTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.svc == nil {
		return nil, apierror.Internal(apierror.DomainSubagent, "service unavailable")
	}
	runID, userID, err := decodeRunIDArgs(ctx, args, t.svc.lg)
	if err != nil {
		return nil, err
	}
	run, _, err := t.svc.CancelForUser(userID, runID)
	if err != nil {
		return nil, err
	}
	return run, nil
}

func decodeRunIDArgs(ctx context.Context, args []byte, lg loggateway.Logger) (string, string, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var in runIDInput
	if err := json.Unmarshal(args, &in); err != nil {
		lg.Warn("failed to unmarshal run_id arguments",
			loggateway.StepID("tool.subagent.decode_run_id"),
			loggateway.Err(err),
		)
		return "", "", apierror.BadRequest(apierror.DomainSubagent, "invalid arguments: "+err.Error())
	}
	userID, _, err := currentContext(ctx)
	if err != nil {
		return "", "", err
	}
	runID := strings.TrimSpace(in.ID)
	if runID == "" {
		return "", "", apierror.BadRequest(apierror.DomainSubagent, "empty run id")
	}
	return runID, userID, nil
}

func currentContext(ctx context.Context) (string, *trpcsession.Session, error) {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return "", nil, apierror.BadRequest(apierror.DomainSubagent, "current session context is unavailable")
	}
	userID := strings.TrimSpace(inv.Session.UserID)
	if userID == "" {
		return "", nil, apierror.BadRequest(apierror.DomainSubagent, "current user id is unavailable")
	}
	if strings.TrimSpace(inv.Session.ID) == "" {
		return "", nil, apierror.BadRequest(apierror.DomainSubagent, "current session id is unavailable")
	}
	return userID, inv.Session, nil
}

func isNestedSubagent(ctx context.Context) bool {
	nested, ok := trpcagent.GetRuntimeStateValueFromContext[bool](
		ctx,
		runtimeStateSubagentRun,
	)
	return ok && nested
}
