package agent

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

var _ biz.RecoveredPlanConsumer = (*TaskOrchestratorImpl)(nil)

// recoveredPlanBundle is a Phase 1/2 pair restored from persisted rows after
// interruption. Alloc may be nil when Phase 2 never completed.
type recoveredPlanBundle struct {
	Plan  *biz.TaskPlan
	Alloc *biz.AllocationPlan
}

// planStore holds TaskPlan/AllocationPlan repositories plus the in-memory
// cache of plans restored at startup. Extracted so TaskOrchestratorImpl stays
// within the AS-COG-01 field budget.
type planStore struct {
	tasks  biz.TaskPlanRepository
	allocs biz.AllocationPlanRepository
	mu     sync.RWMutex
	bySess map[string]*recoveredPlanBundle
	byPlan map[string]string // planID → spiritSessionID
}

func newPlanStore(tasks biz.TaskPlanRepository, allocs biz.AllocationPlanRepository) *planStore {
	return &planStore{
		tasks:  tasks,
		allocs: allocs,
		bySess: make(map[string]*recoveredPlanBundle),
		byPlan: make(map[string]string),
	}
}

func (s *planStore) getTaskPlan(ctx context.Context, id string) (*biz.TaskPlan, error) {
	if s == nil || s.tasks == nil {
		return nil, nil
	}
	return s.tasks.GetByID(ctx, id)
}

func (s *planStore) put(sessionID string, bundle *recoveredPlanBundle) {
	if s == nil || bundle == nil || bundle.Plan == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(bundle.Plan.SpiritSessionID)
	}
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bySess == nil {
		s.bySess = make(map[string]*recoveredPlanBundle)
	}
	if s.byPlan == nil {
		s.byPlan = make(map[string]string)
	}
	s.bySess[sessionID] = bundle
	s.byPlan[bundle.Plan.ID] = sessionID
}

func (s *planStore) hasPlan(planID string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.byPlan[planID]
	return ok
}

func (s *planStore) peek(sessionID string) (*recoveredPlanBundle, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.bySess[strings.TrimSpace(sessionID)]
	return b, ok && b != nil && b.Plan != nil
}

func (s *planStore) consume(sessionID, userMessage string) (*biz.TaskPlan, *biz.AllocationPlan, bool) {
	if s == nil {
		return nil, nil, false
	}
	sessionID = strings.TrimSpace(sessionID)
	userMessage = strings.TrimSpace(userMessage)
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bySess[sessionID]
	if !ok || b == nil || b.Plan == nil {
		return nil, nil, false
	}
	if userMessage != "" && strings.TrimSpace(b.Plan.UserMessage) != "" &&
		strings.TrimSpace(b.Plan.UserMessage) != userMessage {
		return nil, nil, false
	}
	delete(s.bySess, sessionID)
	delete(s.byPlan, b.Plan.ID)
	return b.Plan, b.Alloc, true
}

// PeekRecoveredPlan returns the restored plan for a spirit session without consuming it.
func (o *TaskOrchestratorImpl) PeekRecoveredPlan(spiritSessionID string) (*biz.TaskPlan, *biz.AllocationPlan, bool) {
	if o == nil || o.plans == nil {
		return nil, nil, false
	}
	b, ok := o.plans.peek(spiritSessionID)
	if !ok {
		return nil, nil, false
	}
	return b.Plan, b.Alloc, true
}

// ConsumeRecoveredPlan implements biz.RecoveredPlanConsumer.
func (o *TaskOrchestratorImpl) ConsumeRecoveredPlan(spiritSessionID, userMessage string) (*biz.TaskPlan, *biz.AllocationPlan, bool) {
	if o == nil || o.plans == nil {
		return nil, nil, false
	}
	return o.plans.consume(spiritSessionID, userMessage)
}

// restorePlansForHandle reloads the TaskPlan / AllocationPlan referenced by an
// interrupted OrchestrationHandle. Missing or unrestorable plans are skipped
// with a structured log — Recover still succeeds for checkpoint resume, but
// the cache is left empty so callers cannot pretend a plan was restored.
func (o *TaskOrchestratorImpl) restorePlansForHandle(ctx context.Context, handle *biz.OrchestrationHandle) {
	if o == nil || handle == nil {
		return
	}
	if strings.TrimSpace(handle.TaskPlanID) == "" {
		o.lg.Warn("TaskOrchestrator: skip plan restore, handle has no task_plan_id",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Str("reason", "no_task_plan_id"),
		)
		return
	}
	if o.plans == nil || o.plans.tasks == nil {
		o.lg.Warn("TaskOrchestrator: skip plan restore, task plan repo unavailable",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Str("task_plan_id", handle.TaskPlanID),
			loggateway.Str("reason", "task_plan_repo_nil"),
		)
		return
	}
	plan, err := o.plans.tasks.GetByID(ctx, handle.TaskPlanID)
	if err != nil || plan == nil {
		reason := "task_plan_missing"
		if err != nil && !apierror.IsCode(err, apierror.CodeNotFound) {
			reason = "task_plan_query_failed"
		}
		o.lg.Warn("TaskOrchestrator: skip plan restore, persisted TaskPlan not loaded",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Str("task_plan_id", handle.TaskPlanID),
			loggateway.Str("reason", reason),
			loggateway.Err(err),
		)
		return
	}
	o.tryCacheRestoredPlan(ctx, plan, handle.AllocationID, handle.ID)
}

