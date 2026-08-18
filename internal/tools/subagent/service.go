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

	"github.com/google/uuid"

	"aranea-agents/pkg/apierror"

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

	schemaTypeObject  = "object"
	schemaTypeString  = "string"
	schemaTypeInteger = "integer"

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

type Service struct {
	path           string
	runner         trpcrunner.Runner
	lg             loggateway.Logger
	outboundRouter *outbound.Router // optional: for completion notifications

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
	childSession    string
	requestID       string
	startedAt       time.Time
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
		path:          path,
		runner:        r,
		lg:            lg,
		runes:         newRunesManager(),
		clock:         time.Now,
		runs:          runs,
		running:       make(map[string]*runningRun),
		maxConcurrent: resolveSubagentMaxConcurrency(),
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
	runErr := s.runChild(runCtx, record, started, &result)
	output := sanitizeStoredResult(result.text, record.ResultRunes)
	s.finishRun(runID, output, runErr, record.SummaryRunes)
}

func (s *Service) runChild(
	ctx context.Context,
	record *runRecord,
	started runningRun,
	result *replyAccumulator,
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
		result.consume(evt)
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
		record.Summary = summarizeResult("canceled", summaryRunes)
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
			"current session.",
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
		views = append(views, enrichRunView(r, now))
	}
	return listResult{Runs: views}, nil
}

func (t *getTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: toolSubagentsGet,
		Description: "Get the latest status and result for one " +
			"background subagent run. Optional block_until_ms waits until the run is terminal.",
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
	return enrichRunView(*run, svcNow(t.svc)), nil
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
