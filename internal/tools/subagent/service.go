package subagent

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"

	"aranea-agents/internal/outbound"
	"aranea-agents/pkg/loggateway"

	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

type Service struct {
	path string
	// E2E-H4: defaultRunner 仅作构造期兜底（wire 注入 nil）。业务路径的
	// runner 一律按父会话从 turnCtx 取——构造期 runner 一旦注入即不可变
	// （openclaw taskruninprocess.NewService 同款先例），不存在写竞争。
	defaultRunner  trpcrunner.Runner
	lg             loggateway.Logger
	outboundRouter *outbound.Router // optional: for completion notifications
	usageRecorder  UsageRecorder    // optional: nil skips aux usage recording
	// E2E-H4: turnCtx 按父会话绑定当前 turn 的 runner + attribution，由
	// 编排层 buildTurn 经 SetTurnContext 注入。旧实现是服务级单例字段
	// 末写者胜：runChild 无锁读 vs SetRunner 持锁写 = data race（interface
	// 双字撕裂可 panic），且执行时 runner/attribution 可能已被另一会话的
	// turn 轮换 → 错身份执行 + 计费归属分叉持久落库。Spawn 时把条目捕获进
	// runRecord（per-run 不可变），执行期不再回读服务级状态。
	turnCtx map[string]turnRunnerContext

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
		path:                    path,
		defaultRunner:           r,
		lg:                      lg,
		runes:                   newRunesManager(),
		clock:                   time.Now,
		runs:                    runs,
		running:                 make(map[string]*runningRun),
		turnCtx:                 make(map[string]turnRunnerContext),
		maxConcurrent:           resolveSubagentMaxConcurrency(),
		runBudgetInputTokens:    resolveSubagentTokenBudget(envSubagentRunTokenBudget, defaultRunTokenBudgetInputTokens),
		parentBudgetInputTokens: resolveSubagentTokenBudget(envSubagentParentTokenBudget, defaultParentTokenBudgetInputTokens),
		parentInputTokens:       make(map[string]int64),
	}
	if normalizeLoadedRuns(svc.runs, svc.clock()) {
		if err := svc.persist(); err != nil {
			return nil, apierror.Internal(apierror.DomainSubagent, "persist: "+err.Error())
		}
	}
	return svc, nil
}

// SetTurnContext binds the current turn's runner and provider/model/agent
// attribution to the parent session (E2E-H4). Must be called by the
// orchestrator at buildTurn, after StoreRunner: Spawn snapshots the entry
// into the run record, so the asynchronous child executes on the SPAWNING
// turn's runner — never a later/another session's (末写者胜单例已拆除：
// runChild 无锁读 vs SetRunner 持锁写的 data race 与跨会话错身份执行 +
// 计费归属分叉由此根除）。Entry is overwritten on the session's next
// turn; session turns serialize, so no read can observe a half-written pair.
func (s *Service) SetTurnContext(sessionID string, r trpcrunner.Runner, provider, model, agentID, agentKey string) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	s.sweepTurnCtxLocked(svcNow(s))
	s.turnCtx[sessionID] = turnRunnerContext{
		runner: r,
		attribution: runAttribution{
			Provider: strings.TrimSpace(provider),
			Model:    strings.TrimSpace(model),
			AgentID:  strings.TrimSpace(agentID),
			AgentKey: strings.TrimSpace(agentKey),
		},
		boundAt: svcNow(s),
	}
	s.mu.Unlock()
}

// sweepTurnCtxLocked drops turn bindings older than turnCtxTTL (V 桶 minor:
// 无剔除路径导致内存随会话数单调增长）。条目只在 Spawn 瞬间被快照消费，
// 过期等价于「未绑定」。调用方必须持 s.mu。
func (s *Service) sweepTurnCtxLocked(now time.Time) {
	for k, tc := range s.turnCtx {
		if tc.boundAt.IsZero() || now.Sub(tc.boundAt) > turnCtxTTL {
			delete(s.turnCtx, k)
		}
	}
}

// turnContextFor resolves the runner/attribution bound to the parent
// session's current turn, falling back to the construction-time default
// runner (wire-time nil in prod; test harnesses inject a stub there).
// Caller must hold s.mu.
// V 桶 minor：超过 turnCtxTTL 的绑定视为未绑定，惰性剔除后回退默认。
func (s *Service) turnContextFor(parentSessionID string) turnRunnerContext {
	key := strings.TrimSpace(parentSessionID)
	tc := s.turnCtx[key]
	if tc.runner != nil && (tc.boundAt.IsZero() || svcNow(s).Sub(tc.boundAt) > turnCtxTTL) {
		delete(s.turnCtx, key)
		tc = turnRunnerContext{}
	}
	if tc.runner == nil {
		tc.runner = s.defaultRunner
	}
	return tc
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
