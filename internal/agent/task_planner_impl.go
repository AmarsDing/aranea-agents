package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	v2 "aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// taskPlannerImpl implements biz.TaskPlannerPort.
type taskPlannerImpl struct {
	repo           biz.TaskPlanRepository
	catalog        *biz.LlmProviderModelUsecase
	httpClient     *http.Client
	eventBus       biz.EventBus // P-ORCH: v2 bus for orchestration_progress notices
	orchCache      *biz.OrchestrationCache
	lg             loggateway.Logger
	plannerSetting PlannerModelLookup
	// seq is the v2 Sequencer (nil-safe) used to publish PlanBoard/PlanStep/
	// GraphStage/GraphNode events. Nil = v2 publish skipped (backwards compat).
	seq v2.SequencerPublisher

	// P3 测试钩子——生产为 nil，注入默认实现；测试可替换。
	retryBackoffFn func(attempt int) time.Duration
	llmAttemptFn   decomposeAttemptFn
	// maxDecomposeAttempts 限制瞬时故障重试上限（F8/Y3）：<=0 时用默认值 5。
	// 无限重试会让 turn 永远卡在「规划中」并持续烧 LLM 调用。
	maxDecomposeAttempts int

	// P4-G1 计划校验门：能力清单构建器。nil（旧测试构造路径）时校验门
	// 整体跳过，行为与旧版一致（fail-open）。
	capBuilder *AgentCapabilityBuilder
	// P4-G1 测试钩子——校验门违例时的有界重分解函数；生产为 nil 时回退
	// decomposeTask（同步分解，非流式——修复路径不需要流式中间态）。
	repairDecomposeFn func(ctx context.Context, userMessage string, artifact *biz.IntentArtifact, teamCount int, level biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error)

	// org is optional; when set, authorized playbooks can expand without LLM.
	org biz.OrganizationReader

	// decisions is the M80 decision-record intake (nil = capture skipped,
	// backwards compatible with old construction paths). Attached at wire
	// time via AttachPlannerDecisionCollector.
	decisions decision.Collector

	// auxUsage records planner decompose LLM usage (M83 LBG-6). nil = 落账
	// 跳过（旧测试构造路径），行为与旧版一致。Attached at wire time via
	// AttachPlannerAuxUsageRecorder.
	auxUsage SpiritAuxUsageRecorder
}

// decomposeAttemptFn 是单次 LLM 分解尝试的签名——供 P3 重试循环调用。
// planID 用于生成跨尝试确定性的 subtask 全局 ID（见 planStepID）。
// level 是 P2-5 思考强度路由的任务复杂度（空 = 不覆盖静态配置）。
type decomposeAttemptFn func(ctx context.Context, userMessage string, artifact *biz.IntentArtifact, teamCount int, spiritSessionID, planID string, level biz.ComplexityLevel, onSubTask func(biz.SubTask, int)) ([]biz.SubTask, *biz.PlanTaskDAG, error)

var _ biz.TaskPlannerPort = (*taskPlannerImpl)(nil)

// NewTaskPlanner creates a new TaskPlanner implementation.
// agentReader 为 P4-G1 计划校验门提供能力清单来源；传 nil 时校验门跳过。
func NewTaskPlanner(repo biz.TaskPlanRepository, catalog *biz.LlmProviderModelUsecase, httpClient *http.Client, eventBus biz.EventBus, orchCache *biz.OrchestrationCache, lg loggateway.Logger, plannerSetting PlannerModelLookup, seq v2.SequencerPublisher, agentReader biz.AgentReader) biz.TaskPlannerPort {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	impl := &taskPlannerImpl{
		repo:           repo,
		catalog:        catalog,
		httpClient:     httpClient,
		eventBus:       eventBus,
		orchCache:      orchCache,
		lg:             lg,
		plannerSetting: plannerSetting,
		seq:            seq,
	}
	if agentReader != nil {
		impl.capBuilder = NewAgentCapabilityBuilder(agentReader, lg)
	}
	return impl
}

// AttachPlannerOrganizationReader wires org lookup so Plan can expand an
// authorized playbook without changing the TaskPlannerPort constructor.
func AttachPlannerOrganizationReader(p biz.TaskPlannerPort, org biz.OrganizationReader) {
	if impl, ok := p.(*taskPlannerImpl); ok {
		impl.org = org
	}
}

// AttachPlannerDecisionCollector wires the M80 decision collector so
// emitPlannerDecision double-writes planner_orchestration records (design
// §3.2 row 1) without changing the TaskPlannerPort constructor.
func AttachPlannerDecisionCollector(p biz.TaskPlannerPort, c decision.Collector) {
	if impl, ok := p.(*taskPlannerImpl); ok {
		impl.decisions = c
	}
}

// AttachPlannerAuxUsageRecorder wires the spirit aux usage recorder so
// planner decompose LLM calls are billed (M83 LBG-6) without changing the
// TaskPlannerPort constructor.
func AttachPlannerAuxUsageRecorder(p biz.TaskPlannerPort, rec SpiritAuxUsageRecorder) {
	if impl, ok := p.(*taskPlannerImpl); ok {
		impl.auxUsage = rec
	}
}

