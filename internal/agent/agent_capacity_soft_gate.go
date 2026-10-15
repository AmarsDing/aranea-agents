package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// applyCapacitySoftGate reassigns subtasks whose lead (or members) already sit
// at AgentCapability.Capacity, preferring free roster peers. Soft: if every
// candidate is busy, the original assignment is kept with an annotated reason
// unless CapacityHardGate is enabled (Redis wired), in which case the
// assignment is cleared (capacity_hard_blocked) so PlanExecutor cannot
// oversubscribe the agent across instances.
//
// loads starts from GlobalAgentLoadTracker (live team runs + Redis merge) and
// accumulates provisional assignments within this Allocate call so Capacity=1
// agents are not handed to multiple concurrent DAG nodes in one plan.
func (impl *agentAllocatorImpl) applyCapacitySoftGate(
	ctx context.Context,
	taskPlan *biz.TaskPlan,
	allocations []biz.TaskAllocation,
	assignable []biz.AgentCapability,
	traceID string,
) []biz.TaskAllocation {
	if len(allocations) == 0 {
		return allocations
	}
	caps := capacityByKey(assignable)
	keys := make([]string, 0, len(caps))
	for k := range caps {
		keys = append(keys, k)
	}
	loads := GlobalAgentLoadTracker.SnapshotFor(keys)
	out := make([]biz.TaskAllocation, len(allocations))
	copy(out, allocations)
	hard := CapacityHardGate

	for i := range out {
		alloc := out[i]
		if alloc.AssignedKey == "" {
			continue
		}
		busyLead := softGateBusy(loads, caps, alloc.AssignedKey)
		busyMember := false
		for _, k := range alloc.TeamMemberKeys {
			if softGateBusy(loads, caps, k) {
				busyMember = true
				break
			}
		}
		if !busyLead && !busyMember {
			noteAssignment(loads, alloc)
			continue
		}

		sub := biz.SubTask{ID: alloc.SubTaskID, Name: alloc.SubTaskID}
		if taskPlan != nil && i < len(taskPlan.SubTasks) {
			sub = taskPlan.SubTasks[i]
		}
		pool, prune := impl.matchingPool(sub.DomainPath, assignable, traceID)
		filtered := filterUnderCapacity(pool, loads, caps, alloc.AssignedKey)
		if len(filtered) == 0 {
			out[i] = capacityBusyFallback(impl, alloc, sub, traceID, hard, loads)
			continue
		}

		rematched, err := impl.matchSubTask(ctx, sub, filtered, traceID)
		if err != nil || rematched.AssignedKey == "" {
			out[i] = capacityBusyFallback(impl, alloc, sub, traceID, hard, loads)
			continue
		}
		stampOrgOnAlloc(&rematched, filtered, prune)
		if rematched.MatchReason != "" {
			rematched.MatchReason = rematched.MatchReason + "; capacity_soft_reassign"
		} else {
			rematched.MatchReason = "capacity_soft_reassign"
		}
		if impl.lg != nil {
			impl.lg.Info("capacity soft gate: reassigned under-capacity agent",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("sub_task_id", sub.ID),
				loggateway.Str("from", alloc.AssignedKey),
				loggateway.Str("to", rematched.AssignedKey),
			)
		}
		out[i] = rematched
		noteAssignment(loads, rematched)
	}
	return out
}

func capacityBusyFallback(
	impl *agentAllocatorImpl,
	alloc biz.TaskAllocation,
	sub biz.SubTask,
	traceID string,
	hard bool,
	loads map[string]int,
) biz.TaskAllocation {
	if hard {
		if impl.lg != nil {
			impl.lg.Warn("capacity hard gate: blocking assignment (all candidates busy)",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("sub_task_id", sub.ID),
				loggateway.Str("agent_key", alloc.AssignedKey),
			)
		}
		alloc.AssignedKey = ""
		alloc.AssignedName = ""
		alloc.TeamMemberKeys = nil
		alloc.MatchReason = "capacity_hard_blocked"
		return alloc
	}
	if impl.lg != nil {
		impl.lg.Info("capacity soft gate: all candidates busy, keeping assignment",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("sub_task_id", sub.ID),
			loggateway.Str("agent_key", alloc.AssignedKey),
		)
	}
	if alloc.MatchReason != "" && !strings.Contains(alloc.MatchReason, "capacity_soft") {
		alloc.MatchReason = alloc.MatchReason + "; capacity_soft_kept"
	} else if alloc.MatchReason == "" {
		alloc.MatchReason = "capacity_soft_kept"
	}
	noteAssignment(loads, alloc)
	return alloc
}

func filterUnderCapacity(pool []biz.AgentCapability, loads, caps map[string]int, excludeBusyPrimary string) []biz.AgentCapability {
	var out []biz.AgentCapability
	for _, c := range pool {
		if softGateBusy(loads, caps, c.AgentKey) {
			continue
		}
		// Always drop the busy primary we are replacing; members may still be chosen later.
		if excludeBusyPrimary != "" && c.AgentKey == excludeBusyPrimary {
			continue
		}
		out = append(out, c)
	}
	return out
}
