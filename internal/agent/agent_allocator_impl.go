package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
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

	// Build agent capabilities from catalog
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

	// Heuristic matching never assigns dept_lead / system agents (M78 ORGFAST-01).
	assignable := filterHeuristicAssignable(capabilities)

	// Match each subtask
	isDAG := taskPlan.Strategy == biz.StrategyDAG
	totalSubTasks := len(taskPlan.SubTasks)

	// P-ORCH.5: two-phase parallelization — Phase A parallel matchSubTask
	// (Layer 0-3, no factory), Phase B serial factory creation (below).
	matchStart := time.Now()
	results := impl.runPhaseAMatch(ctx, taskPlan.SubTasks, assignable, traceID)
	matchMs := time.Since(matchStart).Milliseconds()
	staffStart := time.Now()

	// Phase B — serial factory / fallback for failures, then DAG member
	// selection, progress publish, and final assembly. Order follows the
	// input subtask order so the frontend loading sequence (i/N) is stable.
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
		// For dag mode: each subtask becomes a multi-member team (≥2 members).
		// The primary agent (AssignedKey) is the team lead; selectAdditionalMembers
		// prefers same-department complementary roles (M78 ORGFAST-03).
		if isDAG && allocation.AssignedKey != "" {
			// L0 配方成员优先（B.10.21.5）；无配方时按组织补员。
			additional := allocation.TeamMemberKeys
			var crossDept []string
			wantExtra := dagAdditionalMemberCount(taskPlan.UserMessage)
			if len(additional) == 0 {
				additional, crossDept = impl.selectAdditionalMembers(allocation.AssignedKey, pool, wantExtra)
			} else {
				_, crossDept = classifyCrossDeptMembers(allocation.AssignedKey, additional, pool)
			}
			// Domain-filtered pool can be a single specialist (empty
			// domain_path / one 巡检岗). Retry against the full assignable
			// roster so dag does not silently collapse to a 1-person team.
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
		// P-ORCH: per-subtask progress (frontend renders replace-style).
		impl.publishAllocatingProgress(ctx, taskPlan.SpiritSessionID, i+1, totalSubTasks, subTask, allocation)
	}

	// If no subtasks (simple/moderate), allocate the whole plan to a single agent
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

	// P-ORCH: allocation finished progress event.
	allocatedMeta := map[string]any{"total": len(allocations)}
	if len(allocations) > 0 && allocations[0].DepartmentID != "" {
		allocatedMeta["department_id"] = allocations[0].DepartmentID
	}
	impl.publishOrchestrationProgress(ctx, taskPlan.SpiritSessionID, "allocated", allocatedMeta)
	if sketch := collaborationSketch(taskPlan); sketch.SlotCount > 1 {
		impl.publishOrchestrationProgress(ctx, taskPlan.SpiritSessionID, "collaborating", sketch.meta())
	}

	// Build and persist AllocationPlan
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

	// P2-1 (session-eval-20260829-r2 R4-Q1): Spirit LLM 常把中文部门名
	// （"技术部"/"市场部"）当作 agent_keys 传入，原样透传会在下游
	// SpiritAssembly.resolveAgentKeyToIDMap 硬失败 → 零 team_runs 假启动。
	// 这里尽力把 display name / 部门名 / 公司名解析为真实 agent_key；
	// 无法解析的 key 保持原样（既有契约：routing never fails on missing row）。
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
		// P2-1 (R4-Q1): parallel 模式下 agent_keys 数量与子任务一一对应时，
		// 按键序逐个子任务指派（"技术侧(技术部)/内容侧(市场部)/运营侧(运营部)"
		// 语义），而非全部压给 keys[0]。dag 模式不受影响（keys[0] 领队 +
		// 其余成员组队）。数量不匹配时保持 keys[0] 全指派的既有契约。
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
	// Pass 1: department lead.
	for _, c := range caps {
		if c.DepartmentName == "" || !match(c.DepartmentName) {
			continue
		}
		if biz.IsDeptLeadAgent(biz.Agent{AgentKey: c.AgentKey, AgentVariant: c.AgentVariant}) {
			return c.AgentKey, true
		}
	}
	// Pass 2: company lead.
	for _, c := range caps {
		if c.CompanyName == "" || !match(c.CompanyName) {
			continue
		}
		if biz.IsCompanyLeadAgent(biz.Agent{AgentKey: c.AgentKey, AgentVariant: c.AgentVariant}) {
			return c.AgentKey, true
		}
	}
	// Pass 3: any assignable member of the department.
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