// Plan assesses task complexity, optionally decomposes, and outputs a strategy.
func (impl *taskPlannerImpl) Plan(ctx context.Context, input biz.PlanInput) (*biz.TaskPlan, error) {
	traceID := input.TraceID
	if traceID == "" {
		traceID, _ = biz.SpiritTraceIDFromContext(ctx)
	}

	impl.lg.Info("TaskPlanner.Plan 开始",
		loggateway.StepID(biz.SpiritStepPlannerAssess),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("spirit_session_id", input.SpiritSessionID),
	)

	if id := strings.TrimSpace(input.ResumePlanID); id != "" {
		return impl.finishDeferredComplexDecompose(ctx, input, id)
	}

	if plan, ok := impl.planFromNamedPlaybook(ctx, input, traceID); ok {
		return plan, nil
	}
	if HasOrgChainIntent(input.UserMessage) {
		if pb, steps, ok := impl.lookupSolePlaybookIfHeavy(ctx, GearHeavy); ok && len(steps) > 0 {
			if plan, ok := impl.persistPlaybookPlan(ctx, input, traceID, pb, steps); ok {
				return plan, nil
			}
		}
	}

	// Step 0: Query OrchestrationCache for memory-driven routing
	// N2: SkipMemory=true 时跳过缓存查询（roster_miss 重规划路径）。
	var memoryHit *biz.MemoryHit
	if !input.SkipMemory {
		memoryHit = impl.queryMemory(ctx, input, traceID)
	}
	if memoryHit != nil {
		impl.lg.Info("编排缓存命中，跳过完整复杂度评估",
			loggateway.StepID(biz.SpiritStepPlannerMemory),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("cache_id", memoryHit.CacheID),
			loggateway.Float64("dq_score", memoryHit.DQScore),
			loggateway.Str("topology", memoryHit.TopologyUsed),
		)

		// Reuse historical topology as strategy hint. Legacy topologies
		// (coordinator/sequential) are mapped to the closest new-mode strategy:
		// coordinator → dag (multi-member team), sequential → dag (ordered
		// execution). This preserves backward compatibility with cache entries
		// written before the three-mode refactor (direct/parallel/dag).
		topologyHint := biz.TopologyType(memoryHit.TopologyUsed)
		strategy := biz.StrategyDirect
		switch topologyHint {
		case biz.TopologyDirect:
			strategy = biz.StrategyDirect
		case biz.TopologyParallel:
			strategy = biz.StrategyParallel
		case biz.TopologyCoordinator, biz.TopologySequential, biz.TopologyHybrid:
			strategy = biz.StrategyDAG
		}

		// N1 结构校验：记忆复用的子任务必须能通过花名册映射预检，
		// 否则作废记忆命中并落入 LLM 分解（S07 根修：陈旧计划子任务
		// 角色与当前花名册不匹配导致 roster_miss）。
		var recipeSubTasks []biz.SubTask
		if strategy != biz.StrategyDirect && (len(memoryHit.Specialties) > 0 || len(memoryHit.AgentKeysUsed) > 0) {
			recipeSubTasks = recipeReplaySubTasks(memoryHit, input.UserMessage)
			if !impl.validateRecipeSubTasks(ctx, recipeSubTasks) {
				impl.lg.Info("记忆命中子任务结构校验失败，作废记忆命中",
					loggateway.StepID(biz.SpiritStepPlannerMemory),
					loggateway.Str("trace_id", traceID),
					loggateway.Int("subtask_count", len(recipeSubTasks)),
				)
				memoryHit = nil
			}
		}
	}

	if memoryHit != nil {
		// Re-derive strategy from validated memory hit.
		topologyHint := biz.TopologyType(memoryHit.TopologyUsed)
		strategy := biz.StrategyDirect
		switch topologyHint {
		case biz.TopologyDirect:
			strategy = biz.StrategyDirect
		case biz.TopologyParallel:
			strategy = biz.StrategyParallel
		case biz.TopologyCoordinator, biz.TopologySequential, biz.TopologyHybrid:
			strategy = biz.StrategyDAG
		}

		// Build a lightweight plan from memory hit
		plan := &biz.TaskPlan{
			ID:              "tp_" + uuid.NewString(),
			SpiritSessionID: input.SpiritSessionID,
			TraceID:         traceID,
			UserMessage:     input.UserMessage,
			IntentArtifactJSON: func() string {
				if input.IntentArtifact == nil {
					return "{}"
				}
				b, _ := json.Marshal(input.IntentArtifact)
				return string(b)
			}(),
			ComplexityLevel: biz.ComplexityComplex,
			ComplexityScore: memoryHit.DQScore,
			Strategy:        strategy,
			StrategyReason:  "基于历史编排缓存推荐策略",
			TopologyHint:    topologyHint,
			MemoryHit:       memoryHit,
			DomainPath:      memoryHit.DomainPath,
			Status:          biz.TaskPlanStatusDraft,
		}
		if !biz.RecipeKeysReusable(memoryHit.ConstraintFingerprint, impl.currentConstraintFingerprint(ctx, input.UserMessage)) {
			memoryHit.AgentKeysUsed = nil
		}
		if strategy != biz.StrategyDirect && (len(memoryHit.Specialties) > 0 || len(memoryHit.AgentKeysUsed) > 0) {
			plan.SubTasks = recipeReplaySubTasks(memoryHit, input.UserMessage)
			plan.StrategyReason = "配方回放：按专题槽位复用历史专项，跳过分解 LLM"
		}

		saved, err := impl.repo.Create(ctx, plan)
		if err != nil {
			impl.lg.Warn("TaskPlan 持久化失败",
				loggateway.StepID(biz.SpiritStepPlannerPersist),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
			return nil, apierror.Internal(apierror.DomainSpirit, "persist plan").WithCause(err)
		}
		// P4-G5：缓存命中也是策略决策——同样落证据链。
		impl.emitPlannerDecision(ctx, plannerDecision{
			TraceID:         traceID,
			DecisionSource:  "memory_cache",
			Mode:            strings.ToLower(strings.TrimSpace(input.Mode)),
			Strategy:        strategy,
			ComplexityLevel: biz.ComplexityComplex,
			ComplexityScore: memoryHit.DQScore,
			StrategyReason:  plan.StrategyReason,
			SpiritSessionID: input.SpiritSessionID,
		})
		impl.publishPlanCreated(ctx, saved, input.ChatSessionID)
		return saved, nil
	}

	if HasOrgChainIntent(input.UserMessage) {
		return impl.planPlaybookFillRequired(ctx, input, traceID)
	}

	// Fallback: detect team-formation intent from the user message when mode is
	// empty/auto. The DECISION.md prompt instructs the Spirit LLM to pass an
	// explicit mode (direct/parallel/dag), but LLMs don't always comply. This
	// ensures teams are created when the user clearly requests them, regardless
	// of LLM cooperation.
	//
	// 2026-07-04 问题 2 修复：同时提取用户请求的 team 数量约束，传给
	// decomposeTask 让 LLM 生成恰好 N 个 subtask（避免多出 team）。
	teamCount := detectTeamCount(input.UserMessage)
	if teamCount > 0 {
		impl.lg.Info("检测到用户消息中的团队数量约束",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("trace_id", traceID),
			loggateway.Int("team_count", teamCount),
		)
	}
	input = biz.MergeCommitted(ctx, input)

	// P4-G5 决策证据链：decisionSource 记录策略来源——llm_mode（LLM/用户
	// 显式指定）/ keyword_fallback（关键词回退升级）/ complexity_auto
	// （六维评分自动评估）/ memory_cache（编排缓存命中）/ route_commit
	// （编排器提交的 RouteDecision，Plan 不得再改车道）。
	decisionSource := "complexity_auto"
	if input.Committed.Lane == biz.RouteLanePlanTeam {
		decisionSource = "route_commit"
		m := strings.ToLower(strings.TrimSpace(input.Mode))
		if m == "" || m == "auto" || m == "direct" {
			input.Mode = strings.TrimSpace(input.Committed.Mode)
			if input.Mode == "" {
				input.Mode = "dag"
			}
			impl.lg.Info("消费已提交 PlanTeam 路由，禁止入口自降级 direct",
				loggateway.StepID(biz.SpiritStepPlannerAssess),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("committed_mode", input.Mode),
			)
		}
	} else if m := strings.ToLower(strings.TrimSpace(input.Mode)); m == "" || m == "auto" {
		if detected := detectTeamIntent(input.UserMessage); detected != "" {
			impl.lg.Info("检测到用户消息中的团队组建意图，自动升级模式",
				loggateway.StepID(biz.SpiritStepPlannerAssess),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("detected_mode", detected),
				loggateway.Str("original_mode", input.Mode),
				loggateway.Int("team_count", teamCount),
			)
			input.Mode = detected
			decisionSource = "keyword_fallback"
		}
	} else {
		decisionSource = "llm_mode"
	}

	// Step 1: Assess complexity (six dimensions)
	dimensions := impl.assessComplexity(input)
	complexityScore := dimensions.Semantic*0.25 +
		dimensions.Structural*0.15 +
		dimensions.Domain*0.15 +
		dimensions.Tool*0.10 +
		dimensions.Context*0.10 +
		dimensions.Historical*0.25

	var complexityLevel biz.ComplexityLevel
	switch {
	case complexityScore >= 0.6:
		complexityLevel = biz.ComplexityComplex
	case complexityScore >= 0.3:
		complexityLevel = biz.ComplexityModerate
	default:
		complexityLevel = biz.ComplexitySimple
	}
	if input.Committed.Specified() {
		complexityLevel, complexityScore = applyCommittedComplexity(input.Committed, complexityLevel, complexityScore)
	}

	// Honor explicit user/LLM mode: team-forming modes (parallel/dag/coordinator)
	// must trigger decomposition even when the complexity score is low, otherwise
	// the team would never be assembled. This is the root-cause fix for the
	// "user asks for teams but StrategyDirect short-circuits" bug.
	explicitMode := strings.ToLower(strings.TrimSpace(input.Mode))
	modeIsExplicit := explicitMode != "" && explicitMode != "auto"
	// 组队证据闸（包C Q2-C2，session-eval-20260827 方向二根修）：LLM 在强制
	// 规划压力下可能为无组队诉求的任务自选 parallel/dag——S09-t1「搭内容
	// 框架」（单交付物、自我规划、无组织实体）被 llm_mode 组队 → roster_miss
	// → DAG 缺 FuncRef 失败回退，一轮烧 153K/181s。组队是重资源承诺，
	// llm_mode 组队要求结构性证据（组队意图词/数量约束/组织链/派发句式/
	// 模式字面量）或六维评分自然达 Complex；无证据降级 direct 并留痕。
	// 宁直勿组（RouteLLM 非对称代价的镜像）：直答+计划板成本 ≪ 空组队
	// roster_miss。豁免：keyword_fallback（detectTeamIntent 命中即证据）、
	// 显式 agent_keys 路由（用户/系统契约）、force_new、memory/playbook
	// 路径（本函数前段已 return）。
	if modeIsExplicit && shouldForceComplex(explicitMode) && decisionSource == "llm_mode" &&
		complexityLevel != biz.ComplexityComplex && len(input.AgentKeys) == 0 &&
		!hasTeamModeEvidence(input.UserMessage) {
		impl.lg.Info("组队模式无结构证据，降级 direct（组队证据闸）",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("original_mode", explicitMode),
			loggateway.Float64("complexity_score", complexityScore),
		)
		input.Mode = "direct"
		explicitMode = "direct"
		decisionSource = "evidence_gate_downgrade"
	}
	effectiveLevel := complexityLevel
	if modeIsExplicit && shouldForceComplex(input.Mode) && complexityLevel != biz.ComplexityComplex {
		effectiveLevel = biz.ComplexityComplex
		impl.lg.Info("用户显式指定编排模式，强制升级复杂度以触发任务分解",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("mode", input.Mode),
			loggateway.Str("original_level", string(complexityLevel)),
		)
	}
	if input.Committed.Lane == biz.RouteLanePlanSolo && effectiveLevel != biz.ComplexityComplex {
		effectiveLevel = biz.ComplexityComplex
		impl.lg.Info("消费已提交 PlanSolo 路由，抬档分解且不组队",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("trace_id", traceID),
		)
	}

	impl.lg.Info("复杂度评估完成",
		loggateway.StepID(biz.SpiritStepPlannerAssess),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("complexity_level", string(complexityLevel)),
		loggateway.Float64("complexity_score", complexityScore),
	)

	// Step 2: Determine strategy
	strategy, strategyReason, topologyHint := impl.determineStrategy(complexityLevel, complexityScore, input)

	impl.lg.Info("策略路由完成",
		loggateway.StepID(biz.SpiritStepPlannerRoute),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("strategy", string(strategy)),
		loggateway.Str("topology_hint", string(topologyHint)),
	)

	// P4-G5 决策证据链：策略路由决策落 FlowLog（spirit.planner.decision），
	// 供 Monitor 流程日志审计 + MAST 失败模式标注消费。决策时刻的策略
	// 原样记录——后续分解失败/校验门降级走各自已有事件，不回写本条。
	impl.emitPlannerDecision(ctx, plannerDecision{
		TraceID:         traceID,
		DecisionSource:  decisionSource,
		Mode:            strings.ToLower(strings.TrimSpace(input.Mode)),
		Strategy:        strategy,
		ComplexityLevel: complexityLevel,
		ComplexityScore: complexityScore,
		TeamCount:       teamCount,
		KeywordFallback: decisionSource == "keyword_fallback",
		StrategyReason:  strategyReason,
		SpiritSessionID: input.SpiritSessionID,
	})

	// Step 3: Build intent artifact JSON（提前——P2 draft 落库需要）。
	intentArtifactJSON := "{}"
	if input.IntentArtifact != nil {
		b, err := json.Marshal(input.IntentArtifact)
		if err == nil {
			intentArtifactJSON = string(b)
		}
	}

	// Step 4: Decompose task (only for complex, or when explicit team-forming mode forces it)
	var subTasks []biz.SubTask
	var dag *biz.PlanTaskDAG
	var decomposeReason string
	var playbookHit *biz.MemoryHit
	streamPublished := false
	planID := "tp_" + uuid.NewString()
	gear := ClassifyTaskGear(GearInput{
		UserWantsOrgChain: HasOrgChainIntent(input.UserMessage),
		LongTask:          effectiveLevel == biz.ComplexityComplex || complexityScore >= 0.6,
		CompanyNodeCount:  impl.companyNodeCount(ctx),
		FactQuery:         biz.LooksLikeFactQuery(input.UserMessage),
	})
	if pb, steps, ok := impl.lookupSolePlaybookIfHeavy(ctx, gear); ok && len(steps) > 0 {
		subTasks = steps
		dag = buildDAGFromSubTasks(steps)
		decomposeReason = "authorized playbook expand; skip planner LLM"
		playbookHit = playbookMemoryHit(pb)
		if len(steps) >= 2 {
			strategy = biz.StrategyDAG
			strategyReason = decomposeReason
		}
	} else if slots := subTasksFromSubIntents(input.IntentArtifact); len(slots) >= 2 {
		subTasks = slots
		dag = buildDAGFromSubTasks(slots)
		decomposeReason = "composite sub_intents; skip planner LLM"
		strategy, strategyReason, topologyHint = promoteStrategyForSubTasks(strategy, strategyReason, topologyHint, complexityLevel, input.Mode, true)
	} else if effectiveLevel == biz.ComplexityComplex {
		// P2：分解前先落库 draft——持久化与展示同时进行。崩溃后 draft
		// 可恢复、可观测「正在规划」，不再「先分解 60s、最后一次性落库」。
		draft := &biz.TaskPlan{
			ID:                 planID,
			SpiritSessionID:    input.SpiritSessionID,
			TraceID:            traceID,
			UserMessage:        input.UserMessage,
			IntentArtifactJSON: intentArtifactJSON,
			ComplexityLevel:    complexityLevel,
			ComplexityScore:    complexityScore,
			Dimensions:         dimensions,
			Strategy:           strategy,
			StrategyReason:     strategyReason,
			TopologyHint:       topologyHint,
			Status:             biz.TaskPlanStatusDraft,
		}
		saved, err := impl.repo.Create(ctx, draft)
		if err != nil {
			impl.lg.Warn("TaskPlan draft 持久化失败",
				loggateway.StepID(biz.SpiritStepPlannerPersist),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
			return nil, apierror.Internal(apierror.DomainSpirit, "persist draft plan: "+err.Error())
		}

		if input.DeferLLMDecompose {
			saved.DecomposeReason = biz.DecomposeReasonDeferred
			if updated, uerr := impl.repo.Update(ctx, saved); uerr == nil {
				saved = updated
			}
			impl.warnIfRouteViolated(ctx, input.Committed, saved)
			impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "decomposing", map[string]any{
				"plan_id":  planID,
				"deferred": true,
			})
			return saved, nil
		}
		return impl.runComplexLLMDecompose(ctx, saved, input, teamCount, complexityLevel, strategy, strategyReason, topologyHint, gear, playbookHit)
	}

	if len(subTasks) > 0 {
		gear, playbookHit, subTasks, dag, strategy, strategyReason, decomposeReason = impl.applyCrossDeptGearUpgrade(
			ctx, input, gear, playbookHit, subTasks, dag, strategy, strategyReason, decomposeReason, streamPublished)
	}

	// 非分解路径（direct 策略）：单次 Create，无 draft 中间态。
	plan := &biz.TaskPlan{
		ID:                 planID,
		SpiritSessionID:    input.SpiritSessionID,
		TraceID:            traceID,
		UserMessage:        input.UserMessage,
		IntentArtifactJSON: intentArtifactJSON,
		ComplexityLevel:    complexityLevel,
		ComplexityScore:    complexityScore,
		Dimensions:         dimensions,
		SubTasks:           subTasks,
		TaskDAG:            dag,
		DecomposeReason:    decomposeReason,
		Strategy:           strategy,
		StrategyReason:     strategyReason,
		TopologyHint:       topologyHint,
		DomainPath:         PrimaryDomainPath(subTasks),
		MemoryHit:          playbookHit,
		Status:             biz.TaskPlanStatusDraft,
		StreamPublished:    streamPublished,
	}

	impl.lg.Info("持久化 TaskPlan",
		loggateway.StepID(biz.SpiritStepPlannerPersist),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("plan_id", plan.ID),
	)

	saved, err := impl.repo.Create(ctx, plan)
	if err != nil {
		impl.lg.Warn("TaskPlan 持久化失败",
			loggateway.StepID(biz.SpiritStepPlannerPersist),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return nil, apierror.Internal(apierror.DomainSpirit, "persist plan: "+err.Error())
	}

	impl.warnIfRouteViolated(ctx, input.Committed, saved)

	// Publish spirit_plan_created event.
	impl.publishPlanCreated(ctx, saved, input.ChatSessionID)

	return saved, nil
}

// applyCrossDeptGearUpgrade raises medium → heavy when the expanded plan has
// cross-department deliverable dependencies (R9 late upgrade). If the upgrade
// lands on heavy and no playbook was already expanded, look up the company's
// sole authorized playbook. Streamed LLM boards keep their published steps
// (replacing them would desync the v2 board). Cross-dept DAG without a
// playbook stays as-is: that is the correct medium 借调/DAG path.
func (impl *taskPlannerImpl) applyCrossDeptGearUpgrade(
	ctx context.Context,
	input biz.PlanInput,
	gear TaskGear,
	playbookHit *biz.MemoryHit,
	subTasks []biz.SubTask,
	dag *biz.PlanTaskDAG,
	strategy biz.OrchestrationStrategy,
	strategyReason, decomposeReason string,
	streamPublished bool,
) (TaskGear, *biz.MemoryHit, []biz.SubTask, *biz.PlanTaskDAG, biz.OrchestrationStrategy, string, string) {
	if len(subTasks) == 0 || !PlanHasCrossDeptDepends(subTasks) {
		return gear, playbookHit, subTasks, dag, strategy, strategyReason, decomposeReason
	}
	upgraded := UpgradeGearAfterPlan(gear, true)
	if upgraded != gear {
		impl.lg.Info("跨部门交付依赖，分档升重",
			loggateway.StepID(biz.SpiritStepPlannerRoute),
			loggateway.Str("from_gear", string(gear)),
			loggateway.Str("to_gear", string(upgraded)),
			loggateway.Str("spirit_session_id", input.SpiritSessionID),
		)
	}
	gear = upgraded
	if gear != GearHeavy || playbookHit != nil || streamPublished {
		return gear, playbookHit, subTasks, dag, strategy, strategyReason, decomposeReason
	}
	pb, steps, ok := impl.lookupSolePlaybookIfHeavy(ctx, gear)
	if !ok || len(steps) == 0 {
		return gear, playbookHit, subTasks, dag, strategy, strategyReason, decomposeReason
	}
	reason := "authorized playbook expand after cross-dept upgrade"
	return gear, playbookMemoryHit(pb), steps, buildDAGFromSubTasks(steps), biz.StrategyDAG, reason, reason
}

// GetPlan retrieves a plan by ID.
func (impl *taskPlannerImpl) GetPlan(ctx context.Context, planID string) (*biz.TaskPlan, error) {
	plan, err := impl.repo.GetByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// ListPlans returns all plans for a spirit session, newest first (T3.2).
func (impl *taskPlannerImpl) ListPlans(ctx context.Context, spiritSessionID string) ([]*biz.TaskPlan, error) {
	return impl.repo.ListBySpiritSessionID(ctx, spiritSessionID)
}

// QuickAssess performs a pure-computation complexity assessment (P1-2).
// It reuses the six-dimension assessComplexity logic but skips memory cache,
// LLM decomposition, and DB persistence — making it safe to call before the
// Spirit LLM runs. Used by PrePlanningGate to force planning for Moderate/Complex.
//
// Team-intent override: when the user message contains explicit team-formation
// keywords (parallel/coordinator), the level is upgraded to at least Moderate.
// This breaks the chicken-and-egg problem where detectTeamIntent was only
// called inside Plan(), but Plan() is only invoked when ForcePlanning=true
// (which QuickAssess controls). Without this override, team-formation requests
// rated "simple" never trigger planning, and no teams are created.
//
// Explicit-tool-request override (2026-07-28): messages naming a concrete
// snake_case tool identifier (≥2 underscores, e.g. cli_admin_skill_install_from_url)
// or plan_and_execute are likewise upgraded to at least Moderate, so the gate
// forces planning deterministically instead of leaving routing to LLM discretion.
func (impl *taskPlannerImpl) QuickAssess(_ context.Context, input biz.PlanInput) (biz.ComplexityLevel, float64, error) {
	dimensions := impl.assessComplexity(input)
	score := dimensions.Semantic*0.25 +
		dimensions.Structural*0.15 +
		dimensions.Domain*0.15 +
		dimensions.Tool*0.10 +
		dimensions.Context*0.10 +
		dimensions.Historical*0.25

	var level biz.ComplexityLevel
	switch {
	case score >= 0.6:
		level = biz.ComplexityComplex
	case score >= 0.3:
		level = biz.ComplexityModerate
	default:
		level = biz.ComplexitySimple
	}

	// Team-intent override: force at least Moderate so the pre-planning gate
	// triggers ForcePlanning=true and the hard gate calls Plan().
	if detected := detectTeamIntent(input.UserMessage); detected != "" && level == biz.ComplexitySimple {
		level = biz.ComplexityModerate
		if score < 0.3 {
			score = 0.3
		}
		impl.lg.Info("QuickAssess 检测到团队组建意图，升级复杂度以触发规划",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("detected_mode", detected),
			loggateway.Float64("complexity_score", score),
		)
	}

	// Explicit tool/orchestration request override: a message naming a concrete
	// snake_case tool identifier with ≥2 underscores (e.g.
	// cli_admin_skill_install_from_url, plan_and_execute) explicitly requests
	// capabilities beyond a direct answer. Upgrade to at least Moderate so the
	// gate forces the planning path deterministically, instead of leaving the
	// routing decision to LLM discretion.
	// (2026-07-28, session 784a8707: install-skill request scored 0.215 simple;
	// gate notice claimed "直接回答" while the LLM self-routed to
	// plan_and_execute and launched two teams — notice contradicted behavior.)
	if level == biz.ComplexitySimple && explicitToolRequestPattern.MatchString(strings.ToLower(input.UserMessage)) {
		level = biz.ComplexityModerate
		if score < 0.3 {
			score = 0.3
		}
		impl.lg.Info("QuickAssess 检测到显式工具/编排调用请求，升级复杂度以触发规划",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Float64("complexity_score", score),
		)
	}

	// Team-mode-evidence override（P0 档位，session-eval-20260827 F2 纠偏）：
	// ForcePlanning 会走 plan_and_execute，成本远高于一次 intent pass。
	// 任务动作词（HasTaskActionSignal）只禁止 skip intent——由
	// shouldSkipIntentPass 消费；本门只在有组队结构证据时升级 Moderate
	// （派发句式/组队关键词/组织链）。否则 S09-t1「我们来规划内容框架」
	// 与 S08「出一条脚本」会被误强制组队（181s / 153K）。
	if level == biz.ComplexitySimple && hasTeamModeEvidence(input.UserMessage) {
		level = biz.ComplexityModerate
		if score < 0.3 {
			score = 0.3
		}
		impl.lg.Info("QuickAssess 命中组队证据，升级复杂度以触发规划",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Float64("complexity_score", score),
		)
	}

	impl.lg.Debug("QuickAssess 完成",
		loggateway.StepID(biz.SpiritStepPlannerAssess),
		loggateway.Str("complexity_level", string(level)),
		loggateway.Float64("complexity_score", score),
	)
	return level, score, nil
}

// ConfirmPlan applies adjustments and confirms the plan.
func (impl *taskPlannerImpl) ConfirmPlan(ctx context.Context, planID string, adjustments biz.PlanAdjustments) (*biz.TaskPlan, error) {
	plan, err := impl.repo.GetByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan.Status != biz.TaskPlanStatusDraft {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "plan is not in draft status")
	}

	// Apply adjustments
	if len(adjustments.MergeSubTasks) > 0 {
		plan.SubTasks = mergeSubTasks(plan.SubTasks, adjustments.MergeSubTasks)
	}
	if adjustments.SplitSubTask != "" {
		plan.SubTasks = splitSubTask(plan.SubTasks, adjustments.SplitSubTask)
	}
	if adjustments.AddSubTask != nil {
		newTask := biz.SubTask{
			ID:                   "st_" + uuid.NewString(),
			Name:                 adjustments.AddSubTask.Name,
			Description:          adjustments.AddSubTask.Description,
			DependsOn:            adjustments.AddSubTask.DependsOn,
			RequiredCapabilities: adjustments.AddSubTask.RequiredCapabilities,
			Priority:             adjustments.AddSubTask.Priority,
			EstimatedComplexity:  0.5,
		}
		plan.SubTasks = append(plan.SubTasks, newTask)
	}
	if adjustments.RemoveSubTask != "" {
		plan.SubTasks = removeSubTask(plan.SubTasks, adjustments.RemoveSubTask)
	}
	if adjustments.StrategyOverride != "" {
		plan.Strategy = biz.OrchestrationStrategy(adjustments.StrategyOverride)
		plan.StrategyReason = "Spirit LLM 覆盖策略: " + adjustments.Reason
	}

	// Rebuild DAG if subtasks changed
	if len(adjustments.MergeSubTasks) > 0 || adjustments.SplitSubTask != "" || adjustments.AddSubTask != nil || adjustments.RemoveSubTask != "" {
		plan.TaskDAG = buildDAGFromSubTasks(plan.SubTasks)
	}

	plan.Status = biz.TaskPlanStatusConfirmed

	impl.lg.Info("TaskPlan 确认",
		loggateway.StepID(biz.SpiritStepPlannerConfirm),
		loggateway.Str("plan_id", planID),
		loggateway.Str("strategy", string(plan.Strategy)),
	)

	saved, err := impl.repo.Update(ctx, plan)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainSpirit, "update plan").WithCause(err)
	}
	return saved, nil
}

