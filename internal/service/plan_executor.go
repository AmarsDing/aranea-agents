package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// executorRepos is the narrow repo interface required by PlanExecutor.
// Satisfied by composing PlanStepV2Repo + TeamStageV2Repo + PlanBoardV2Writer.
// Methods ≤ 5 per interface (CS-B4).
type executorRepos interface {
	UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error)
	UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error)
	UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error)
	GetPlanStep(ctx context.Context, id string) (biz.PlanStep, error)
}

// sequencerPublisher mirrors v2.SequencerPublisher to avoid importing
// internal/agent/v2 (service → agent would be a reversed dependency).
// The *v2.Sequencer satisfies this interface.
type sequencerPublisher interface {
	Publish(ctx context.Context, e biz.Event)
}

// PlanExecutor is the forward DAG scheduler that replaces spirit_team's
// reverse-sync updatePlanStepForTeam. It dispatches PlanSteps to a
// TeamOrchestrator, correlates team completions, and cascades downstream
// steps (or skips them on failure).
//
// Lifecycle:
//   - Subscribe(ctx, board) blocks until all steps reach a terminal status
//     (completed / failed / skipped) or ctx is canceled.
//   - Each dispatched step emits: TeamStageCreated + PlanStepStarted events.
//   - On completion: PlanStepCompleted event.
//   - On failure: PlanStepFailed event + cascade-skip all transitive downstream.
type PlanExecutor struct {
	repos executorRepos
	orch  TeamOrchestrator
	seq   sequencerPublisher
	lg    loggateway.Logger
}

// NewPlanExecutor constructs a PlanExecutor. All dependencies are required.
func NewPlanExecutor(repos executorRepos, orch TeamOrchestrator, seq sequencerPublisher, lg loggateway.Logger) *PlanExecutor {
	return &PlanExecutor{
		repos: repos,
		orch:  orch,
		seq:   seq,
		lg:    lg.With(loggateway.Domain("plan_executor")),
	}
}

// Subscribe starts DAG execution for the given board and blocks until all
// steps reach a terminal status or ctx is canceled.
func (e *PlanExecutor) Subscribe(ctx context.Context, board biz.PlanBoard) error {
	if len(board.Steps) == 0 {
		return nil
	}
	r := newDagRun(e, board)
	return r.run(ctx)
}

// dagRun encapsulates the per-Subscribe DAG state. Created fresh for each
// Subscribe call; not safe for concurrent reuse (one Subscribe = one dagRun).
type dagRun struct {
	pe    *PlanExecutor
	board biz.PlanBoard

	mu        sync.Mutex
	stepsByID map[string]*biz.PlanStep
	dependents map[string][]string // stepID → stepIDs that depend on it
	wg        sync.WaitGroup
}

func newDagRun(pe *PlanExecutor, board biz.PlanBoard) *dagRun {
	stepsByID := make(map[string]*biz.PlanStep, len(board.Steps))
	dependents := make(map[string][]string)
	for i := range board.Steps {
		s := &board.Steps[i]
		stepsByID[s.ID] = s
		for _, dep := range s.DependsOn {
			dependents[dep] = append(dependents[dep], s.ID)
		}
	}
	return &dagRun{
		pe:         pe,
		board:      board,
		stepsByID:  stepsByID,
		dependents: dependents,
	}
}