// layer2Shared 是一次 Allocate 调用内 Layer2 匹配的共享状态（P3b，2026-08-21）。
//
// Phase A 跨子任务并行（P-ORCH.5）后，Layer2 内部仍有两个串行热点：
//   - 每个进入 Layer2 的子任务把 N 个 Agent 文本随 taskText 整包重嵌
//     （4 子任务 = 4×(N+1) 条嵌入、4 次大 RPC；Agent 文本完全相同，纯重复）；
//   - 候选融合的 perfRepo.Get 对每个子任务的每个候选各点查一次
//     （N×M 次顺序 DB 往返）。
//
// layer2Shared 把二者降为 per-Allocate 一次性：Agent 向量 sync.Once 懒加载
// 共享（子任务只嵌入自己的 taskText 单条），perf 按 agentKey 去重缓存。
// 生命周期 per-Allocate（非 impl 字段），跨 Allocate 天然并发安全。
type layer2Shared struct {
	embedOnce sync.Once
	agentCaps []biz.AgentCapability // 与 agentVecs 对齐（已滤系统 Agent）
	agentVecs [][]float32
	embedErr  error

	perfMu    sync.Mutex
	perfCache map[string]*biz.AgentPerformance // key 存在 = 已查过（值可为 nil）
}

// errLayer2NoCandidates / errLayer2DimMismatch 是 loadAgentVectors 的哨兵错误，
// 调用方据此区分「无候选静默回退」与「真实失败告警回退」。
var (
	errLayer2NoCandidates = errors.New("layer2: no non-system agent candidates")
	errLayer2DimMismatch  = errors.New("layer2: embedding dimension mismatch")
)

func newLayer2Shared() *layer2Shared {
	return &layer2Shared{perfCache: make(map[string]*biz.AgentPerformance)}
}

// ensureLayer2Shared 容忍 nil（旧测试构造路径直接调 matchLayer2/matchSubTask
// 时不传共享态）——退化为一次性实例，行为与改造前一致。
func ensureLayer2Shared(s *layer2Shared) *layer2Shared {
	if s != nil {
		return s
	}
	return newLayer2Shared()
}

// loadAgentVectors 懒加载全部非系统 Agent 的能力文本嵌入：仅首次调用真实
// 发起一次批量 Embed RPC，后续调用与并发 goroutine 共享结果（含共享错误——
// 嵌入失败时所有子任务一致回退 TF-IDF，与改造前逐子任务失败的语义等价）。
func (s *layer2Shared) loadAgentVectors(ctx context.Context, embedder knowledge.Embedder, capabilities []biz.AgentCapability) ([]biz.AgentCapability, [][]float32, error) {
	s.embedOnce.Do(func() {
		var texts []string
		for _, cap := range capabilities {
			if !cap.IsHeuristicAssignable() {
				continue
			}
			s.agentCaps = append(s.agentCaps, cap)
			texts = append(texts, buildAgentCapabilityText(cap))
		}
		if len(s.agentCaps) == 0 {
			s.embedErr = errLayer2NoCandidates
			return
		}
		vecs, err := embedder.Embed(ctx, texts)
		if err != nil {
			s.embedErr = err
			return
		}
		if len(vecs) != len(texts) || len(vecs[0]) == 0 {
			s.embedErr = errLayer2DimMismatch
			return
		}
		s.agentVecs = vecs
	})
	return s.agentCaps, s.agentVecs, s.embedErr
}

// perfFor 返回 agentKey 的 "general" 履历（按 key 去重缓存：同一 Agent 一次
// Allocate 只点查一次；查询失败/无记录同样缓存 nil，不重复打 DB）。
func (s *layer2Shared) perfFor(ctx context.Context, repo biz.AgentPerformanceRepository, agentKey string) *biz.AgentPerformance {
	if repo == nil {
		return nil
	}
	s.perfMu.Lock()
	defer s.perfMu.Unlock()
	if perf, ok := s.perfCache[agentKey]; ok {
		return perf
	}
	perf, err := repo.Get(ctx, agentKey, "general")
	if err != nil {
		perf = nil
	}
	s.perfCache[agentKey] = perf
	return perf
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
	// P3b: 跨子任务共享 Layer2 状态——Agent 嵌入向量只算一次、perf 按 key 去重。
	// Preload vectors from the full assignable catalog so per-subtask org
	// prune cannot race on embedOnce with different candidate sets (M78).
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
			return nil // never surface matchSubTask errors; Phase B handles them
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
	// Determine assigned type based on estimated complexity
	assignedType := "agent"
	if subTask.EstimatedComplexity >= 0.5 {
		assignedType = "team"
	}

	// L0 / roster / L1（domain_path 非空时启用；为空直接落入旧管线，不变量 1）。
	// Roster 必须先于 L1：回填使命与任务零重叠仍会过旧阈值。
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
		// Knowledge-report specialties (研究/调研) are often missing from
		// it-ops rosters. Fall back to 办公/文档 rather than failing the
		// whole Team path with "no roster specialist".
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

	// L2 performance: domain 履历优先，回退 capability 履历。
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

	// Layer 1: Exact match — find agents whose Roles overlap with required_capabilities
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

	// Closed roster: a known specialty must bind a pre-built specialist.
	// Do not L2/L3-pick or low-score-assign a random catalog agent.
	if strings.TrimSpace(subTask.DomainPath) != "" {
		return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for subtask %s", subTask.ID)
	}

	// Empty domain + clear task words → roster miss, never L3 cold-start
	// (company-wide 海选). Vague / chit-chat without task signals may still
	// use Layer 2/3 on the unpruned catalog (NFR-78-06).
	taskBlob := strings.TrimSpace(subTask.Name + " " + subTask.Description)
	if biz.HasTaskActionSignal(taskBlob) {
		return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for subtask %s", subTask.ID)
	}

	// Layer 2: Semantic match — keyword-based similarity between task and agent capabilities
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

	// Layer 3: LLM cold start — use LLM to select from agent list
	llmMatch, err := impl.llmColdStart(ctx, subTask, capabilities, traceID)
	if err == nil && llmMatch != "" {
		// Find the matched agent's display name
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
			MatchScore:    bestScore, // carry forward the best exact score as fallback reference
			MatchLayer:    "llm_cold_start",
			MatchReason:   "LLM 冷启动匹配",
			FallbackKey:   bestMatch.AgentKey,
			FallbackScore: bestScore,
		}, nil
	}

	return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for subtask %s", subTask.ID)
}