// queryMemory queries the OrchestrationCache for similar past orchestrations.
// Returns a MemoryHit if a high-quality historical match is found, nil otherwise.
func (impl *taskPlannerImpl) queryMemory(ctx context.Context, input biz.PlanInput, traceID string) *biz.MemoryHit {
	if impl.orchCache == nil {
		return nil
	}

	// Try SuggestTopology first for a quick hit
	topology, hit := impl.orchCache.SuggestTopology(input.UserMessage)
	if !hit {
		return nil
	}

	// Query for detailed cache entries
	cacheEntries, err := impl.orchCache.QueryByTaskPattern(ctx, input.UserMessage)
	if err != nil || len(cacheEntries) == 0 {
		return nil
	}

	best := cacheEntries[0]
	if best.DQScore < 0.7 {
		impl.lg.Info("编排缓存命中但 DQ 分数不足",
			loggateway.StepID(biz.SpiritStepPlannerMemory),
			loggateway.Str("trace_id", traceID),
			loggateway.Float64("dq_score", best.DQScore),
		)
		return nil
	}

	return &biz.MemoryHit{
		CacheID:               best.TaskPattern,
		DQScore:               best.DQScore,
		TopologyUsed:          string(topology),
		AgentKeysUsed:         best.AgentKeys,
		DomainPath:            best.DomainPath,
		Specialties:           append([]string(nil), best.Specialties...),
		ConstraintFingerprint: best.ConstraintFingerprint,
	}
}

