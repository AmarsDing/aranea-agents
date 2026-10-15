package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"

	"net/http"
	"strings"

	"github.com/google/uuid"

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
