package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"

	"fmt"

	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"time"
)

// agentAllocatorImpl implements biz.AgentAllocatorPort.
type agentAllocatorImpl struct {
	repo               biz.AllocationPlanRepository
	agentReader        biz.AgentReader
	perfRepo           biz.AgentPerformanceRepository
	orchCache          *biz.OrchestrationCache
	capBuilder         *AgentCapabilityBuilder
	catalog            *biz.LlmProviderModelUsecase
	httpClient         *http.Client
	bus                biz.EventBus // Phase 3b-D: v2 EventBus
	lg                 loggateway.Logger
	embedder           knowledge.Embedder
	agentFactory       biz.AgentFactory
	plannerSetting     PlannerModelLookup
	staffingAdvisor    biz.StaffingAdvisor
	staffingTimeout    time.Duration
	allowFactoryCreate bool // opt-in; default hot path never creates agents

	// auxUsage records allocator cold-start LLM usage (M83 LBG-6). nil = 落账
	// 跳过（旧测试构造路径），行为与旧版一致。Attached at wire time via
	// AttachAllocatorAuxUsageRecorder.
	auxUsage SpiritAuxUsageRecorder
}

var _ biz.AgentAllocatorPort = (*agentAllocatorImpl)(nil)

// NewAgentAllocator creates a new AgentAllocator implementation.
func NewAgentAllocator(
	repo biz.AllocationPlanRepository,
	agentReader biz.AgentReader,
	perfRepo biz.AgentPerformanceRepository,
	orchCache *biz.OrchestrationCache,
	capBuilder *AgentCapabilityBuilder,
	catalog *biz.LlmProviderModelUsecase,
	httpClient *http.Client,
	bus biz.EventBus,
	lg loggateway.Logger,
	embedder knowledge.Embedder,
	agentFactory biz.AgentFactory,
	plannerSetting PlannerModelLookup,
) biz.AgentAllocatorPort {
	return &agentAllocatorImpl{
		repo:           repo,
		agentReader:    agentReader,
		perfRepo:       perfRepo,
		orchCache:      orchCache,
		capBuilder:     capBuilder,
		catalog:        catalog,
		httpClient:     httpClient,
		bus:            bus,
		lg:             lg,
		embedder:       embedder,
		agentFactory:   agentFactory,
		plannerSetting: plannerSetting,
	}
}

// AttachAllocatorAuxUsageRecorder wires the spirit aux usage recorder so
// allocator cold-start match LLM calls are billed (M83 LBG-6) without
// changing the NewAgentAllocator constructor.
func AttachAllocatorAuxUsageRecorder(a biz.AgentAllocatorPort, rec SpiritAuxUsageRecorder) {
	if impl, ok := a.(*agentAllocatorImpl); ok {
		impl.auxUsage = rec
	}
}

