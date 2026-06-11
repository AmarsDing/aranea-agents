package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	kerrors "github.com/go-kratos/kratos/v2/errors"

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

	// Notification limits for outbound completion messages.
	notifyMaxSummaryRunes = 200
	notifyTimeoutSec     = 5

	toolSubagentsSpawn  = "subagents_spawn"
	toolSubagentsList   = "subagents_list"
	toolSubagentsGet    = "subagents_get"
	toolSubagentsCancel = "subagents_cancel"

	argTask           = "task"
	argID             = "id"
	argTimeoutSeconds = "timeout_seconds"

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
	// ResultRunes is the max rune count for stored subagent results.
	// When <= 0, defaultStoredResultRunes (4000) is used.
	ResultRunes int
	// SummaryRunes is the max rune count for stored subagent summaries.
	// When <= 0, defaultStoredSummaryRunes (240) is used.
	SummaryRunes int
}

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
		return nil, kerrors.InternalServer("SUBAGENT", "empty state dir")
	}

	path := filepath.Join(
		strings.TrimSpace(stateDir),
		subagentDirName,
		subagentRunsFileName,
	)
	runs, err := loadRuns(path, lg)
	if err != nil {
		return nil, kerrors.InternalServer("SUBAGENT", "load runs: "+err.Error())
	}

	svc := &Service{
		path:    path,
		runner:  r,
		lg:      lg,
		runes:   newRunesManager(),
		clock:   time.Now,
		runs:    runs,
		running: make(map[string]*runningRun),
	}
	if normalizeLoadedRuns(svc.runs, svc.clock()) {
		if err := svc.persist(); err != nil {
			return nil, kerrors.InternalServer("SUBAGENT", "persist: "+err.Error())
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
		return trpcsubagent.Run{}, kerrors.InternalServer("SUBAGENT", "nil service")
	}
	if s.baseCtx == nil {
		return trpcsubagent.Run{}, kerrors.InternalServer("SUBAGENT", "not started")
	}

	nested, _ := trpcagent.GetRuntimeStateValueFromContext[bool](ctx, runtimeStateSubagentRun)
	if nested {
		return trpcsubagent.Run{}, kerrors.BadRequest("SUBAGENT", "nested subagent spawn is not allowed")
	}

	// Concurrency limit: check and reserve slot atomically to avoid TOCTOU race.
	s.mu.Lock()
	if s.runner == nil {
		s.mu.Unlock()
		return trpcsubagent.Run{}, kerrors.InternalServer("SUBAGENT", "runner not configured")
	}
	if len(s.running) >= defaultMaxConcurrentSubAgents {
		s.mu.Unlock()
		return trpcsubagent.Run{}, kerrors.New(429, "SUBAGENT", fmt.Sprintf("too many concurrent sub-agents (limit: %d)", defaultMaxConcurrentSubAgents))
	}

	if err := validateSpawnRequest(req); err != nil {
		s.mu.Unlock()
		return trpcsubagent.Run{}, err
	}

	record := s.newRunRecord(req)
	s.runs[record.ID] = record
	s.mu.Unlock()

	if err := s.persist(); err != nil {
		s.mu.Lock()
		delete(s.runs, record.ID)
		s.mu.Unlock()
		return trpcsubagent.Run{}, kerrors.InternalServer("SUBAGENT", "persist: "+err.Error())
	}
	view := record.publicView()

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
		return kerrors.BadRequest("SUBAGENT", "empty owner")
	}
	if strings.TrimSpace(req.ParentSessionID) == "" {
		return kerrors.BadRequest("SUBAGENT", "empty parent session id")
	}
	if strings.TrimSpace(req.Task) == "" {
		return kerrors.BadRequest("SUBAGENT", "empty task")
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
			Status:          trpcsubagent.StatusQueued,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		OwnerUserID: strings.TrimSpace(req.OwnerUserID),
		ResultRunes: resultRunes,
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
		return nil, false, kerrors.InternalServer("SUBAGENT", "persist: "+err.Error())
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
		return kerrors.InternalServer("SUBAGENT", "nil run record")
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
			trpcmodel.NewSystemMessage(subagentRunPrompt),
		}),
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
		return nil, nil, runningRun{}, kerrors.BadRequest("SUBAGENT", "run canceled before start")
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
		return nil, nil, runningRun{}, kerrors.InternalServer("SUBAGENT", "persist: "+err.Error())
	}
	return clone, runCtx, started, nil
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

	// Notify via outbound router if configured
	s.mu.Lock()
	router := s.outboundRouter
	s.mu.Unlock()
	if router != nil && record.Status == trpcsubagent.StatusCompleted {
		s.doNotifyCompletion(router, record)
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
		a.err = errors.New(evt.Error.Message)
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
}

