package subagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/apierror"

	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

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

	s.mu.Lock()
	// E2E-H4: resolve the runner bound to the PARENT SESSION's current turn
	// (injected at buildTurn via SetTurnContext), falling back to the
	// construction-time default. Never a cross-session last-writer singleton.
	tc := s.turnContextFor(req.ParentSessionID)
	if tc.runner == nil {
		s.mu.Unlock()
		return trpcsubagent.Run{}, apierror.Internal(apierror.DomainSubagent, "runner not configured")
	}
	if len(s.running) >= s.maxConcurrent {

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
	// Capture the spawning turn's runner into the record (immutable for the
	// run's whole async lifetime); attribution is resolved inside
	// newRunRecord from the same per-session context.
	record.runner = tc.runner
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

		Attribution: s.turnContextFor(req.ParentSessionID).attribution,
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
