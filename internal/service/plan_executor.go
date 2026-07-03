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
// Satisfied by composing PlanStepV2Repo + TeamStageV2Repo + PlanBoardV2Writer
// + GraphStageV2Repo + GraphNodeV2Repo. Methods ≤ 5 per interface (CS-B4).
//
// 2026-07-04 补齐：新增 UpsertGraphStage / UpsertGraphNode / GetGraphStageByPlanBoard
// 用于同步创建 GraphStage 和更新 GraphNode 状态（与 PlanBoard 一对一关联）。
type executorRepos interface {
	UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error)
	UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error)
	UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error)
	GetPlanStep(ctx context.Context, id string) (biz.PlanStep, error)
	UpsertGraphStage(ctx context.Context, gs biz.GraphStage) (biz.GraphStage, error)
	UpsertGraphNode(ctx context.Context, gn biz.GraphNode) (biz.GraphNode, error)
	GetGraphStageByPlanBoard(ctx context.Context, planBoardID string) (biz.GraphStage, error)
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
//
// 2026-07-04 补齐：在 DAG 执行前同步创建 GraphStage（与 PlanBoard 一对一关联）
// 和 GraphNode 列表（每个 PlanStep 对应一个 GraphNode），并发布 v2 事件。
func (e *PlanExecutor) Subscribe(ctx context.Context, board biz.PlanBoard) error {
	if len(board.Steps) == 0 {
		return nil
	}
	// 同步创建 GraphStage（与 PlanBoard 一对一）。失败不阻断主流程，
	// 仅记录日志（GraphStage 是可视化层，缺失不影响 DAG 调度正确性）。
	e.initGraphStage(ctx, board)
	r := newDagRun(e, board)
	return r.run(ctx)
}

// initGraphStage creates the GraphStage (and its GraphNodes) for the given
// PlanBoard if it doesn't already exist. Idempotent: if a GraphStage is
// already associated with the PlanBoard, it's left as-is.
//
// 设计：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.2.4
// GraphStage ID 由 planBoardID 确定性派生（uuid.NewSHA1(aranea.graph_stage.v2, planBoardID)）。
// GraphNode ID = plan_step.id（直接复用，确定性）。
func (e *PlanExecutor) initGraphStage(ctx context.Context, board biz.PlanBoard) {
	// 检查是否已存在 GraphStage（避免重复创建）。
	if existing, err := e.repos.GetGraphStageByPlanBoard(ctx, board.ID); err == nil && existing.ID != "" {
		// 已存在，跳过创建（可能来自历史 Subscribe 或 crash recovery）。
		return
	}
	// 派生 GraphStage ID（确定性，确保多次调用产生相同 ID）。
	gsID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v2:"+board.ID)).String()
	now := time.Now()
	gs := biz.GraphStage{
		ID:          gsID,
		TaskID:      board.TaskID,
		TurnID:      board.TurnID,
		SessionID:   board.SessionID,
		PlanBoardID: board.ID,
		Status:      biz.GraphStageStatusRunning,
		StartedAt:   now,
		Seq:         board.Seq, // 与 PlanBoard 同 Seq
		Version:     1,
	}
	// 构建 GraphNode 列表（每个 PlanStep 对应一个 GraphNode）。
	nodes := make([]biz.GraphNode, 0, len(board.Steps))
	for _, step := range board.Steps {
		gn := biz.GraphNode{
			ID:           step.ID, // GraphNode.ID = PlanStep.ID（确定性派生）
			GraphStageID: gsID,
			Label:        step.Label,
			DagNodeID:    step.ID,
			Status:       biz.MapPlanStepToGraphNodeStatus(step.Status),
			DependsOn:    append([]string(nil), step.DependsOn...),
		}
		nodes = append(nodes, gn)
	}
	gs.Nodes = nodes
	// 持久化 GraphStage。
	if _, err := e.repos.UpsertGraphStage(ctx, gs); err != nil {
		e.lg.Warn("upsert graph_stage failed (non-blocking)",
			loggateway.Str("graph_stage_id", gsID),
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Err(err),
		)
		return
	}
	// 持久化 GraphNodes。
	for _, gn := range nodes {
		if _, err := e.repos.UpsertGraphNode(ctx, gn); err != nil {
			e.lg.Warn("upsert graph_node failed (non-blocking)",
				loggateway.Str("graph_node_id", gn.ID),
				loggateway.Err(err),
			)
		}
	}
	// 发布 GraphStageCreatedEvent。
	e.seq.Publish(ctx, biz.NewGraphStageCreatedEvent(gs))
}