type runIDInput struct {
	ID string `json:"id"`
}

type listResult struct {
	Runs []trpcsubagent.Run `json:"runs,omitempty"`
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
	return &trpctool.Declaration{
		Name: toolSubagentsSpawn,
		Description: "Spawn one background subagent for the current " +
			"session. Use this for long-running work, parallelizable " +
			"work, or independent verification. It returns " +
			"immediately with a run id.",
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
			},
		},
	}
}

func (t *spawnTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.svc == nil {
		return nil, kerrors.InternalServer("SUBAGENT", "service unavailable")
	}
	if isNestedSubagent(ctx) {
		return nil, kerrors.BadRequest("SUBAGENT", "nested subagent spawn is not supported")
	}

	var in spawnInput
	if err := json.Unmarshal(args, &in); err != nil {
		t.svc.lg.Warn("failed to unmarshal spawn arguments",
			loggateway.StepID("tool.subagent.spawn"),
			loggateway.Err(err),
		)
		return nil, kerrors.BadRequest("SUBAGENT", "invalid arguments: "+err.Error())
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
		ResultRunes:     resultRunes,
		SummaryRunes:    summaryRunes,
	})
	if err != nil {
		return nil, err
	}
	return run, nil
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
		return nil, kerrors.InternalServer("SUBAGENT", "service unavailable")
	}
	if len(args) > 0 && strings.TrimSpace(string(args)) != "" &&
		strings.TrimSpace(string(args)) != "{}" {
		var ignored map[string]any
		if err := json.Unmarshal(args, &ignored); err != nil {
			t.svc.lg.Warn("failed to unmarshal list arguments",
				loggateway.StepID("tool.subagent.list"),
				loggateway.Err(err),
			)
			return nil, kerrors.BadRequest("SUBAGENT", "invalid arguments: "+err.Error())
		}
	}

	userID, sess, err := currentContext(ctx)
	if err != nil {
		return nil, err
	}
	return listResult{
		Runs: t.svc.ListForUser(
			userID,
			trpcsubagent.ListFilter{
				ParentSessionID: sess.ID,
			},
		),
	}, nil
}

func (t *getTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: toolSubagentsGet,
		Description: "Get the latest status and result for one " +
			"background subagent run.",
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

func (t *getTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.svc == nil {
		return nil, kerrors.InternalServer("SUBAGENT", "service unavailable")
	}
	runID, userID, err := decodeRunIDArgs(ctx, args, t.svc.lg)
	if err != nil {
		return nil, err
	}
	return t.svc.GetForUser(userID, runID)
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
		return nil, kerrors.InternalServer("SUBAGENT", "service unavailable")
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
		return "", "", kerrors.BadRequest("SUBAGENT", "invalid arguments: "+err.Error())
	}
	userID, _, err := currentContext(ctx)
	if err != nil {
		return "", "", err
	}
	runID := strings.TrimSpace(in.ID)
	if runID == "" {
		return "", "", kerrors.BadRequest("SUBAGENT", "empty run id")
	}
	return runID, userID, nil
}

func currentContext(ctx context.Context) (string, *trpcsession.Session, error) {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return "", nil, kerrors.BadRequest("SUBAGENT", "current session context is unavailable")
	}
	userID := strings.TrimSpace(inv.Session.UserID)
	if userID == "" {
		return "", nil, kerrors.BadRequest("SUBAGENT", "current user id is unavailable")
	}
	if strings.TrimSpace(inv.Session.ID) == "" {
		return "", nil, kerrors.BadRequest("SUBAGENT", "current session id is unavailable")
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