// selectAdditionalMembers picks `count` additional agent keys from the
// capability pool, excluding the primary key and non-assignable roles.
// Preference: same department + complementary roles, then same department,
// then cross-department (returned separately for borrow marking).
func (impl *agentAllocatorImpl) selectAdditionalMembers(primaryKey string, capabilities []biz.AgentCapability, count int) (members []string, crossDept []string) {
	return pickComplementaryMembers(primaryKey, capabilities, count)
}

// dagAdditionalMemberCount is how many extra specialists a dag team should
// add besides the lead. "三人/三位专家" means one team of three, not three
// teams — ask for two complements. "三个团队" stays at one complement.
func dagAdditionalMemberCount(userMessage string) int {
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return 1
	}
	if strings.Contains(msg, "三个团队") || strings.Contains(msg, "三支团队") || strings.Contains(msg, "3个团队") {
		return 1
	}
	if strings.Contains(msg, "三人") || strings.Contains(msg, "三位") || strings.Contains(msg, "3人") {
		return 2
	}
	return 1
}

func pickComplementaryMembers(primaryKey string, capabilities []biz.AgentCapability, count int) (members []string, crossDept []string) {
	if count <= 0 || len(capabilities) == 0 {
		return nil, nil
	}
	var primary *biz.AgentCapability
	for i := range capabilities {
		if capabilities[i].AgentKey == primaryKey {
			primary = &capabilities[i]
			break
		}
	}
	primaryDept := ""
	var primaryRoles []string
	if primary != nil {
		primaryDept = primary.DepartmentID
		primaryRoles = primary.Roles
	}

	used := map[string]struct{}{primaryKey: {}}
	add := func(cap biz.AgentCapability) bool {
		if cap.AgentKey == "" {
			return false
		}
		if _, ok := used[cap.AgentKey]; ok {
			return false
		}
		if !cap.IsHeuristicAssignable() {
			return false
		}
		used[cap.AgentKey] = struct{}{}
		members = append(members, cap.AgentKey)
		if primaryDept != "" && cap.DepartmentID != "" && cap.DepartmentID != primaryDept {
			crossDept = append(crossDept, cap.AgentKey)
		}
		return len(members) >= count
	}

	passes := []func(biz.AgentCapability) bool{
		func(cap biz.AgentCapability) bool {
			return primaryDept != "" && cap.DepartmentID == primaryDept && hasComplementaryRole(primaryRoles, cap.Roles)
		},
		func(cap biz.AgentCapability) bool {
			return primaryDept != "" && cap.DepartmentID == primaryDept
		},
		func(cap biz.AgentCapability) bool {
			return hasComplementaryRole(primaryRoles, cap.Roles)
		},
		func(biz.AgentCapability) bool { return true },
	}
	for _, pred := range passes {
		for _, cap := range capabilities {
			if !pred(cap) {
				continue
			}
			if add(cap) {
				return members, crossDept
			}
		}
	}
	return members, crossDept
}

func hasComplementaryRole(lead, cand []string) bool {
	if len(lead) == 0 || len(cand) == 0 {
		return true
	}
	leadSet := make(map[string]struct{}, len(lead))
	for _, r := range lead {
		if t := strings.TrimSpace(r); t != "" {
			leadSet[t] = struct{}{}
		}
	}
	if len(leadSet) == 0 {
		return true
	}
	for _, r := range cand {
		if _, ok := leadSet[strings.TrimSpace(r)]; !ok {
			return true
		}
	}
	return false
}