// Allocate matches each SubTask in the TaskPlan to the best Agent or Team.
func (impl *agentAllocatorImpl) Allocate(ctx context.Context, taskPlan *biz.TaskPlan) (*biz.AllocationPlan, error) {
	if taskPlan == nil {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "task plan is required")
	}

	traceID := taskPlan.TraceID
	if traceID == "" {
		traceID, _ = biz.SpiritTraceIDFromContext(ctx)
	}

	allocStart := time.Now()
	impl.lg.Info("AgentAllocator.Allocate 开始",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("task_plan_id", taskPlan.ID),
	)

	capStart := time.Now()
	capabilities, err := impl.capBuilder.BuildAll(ctx)
	if err != nil {
		impl.lg.Warn("构建 Agent 能力列表失败",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return nil, apierror.Internal(apierror.DomainSpirit, "build capabilities").WithCause(err)
	}
	capMs := time.Since(capStart).Milliseconds()

	assignable := filterHeuristicAssignable(capabilities)

	isDAG := taskPlan.Strategy == biz.StrategyDAG
	totalSubTasks := len(taskPlan.SubTasks)

	matchStart := time.Now()
	results := impl.runPhaseAMatch(ctx, taskPlan.SubTasks, assignable, traceID)
	matchMs := time.Since(matchStart).Milliseconds()
	staffStart := time.Now()

	allocations := make([]biz.TaskAllocation, 0, totalSubTasks)
	for i, subTask := range taskPlan.SubTasks {
		res := results[i]
		var allocation biz.TaskAllocation
		if res.matchErr == nil {
			allocation = res.alloc
		} else {
			impl.lg.Warn("子任务匹配失败，尝试主管 staffing（花名册内）",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("sub_task_id", subTask.ID),
				loggateway.Err(res.matchErr),
			)
			pool, prune := impl.matchingPool(subTask.DomainPath, assignable, traceID)
			if staffed, ok := impl.tryStaffing(ctx, subTask, pool, prune, traceID); ok {
				allocation = staffed
			} else if impl.allowFactoryCreate {
				if factoryAlloc, ok := impl.tryAgentFactoryForSubTask(ctx, subTask, taskPlan.SpiritSessionID, traceID, assignable); ok {
					allocation = factoryAlloc
				} else {
					return nil, impl.failRoster(ctx, taskPlan.SpiritSessionID, subTask, assignable)
				}
			} else {
				return nil, impl.failRoster(ctx, taskPlan.SpiritSessionID, subTask, assignable)
			}
		}
		pool, prune := impl.matchingPool(subTask.DomainPath, assignable, traceID)

		if isDAG && allocation.AssignedKey != "" {

			additional := allocation.TeamMemberKeys
			var crossDept []string
			wantExtra := dagAdditionalMemberCount(taskPlan.UserMessage)
			if len(additional) == 0 {
				additional, crossDept = impl.selectAdditionalMembers(allocation.AssignedKey, pool, wantExtra)
			} else {
				_, crossDept = classifyCrossDeptMembers(allocation.AssignedKey, additional, pool)
			}

			if len(additional) == 0 {
				additional, crossDept = impl.selectAdditionalMembers(allocation.AssignedKey, assignable, wantExtra)
			}
			if len(additional) > 0 {
				allocation.TeamMemberKeys = additional
				allocation.CrossDeptMemberKeys = crossDept
				allocation.AssignedType = "team"
				impl.lg.Info("DAG 模式：为子任务分配多成员团队",
					loggateway.StepID(biz.SpiritStepAllocatorMatch),
					loggateway.Str("trace_id", traceID),
					loggateway.Str("sub_task_id", subTask.ID),
					loggateway.Str("lead", allocation.AssignedKey),
					loggateway.Str("members", strings.Join(additional, ",")),
				)
			} else {
				impl.lg.Warn("DAG 模式未能补齐第二名成员，花名册可分配专家不足",
					loggateway.StepID(biz.SpiritStepAllocatorMatch),
					loggateway.Str("trace_id", traceID),
					loggateway.Str("sub_task_id", subTask.ID),
					loggateway.Str("lead", allocation.AssignedKey),
				)
			}
		}
		stampOrgOnAlloc(&allocation, pool, prune)
		observeOrgFastDeptLead(allocation.MatchLayer)
		allocations = append(allocations, allocation)

		impl.publishAllocatingProgress(ctx, taskPlan.SpiritSessionID, i+1, totalSubTasks, subTask, allocation)
	}

	if len(taskPlan.SubTasks) == 0 {
		pool, prune := impl.matchingPool(taskPlan.DomainPath, assignable, traceID)
		allocation, err := impl.matchWholePlan(ctx, taskPlan, pool, traceID)
		if err != nil {
			st := biz.SubTask{ID: "whole", Name: taskPlan.UserMessage, DomainPath: taskPlan.DomainPath}
			if staffed, ok := impl.tryStaffing(ctx, st, pool, prune, traceID); ok {
				allocation = staffed
			} else if impl.allowFactoryCreate {
				if factoryAlloc, ok := impl.tryAgentFactoryForPlan(ctx, taskPlan, traceID, pool); ok {
					allocation = factoryAlloc
				} else {
					return nil, impl.failRoster(ctx, taskPlan.SpiritSessionID, st, assignable)
				}
			} else {
				return nil, impl.failRoster(ctx, taskPlan.SpiritSessionID, st, assignable)
			}
		}
		stampOrgOnAlloc(&allocation, pool, prune)
		observeOrgFastDeptLead(allocation.MatchLayer)
		allocations = append(allocations, allocation)
	}

	impl.lg.Info("子任务匹配完成",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.Int("allocation_count", len(allocations)),
		loggateway.Int64("cap_ms", capMs),
		loggateway.Int64("match_ms", matchMs),
		loggateway.Int64("staff_ms", time.Since(staffStart).Milliseconds()),
		loggateway.Int64("total_ms", time.Since(allocStart).Milliseconds()),
	)

	allocatedMeta := map[string]any{"total": len(allocations)}
	if len(allocations) > 0 && allocations[0].DepartmentID != "" {
		allocatedMeta["department_id"] = allocations[0].DepartmentID
	}
	impl.publishOrchestrationProgress(ctx, taskPlan.SpiritSessionID, "allocated", allocatedMeta)
	if sketch := collaborationSketch(taskPlan); sketch.SlotCount > 1 {
		impl.publishOrchestrationProgress(ctx, taskPlan.SpiritSessionID, "collaborating", sketch.meta())
	}

	plan := &biz.AllocationPlan{
		ID:              "ap_" + uuid.NewString(),
		TaskPlanID:      taskPlan.ID,
		SpiritSessionID: taskPlan.SpiritSessionID,
		TraceID:         traceID,
		Allocations:     allocations,
		Status:          biz.AllocationStatusDraft,
	}

	impl.lg.Info("持久化 AllocationPlan",
		loggateway.StepID(biz.SpiritStepAllocatorPersist),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("allocation_plan_id", plan.ID),
	)

	saved, err := impl.persistAllocationPlan(ctx, plan, traceID)
	if err != nil {
		return nil, err
	}
	return saved, nil
}

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

