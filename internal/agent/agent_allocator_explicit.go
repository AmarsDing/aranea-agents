package agent

import (
	"aranea-agents/internal/biz"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"

	"strings"

	"github.com/google/uuid"
)

// AllocateExplicit implements biz.AgentAllocatorPort.AllocateExplicit.
// Explicit routing skips every heuristic layer: agentKeys[0] executes every
// subtask (lead), remaining keys become team members under dag strategy.
// Display names resolve best-effort via AgentReader; unknown keys fall back
// to the key itself so routing never fails on a missing catalog row.
func (impl *agentAllocatorImpl) AllocateExplicit(ctx context.Context, taskPlan *biz.TaskPlan, agentKeys []string) (*biz.AllocationPlan, error) {
	if taskPlan == nil {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "task plan is required")
	}
	keys := make([]string, 0, len(agentKeys))
	seen := make(map[string]bool, len(agentKeys))
	for _, k := range agentKeys {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "agent_keys is required for explicit allocation")
	}

	traceID := taskPlan.TraceID
	if traceID == "" {
		traceID, _ = biz.SpiritTraceIDFromContext(ctx)
	}

	keys = impl.resolveExplicitKeys(ctx, keys, traceID)

	impl.lg.Info("AgentAllocator.AllocateExplicit 显式路由",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("task_plan_id", taskPlan.ID),
		loggateway.Str("agent_keys", strings.Join(keys, ",")),
	)

	isDAG := taskPlan.Strategy == biz.StrategyDAG
	displayName := func(agentKey string) string {
		if impl.agentReader != nil {
			if ag, err := impl.agentReader.GetAgentByAgentKey(ctx, agentKey); err == nil && ag.DisplayName != "" {
				return ag.DisplayName
			}
		}
		return agentKey
	}
	buildAlloc := func(subTaskID, subTaskName string) biz.TaskAllocation {
		alloc := biz.TaskAllocation{
			SubTaskID:    subTaskID,
			SubTaskName:  subTaskName,
			AssignedType: "agent",
			AssignedKey:  keys[0],
			AssignedName: displayName(keys[0]),
			MatchScore:   1.0,
			MatchLayer:   "explicit",
			MatchReason:  "Spirit 显式指定 Agent（agent_keys）",
		}
		if isDAG && len(keys) > 1 {
			alloc.AssignedType = "team"
			alloc.TeamMemberKeys = append([]string(nil), keys[1:]...)
		}
		return alloc
	}

	var allocations []biz.TaskAllocation
	if len(taskPlan.SubTasks) == 0 {
		allocations = append(allocations, buildAlloc("whole", taskPlan.UserMessage))
	} else {
		allocations = make([]biz.TaskAllocation, 0, len(taskPlan.SubTasks))

		perSubtask := !isDAG && len(keys) == len(taskPlan.SubTasks) && len(keys) > 1
		for i, st := range taskPlan.SubTasks {
			alloc := buildAlloc(st.ID, st.Name)
			if perSubtask {
				alloc.AssignedKey = keys[i]
				alloc.AssignedName = displayName(keys[i])
				alloc.MatchReason = "Spirit 显式指定 Agent（agent_keys 与子任务一一对应）"
			}
			allocations = append(allocations, alloc)
		}
	}

	plan := &biz.AllocationPlan{
		ID:              "ap_" + uuid.NewString(),
		TaskPlanID:      taskPlan.ID,
		SpiritSessionID: taskPlan.SpiritSessionID,
		TraceID:         traceID,
		Allocations:     allocations,
		Status:          biz.AllocationStatusDraft,
	}
	return impl.persistAllocationPlan(ctx, plan, traceID)
}

// resolveExplicitKeys best-effort maps caller-supplied explicit keys to real
// agent_keys: exact agent_key → itself; display name → its key; department /
// company name (e.g. "技术部") → that org node's lead agent key. Unresolvable
// inputs are kept verbatim (contract: explicit routing never fails on a
// missing catalog row — downstream assembly is the arbiter). Remaps and
// unresolvable inputs are warn-logged for audit.
func (impl *agentAllocatorImpl) resolveExplicitKeys(ctx context.Context, keys []string, traceID string) []string {
	if impl.capBuilder == nil || len(keys) == 0 {
		return keys
	}
	caps, err := impl.capBuilder.BuildAll(ctx)
	if err != nil || len(caps) == 0 {
		if err != nil {
			impl.lg.Warn("显式路由解析：构建能力目录失败，keys 原样透传",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
		}
		return keys
	}

	byKey := make(map[string]struct{}, len(caps))
	byName := make(map[string]string, len(caps))
	for _, c := range caps {
		byKey[c.AgentKey] = struct{}{}
		if n := strings.TrimSpace(c.DisplayName); n != "" {
			if _, dup := byName[n]; !dup {
				byName[n] = c.AgentKey
			}
		}
	}

	out := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		resolved := k
		if _, ok := byKey[k]; !ok {
			if real, hit := byName[k]; hit {
				resolved = real
			} else if real, hit := resolveOrgNameKey(k, caps); hit {
				resolved = real
			}
		}
		if resolved != k {
			impl.lg.Warn("显式路由 agent_key 重映射",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("input", k),
				loggateway.Str("resolved", resolved),
			)
		} else if _, ok := byKey[k]; !ok {
			impl.lg.Warn("显式路由 agent_key 无法解析，原样透传（下游校验将裁决）",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("input", k),
			)
		}
		if _, dup := seen[resolved]; dup {
			continue
		}
		seen[resolved] = struct{}{}
		out = append(out, resolved)
	}
	if len(out) == 0 {
		return keys
	}
	return out
}

// resolveOrgNameKey resolves an organization display name (department like
// "技术部" or company name) to an agent_key. Departments prefer their dept
// lead; companies prefer their company lead. Falls back to any positioned
// member of the department when no lead exists.
func resolveOrgNameKey(name string, caps []biz.AgentCapability) (string, bool) {
	norm := normalizeOrgName(name)
	if norm == "" {
		return "", false
	}
	match := func(orgName string) bool {
		n := normalizeOrgName(orgName)
		if n == "" {
			return false
		}
		return n == norm || strings.Contains(n, norm) || strings.Contains(norm, n)
	}

	for _, c := range caps {
		if c.DepartmentName == "" || !match(c.DepartmentName) {
			continue
		}
		if biz.IsDeptLeadAgent(biz.Agent{AgentKey: c.AgentKey, AgentVariant: c.AgentVariant}) {
			return c.AgentKey, true
		}
	}

	for _, c := range caps {
		if c.CompanyName == "" || !match(c.CompanyName) {
			continue
		}
		if biz.IsCompanyLeadAgent(biz.Agent{AgentKey: c.AgentKey, AgentVariant: c.AgentVariant}) {
			return c.AgentKey, true
		}
	}

	for _, c := range caps {
		if c.DepartmentName != "" && match(c.DepartmentName) && c.IsHeuristicAssignable() {
			return c.AgentKey, true
		}
	}
	return "", false
}

// normalizeOrgName strips common org suffixes so "技术部" matches "技术开发部"
// (normalized "技术" vs "技术开发", containment match).
func normalizeOrgName(s string) string {
	s = strings.TrimSpace(s)
	for _, suffix := range []string{"部门", "中心", "团队", "部", "组", "处", "科"} {
		s = strings.TrimSuffix(s, suffix)
	}
	return s
}