// run dispatches root steps and blocks until all steps are terminal.
// If no root steps exist (all steps have dependencies — a cycle or empty
// board), the WaitGroup count stays 0 and Wait returns immediately.
func (r *dagRun) run(ctx context.Context) error {
	// Dispatch root steps (empty DependsOn). Add to WaitGroup before
	// starting the goroutine to guarantee Wait sees the correct count.
	for i := range r.board.Steps {
		s := &r.board.Steps[i]
		if len(s.DependsOn) == 0 && s.Status == biz.PlanStepStatusPending {
			r.wg.Add(1)
			r.dispatch(ctx, s)
		}
	}
	// Wait for all goroutines (root + downstream) to finish.
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// dispatch sends a single step to the TeamOrchestrator and listens for its
// completion. Runs in its own goroutine (via safego.Go).
func (r *dagRun) dispatch(ctx context.Context, step *biz.PlanStep) {
	safego.Go(ctx, "plan_executor.dispatch."+step.ID, func() {
		defer r.wg.Done()
		r.dispatchStep(ctx, step)
	})
}

// dispatchStep performs the full dispatch lifecycle for one step:
// create TeamStage → persist → publish → transition step to Running →
// persist → publish → call orchestrator → await completion.
func (r *dagRun) dispatchStep(ctx context.Context, step *biz.PlanStep) {
	now := time.Now()
	// 1. Create TeamStage.
	tsID := uuid.NewString()
	ts := biz.TeamStage{
		ID:        tsID,
		TaskID:    r.board.TaskID,
		TurnID:    r.board.TurnID,
		SessionID: r.board.SessionID,
		DagNodeID: step.ID,
		Status:    biz.TeamStageStatusPending,
		Stage:     biz.TeamStageStageAssembled,
		StartedAt: now,
		Version:   1,
	}
	if _, err := r.pe.repos.UpsertTeamStage(ctx, ts); err != nil {
		r.pe.lg.Error("upsert team_stage failed",
			loggateway.Str("team_stage_id", tsID), loggateway.Err(err))
		r.failStep(ctx, step, "persist team_stage: "+err.Error())
		return
	}
	// 2. Publish TeamStageCreated.
	r.pe.seq.Publish(ctx, biz.NewTeamStageCreatedEvent(ts))
	// 3. Transition step to Running.
	r.mu.Lock()
	step.MappedTeamStageID = tsID
	if err := step.Transition(biz.PlanStepStatusRunning); err != nil {
		r.mu.Unlock()
		r.pe.lg.Error("transition to running failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
		r.failStep(ctx, step, "transition: "+err.Error())
		return
	}
	step.StartedAt = now
	step.Version++
	runningStep := *step
	r.mu.Unlock()
	// 4. Persist + publish PlanStepStarted.
	if _, err := r.pe.repos.UpsertPlanStep(ctx, runningStep); err != nil {
		r.pe.lg.Error("upsert plan_step (running) failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, biz.NewPlanStepStartedEvent(runningStep, r.board.SessionID))
	// 5. Call orchestrator.
	ch, err := r.pe.orch.Orchestrate(ctx, runningStep, ts)
	if err != nil {
		r.pe.lg.Error("orchestrate failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
		r.failStep(ctx, step, "orchestrate: "+err.Error())
		return
	}
	// 6. Await completion (single event then channel closes).
	select {
	case ev, ok := <-ch:
		if !ok {
			r.failStep(ctx, step, "orchestrator channel closed without event")
			return
		}
		r.handleCompletion(ctx, step, ev)
	case <-ctx.Done():
		return
	}
}

// handleCompletion processes a TeamCompleteEvent: marks the step completed or
// failed, then checks downstream.
func (r *dagRun) handleCompletion(ctx context.Context, step *biz.PlanStep, ev biz.TeamCompleteEvent) {
	now := time.Now()
	r.mu.Lock()
	if ev.Success {
		_ = step.Transition(biz.PlanStepStatusCompleted)
		step.CompletedAt = &now
	} else {
		_ = step.Transition(biz.PlanStepStatusFailed)
		step.CompletedAt = &now
		step.Error = &biz.StepError{Code: "team_failed", Message: ev.ErrorMsg}
	}
	step.Version++
	current := *step
	r.mu.Unlock()
	// Persist + publish.
	if _, err := r.pe.repos.UpsertPlanStep(ctx, current); err != nil {
		r.pe.lg.Error("upsert plan_step (terminal) failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
	}
	if ev.Success {
		r.pe.seq.Publish(ctx, biz.NewPlanStepCompletedEvent(current, r.board.SessionID))
		r.checkDownstream(ctx, step.ID)
	} else {
		r.pe.seq.Publish(ctx, biz.NewPlanStepFailedEvent(current, r.board.SessionID))
		r.cascadeSkip(ctx, step.ID)
	}
}

// failStep marks a step as failed without orchestrator completion (used for
// internal errors like persist failures or orchestrator invocation errors).
func (r *dagRun) failStep(ctx context.Context, step *biz.PlanStep, msg string) {
	now := time.Now()
	r.mu.Lock()
	_ = step.Transition(biz.PlanStepStatusFailed)
	step.CompletedAt = &now
	step.Error = &biz.StepError{Code: "internal", Message: msg}
	step.Version++
	current := *step
	r.mu.Unlock()
	if _, err := r.pe.repos.UpsertPlanStep(ctx, current); err != nil {
		r.pe.lg.Error("upsert plan_step (failStep) failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, biz.NewPlanStepFailedEvent(current, r.board.SessionID))
	r.cascadeSkip(ctx, step.ID)
}

// checkDownstream dispatches pending steps whose dependencies are now all
// completed. Called after a step completes successfully.
func (r *dagRun) checkDownstream(ctx context.Context, completedID string) {
	deps := r.dependents[completedID]
	for _, depID := range deps {
		r.mu.Lock()
		depStep, ok := r.stepsByID[depID]
		if !ok || depStep.Status != biz.PlanStepStatusPending {
			r.mu.Unlock()
			continue
		}
		allCompleted := true
		for _, d := range depStep.DependsOn {
			s := r.stepsByID[d]
			if s == nil || s.Status != biz.PlanStepStatusCompleted {
				allCompleted = false
				break
			}
		}
		if allCompleted {
			r.wg.Add(1)
			r.mu.Unlock()
			r.dispatch(ctx, depStep)
		} else {
			r.mu.Unlock()
		}
	}
}

// cascadeSkip marks all transitive downstream dependents of a failed step as
// skipped (BFS). Each skipped step publishes a PlanStepSkippedEvent.
func (r *dagRun) cascadeSkip(ctx context.Context, failedID string) {
	queue := []string{failedID}
	visited := make(map[string]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true
		for _, depID := range r.dependents[current] {
			r.mu.Lock()
			depStep, ok := r.stepsByID[depID]
			if !ok || depStep.Status != biz.PlanStepStatusPending {
				r.mu.Unlock()
				continue
			}
			reason := fmt.Sprintf("dependency %s failed", failedID)
			_ = depStep.Transition(biz.PlanStepStatusSkipped)
			depStep.Version++
			skipped := *depStep
			r.mu.Unlock()
			if _, err := r.pe.repos.UpsertPlanStep(ctx, skipped); err != nil {
				r.pe.lg.Error("upsert plan_step (skipped) failed",
					loggateway.Str("step_id", skipped.ID), loggateway.Err(err))
			}
			r.pe.seq.Publish(ctx, biz.NewPlanStepSkippedEvent(skipped, r.board.SessionID, reason))
			queue = append(queue, depID)
		}
	}
}