// persistAllocationPlan persists an AllocationPlan and publishes the
// spirit_allocation_created event. Shared by Allocate and AllocateExplicit.
func (impl *agentAllocatorImpl) persistAllocationPlan(ctx context.Context, plan *biz.AllocationPlan, traceID string) (*biz.AllocationPlan, error) {
	saved, err := impl.repo.Create(ctx, plan)
	if err != nil {
		impl.lg.Warn("AllocationPlan 持久化失败",
			loggateway.StepID(biz.SpiritStepAllocatorPersist),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return nil, apierror.Internal(apierror.DomainSpirit, "persist allocation plan").WithCause(err)
	}
	impl.publishAllocationCreated(ctx, saved)
	return saved, nil
}

// GetAllocation retrieves an allocation plan by ID.
func (impl *agentAllocatorImpl) GetAllocation(ctx context.Context, allocationID string) (*biz.AllocationPlan, error) {
	return impl.repo.GetByID(ctx, allocationID)
}

// phaseAResult carries one subtask's Phase A match outcome (P-ORCH.5).
type phaseAResult struct {
	alloc    biz.TaskAllocation
	matchErr error
}

// runPhaseAMatch executes Phase A of the two-phase allocation (P-ORCH.5):
// parallel matchSubTask (Layer 0-3, no factory) across all subtasks.
// Each goroutine writes its result to a pre-allocated slice slot so the
// input subtask order is preserved regardless of completion timing.
// Match errors are captured per-slot, never returned — Phase B handles them.
func (impl *agentAllocatorImpl) runPhaseAMatch(ctx context.Context, subTasks []biz.SubTask, capabilities []biz.AgentCapability, traceID string) []phaseAResult {
	results := make([]phaseAResult, len(subTasks))
	if len(subTasks) == 0 {
		return results
	}

	shared := newLayer2Shared()
	if impl.embedder != nil {
		_, _, _ = shared.loadAgentVectors(ctx, impl.embedder, capabilities)
	}
	g, gctx := errgroup.WithContext(ctx)
	for i, subTask := range subTasks {
		i, subTask := i, subTask
		g.Go(func() error {
			pool, _ := impl.matchingPool(subTask.DomainPath, capabilities, traceID)
			alloc, err := impl.matchSubTask(gctx, subTask, pool, traceID, shared)
			results[i] = phaseAResult{alloc: alloc, matchErr: err}
			return nil
		})
	}
	_ = g.Wait()
	return results
}

// matchSubTask matches a single subtask to the best agent/team.
// shared 为可选的 per-Allocate Layer2 共享态（P3b）；不传时退化为一次性实例。
func (impl *agentAllocatorImpl) matchSubTask(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string, sharedOpt ...*layer2Shared) (biz.TaskAllocation, error) {
	var shared *layer2Shared
	if len(sharedOpt) > 0 {
		shared = ensureLayer2Shared(sharedOpt[0])
	} else {
		shared = newLayer2Shared()
	}

	assignedType := "agent"
	if subTask.EstimatedComplexity >= 0.5 {
		assignedType = "team"
	}

	if subTask.DomainPath != "" {
		if cap, members, dq, ok := impl.tryDomainRecipe(subTask.DomainPath, capabilities, traceID); ok {
			alloc := biz.TaskAllocation{
				SubTaskID:    subTask.ID,
				SubTaskName:  subTask.Name,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   dq,
				MatchLayer:   "domain_recipe",
				MatchReason:  fmt.Sprintf("领域配方复用 (domain: %s, DQ %.2f)", subTask.DomainPath, dq),
			}
			if len(members) > 0 {
				alloc.TeamMemberKeys = members
			}
			return alloc, nil
		}
		if cap, backup, ok := bindRosterSpecialist(subTask.DomainPath, subTask.Name+" "+subTask.Description, capabilities); ok {
			alloc := biz.TaskAllocation{
				SubTaskID:    subTask.ID,
				SubTaskName:  subTask.Name,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   1,
				MatchLayer:   "roster",
				MatchReason:  "专项花名册绑定 " + NormalizeDomainPath(subTask.DomainPath),
			}
			if backup != "" {
				alloc.FallbackKey = backup
			}
			return alloc, nil
		}

		if spec := NormalizeDomainPath(subTask.DomainPath); spec == "研究/调研" {
			if cap, backup, ok := bindRosterSpecialist("办公/文档", subTask.Name+" "+subTask.Description, capabilities); ok {
				alloc := biz.TaskAllocation{
					SubTaskID:    subTask.ID,
					SubTaskName:  subTask.Name,
					AssignedType: assignedType,
					AssignedKey:  cap.AgentKey,
					AssignedName: cap.DisplayName,
					MatchScore:   0.8,
					MatchLayer:   "roster",
					MatchReason:  "研究/调研 名册缺失，回退 办公/文档",
				}
				if backup != "" {
					alloc.FallbackKey = backup
				}
				return alloc, nil
			}
		}
		taskText := subTask.Name + " " + subTask.Description
		if cap, score, candCount, ok := impl.tryMissionMatch(ctx, taskText, subTask.DomainPath, capabilities, traceID); ok {
			return biz.TaskAllocation{
				SubTaskID:    subTask.ID,
				SubTaskName:  subTask.Name,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   score,
				MatchLayer:   "mission",
				MatchReason:  missionMatchReason(subTask.DomainPath, candCount, score),
			}, nil
		}
	}

	if impl.perfRepo != nil {
		taskTypes := make([]string, 0, 2)
		if subTask.DomainPath != "" {
			taskTypes = append(taskTypes, "domain:"+subTask.DomainPath)
		}
		if len(subTask.RequiredCapabilities) > 0 {
			taskTypes = append(taskTypes, subTask.RequiredCapabilities[0])
		}
		for _, taskType := range taskTypes {
			bestPerfs, err := impl.perfRepo.GetBestForTaskType(ctx, taskType, 1)
			if err != nil || len(bestPerfs) == 0 {
				continue
			}
			bestAgentKey := bestPerfs[0].AgentKey
			for _, cap := range capabilities {
				if cap.AgentKey == bestAgentKey {
					impl.lg.Info("AgentPerformance.GetBestForTaskType 命中",
						loggateway.StepID(biz.SpiritStepAllocatorMatch),
						loggateway.Str("trace_id", traceID),
						loggateway.Str("sub_task_id", subTask.ID),
						loggateway.Str("agent_key", bestAgentKey),
						loggateway.Str("task_type", taskType),
						loggateway.Float64("success_rate", bestPerfs[0].SuccessRate),
					)
					return biz.TaskAllocation{
						SubTaskID:    subTask.ID,
						SubTaskName:  subTask.Name,
						AssignedType: assignedType,
						AssignedKey:  bestAgentKey,
						AssignedName: cap.DisplayName,
						MatchScore:   bestPerfs[0].SuccessRate,
						MatchLayer:   "performance",
						MatchReason:  fmt.Sprintf("历史性能最优 (成功率 %.2f, DQ %.2f)", bestPerfs[0].SuccessRate, bestPerfs[0].AvgDQScore),
					}, nil
				}
			}
		}
	}

	bestMatch, bestScore, matchReason := impl.exactMatch(subTask.RequiredCapabilities, capabilities)

	if bestScore > 0.5 {
		return biz.TaskAllocation{
			SubTaskID:    subTask.ID,
			SubTaskName:  subTask.Name,
			AssignedType: assignedType,
			AssignedKey:  bestMatch.AgentKey,
			AssignedName: bestMatch.DisplayName,
			MatchScore:   bestScore,
			MatchLayer:   "exact",
			MatchReason:  matchReason,
		}, nil
	}

	if strings.TrimSpace(subTask.DomainPath) != "" {
		return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for subtask %s", subTask.ID)
	}

	taskBlob := strings.TrimSpace(subTask.Name + " " + subTask.Description)
	if biz.HasTaskActionSignal(taskBlob) {
		return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for subtask %s", subTask.ID)
	}

	semCap, semScore, semReason := impl.matchLayer2(ctx, subTask, capabilities, traceID, shared)
	if semScore > 0.3 && semCap.AgentKey != "" {
		impl.lg.Info("Layer 2 语义匹配命中",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("sub_task_id", subTask.ID),
			loggateway.Str("agent_key", semCap.AgentKey),
			loggateway.Float64("score", semScore),
		)
		return biz.TaskAllocation{
			SubTaskID:    subTask.ID,
			SubTaskName:  subTask.Name,
			AssignedType: assignedType,
			AssignedKey:  semCap.AgentKey,
			AssignedName: semCap.DisplayName,
			MatchScore:   semScore,
			MatchLayer:   "semantic",
			MatchReason:  semReason,
		}, nil
	}

	llmMatch, err := impl.llmColdStart(ctx, subTask, capabilities, traceID)
	if err == nil && llmMatch != "" {

		displayName := llmMatch
		for _, cap := range capabilities {
			if cap.AgentKey == llmMatch {
				displayName = cap.DisplayName
				break
			}
		}
		return biz.TaskAllocation{
			SubTaskID:     subTask.ID,
			SubTaskName:   subTask.Name,
			AssignedType:  assignedType,
			AssignedKey:   llmMatch,
			AssignedName:  displayName,
			MatchScore:    bestScore,
			MatchLayer:    "llm_cold_start",
			MatchReason:   "LLM 冷启动匹配",
			FallbackKey:   bestMatch.AgentKey,
			FallbackScore: bestScore,
		}, nil
	}

	return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for subtask %s", subTask.ID)
}

// matchWholePlan handles allocation for plans without subtasks (simple/moderate).
func (impl *agentAllocatorImpl) matchWholePlan(ctx context.Context, taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability, traceID string) (biz.TaskAllocation, error) {

	assignedType := "agent"
	if taskPlan.Strategy == biz.StrategyCoordinator || taskPlan.Strategy == biz.StrategyParallel || taskPlan.Strategy == biz.StrategyDAG {
		assignedType = "team"
	}

	if taskPlan.DomainPath != "" {
		if cap, members, dq, ok := impl.tryDomainRecipe(taskPlan.DomainPath, capabilities, traceID); ok {
			alloc := biz.TaskAllocation{
				SubTaskID:    "whole",
				SubTaskName:  taskPlan.UserMessage,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   dq,
				MatchLayer:   "domain_recipe",
				MatchReason:  fmt.Sprintf("领域配方复用 (domain: %s, DQ %.2f)", taskPlan.DomainPath, dq),
			}
			if len(members) > 0 {
				alloc.TeamMemberKeys = members
			}
			return alloc, nil
		}
		if cap, backup, ok := bindRosterSpecialist(taskPlan.DomainPath, taskPlan.UserMessage, capabilities); ok {
			alloc := biz.TaskAllocation{
				SubTaskID:    "whole",
				SubTaskName:  taskPlan.UserMessage,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   1,
				MatchLayer:   "roster",
				MatchReason:  "专项花名册绑定 " + NormalizeDomainPath(taskPlan.DomainPath),
			}
			if backup != "" {
				alloc.FallbackKey = backup
			}
			return alloc, nil
		}
		if cap, score, candCount, ok := impl.tryMissionMatch(ctx, taskPlan.UserMessage, taskPlan.DomainPath, capabilities, traceID); ok {
			return biz.TaskAllocation{
				SubTaskID:    "whole",
				SubTaskName:  taskPlan.UserMessage,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   score,
				MatchLayer:   "mission",
				MatchReason:  missionMatchReason(taskPlan.DomainPath, candCount, score),
			}, nil
		}
	}

	if impl.perfRepo != nil {
		capHints := extractCapabilityHints(taskPlan.UserMessage)
		taskTypes := make([]string, 0, 2)
		if taskPlan.DomainPath != "" {
			taskTypes = append(taskTypes, "domain:"+taskPlan.DomainPath)
		}
		taskTypes = append(taskTypes, capHints[0])
		for _, taskType := range taskTypes {
			bestPerfs, err := impl.perfRepo.GetBestForTaskType(ctx, taskType, 1)
			if err != nil || len(bestPerfs) == 0 {
				continue
			}
			bestAgentKey := bestPerfs[0].AgentKey
			for _, cap := range capabilities {
				if cap.AgentKey == bestAgentKey {
					impl.lg.Info("AgentPerformance.GetBestForTaskType 命中 (whole plan)",
						loggateway.StepID(biz.SpiritStepAllocatorMatch),
						loggateway.Str("trace_id", traceID),
						loggateway.Str("agent_key", bestAgentKey),
						loggateway.Str("task_type", taskType),
						loggateway.Float64("success_rate", bestPerfs[0].SuccessRate),
					)
					return biz.TaskAllocation{
						SubTaskID:    "whole",
						SubTaskName:  taskPlan.UserMessage,
						AssignedType: assignedType,
						AssignedKey:  bestAgentKey,
						AssignedName: cap.DisplayName,
						MatchScore:   bestPerfs[0].SuccessRate,
						MatchLayer:   "performance",
						MatchReason:  fmt.Sprintf("历史性能最优 (成功率 %.2f, DQ %.2f)", bestPerfs[0].SuccessRate, bestPerfs[0].AvgDQScore),
					}, nil
				}
			}
		}
	}

	capHints := extractCapabilityHints(taskPlan.UserMessage)
	bestMatch, bestScore, matchReason := impl.exactMatch(capHints, capabilities)

	if bestScore > 0.3 && bestMatch.AgentKey != "" {
		return biz.TaskAllocation{
			SubTaskID:    "whole",
			SubTaskName:  taskPlan.UserMessage,
			AssignedType: assignedType,
			AssignedKey:  bestMatch.AgentKey,
			AssignedName: bestMatch.DisplayName,
			MatchScore:   bestScore,
			MatchLayer:   "exact",
			MatchReason:  matchReason,
		}, nil
	}

	if strings.TrimSpace(taskPlan.DomainPath) != "" {
		return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for plan")
	}

	semCap, semScore, semReason := impl.matchLayer2ForPlan(ctx, taskPlan, capabilities, traceID)
	if semScore > 0.3 && semCap.AgentKey != "" {
		impl.lg.Info("Layer 2 语义匹配命中 (whole plan)",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("agent_key", semCap.AgentKey),
			loggateway.Float64("score", semScore),
		)
		return biz.TaskAllocation{
			SubTaskID:    "whole",
			SubTaskName:  taskPlan.UserMessage,
			AssignedType: assignedType,
			AssignedKey:  semCap.AgentKey,
			AssignedName: semCap.DisplayName,
			MatchScore:   semScore,
			MatchLayer:   "semantic",
			MatchReason:  semReason,
		}, nil
	}

	llmMatch, err := impl.llmColdStartForPlan(ctx, taskPlan, capabilities, traceID)
	if err == nil && llmMatch != "" {
		displayName := llmMatch
		for _, cap := range capabilities {
			if cap.AgentKey == llmMatch {
				displayName = cap.DisplayName
				break
			}
		}
		return biz.TaskAllocation{
			SubTaskID:     "whole",
			SubTaskName:   taskPlan.UserMessage,
			AssignedType:  assignedType,
			AssignedKey:   llmMatch,
			AssignedName:  displayName,
			MatchScore:    bestScore,
			MatchLayer:    "llm_cold_start",
			MatchReason:   "LLM 冷启动匹配",
			FallbackKey:   bestMatch.AgentKey,
			FallbackScore: bestScore,
		}, nil
	}

	return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for plan")
}

// tryAgentFactoryForSubTask calls AgentFactory to dynamically create an Agent
// when 4-layer matching fails for a subtask (P1-4). Returns the allocation
// and true on success; returns zero value and false when AgentFactory is
// unavailable or creation fails (caller should fall back).
func (impl *agentAllocatorImpl) tryAgentFactoryForSubTask(ctx context.Context, subTask biz.SubTask, spiritSessionID, traceID string, pool []biz.AgentCapability) (biz.TaskAllocation, bool) {
	if impl.agentFactory == nil {
		return biz.TaskAllocation{}, false
	}
	desc := strings.TrimSpace(subTask.Description)
	if desc == "" {
		desc = subTask.Name
	}
	domain := "engineering"
	if subTask.DomainPath != "" {
		domain = TopLevelDomain(subTask.DomainPath)
	}
	profile := biz.TaskProfile{
		RequiredCapabilities: subTask.RequiredCapabilities,
		Domain:               domain,
		DomainPath:           subTask.DomainPath,
		TaskDescription:      desc,
		SpiritSessionID:      spiritSessionID,
		DepartmentID:         departmentIDFromPool(subTask.DomainPath, pool),
	}
	agentKey, err := impl.agentFactory.EnsureAgent(ctx, profile)
	if err != nil {
		impl.lg.Warn("AgentFactory 创建失败，降级为 fallback",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("sub_task_id", subTask.ID),
			loggateway.Err(err),
		)
		return biz.TaskAllocation{}, false
	}
	impl.lg.Info("AgentFactory 动态创建 Agent 命中",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("sub_task_id", subTask.ID),
		loggateway.AgentKey(agentKey),
	)
	return biz.TaskAllocation{
		SubTaskID:    subTask.ID,
		SubTaskName:  subTask.Name,
		AssignedType: "agent",
		AssignedKey:  agentKey,
		AssignedName: subTask.Name,
		MatchScore:   0,
		MatchLayer:   "factory",
		MatchReason:  "AgentFactory 动态创建",
	}, true
}

// tryAgentFactoryForPlan calls AgentFactory for a whole-plan allocation when
// 4-layer matching fails (P1-4). Same contract as tryAgentFactoryForSubTask.
func (impl *agentAllocatorImpl) tryAgentFactoryForPlan(ctx context.Context, taskPlan *biz.TaskPlan, traceID string, pool []biz.AgentCapability) (biz.TaskAllocation, bool) {
	if impl.agentFactory == nil {
		return biz.TaskAllocation{}, false
	}
	domain := "engineering"
	if taskPlan.DomainPath != "" {
		domain = TopLevelDomain(taskPlan.DomainPath)
	}
	profile := biz.TaskProfile{
		RequiredCapabilities: extractCapabilityHints(taskPlan.UserMessage),
		Domain:               domain,
		DomainPath:           taskPlan.DomainPath,
		TaskDescription:      taskPlan.UserMessage,
		SpiritSessionID:      taskPlan.SpiritSessionID,
		DepartmentID:         departmentIDFromPool(taskPlan.DomainPath, pool),
	}
	agentKey, err := impl.agentFactory.EnsureAgent(ctx, profile)
	if err != nil {
		impl.lg.Warn("AgentFactory 创建失败 (whole plan)，降级为 fallback",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return biz.TaskAllocation{}, false
	}
	impl.lg.Info("AgentFactory 动态创建 Agent 命中 (whole plan)",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.AgentKey(agentKey),
	)
	return biz.TaskAllocation{
		SubTaskID:    "whole",
		SubTaskName:  taskPlan.UserMessage,
		AssignedType: "agent",
		AssignedKey:  agentKey,
		AssignedName: taskPlan.UserMessage,
		MatchScore:   0,
		MatchLayer:   "factory",
		MatchReason:  "AgentFactory 动态创建",
	}, true
}

// fallbackAllocation creates a fallback allocation when matching fails.
func (impl *agentAllocatorImpl) fallbackAllocation(subTask biz.SubTask, capabilities []biz.AgentCapability) biz.TaskAllocation {
	if len(capabilities) > 0 {
		return biz.TaskAllocation{
			SubTaskID:    subTask.ID,
			SubTaskName:  subTask.Name,
			AssignedType: "agent",
			AssignedKey:  capabilities[0].AgentKey,
			AssignedName: capabilities[0].DisplayName,
			MatchScore:   0,
			MatchLayer:   "fallback",
			MatchReason:  "匹配失败，使用第一个可用 Agent",
		}
	}
	return biz.TaskAllocation{
		SubTaskID:    subTask.ID,
		SubTaskName:  subTask.Name,
		AssignedType: "agent",
		AssignedKey:  biz.SpiritAgentKey,
		AssignedName: "Spirit",
		MatchScore:   0,
		MatchLayer:   "fallback",
		MatchReason:  "无可用 Agent，降级为 Spirit 直接回答",
	}
}

// fallbackWholePlanAllocation creates a fallback allocation for a whole plan.
func (impl *agentAllocatorImpl) fallbackWholePlanAllocation(taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability) biz.TaskAllocation {
	if len(capabilities) > 0 {
		return biz.TaskAllocation{
			SubTaskID:    "whole",
			SubTaskName:  taskPlan.UserMessage,
			AssignedType: "agent",
			AssignedKey:  capabilities[0].AgentKey,
			AssignedName: capabilities[0].DisplayName,
			MatchScore:   0,
			MatchLayer:   "fallback",
			MatchReason:  "匹配失败，使用第一个可用 Agent",
		}
	}
	return biz.TaskAllocation{
		SubTaskID:    "whole",
		SubTaskName:  taskPlan.UserMessage,
		AssignedType: "agent",
		AssignedKey:  biz.SpiritAgentKey,
		AssignedName: "Spirit",
		MatchScore:   0,
		MatchLayer:   "fallback",
		MatchReason:  "无可用 Agent，降级为 Spirit 直接回答",
	}
}