// restoreOrphanedPlans reloads non-terminal TaskPlans that have no interrupted
// OrchestrationHandle (the current plan_and_execute path does not persist
// handles). Incomplete drafts (team strategy with empty SubTasks) are skipped.
func (o *TaskOrchestratorImpl) restoreOrphanedPlans(ctx context.Context) {
	if o == nil || o.plans == nil || o.plans.tasks == nil {
		return
	}
	plans, err := o.plans.tasks.ListByStatuses(ctx, biz.RecoverableTaskPlanStatuses)
	if err != nil {
		o.lg.Warn("TaskOrchestrator: list recoverable TaskPlans failed",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("reason", "list_plans_failed"),
			loggateway.Err(err),
		)
		return
	}
	var restored, skipped int
	for _, plan := range plans {
		if plan == nil || o.plans.hasPlan(plan.ID) {
			continue
		}
		if o.tryCacheRestoredPlan(ctx, plan, "", "") {
			restored++
		} else {
			skipped++
		}
	}
	if restored > 0 || skipped > 0 || len(plans) > 0 {
		o.lg.Info("TaskOrchestrator: orphaned plan restore finished",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Int("listed", len(plans)),
			loggateway.Int("restored", restored),
			loggateway.Int("skipped", skipped),
		)
	}
}

// tryCacheRestoredPlan validates a persisted plan and caches it when restorable.
// Returns true when the plan was cached.
func (o *TaskOrchestratorImpl) tryCacheRestoredPlan(ctx context.Context, plan *biz.TaskPlan, allocationID, orchestrationID string) bool {
	if plan == nil {
		return false
	}
	reason, ok := restorablePlanReason(plan)
	if !ok {
		o.lg.Warn("TaskOrchestrator: skip unrestorable TaskPlan",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", orchestrationID),
			loggateway.Str("task_plan_id", plan.ID),
			loggateway.Str("spirit_session_id", plan.SpiritSessionID),
			loggateway.Str("status", string(plan.Status)),
			loggateway.Str("strategy", string(plan.Strategy)),
			loggateway.Int("subtask_count", len(plan.SubTasks)),
			loggateway.Str("reason", reason),
		)
		return false
	}

	plan.DomainPath = PrimaryDomainPath(plan.SubTasks)
	alloc := o.loadAllocationForPlan(ctx, plan, allocationID)
	o.plans.put(plan.SpiritSessionID, &recoveredPlanBundle{Plan: plan, Alloc: alloc})

	o.lg.Info("TaskOrchestrator: Phase 1/2 plan restored",
		loggateway.StepID(biz.SpiritStepOrchestratorRecover),
		loggateway.Str("orchestration_id", orchestrationID),
		loggateway.Str("task_plan_id", plan.ID),
		loggateway.Str("spirit_session_id", plan.SpiritSessionID),
		loggateway.Str("status", string(plan.Status)),
		loggateway.Str("strategy", string(plan.Strategy)),
		loggateway.Int("subtask_count", len(plan.SubTasks)),
		loggateway.Bool("allocation_restored", alloc != nil),
	)
	return true
}

func (o *TaskOrchestratorImpl) loadAllocationForPlan(ctx context.Context, plan *biz.TaskPlan, allocationID string) *biz.AllocationPlan {
	if o.plans == nil || o.plans.allocs == nil || plan == nil {
		return nil
	}
	allocationID = strings.TrimSpace(allocationID)
	if allocationID != "" {
		alloc, err := o.plans.allocs.GetByID(ctx, allocationID)
		if err != nil {
			o.lg.Warn("TaskOrchestrator: AllocationPlan GetByID failed, trying session list",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("allocation_id", allocationID),
				loggateway.Str("task_plan_id", plan.ID),
				loggateway.Str("reason", "allocation_get_failed"),
				loggateway.Err(err),
			)
		} else if alloc != nil {
			return alloc
		}
	}
	if strings.TrimSpace(plan.SpiritSessionID) == "" {
		return nil
	}
	list, err := o.plans.allocs.ListBySpiritSessionID(ctx, plan.SpiritSessionID)
	if err != nil {
		o.lg.Warn("TaskOrchestrator: list AllocationPlans failed",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("task_plan_id", plan.ID),
			loggateway.Str("spirit_session_id", plan.SpiritSessionID),
			loggateway.Str("reason", "allocation_list_failed"),
			loggateway.Err(err),
		)
		return nil
	}
	for _, a := range list {
		if a != nil && a.TaskPlanID == plan.ID {
			return a
		}
	}
	return nil
}

// restorablePlanReason returns ok=false with a stable reason code when a
// persisted plan must not be treated as recovered.
func restorablePlanReason(plan *biz.TaskPlan) (reason string, ok bool) {
	if plan == nil {
		return "plan_nil", false
	}
	if !biz.IsRecoverableTaskPlanStatus(plan.Status) {
		return "terminal_status", false
	}
	if strings.TrimSpace(plan.SpiritSessionID) == "" {
		return "no_spirit_session_id", false
	}
	if planNeedsSubtasksToResume(plan) && len(plan.SubTasks) == 0 {
		return "incomplete_draft_no_subtasks", false
	}
	return "", true
}

func planNeedsSubtasksToResume(plan *biz.TaskPlan) bool {
	switch plan.Strategy {
	case biz.StrategyDirect, "":
		return false
	default:
		return true
	}
}