func classifyCrossDeptMembers(primaryKey string, memberKeys []string, capabilities []biz.AgentCapability) (homeDept string, crossDept []string) {
	byKey := make(map[string]biz.AgentCapability, len(capabilities))
	for _, cap := range capabilities {
		byKey[cap.AgentKey] = cap
	}
	if p, ok := byKey[primaryKey]; ok {
		homeDept = p.DepartmentID
	}
	if homeDept == "" {
		return "", nil
	}
	for _, k := range memberKeys {
		cap, ok := byKey[k]
		if !ok || cap.DepartmentID == "" || cap.DepartmentID == homeDept {
			continue
		}
		crossDept = append(crossDept, k)
	}
	return homeDept, crossDept
}

// exactMatch performs Layer 1 matching: overlap between required_capabilities and agent Roles.
func (impl *agentAllocatorImpl) exactMatch(requiredCapabilities []string, capabilities []biz.AgentCapability) (biz.AgentCapability, float64, string) {
	if len(requiredCapabilities) == 0 || len(capabilities) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	type scored struct {
		cap   biz.AgentCapability
		score float64
	}

	var candidates []scored
	for _, cap := range capabilities {
		if !cap.IsHeuristicAssignable() {
			continue
		}
		overlapRatio := computeOverlapRatio(requiredCapabilities, cap.Roles)
		if overlapRatio == 0 {
			continue
		}
		// Score = overlap_ratio * 0.7 + historical_success_rate * 0.3
		// For now, historical_success_rate defaults to 0.5 (no history yet)
		historicalRate := 0.5
		score := overlapRatio*0.7 + float64(historicalRate)*0.3
		candidates = append(candidates, scored{cap: cap, score: score})
	}

	if len(candidates) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	// Pick the best
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	reason := fmt.Sprintf("角色重叠率 %.2f, 综合得分 %.2f", computeOverlapRatio(requiredCapabilities, best.cap.Roles), best.score)
	return best.cap, best.score, reason
}

// matchLayer2 performs Layer 2 matching: semantic similarity between task description and agent capabilities.
//
// When an embedder is configured, it uses in-memory embedding cosine similarity (the embedder is
// typically backed by OpenAI/Gemini/Ollama text-embedding APIs) for true semantic matching. When
// the embedder is nil or fails, it gracefully falls back to the existing TF-IDF keyword-based
// matching — no hard error is surfaced to the caller.
//
// Note: pgvector SQL similarity (`<=>` operator on the `vector_embeddings` table) is used by the
// memory domain (internal/data/vector/pgvector_fact.go) for memory retrieval, not by the allocator.
// The allocator computes cosine in Go memory to avoid a Postgres round-trip on the hot allocation path.
func (impl *agentAllocatorImpl) matchLayer2(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string, sharedOpt ...*layer2Shared) (biz.AgentCapability, float64, string) {
	if len(capabilities) == 0 {
		return biz.AgentCapability{}, 0, ""
	}
	var shared *layer2Shared
	if len(sharedOpt) > 0 {
		shared = ensureLayer2Shared(sharedOpt[0])
	} else {
		shared = newLayer2Shared()
	}

	// Try embedding-based matching first; fall back to TF-IDF on failure or nil embedder.
	if impl.embedder != nil {
		if cap, score, reason, ok := impl.matchLayer2Embedding(ctx, subTask, capabilities, traceID, shared); ok {
			return cap, score, reason
		}
	}

	return impl.matchLayer2TFIDF(ctx, subTask, capabilities, traceID, shared)
}

