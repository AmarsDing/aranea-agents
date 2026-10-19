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
		runner:                  r,
		lg:                      lg,
		runes:                   newRunesManager(),
		clock:                   time.Now,
		runs:                    runs,
		running:                 make(map[string]*runningRun),
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
