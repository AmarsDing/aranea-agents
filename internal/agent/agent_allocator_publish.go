// Code owned by package agent; extracted from agent_allocator_match_helpers.go for AS-COG-01 file-size fitness.

package agent

import (
	"aranea-agents/internal/biz"

	"context"
)

// publishAllocationCreated publishes the spirit_allocation_created event after an AllocationPlan is persisted.
func (impl *agentAllocatorImpl) publishAllocationCreated(ctx context.Context, plan *biz.AllocationPlan) {
	if impl.bus == nil || plan == nil {
		return
	}
	spiritSessionID := plan.SpiritSessionID
	meta := map[string]any{
		"allocation_id":     plan.ID,
		"task_plan_id":      plan.TaskPlanID,
		"spirit_session_id": spiritSessionID,
		"allocation_count":  len(plan.Allocations),
		"status":            string(plan.Status),
		"agent_key":         "agent-allocator",
	}
	impl.bus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "allocation_created", "", meta))
}

// publishAllocatingProgress emits a per-subtask allocating progress event
// (P-ORCH). index is 1-based; total is the number of subtasks.
func (impl *agentAllocatorImpl) publishAllocatingProgress(ctx context.Context, spiritSessionID string, index, total int, subTask biz.SubTask, alloc biz.TaskAllocation) {
	extra := map[string]any{
		"index":    index,
		"total":    total,
		"sub_task": subTask.Name,
	}
	if spec := NormalizeDomainPath(subTask.DomainPath); spec != "" {
		extra["specialty"] = spec
	}
	if alloc.AssignedName != "" {
		extra["agent_name"] = alloc.AssignedName
	}
	if alloc.MatchLayer != "" {
		extra["match_layer"] = alloc.MatchLayer
	}
	if alloc.DepartmentID != "" {
		extra["department_id"] = alloc.DepartmentID
	}
	impl.publishOrchestrationProgress(ctx, spiritSessionID, "allocating", extra)
}

func (impl *agentAllocatorImpl) failRoster(ctx context.Context, spiritSessionID string, subTask biz.SubTask, roster []biz.AgentCapability) error {
	impl.publishOrchestrationProgress(ctx, spiritSessionID, "allocate_failed", map[string]any{
		"sub_task":  subTask.Name,
		"specialty": NormalizeDomainPath(subTask.DomainPath),
		"reason":    "no_roster_specialist",
	})
	return rosterMissError(subTask, roster)
}

// publishOrchestrationProgress emits an orchestration_progress
// SystemNoticeEvent (WS-only, not persisted) so the frontend can render
// fine-grained loading feedback during allocation. Nil bus or empty session
// → skipped.
func (impl *agentAllocatorImpl) publishOrchestrationProgress(ctx context.Context, spiritSessionID, phase string, extra map[string]any) {
	if impl.bus == nil || spiritSessionID == "" {
		return
	}
	meta := map[string]any{"phase": phase}
	for k, v := range extra {
		meta[k] = v
	}
	impl.bus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_progress", "orchestration progress: "+phase, meta))
}