// matchLayer2Embedding uses embedding cosine similarity for semantic matching.
// Returns ok=false when the embedder fails or produces unusable vectors; the caller falls back to TF-IDF.
//
// P3b: Agent 侧向量经 shared.loadAgentVectors per-Allocate 一次性批量嵌入共享，
// 每个子任务只嵌入自己的 taskText 单条；候选融合的 perf 履历经 shared.perfFor
// 按 agentKey 去重点查，不再 N×M 次顺序打 DB。
func (impl *agentAllocatorImpl) matchLayer2Embedding(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string, shared *layer2Shared) (biz.AgentCapability, float64, string, bool) {
	taskText := subTask.Name + " " + subTask.Description
	for _, cap := range subTask.RequiredCapabilities {
		taskText += " " + cap
	}

	agentCaps, agentVecs, err := shared.loadAgentVectors(ctx, impl.embedder, capabilities)
	if err != nil {
		if !errors.Is(err, errLayer2NoCandidates) {
			impl.lg.Warn("Layer 2 embedding 失败，降级为 TF-IDF",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
		}
		return biz.AgentCapability{}, 0, "", false
	}

	// 仅嵌入当前子任务的 taskText（Agent 向量已共享）。
	vectors, err := impl.embedder.Embed(ctx, []string{taskText})
	if err != nil {
		impl.lg.Warn("Layer 2 taskText embedding 失败，降级为 TF-IDF",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return biz.AgentCapability{}, 0, "", false
	}
	if len(vectors) != 1 || len(vectors[0]) == 0 {
		impl.lg.Warn("Layer 2 embedding 维度不匹配，降级为 TF-IDF",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Int("expected", 1),
			loggateway.Int("got", len(vectors)),
		)
		return biz.AgentCapability{}, 0, "", false
	}

	taskVec := vectors[0]

	allow := make(map[string]struct{}, len(capabilities))
	for _, cap := range capabilities {
		allow[cap.AgentKey] = struct{}{}
	}

	type scored struct {
		cap   biz.AgentCapability
		score float64
	}

	var candidates []scored
	for i, cap := range agentCaps {
		if !cap.IsHeuristicAssignable() {
			continue
		}
		if _, ok := allow[cap.AgentKey]; !ok {
			continue
		}
		sim := cosineSimilarity32(taskVec, agentVecs[i])
		if sim <= 0 {
			continue
		}
		// Blend with historical success rate (same weighting as TF-IDF path).
		score := sim
		if perf := shared.perfFor(ctx, impl.perfRepo, cap.AgentKey); perf != nil {
			score = score*0.6 + perf.SuccessRate*0.4
		}
		candidates = append(candidates, scored{cap: cap, score: score})
	}

	if len(candidates) == 0 {
		return biz.AgentCapability{}, 0, "", false
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	reason := fmt.Sprintf("向量语义匹配 (cosine: %.2f)", best.score)
	return best.cap, best.score, reason, true
}

// matchLayer2TFIDF is the TF-IDF keyword-based fallback for Layer 2 matching.
// P3b: perf 履历经 shared.perfFor 按 agentKey 去重点查。
func (impl *agentAllocatorImpl) matchLayer2TFIDF(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string, shared *layer2Shared) (biz.AgentCapability, float64, string) {
	taskText := subTask.Name + " " + subTask.Description
	for _, cap := range subTask.RequiredCapabilities {
		taskText += " " + cap
	}

	type scored struct {
		cap   biz.AgentCapability
		score float64
	}

	var candidates []scored
	for _, cap := range capabilities {
		if !cap.IsHeuristicAssignable() {
			continue
		}
		score := computeSemanticScore(taskText, cap)
		// Combine with historical success rate if available
		if perf := shared.perfFor(ctx, impl.perfRepo, cap.AgentKey); perf != nil {
			score = score*0.6 + perf.SuccessRate*0.4
		}
		if score > 0 {
			candidates = append(candidates, scored{cap: cap, score: score})
		}
	}

	if len(candidates) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	reason := fmt.Sprintf("TF-IDF 语义匹配 (score: %.2f)", best.score)
	return best.cap, best.score, reason
}

// matchLayer2ForPlan performs Layer 2 matching for a whole plan (no subtasks).
func (impl *agentAllocatorImpl) matchLayer2ForPlan(ctx context.Context, taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability, traceID string) (biz.AgentCapability, float64, string) {
	if len(capabilities) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	type scored struct {
		cap   biz.AgentCapability
		score float64
	}

	var candidates []scored
	for _, cap := range capabilities {
		if !cap.IsHeuristicAssignable() {
			continue
		}
		score := computeSemanticScore(taskPlan.UserMessage, cap)
		if impl.perfRepo != nil {
			perf, err := impl.perfRepo.Get(ctx, cap.AgentKey, "general")
			if err == nil && perf != nil {
				score = score*0.6 + perf.SuccessRate*0.4
			}
		}
		if score > 0 {
			candidates = append(candidates, scored{cap: cap, score: score})
		}
	}

	if len(candidates) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	reason := fmt.Sprintf("语义匹配 (score: %.2f)", best.score)
	return best.cap, best.score, reason
}

// computeSemanticScore computes a TF-IDF-like keyword overlap score between
// a task description and an agent's capability profile.
//
// This function powers the TF-IDF fallback path in matchLayer2TFIDF. The
// primary Layer 2 path now uses embedding cosine similarity via matchLayer2Embedding;
// this keyword scorer remains as the graceful-degradation fallback when the
// embedder is unavailable or fails.
func computeSemanticScore(taskDesc string, cap biz.AgentCapability) float64 {
	return semanticScoreText(taskDesc, buildAgentCapabilityText(cap))
}

// semanticScoreText computes a TF-IDF-like keyword overlap score between a
// task description and an arbitrary agent text corpus.
func semanticScoreText(taskDesc, agentText string) float64 {
	taskTokens := tokenizeForSemantic(taskDesc)
	agentTokens := tokenizeForSemantic(agentText)

	if len(taskTokens) == 0 || len(agentTokens) == 0 {
		return 0
	}

	// Compute TF for task tokens
	taskTF := make(map[string]float64, len(taskTokens))
	for _, t := range taskTokens {
		taskTF[t]++
	}
	for k := range taskTF {
		taskTF[k] /= float64(len(taskTokens))
	}

	// Compute IDF-like weighting: how many agent tokens match each task token
	agentSet := make(map[string]bool, len(agentTokens))
	for _, t := range agentTokens {
		agentSet[t] = true
	}

	var score float64
	for token, tf := range taskTF {
		if agentSet[token] {
			// Simple IDF approximation: matched tokens get higher weight
			score += tf * 2.0
		}
	}

	// Normalize to [0, 1]
	score = score / (1.0 + score)

	// Apply sigmoid-like scaling for better separation
	score = 1.0 / (1.0 + math.Exp(-6*(score-0.5)))

	return score
}

// tokenizeForSemantic splits text into lowercase tokens for semantic comparison.
func tokenizeForSemantic(text string) []string {
	text = strings.ToLower(text)
	// Replace common separators with spaces
	for _, sep := range []string{"-", "_", "/", ".", ",", "，", "、", "：", "："} {
		text = strings.ReplaceAll(text, sep, " ")
	}
	fields := strings.Fields(text)
	// Filter out very short tokens
	var result []string
	for _, f := range fields {
		if len(f) >= 2 {
			result = append(result, f)
		}
	}
	return result
}

// buildAgentCapabilityText concatenates an agent's capability profile into a
// single text blob suitable for embedding. The field order matches the
// TF-IDF corpus built by computeSemanticScore so both paths compare the same text.
func buildAgentCapabilityText(cap biz.AgentCapability) string {
	parts := []string{cap.DisplayName, cap.Description}
	parts = append(parts, cap.Roles...)
	parts = append(parts, cap.Domains...)
	parts = append(parts, cap.Tools...)
	parts = append(parts, cap.Skills...)
	return strings.Join(parts, " ")
}

// cosineSimilarity32 computes the cosine similarity between two float32 vectors.
// Returns 0 for empty or mismatched-length vectors, or when either vector has zero magnitude.
func cosineSimilarity32(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// llmColdStart performs Layer 3 matching: use LLM to select the best agent.
func (impl *agentAllocatorImpl) llmColdStart(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string) (string, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return "", apierror.Internal(apierror.DomainSpirit, "LLM catalog or HTTP client not configured")
	}

	capabilities = capLLMColdStart(filterHeuristicAssignable(capabilities))
	if len(capabilities) == 0 {
		return "", nil
	}

	prompt := buildAllocatorColdStartPrompt(subTask, capabilities)

	// Resolve planner model via system setting (specify/inherit) with session
	// model fallback. Replaces legacy env-var + catalog-first approach.
	setting := biz.PlannerModelSetting{Mode: biz.PlannerModelModeInherit}
	if impl.plannerSetting != nil {
		if s, err := impl.plannerSetting.GetPlannerModel(ctx); err == nil {
			setting = s
		}
	}
	sessionProvider, sessionModel := biz.PlannerSessionModelFromCtx(ctx)
	provider, model := ResolvePlannerModel(ctx, setting, sessionProvider, sessionModel, impl.catalog, impl.lg, biz.SpiritStepAllocatorMatch, "AgentAllocator")
	if provider == "" || model == "" {
		return "", apierror.Internal(apierror.DomainSpirit, "no provider/model configured for agent allocation (set planner_model_mode in system settings or add enabled models in catalog)")
	}

	row, err := impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return "", apierror.Internal(apierror.DomainSpirit, "get provider config").WithCause(err)
	}

	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)
	ApplyThinkingCapability(&cfg, row.CapabilitiesExplicit, row.Capabilities.Thinking)

	// P2-5：按子任务预估复杂度路由 thinking effort。
	// EstimatedComplexity >= 0.6 → complex, >= 0.3 → moderate, else simple
	// （与 Plan() 六维评估的 ComplexityLevel 映射同区间）。
	level := biz.ComplexitySimple
	if subTask.EstimatedComplexity >= 0.6 {
		level = biz.ComplexityComplex
	} else if subTask.EstimatedComplexity >= 0.3 {
		level = biz.ComplexityModerate
	}
	if eff := biz.ResolveThinkingEffort(cfg.ThinkingEffort, level); eff != "" {
		cfg.ThinkingEffort = eff
	}

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: fmt.Sprintf("Select the best agent for this subtask:\n\nName: %s\nDescription: %s\nRequired Capabilities: %v", subTask.Name, subTask.Description, subTask.RequiredCapabilities)},
	}

	callCtx, cancel := context.WithTimeout(ctx, tools.AllocateLLMTimeout)
	defer cancel()

	callStart := time.Now()
	text, _, promptTok, completionTok, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)
	// M83 LBG-6：冷启动匹配调用落账（此前 token 被丢弃）。RunID 用子任务
	// ID 做关联。
	recordSpiritAuxUsage(ctx, impl.auxUsage, impl.lg, biz.SpiritStepAllocatorMatch, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxAllocatorMatch,
		RunID:         subTask.ID,
		Provider:      provider,
		Model:         model,
		Status:        spiritAuxCallStatus(err),
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		UsageSource:   biz.UsageSourceResponse,
		Effort:        cfg.ThinkingEffort,
		Latency:       time.Since(callStart),
		ErrMsg:        spiritAuxErrMsg(err),
		MetadataJSON:  spiritAuxMeta(ctx, "spirit_allocator_match"),
	})
	if err != nil {
		return "", apierror.Internal(apierror.DomainSpirit, "LLM call failed").WithCause(err)
	}

	// Parse the agent_key from the response
	agentKey := parseAllocatorColdStartResponse(text)
	// TEMP-DEBUG: validate LLM-returned agent_key exists in capabilities to prevent hallucination
	if agentKey != "" {
		found := false
		for _, cap := range capabilities {
			if cap.AgentKey == agentKey {
				found = true
				break
			}
		}
		if !found {
			impl.lg.Warn("LLM cold-start returned unknown agent_key, falling back",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("returned_key", agentKey),
			)
			return "", nil
		}
	}
	return agentKey, nil
}