// dagRun encapsulates the per-Subscribe DAG state. Created fresh for each
// Subscribe call; not safe for concurrent reuse (one Subscribe = one dagRun).
type dagRun struct {
	pe    *PlanExecutor
	board biz.PlanBoard

	// graphStageID 是与 PlanBoard 一对一关联的 GraphStage 的 ID（在 initGraphStage
	// 中创建）。如果创建失败则为空，此时跳过 GraphNode 更新。
	graphStageID string

	mu         sync.Mutex
	stepsByID  map[string]*biz.PlanStep
	dependents map[string][]string // stepID → stepIDs that depend on it
	wg         sync.WaitGroup
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
	// 查找与 PlanBoard 关联的 GraphStage ID（在 initGraphStage 中创建）。
	// 如果未找到（创建失败或未调用 initGraphStage），graphStageID 为空，
	// dagRun 会跳过 GraphNode 更新，但 DAG 调度仍正常运行。
	var gsID string
	if existing, err := pe.repos.GetGraphStageByPlanBoard(context.Background(), board.ID); err == nil && existing.ID != "" {
		gsID = existing.ID
	}
	return &dagRun{
		pe:           pe,
		board:        board,
		graphStageID: gsID,
		stepsByID:    stepsByID,
		dependents:   dependents,
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
	// 2026-07-04 补齐：同步更新 GraphNode 状态为 Running，回填 TeamStageID。
	r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusRunning, tsID)
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
		// 2026-07-04 补齐：GraphNode → Completed
		r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusCompleted, "")
		r.checkDownstream(ctx, step.ID)
	} else {
		r.pe.seq.Publish(ctx, biz.NewPlanStepFailedEvent(current, r.board.SessionID))
		// 2026-07-04 补齐：GraphNode → Failed
		r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusFailed, "")
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
	// 2026-07-04 补齐：GraphNode → Failed
	r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusFailed, "")
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
			// 2026-07-04 补齐：GraphNode → Interrupted（skipped 映射为 interrupted）
			r.updateGraphNode(ctx, depID, biz.GraphNodeStatusInterrupted, "")
			queue = append(queue, depID)
		}
	}
}

// updateGraphNode updates the GraphNode status (and optionally TeamStageID)
// and publishes a GraphNodeUpdatedEvent. No-op if graphStageID is empty
// (GraphStage creation failed or was skipped).
//
// 设计：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.7.5
// GraphNode 状态由 PlanStep.Status 通过 MapPlanStepToGraphNodeStatus 映射得到。
// TeamStageID 在 dispatchStep 时回填，便于前端高亮显示对应节点。
func (r *dagRun) updateGraphNode(ctx context.Context, stepID string, status biz.GraphNodeStatus, teamStageID string) {
	if r.graphStageID == "" {
		return // GraphStage 未创建，跳过
	}
	gn := biz.GraphNode{
		ID:           stepID, // GraphNode.ID = PlanStep.ID
		GraphStageID: r.graphStageID,
		Status:       status,
	}
	if teamStageID != "" {
		gn.TeamStageID = teamStageID
	}
	if _, err := r.pe.repos.UpsertGraphNode(ctx, gn); err != nil {
		r.pe.lg.Warn("upsert graph_node (status update) failed (non-blocking)",
			loggateway.Str("graph_node_id", stepID),
			loggateway.Str("status", string(status)),
			loggateway.Err(err),
		)
		return
	}
	// 发布 GraphNodeUpdatedEvent。taskID 和 spiritSessionID 从 board 派生。
	r.pe.seq.Publish(ctx, biz.NewGraphNodeUpdatedEvent(gn, r.board.TaskID, r.board.SessionID))
}