func recipeReplaySubTasks(hit *biz.MemoryHit, userMessage string) []biz.SubTask {
	if hit == nil {
		return nil
	}
	if len(hit.Specialties) > 0 {
		out := make([]biz.SubTask, 0, len(hit.Specialties))
		for i, spec := range hit.Specialties {
			spec = strings.TrimSpace(spec)
			if spec == "" {
				continue
			}
			out = append(out, biz.SubTask{
				ID:          fmt.Sprintf("st_recipe_%d", i+1),
				Name:        spec,
				Description: userMessage,
				DomainPath:  spec,
			})
		}
		if len(out) > 0 {
			return out
		}
	}
	label := strings.TrimSpace(userMessage)
	if rs := []rune(label); len(rs) > 80 {
		label = string(rs[:80])
	}
	return []biz.SubTask{{
		ID:          "st_recipe_replay",
		Name:        label,
		Description: userMessage,
		DomainPath:  hit.DomainPath,
	}}
}

// validateRecipeSubTasks N1 结构校验：记忆复用的子任务必须能通过花名册
// 映射预检（bindRosterSpecialist），任一子任务无法映射即判定整个记忆命中
// 结构无效。fail-open：capBuilder 缺失或 BuildAll 失败时跳过校验（与
// decomposeCapabilityTags 的 fail-open 语义一致）。
func (impl *taskPlannerImpl) validateRecipeSubTasks(ctx context.Context, subTasks []biz.SubTask) bool {
	if len(subTasks) == 0 {
		return false
	}
	if impl.capBuilder == nil {
		return true
	}
	caps, err := impl.capBuilder.BuildAll(ctx)
	if err != nil {
		impl.lg.Warn("记忆命中结构校验：能力清单构建失败，跳过校验（fail-open）",
			loggateway.StepID(biz.SpiritStepPlannerMemory),
			loggateway.Err(err),
		)
		return true
	}
	assignable := filterHeuristicAssignable(caps)
	for _, st := range subTasks {
		if _, _, ok := bindRosterSpecialist(st.DomainPath, st.Name+" "+st.Description, assignable); !ok {
			impl.lg.Info("记忆命中子任务花名册映射失败",
				loggateway.StepID(biz.SpiritStepPlannerMemory),
				loggateway.Str("subtask_id", st.ID),
				loggateway.Str("subtask_name", st.Name),
				loggateway.Str("domain_path", st.DomainPath),
			)
			return false
		}
	}
	return true
}