// matchWholePlan handles allocation for plans without subtasks (simple/moderate).
func (impl *agentAllocatorImpl) matchWholePlan(ctx context.Context, taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability, traceID string) (biz.TaskAllocation, error) {
	// For simple/moderate plans, use the strategy to determine assignment
	assignedType := "agent"
	if taskPlan.Strategy == biz.StrategyCoordinator || taskPlan.Strategy == biz.StrategyParallel || taskPlan.Strategy == biz.StrategyDAG {
		assignedType = "team"
	}

	// L0/L1: 使命驱动匹配（plan 级主导域非空时启用）。
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

	// L2 performance: domain 履历优先，回退 capability hint 履历。
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

	// Try exact match using user message keywords as capability hints
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

	// Layer 2: Semantic match for whole plan
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

	// LLM cold start for whole plan
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

// llmColdStartForPlan uses LLM to select an agent for a whole plan (no subtasks).
func (impl *agentAllocatorImpl) llmColdStartForPlan(ctx context.Context, taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability, traceID string) (string, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return "", apierror.Internal(apierror.DomainSpirit, "LLM catalog or HTTP client not configured")
	}

	capabilities = capLLMColdStart(filterHeuristicAssignable(capabilities))
	if len(capabilities) == 0 {
		return "", nil
	}

	prompt := buildAllocatorColdStartPromptForPlan(taskPlan, capabilities)

	// Resolve planner model via system setting (specify/inherit) with session
	// model fallback. Replaces legacy env-var + catalog-first approach.
	setting := biz.PlannerModelSetting{Mode: biz.PlannerModelModeInherit}
	if impl.plannerSetting != nil {
		if s, err := impl.plannerSetting.GetPlannerModel(ctx); err == nil {
			setting = s
		}
	}
	sessionProvider, sessionModel := biz.PlannerSessionModelFromCtx(ctx)
	provider, model := ResolvePlannerModel(ctx, setting, sessionProvider, sessionModel, impl.catalog, impl.lg, biz.SpiritStepAllocatorMatch, "AgentAllocator")
	if provider == "" || model == "" {
		return "", apierror.Internal(apierror.DomainSpirit, "no provider/model configured for agent allocation (set planner_model_mode in system settings or add enabled models in catalog)")
	}

	row, err := impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return "", apierror.Internal(apierror.DomainSpirit, "get provider config").WithCause(err)
	}

	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)
	ApplyThinkingCapability(&cfg, row.CapabilitiesExplicit, row.Capabilities.Thinking)

	// P2-5：按计划复杂度路由 thinking effort（来自外层 Plan() 的六维评估）。
	if eff := biz.ResolveThinkingEffort(cfg.ThinkingEffort, taskPlan.ComplexityLevel); eff != "" {
		cfg.ThinkingEffort = eff
	}

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: fmt.Sprintf("Select the best agent for this task:\n\n%s", taskPlan.UserMessage)},
	}

	callCtx, cancel := context.WithTimeout(ctx, tools.AllocateLLMTimeout)
	defer cancel()

	callStart := time.Now()
	text, _, promptTok, completionTok, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)
	// M83 LBG-6：整计划冷启动匹配调用落账（此前 token 被丢弃）。RunID 用
	// spirit trace 做关联（整计划无子任务 ID）。
	recordSpiritAuxUsage(ctx, impl.auxUsage, impl.lg, biz.SpiritStepAllocatorMatch, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxAllocatorMatch,
		RunID:         traceID,
		Provider:      provider,
		Model:         model,
		Status:        spiritAuxCallStatus(err),
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		UsageSource:   biz.UsageSourceResponse,
		Effort:        cfg.ThinkingEffort,
		Latency:       time.Since(callStart),
		ErrMsg:        spiritAuxErrMsg(err),
		MetadataJSON:  spiritAuxMeta(ctx, "spirit_allocator_match"),
	})
	if err != nil {
		return "", apierror.Internal(apierror.DomainSpirit, "LLM call failed").WithCause(err)
	}

	agentKey := parseAllocatorColdStartResponse(text)
	// TEMP-DEBUG: validate LLM-returned agent_key exists in capabilities to prevent hallucination
	if agentKey != "" {
		found := false
		for _, cap := range capabilities {
			if cap.AgentKey == agentKey {
				found = true
				break
			}
		}
		if !found {
			impl.lg.Warn("LLM cold-start (plan) returned unknown agent_key, falling back",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("returned_key", agentKey),
			)
			return "", nil
		}
	}
	return agentKey, nil
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

// computeOverlapRatio computes the ratio of overlap between required and available.
func computeOverlapRatio(required []string, available []string) float64 {
	if len(required) == 0 {
		return 0
	}
	availSet := make(map[string]bool, len(available))
	for _, a := range available {
		availSet[strings.ToLower(strings.TrimSpace(a))] = true
	}
	overlap := 0
	for _, r := range required {
		if availSet[strings.ToLower(strings.TrimSpace(r))] {
			overlap++
		}
	}
	return float64(overlap) / float64(len(required))
}

// extractCapabilityHints extracts capability hints from a user message.
func extractCapabilityHints(userMessage string) []string {
	// Simple keyword-based extraction
	keywords := []string{
		"go-backend", "go-kratos", "vue3-frontend", "quasar-ui",
		"devops", "database", "architecture", "testing",
		"security", "research", "documentation", "api-design",
	}
	var hints []string
	msgLower := strings.ToLower(userMessage)
	for _, kw := range keywords {
		if strings.Contains(msgLower, strings.ReplaceAll(kw, "-", " ")) ||
			strings.Contains(msgLower, kw) {
			hints = append(hints, kw)
		}
	}
	if len(hints) == 0 {
		hints = append(hints, "general")
	}
	return hints
}

// buildAllocatorColdStartPrompt creates the system prompt for LLM-based agent selection.
func buildAllocatorColdStartPrompt(subTask biz.SubTask, capabilities []biz.AgentCapability) string {
	agentList := formatCapabilitiesForPrompt(capabilities)

	return `You are an agent allocation specialist. Select the best agent for a given subtask.

Rules:
- Output ONLY the agent_key of the best matching agent, no markdown, no explanation
- Consider the subtask's required capabilities and description
- If no agent is a good fit, output "none"

Available agents:
` + agentList
}