func (impl *taskPlannerImpl) warnIfRouteViolated(ctx context.Context, committed biz.RouteDecision, plan *biz.TaskPlan) {
	if impl == nil || plan == nil {
		return
	}
	if err := biz.CheckRouteHonored(committed, plan.Strategy, len(plan.SubTasks), plan.DecomposeReason); err != nil {
		impl.lg.Warn("FIT-ROUTE-1 提交档被下游改写",
			loggateway.StepID(biz.SpiritStepPlannerRoute),
			loggateway.Str("committed_lane", string(committed.Lane)),
			loggateway.Str("strategy", string(plan.Strategy)),
			loggateway.Int("subtask_count", len(plan.SubTasks)),
			loggateway.Err(err),
		)
	}
}

// plannerDecision 是 P4-G5 策略决策证据链的记录单元。
type plannerDecision struct {
	TraceID         string
	DecisionSource  string // llm_mode / keyword_fallback / complexity_auto / memory_cache
	Mode            string
	Strategy        biz.OrchestrationStrategy
	ComplexityLevel biz.ComplexityLevel
	ComplexityScore float64
	TeamCount       int
	KeywordFallback bool
	StrategyReason  string
	SpiritSessionID string
}

// emitPlannerDecision 把策略路由决策经 event.EmitDecision 统一双写
// （decision_records + spirit.planner.decision flowlog）。决策时刻的快照原样
// 落盘——后续分解失败或校验门降级各有专属事件，不回写本条，保证证据链只读追加。
func (impl *taskPlannerImpl) emitPlannerDecision(ctx context.Context, d plannerDecision) {
	// Mode 为空（用户未显式指定）时策略即生效 mode——outcome 不允许空。
	mode := d.Mode
	if mode == "" {
		mode = string(d.Strategy)
	}
	confidence := d.ComplexityScore
	// SP-1b（2026-08-29 Q2 根修）：统一入口一次完成 decision_records +
	// flowlog 双写，消除手工两调用的偏斜风险；collector 为 nil（旧构造
	// 路径/CLI）时 EmitDecision 内部记 collector_nil，flowlog 仍落。
	event.EmitDecision(ctx, impl.decisions, decision.Record{
		DecisionKey: uuid.NewString(),
		Category:    decision.CategoryPlannerOrchestration,
		Scenario:    fmt.Sprintf("策略路由: %s/%s", mode, d.Strategy),
		Reasoning:   d.StrategyReason,
		Outcome:     "selected_" + mode,
		Confidence:  &confidence,
		ActorType:   decision.ActorSystem,
		ActorKey:    "system:task_planner",
		SourceRef:   decision.SourceRef{FlowTraceID: d.TraceID},
		Metadata: map[string]any{
			"decision_source":    d.DecisionSource,
			"fallback_triggered": d.KeywordFallback,
			"complexity_level":   string(d.ComplexityLevel),
			"team_count":         d.TeamCount,
			"spirit_session_id":  d.SpiritSessionID,
		},
	}, "spirit.planner.decision", "策略决策",
		event.P("decision_source", d.DecisionSource),
		event.P("mode", d.Mode),
		event.P("strategy", string(d.Strategy)),
		event.P("complexity_level", string(d.ComplexityLevel)),
		event.P("complexity_score", d.ComplexityScore),
		event.P("team_count", d.TeamCount),
		event.P("fallback_triggered", d.KeywordFallback),
		event.P("strategy_reason", d.StrategyReason),
		event.P("spirit_session_id", d.SpiritSessionID),
	)
}