// buildAllocatorColdStartPromptForPlan creates the system prompt for LLM-based agent selection for a whole plan.
func buildAllocatorColdStartPromptForPlan(taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability) string {
	agentList := formatCapabilitiesForPrompt(capabilities)

	return `You are an agent allocation specialist. Select the best agent for a given task.

Rules:
- Output ONLY the agent_key of the best matching agent, no markdown, no explanation
- Consider the task description and context
- If no agent is a good fit, output "none"

Available agents:
` + agentList
}

// formatCapabilitiesForPrompt formats agent capabilities for LLM prompt.
func formatCapabilitiesForPrompt(capabilities []biz.AgentCapability) string {
	var lines []string
	for _, cap := range capabilities {
		line := fmt.Sprintf("- agent_key: %s, display_name: %s, roles: %v, domain_path: %s, mission: %s, description: %s",
			cap.AgentKey, cap.DisplayName, cap.Roles, cap.DomainPath, cap.Mission, cap.Description)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// parseAllocatorColdStartResponse extracts the agent_key from LLM response.
func parseAllocatorColdStartResponse(text string) string {
	text = strings.TrimSpace(text)
	// Strip markdown fences if present
	if m := fenceRE.FindStringSubmatch(text); len(m) > 1 {
		text = strings.TrimSpace(m[1])
	}
	// The response should be just the agent_key
	// Try to parse as JSON first
	var parsed struct {
		AgentKey string `json:"agent_key"`
	}
	if json.Unmarshal([]byte(text), &parsed) == nil && parsed.AgentKey != "" {
		return parsed.AgentKey
	}
	// Otherwise treat the whole text as the agent_key
	text = strings.TrimPrefix(text, "- agent_key:")
	text = strings.TrimSpace(text)
	if text == "none" || text == "" {
		return ""
	}
	return text
}

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