// emitTeamCountMismatchGate 把 team_count_mismatch 截断/放行双写为
// system_guard 决策记录（设计 §3.2 row 3，G-R4）。collector 为 nil 时静默
// 跳过。action 取 "truncate"（截取前 N 个）或 "proceed"（按实际数量放行），
// 分别映射 outcome truncated / proceeded。
func (impl *taskPlannerImpl) emitTeamCountMismatchGate(ctx context.Context, traceID, spiritSessionID, action string, requested, decomposed int, dropped []string) {
	outcome := "proceeded"
	verb := "按实际数量放行"
	if action == "truncate" {
		outcome = "truncated"
		verb = fmt.Sprintf("截取前 %d 个", requested)
	}
	extra := map[string]any{"spirit_session_id": spiritSessionID}
	if len(dropped) > 0 {
		extra["dropped_subtask_names"] = dropped
	}
	event.EmitGate(ctx, impl.decisions, decision.GateDecision{
		TriggerRule:   decision.TriggerTeamCountMismatch,
		Outcome:       outcome,
		Scenario:      fmt.Sprintf("分解子任务数(%d)与用户请求团队数(%d)不符", decomposed, requested),
		Reasoning:     fmt.Sprintf("用户请求 %d 个团队，LLM 分解 %d 个子任务，校验门有界重分解修复未果，%s", requested, decomposed, verb),
		GuardName:     "plan_verifier",
		FlowTraceID:   traceID,
		Entities:      []decision.EntityRef{{Type: "spirit_session", Key: spiritSessionID}},
		ObservedValue: decomposed,
		Threshold:     requested,
		Action:        action,
		Extra:         extra,
	})
}

// taskRequestWords 已并入 biz.taskActionPatterns。QuickAssess 第三
// override 消费 hasTeamModeEvidence（组队才 ForcePlanning）；任务动作词
// 只进 shouldSkipIntentPass，不再单独抬档。

// resolvePlannerProviderModel and resolveFallbackProviderModelFromCatalog
// were removed in favor of ResolvePlannerModel (planner_model_resolver.go),
// which reads the planner_model_mode system setting and falls back to the
// session's effective model (inherit mode) or the first enabled catalog model.

// parseClarificationRequest 检测分解 LLM 的澄清出口输出（Q7，
// session-eval-20260827 P4 根修）：{"needs_clarification": true,
// "questions": [...]}。命中返回清洗后的问题列表；未命中返回 nil,false。
// 必须先于 parseDecompositionOutput 调用——澄清输出是 JSON object，
// 走数组解析会得到空结果，误入 decompose_empty 降级。

// ---------------------------------------------------------------------------
// P3：分解调用可靠性辅助——错误分类 + 重试退避。
// ---------------------------------------------------------------------------
