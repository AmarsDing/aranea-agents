package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// taskPlannerImpl implements biz.TaskPlannerPort.
type taskPlannerImpl struct {
	repo           biz.TaskPlanRepository
	catalog        *biz.LlmProviderModelUsecase
	httpClient     *http.Client
	bus            biz.ActivityEventBus
	orchCache      *biz.OrchestrationCache
	lg             loggateway.Logger
	plannerSetting PlannerModelLookup
	// seq is the v2 Sequencer (nil-safe) used to publish PlanBoard/PlanStep/
	// GraphStage/GraphNode events. Nil = v2 publish skipped (backwards compat).
	seq v2.SequencerPublisher
}

var _ biz.TaskPlannerPort = (*taskPlannerImpl)(nil)

// NewTaskPlanner creates a new TaskPlanner implementation.
func NewTaskPlanner(repo biz.TaskPlanRepository, catalog *biz.LlmProviderModelUsecase, httpClient *http.Client, bus biz.ActivityEventBus, orchCache *biz.OrchestrationCache, lg loggateway.Logger, plannerSetting PlannerModelLookup, seq v2.SequencerPublisher) biz.TaskPlannerPort {
	return &taskPlannerImpl{
		repo:           repo,
		catalog:        catalog,
		httpClient:     httpClient,
		bus:            bus,
		orchCache:      orchCache,
		lg:             lg,
		plannerSetting: plannerSetting,
		seq:            seq,
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

	// Step 0: Query OrchestrationCache for memory-driven routing
	memoryHit := impl.queryMemory(ctx, input, traceID)
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
			Status:          biz.TaskPlanStatusDraft,
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
		impl.publishPlanCreated(ctx, saved, input.ChatSessionID)
		return saved, nil
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
	if m := strings.ToLower(strings.TrimSpace(input.Mode)); m == "" || m == "auto" {
		if detected := detectTeamIntent(input.UserMessage); detected != "" {
			impl.lg.Info("检测到用户消息中的团队组建意图，自动升级模式",
				loggateway.StepID(biz.SpiritStepPlannerAssess),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("detected_mode", detected),
				loggateway.Str("original_mode", input.Mode),
				loggateway.Int("team_count", teamCount),
			)
			input.Mode = detected
		}
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

	// Honor explicit user/LLM mode: team-forming modes (parallel/dag/coordinator)
	// must trigger decomposition even when the complexity score is low, otherwise
	// the team would never be assembled. This is the root-cause fix for the
	// "user asks for teams but StrategyDirect short-circuits" bug.
	explicitMode := strings.ToLower(strings.TrimSpace(input.Mode))
	modeIsExplicit := explicitMode != "" && explicitMode != "auto"
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

	// Step 3: Decompose task (only for complex, or when explicit team-forming mode forces it)
	var subTasks []biz.SubTask
	var dag *biz.PlanTaskDAG
	var decomposeReason string
	if effectiveLevel == biz.ComplexityComplex {
		var err error
		subTasks, dag, err = impl.decomposeTask(ctx, input.UserMessage, input.IntentArtifact, teamCount)
		if err != nil {
			impl.lg.Warn("任务分解失败，降级为 direct 策略",
				loggateway.StepID(biz.SpiritStepPlannerDecompose),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
			strategy = biz.StrategyDirect
			strategyReason = "任务分解失败，降级为 direct"
			decomposeReason = "decompose_failed"
		} else if len(subTasks) > 0 {
			decomposeReason = fmt.Sprintf("分解为 %d 个子任务", len(subTasks))
			// 2026-07-04 问题 2 修复：当用户明确请求 N 个 team 但 LLM 产生
			// 不符数量的 subtask 时，截取前 N 个或记录警告。这是兜底——
			// buildDecompositionPrompt 已通过 prompt 硬约束 LLM，但 LLM
			// 可能不严格遵守。
			if teamCount > 0 && len(subTasks) > teamCount {
				impl.lg.Warn("LLM 分解的 subtask 数量超出用户请求的 team 数量，截取前 N 个",
					loggateway.StepID(biz.SpiritStepPlannerDecompose),
					loggateway.Str("trace_id", traceID),
					loggateway.Int("requested_team_count", teamCount),
					loggateway.Int("decomposed_subtask_count", len(subTasks)),
				)
				subTasks = subTasks[:teamCount]
				// 清理截取后悬挂的 DependsOn 引用，避免 DAG 执行失败
				validIDs := make(map[string]bool, len(subTasks))
				for _, st := range subTasks {
					validIDs[st.ID] = true
				}
				for i := range subTasks {
					filtered := subTasks[i].DependsOn[:0]
					for _, depID := range subTasks[i].DependsOn {
						if validIDs[depID] {
							filtered = append(filtered, depID)
						}
					}
					subTasks[i].DependsOn = filtered
				}
				dag = buildDAGFromSubTasks(subTasks)
				decomposeReason = fmt.Sprintf("分解为 %d 个子任务（按用户请求截取）", len(subTasks))
			} else if teamCount > 0 && len(subTasks) < teamCount {
				impl.lg.Warn("LLM 分解的 subtask 数量少于用户请求的 team 数量",
					loggateway.StepID(biz.SpiritStepPlannerDecompose),
					loggateway.Str("trace_id", traceID),
					loggateway.Int("requested_team_count", teamCount),
					loggateway.Int("decomposed_subtask_count", len(subTasks)),
				)
			}
			// Strategy is determined solely by the explicit mode (or detected
			// team intent). We no longer auto-refine based on DAG shape — the
			// LLM is the decision authority. When mode is empty (no explicit
			// request and no detected intent), strategy stays "direct" even if
			// decomposition produced subtasks; the subtasks are logged for
			// analysis but not executed by the orchestrator.
		}
	}

	// Step 4: Build intent artifact JSON
	intentArtifactJSON := "{}"
	if input.IntentArtifact != nil {
		b, err := json.Marshal(input.IntentArtifact)
		if err == nil {
			intentArtifactJSON = string(b)
		}
	}

	// Step 6: Build and persist TaskPlan
	plan := &biz.TaskPlan{
		ID:                 "tp_" + uuid.NewString(),
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
		MemoryHit:          nil, // Memory hit is handled in Step 0; normal path has no cache hit
		Status:             biz.TaskPlanStatusDraft,
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

	// Publish spirit_plan_created event.
	impl.publishPlanCreated(ctx, saved, input.ChatSessionID)

	return saved, nil
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
		CacheID:       best.TaskPattern,
		DQScore:       best.DQScore,
		TopologyUsed:  string(topology),
		AgentKeysUsed: best.AgentKeys,
	}
}

// assessComplexity computes the six-dimension complexity assessment.
func (impl *taskPlannerImpl) assessComplexity(input biz.PlanInput) biz.DimensionScores {
	dims := biz.DimensionScores{}

	// Semantic: estimate from message length and intent kind (weight 0.25)
	runeCount := len([]rune(input.UserMessage))
	if runeCount > 500 {
		dims.Semantic = 0.7
	} else if runeCount > 200 {
		dims.Semantic = 0.4
	} else {
		dims.Semantic = 0.2
	}
	if input.IntentArtifact != nil {
		switch input.IntentArtifact.IntentKind {
		case "research", "debug":
			dims.Semantic += 0.15
		case "code_change":
			dims.Semantic += 0.1
		}
		if dims.Semantic > 1.0 {
			dims.Semantic = 1.0
		}
	}

	// Structural: check if user message contains multiple questions/tasks (weight 0.15)
	dims.Structural = assessStructural(input.UserMessage)

	// Domain: evaluate from intent kind and risk flags (weight 0.15)
	dims.Domain = assessDomain(input.IntentArtifact)

	// Tool: evaluate from intent kind and search hints (weight 0.10)
	dims.Tool = assessTool(input.IntentArtifact)

	// Context: check message length and ambiguity count (weight 0.10)
	dims.Context = assessContext(input.UserMessage, input.IntentArtifact)

	// Historical: use HistoryDQScore from input (weight 0.25)
	dims.Historical = input.HistoryDQScore

	return dims
}

// assessStructural evaluates structural complexity (multiple tasks/questions).
func assessStructural(userMessage string) float64 {
	score := 0.0
	// Count sentence-ending punctuation as proxy for multiple tasks
	sentenceEnders := regexp.MustCompile(`[。！？.!?\n]`)
	matches := sentenceEnders.FindAllString(userMessage, -1)
	if len(matches) >= 5 {
		score += 0.5
	} else if len(matches) >= 3 {
		score += 0.3
	} else {
		score += 0.1
	}

	// Check for enumeration patterns (1. 2. 3. or - - -)
	enumPatterns := regexp.MustCompile(`(?:\d+[.、)]\s|[-*]\s)`)
	enumMatches := enumPatterns.FindAllString(userMessage, -1)
	if len(enumMatches) >= 3 {
		score += 0.4
	} else if len(enumMatches) >= 1 {
		score += 0.2
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// assessDomain evaluates domain complexity from intent kind and risk flags.
func assessDomain(artifact *biz.IntentArtifact) float64 {
	if artifact == nil {
		return 0.1
	}
	score := 0.0
	// Risk flags indicate cross-domain concerns
	if len(artifact.RiskFlags) >= 2 {
		score += 0.4
	} else if len(artifact.RiskFlags) >= 1 {
		score += 0.2
	}
	// Certain intent kinds imply cross-domain work
	switch artifact.IntentKind {
	case "research", "debug":
		score += 0.3
	case "code_change":
		score += 0.15
	}
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.1 {
		score = 0.1
	}
	return score
}

// assessTool evaluates tool complexity from intent kind and search hints.
func assessTool(artifact *biz.IntentArtifact) float64 {
	if artifact == nil {
		return 0.1
	}
	score := 0.0
	// Certain intent kinds imply tool usage
	switch artifact.IntentKind {
	case "code_change", "debug":
		score += 0.3
	case "research":
		score += 0.2
	}
	// More search hints suggest more tool lookups needed
	if len(artifact.SearchHints) >= 3 {
		score += 0.3
	} else if len(artifact.SearchHints) >= 1 {
		score += 0.15
	}
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.1 {
		score = 0.1
	}
	return score
}

// assessContext evaluates context complexity.
func assessContext(userMessage string, artifact *biz.IntentArtifact) float64 {
	score := 0.0
	// Message length factor
	runeCount := len([]rune(userMessage))
	if runeCount > 1000 {
		score += 0.3
	} else if runeCount > 500 {
		score += 0.2
	} else {
		score += 0.1
	}

	// Ambiguity count
	if artifact != nil {
		ambCount := len(artifact.Ambiguities)
		if ambCount >= 3 {
			score += 0.5
		} else if ambCount >= 1 {
			score += 0.3
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// determineStrategy selects the orchestration strategy based on the explicit
// mode passed by the Spirit LLM. The three valid modes are: direct, parallel,
// dag (see DECISION.md). Empty/auto mode defaults to direct.
//
// The complexity level/score are still computed for logging and metrics, but
// no longer drive strategy selection — the LLM is the sole decision authority.
func (impl *taskPlannerImpl) determineStrategy(_ biz.ComplexityLevel, _ float64, input biz.PlanInput) (biz.OrchestrationStrategy, string, biz.TopologyType) {
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	switch mode {
	case "direct":
		return biz.StrategyDirect, "用户指定 direct 模式", biz.TopologyDirect
	case "parallel":
		return biz.StrategyParallel, "用户指定 parallel 模式", biz.TopologyParallel
	case "dag":
		return biz.StrategyDAG, "用户指定 dag 模式", biz.TopologyHybrid
	}
	// Empty / auto / unknown mode: default to direct. The LLM is instructed
	// (DECISION.md) to always pass an explicit mode; reaching this branch means
	// the LLM did not comply — direct is the safest fallback.
	return biz.StrategyDirect, "未指定 mode，默认 direct 模式", biz.TopologyDirect
}

// shouldForceComplex returns true when the explicit mode requires subtask
// decomposition so the allocator has subtasks to assign agents to.
// Team-forming modes (parallel/dag) need decomposition regardless of the
// complexity score; otherwise a "simple" score would skip decomposition and
// the team would never be assembled.
func shouldForceComplex(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "parallel", "dag":
		return true
	}
	return false
}

// detectTeamCount extracts the user's explicit team count from the message.
// Returns 0 when no count is specified (caller should use default range).
// Recognized patterns:
//   - "2个团队", "3支团队", "两个team", "三个团队"
//   - "组建3个团队", "分派两个团队", "创建2个团队"
//   - "two teams", "3 teams", "five teams"
//
// 2026-07-04 问题 2 修复：原 detectTeamIntent 只返回 mode（"dag"），
// 丢弃数量约束。这导致 LLM decomposition 自由产生 2-6 个 subtask，
// orchestrateDAG 为每个 subtask 创建一个 team，最终 team 数量与用户
// 请求不符（用户要 2 个 team，可能多出 3-5 个）。
func detectTeamCount(message string) int {
	lower := strings.ToLower(message)
	// 1. 阿拉伯数字 + 量词 + team/团队：例如 "2个团队"、"3支team"、"5 teams"
	reDigit := regexp.MustCompile(`(\d+)\s*(?:个|支)?\s*(?:teams?|团队)`)
	if m := reDigit.FindStringSubmatch(lower); len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n <= 20 {
			return n
		}
	}
	// 2. 中文数字 + 量词 + team/团队：例如 "两个团队"、"三支team"
	cnNumMap := map[string]int{
		"一": 1, "两": 2, "二": 2, "三": 3, "四": 4, "五": 5,
		"六": 6, "七": 7, "八": 8, "九": 9, "十": 10,
	}
	reCN := regexp.MustCompile(`(一|两|二|三|四|五|六|七|八|九|十)\s*(?:个|支)?\s*(?:teams?|团队)`)
	if m := reCN.FindStringSubmatch(lower); len(m) >= 2 {
		if n, ok := cnNumMap[m[1]]; ok && n > 0 {
			return n
		}
	}
	// 3. 英文单词数字 + teams：例如 "two teams"、"three teams"
	enNumMap := map[string]int{
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
		"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	}
	reEN := regexp.MustCompile(`\b(one|two|three|four|five|six|seven|eight|nine|ten)\s+teams?\b`)
	if m := reEN.FindStringSubmatch(lower); len(m) >= 2 {
		if n, ok := enNumMap[m[1]]; ok && n > 0 {
			return n
		}
	}
	return 0
}

// detectTeamIntent scans the user message for explicit team-formation keywords
// and returns the recommended mode ("parallel" or "dag"). Returns "" if no
// team intent is detected.
//
// This is a backend-side fallback for when the Spirit LLM passes mode="" or
// "auto" despite the user explicitly asking for teams. The DECISION.md prompt
// instructs the LLM to pass explicit mode, but LLMs don't always comply —
// this ensures teams are created when the user clearly requests them.
//
// Mode selection rationale (aligned with DECISION.md three-mode system):
//   - "分派N个团队"/"组建团队"/"团队协作" (team formation keywords) → dag:
//     user expects one or more multi-member teams (≥2 members each).
//   - "并行"/"同时执行" (parallel keywords) → parallel: each subtask gets 1
//     agent, no multi-member teams formed. Use when subtasks are independent.
//   - Team keywords take precedence over parallel keywords: if the user
//     mentions "团队" they want dag (multi-member collaboration), not parallel.
func detectTeamIntent(message string) string {
	lower := strings.ToLower(message)
	// Team formation keywords: user wants one or more multi-member teams.
	// Includes quantity-based patterns ("分派两个团队") and generic team
	// keywords ("组建团队", "团队协作"). All route to `dag` mode so each team
	// can have ≥2 members (parallel would create 1-member teams, violating
	// the "team ≥2 members" rule in DECISION.md).
	teamKeywords := []string{
		// Quantity-based team requests (highest precedence)
		"两个team", "2个team", "两支team", "2支team",
		"两个团队", "2个团队", "两支团队", "2支团队",
		"三个team", "3个team", "三支team", "3支team",
		"三个团队", "3个团队", "三支团队", "3支团队",
		"多个团队", "多个team", "多支团队", "多支team",
		"分派两个", "分派2个", "分派三个", "分派3个",
		"分派团队", "分派team",
		// Generic team formation keywords
		// 2026-07-04 修复：扩展关键词，识别"组建一个团队"、"组建三个团队"等
		// 数量+量词变体，避免 QuickAssess 误判为 simple 导致不触发规划。
		"组建团队", "组建队", "组建team", "form a team", "build a team",
		"团队协作", "团队a", "团队b", "团队c",
		"组建一个团队", "组建一支团队", "组建1个团队", "组建1支团队",
		"组建两个团队", "组建两支团队", "组建2个团队", "组建2支团队",
		"组建三个团队", "组建三支团队", "组建3个团队", "组建3支团队",
		"组建多个团队", "组建多支团队",
		"创建团队", "创建一个团队", "创建一支团队",
		"成立团队", "成立一个团队",
		"编排团队", "编排一个团队",
		"调度团队", "调度一个团队",
	}
	for _, kw := range teamKeywords {
		if strings.Contains(lower, kw) {
			return "dag"
		}
	}
	// Parallel keywords: user wants concurrent independent subtasks. parallel
	// mode creates 1 agent per subtask (no multi-member teams), which is the
	// correct semantic for "并行处理多个独立子任务".
	parallelKeywords := []string{"并行", "同时执行", "并行处理", "parallel", "同时工作", "concurrently"}
	for _, kw := range parallelKeywords {
		if strings.Contains(lower, kw) {
			return "parallel"
		}
	}
	return ""
}

// decomposeTask uses LLM to decompose a complex task into subtasks (T1.6).
// teamCount > 0 时将作为硬约束传给 LLM，要求生成恰好 N 个 subtask（每个
// subtask 在 orchestrateDAG 中对应一个 team）。teamCount = 0 时使用默认范围。
func (impl *taskPlannerImpl) decomposeTask(ctx context.Context, userMessage string, artifact *biz.IntentArtifact, teamCount int) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "LLM catalog or HTTP client not configured")
	}

	prompt := buildDecompositionPrompt(userMessage, artifact, teamCount)

	// Resolve planner model via system setting (specify/inherit) with session
	// model fallback. Replaces legacy env-var + catalog-first approach.
	setting := biz.PlannerModelSetting{Mode: biz.PlannerModelModeInherit}
	if impl.plannerSetting != nil {
		if s, err := impl.plannerSetting.GetPlannerModel(ctx); err == nil {
			setting = s
		}
	}
	sessionProvider, sessionModel := biz.PlannerSessionModelFromCtx(ctx)
	provider, model := ResolvePlannerModel(ctx, setting, sessionProvider, sessionModel, impl.catalog, impl.lg, biz.SpiritStepPlannerAssess, "TaskPlanner")
	if provider == "" || model == "" {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "no provider/model configured for task decomposition (set planner_model_mode in system settings or add enabled models in catalog)")
	}

	row, err := impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "get provider config").WithCause(err)
	}

	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Decompose the following task:\n\n" + userMessage},
	}

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	text, _, _, _, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)
	if err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "LLM call failed").WithCause(err)
	}

	text = stripDecompositionFences(text)
	subTasks, err := parseDecompositionOutput(text)
	if err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "parse decomposition").WithCause(err)
	}

	if len(subTasks) == 0 {
		return nil, nil, nil
	}

	// Validate: no cycles, all depends_on references exist
	if err := validateSubTaskDAG(subTasks); err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "invalid DAG").WithCause(err)
	}

	dag := buildDAGFromSubTasks(subTasks)
	return subTasks, dag, nil
}

// buildDecompositionPrompt creates the system prompt for task decomposition.
// teamCount > 0 时强制要求 LLM 生成恰好 N 个 subtask（用户明确请求 N 个 team）；
// teamCount = 0 时使用默认 2-6 范围。
//
// 2026-07-04 问题 2 修复：原 prompt 固定 "2-6 subtasks"，未传递用户的数量
// 约束。当用户说"派出2个team"时，LLM 可能产生 5 个 subtask，orchestrateDAG
// 为每个 subtask 创建一个 team，导致最终多出 3 个 team。
func buildDecompositionPrompt(userMessage string, artifact *biz.IntentArtifact, teamCount int) string {
	intentContext := ""
	if artifact != nil {
		intentContext = fmt.Sprintf("\nIntent analysis:\n- Refined goal: %s\n- Intent kind: %s\n- Risk flags: %v",
			artifact.RefinedGoal,
			artifact.IntentKind,
			artifact.RiskFlags,
		)
	}

	countRule := "Break down complex tasks into 2-6 subtasks."
	if teamCount > 0 {
		// 用户明确请求 N 个 team：硬约束生成恰好 N 个 subtask。
		// 每个 subtask 将在 orchestrateDAG 中对应一个独立 team。
		countRule = fmt.Sprintf(`The user has explicitly requested EXACTLY %d teams.
You MUST produce EXACTLY %d subtasks — no more, no less.
Each subtask will be assigned to one dedicated team, so the subtask count MUST equal the requested team count.`, teamCount, teamCount)
	}

	return fmt.Sprintf(`You are a task decomposition specialist. %s

Rules:
- Each subtask must have: id (st_1, st_2, etc.), name, description, depends_on (array of other subtask IDs), required_capabilities (from the predefined list), priority (1-5, 1=highest), estimated_complexity (0.0-1.0)
- The "name" field MUST be a short noun-phrase suitable for displaying as a team name (e.g. "Code Analysis Team", "Data Pipeline Builder"), NOT a sentence-length task description. The "name" will be shown to the user as the team's display name; "id" is internal-only and never shown.
- Output ONLY a JSON array, no markdown fences, no commentary
- required_capabilities must use these predefined tags: go-backend, go-kratos, vue3-frontend, quasar-ui, devops, database, architecture, testing, security, research, documentation, api-design
- depends_on must only reference IDs of other subtasks in the array
- No circular dependencies allowed
- Subtasks should be independently executable where possible`, countRule) + intentContext
}

// resolvePlannerProviderModel and resolveFallbackProviderModelFromCatalog
// were removed in favor of ResolvePlannerModel (planner_model_resolver.go),
// which reads the planner_model_mode system setting and falls back to the
// session's effective model (inherit mode) or the first enabled catalog model.

// parseDecompositionOutput parses the LLM output into SubTask slice.
func parseDecompositionOutput(text string) ([]biz.SubTask, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	// Extract JSON array
	if i := strings.Index(text, "["); i >= 0 {
		if j := strings.LastIndex(text, "]"); j > i {
			text = text[i : j+1]
		}
	}

	var rawTasks []struct {
		ID                   string   `json:"id"`
		Name                 string   `json:"name"`
		Description          string   `json:"description"`
		DependsOn            []string `json:"depends_on"`
		RequiredCapabilities []string `json:"required_capabilities"`
		Priority             int      `json:"priority"`
		EstimatedComplexity  float64  `json:"estimated_complexity"`
	}

	if err := json.Unmarshal([]byte(text), &rawTasks); err != nil {
		return nil, apierror.Internal(apierror.DomainSpirit, "json unmarshal").WithCause(err)
	}

	// 2026-07-05 修复：LLM prompt 指定生成 st_1/st_2/... 格式的 ID，
	// 跨 session 会冲突（plan_steps_v2 表 id 字段全局 UNIQUE）。这里将
	// LLM 返回的 ID 重写为 st_<uuid>，同步重写 DependsOn 中的引用。
	// 所有后端/前端代码都不解析 ID 内容，仅作不透明字符串使用，因此
	// 重写不会破坏引用链。
	idRemap := make(map[string]string, len(rawTasks))
	for _, rt := range rawTasks {
		if strings.TrimSpace(rt.ID) == "" || strings.TrimSpace(rt.Name) == "" {
			continue
		}
		if _, exists := idRemap[rt.ID]; !exists {
			idRemap[rt.ID] = "st_" + uuid.NewString()
		}
	}

	subTasks := make([]biz.SubTask, 0, len(rawTasks))
	for _, rt := range rawTasks {
		if strings.TrimSpace(rt.ID) == "" || strings.TrimSpace(rt.Name) == "" {
			continue
		}
		if rt.DependsOn == nil {
			rt.DependsOn = []string{}
		}
		if rt.RequiredCapabilities == nil {
			rt.RequiredCapabilities = []string{}
		}
		if rt.Priority == 0 {
			rt.Priority = 3
		}
		if rt.EstimatedComplexity == 0 {
			rt.EstimatedComplexity = 0.5
		}
		// 重写 DependsOn 中的旧 ID 引用为新 UUID 格式
		rewiredDeps := make([]string, 0, len(rt.DependsOn))
		for _, depID := range rt.DependsOn {
			if newID, ok := idRemap[depID]; ok {
				rewiredDeps = append(rewiredDeps, newID)
			} else {
				// 未知引用保留原值，validateSubTaskDAG 会报错
				rewiredDeps = append(rewiredDeps, depID)
			}
		}
		subTasks = append(subTasks, biz.SubTask{
			ID:                   idRemap[rt.ID],
			Name:                 rt.Name,
			Description:          rt.Description,
			DependsOn:            rewiredDeps,
			RequiredCapabilities: rt.RequiredCapabilities,
			Priority:             rt.Priority,
			EstimatedComplexity:  rt.EstimatedComplexity,
		})
	}

	return subTasks, nil
}

// validateSubTaskDAG checks for cycles and invalid references.
func validateSubTaskDAG(tasks []biz.SubTask) error {
	idSet := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		idSet[t.ID] = true
	}

	// Check all depends_on references exist
	for _, t := range tasks {
		for _, depID := range t.DependsOn {
			if !idSet[depID] {
				return apierror.BadRequest(apierror.DomainSpirit, "subtask %s depends on non-existent subtask %s", t.ID, depID)
			}
		}
	}

	// Check for cycles using DFS
	const (
		white = 0
		gray  = 1
		black = 2
	)
	colors := make(map[string]int, len(tasks))
	for _, t := range tasks {
		colors[t.ID] = white
	}

	taskMap := make(map[string]*biz.SubTask, len(tasks))
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
	}

	var dfs func(id string) error
	dfs = func(id string) error {
		colors[id] = gray
		t := taskMap[id]
		if t != nil {
			for _, depID := range t.DependsOn {
				switch colors[depID] {
				case gray:
					return apierror.BadRequest(apierror.DomainSpirit, "cycle detected: %s → %s", id, depID)
				case white:
					if err := dfs(depID); err != nil {
						return err
					}
				}
			}
		}
		colors[id] = black
		return nil
	}

	for _, t := range tasks {
		if colors[t.ID] == white {
			if err := dfs(t.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildDAGFromSubTasks constructs a PlanTaskDAG from subtasks.
func buildDAGFromSubTasks(tasks []biz.SubTask) *biz.PlanTaskDAG {
	if len(tasks) == 0 {
		return nil
	}

	dag := &biz.PlanTaskDAG{
		Nodes: tasks,
	}

	// Compute root IDs (no dependencies)
	hasIncoming := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		hasIncoming[t.ID] = false
	}
	for _, t := range tasks {
		for _, depID := range t.DependsOn {
			hasIncoming[depID] = true
		}
	}
	for id, has := range hasIncoming {
		if !has {
			dag.RootIDs = append(dag.RootIDs, id)
		}
	}

	// Compute leaf IDs (nothing depends on them)
	hasOutgoing := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		for _, depID := range t.DependsOn {
			hasOutgoing[depID] = true
		}
	}
	for _, t := range tasks {
		if !hasOutgoing[t.ID] {
			dag.LeafIDs = append(dag.LeafIDs, t.ID)
		}
	}

	return dag
}

// mergeSubTasks merges specified subtasks into one.
func mergeSubTasks(tasks []biz.SubTask, ids []string) []biz.SubTask {
	if len(ids) < 2 {
		return tasks
	}
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	var merged biz.SubTask
	var remaining []biz.SubTask
	first := true
	for _, t := range tasks {
		if idSet[t.ID] {
			if first {
				merged = biz.SubTask{
					ID:                   t.ID,
					Name:                 t.Name,
					Description:          t.Description,
					DependsOn:            t.DependsOn,
					RequiredCapabilities: t.RequiredCapabilities,
					Priority:             t.Priority,
					EstimatedComplexity:  t.EstimatedComplexity,
				}
				first = false
			} else {
				merged.Description += "; " + t.Description
				for _, cap := range t.RequiredCapabilities {
					merged.RequiredCapabilities = append(merged.RequiredCapabilities, cap)
				}
				for _, dep := range t.DependsOn {
					if !idSet[dep] {
						merged.DependsOn = append(merged.DependsOn, dep)
					}
				}
				if t.Priority < merged.Priority {
					merged.Priority = t.Priority
				}
				merged.EstimatedComplexity = max(merged.EstimatedComplexity, t.EstimatedComplexity)
			}
		} else {
			remaining = append(remaining, t)
		}
	}
	if !first {
		remaining = append(remaining, merged)
	}
	return remaining
}

// splitSubTask splits a subtask into two.
func splitSubTask(tasks []biz.SubTask, id string) []biz.SubTask {
	var result []biz.SubTask
	for _, t := range tasks {
		if t.ID == id {
			// Split into two: original + new
			split := biz.SubTask{
				ID:                   id + "_b",
				Name:                 t.Name + " (Part 2)",
				Description:          t.Description,
				DependsOn:            []string{id},
				RequiredCapabilities: t.RequiredCapabilities,
				Priority:             t.Priority + 1,
				EstimatedComplexity:  t.EstimatedComplexity / 2,
			}
			t.Name = t.Name + " (Part 1)"
			t.EstimatedComplexity = t.EstimatedComplexity / 2
			result = append(result, t, split)
		} else {
			// Update references to the split task
			for i, dep := range t.DependsOn {
				if dep == id {
					t.DependsOn[i] = id + "_b"
				}
			}
			result = append(result, t)
		}
	}
	return result
}

// removeSubTask removes a subtask by ID.
func removeSubTask(tasks []biz.SubTask, id string) []biz.SubTask {
	var result []biz.SubTask
	for _, t := range tasks {
		if t.ID == id {
			continue
		}
		// Remove the deleted ID from depends_on
		var filteredDeps []string
		for _, dep := range t.DependsOn {
			if dep != id {
				filteredDeps = append(filteredDeps, dep)
			}
		}
		t.DependsOn = filteredDeps
		result = append(result, t)
	}
	return result
}

var fenceRE = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")

// stripDecompositionFences removes optional markdown code fences from model output.
func stripDecompositionFences(s string) string {
	s = strings.TrimSpace(s)
	if m := fenceRE.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return s
}

// publishPlanCreated publishes the spirit_plan_created event after a TaskPlan is persisted.
// The spirit_team_assembled event is NOT published here — it is only published by
// SpiritTeamAssembler when a real team is actually created, preventing ghost team
// entries in the frontend with fake plan IDs.
func (impl *taskPlannerImpl) publishPlanCreated(ctx context.Context, plan *biz.TaskPlan, chatSessionID string) {
	if impl.bus == nil || plan == nil {
		return
	}
	spiritSessionID := plan.SpiritSessionID

	// Use chatSessionID as the primary SessionID so the plan activity appears
	// in the chat session timeline. The frontend WebSocket subscription filters
	// by chat session ID, so plan status updates (from updatePlanStepForTeam)
	// must also use this SessionID to reach the frontend in real time.
	// Falls back to spiritSessionID when chatSessionID is empty (pre-existing
	// behavior for paths that don't have the chat session ID).
	sessionID := chatSessionID
	if sessionID == "" {
		sessionID = spiritSessionID
	}

	// Map plan.SubTasks → []ActivityPlanStep so the frontend PlanBlock can render
	// the task decomposition list (design m59-chat-ui-v7.html "任务拆解" section).
	// Without this, the PlanBlock only shows an empty header.
	steps := make([]biz.ActivityPlanStep, 0, len(plan.SubTasks))
	for _, st := range plan.SubTasks {
		steps = append(steps, biz.ActivityPlanStep{
			ID:        st.ID,
			Label:     st.Name,
			Status:    biz.ActivityStatusPending,
			DependsOn: st.DependsOn,
		})
	}

	// Resolve the root task activity ID from context so the plan nests under
	// the task in the Activity tree (frontend ActivityStream recursive rendering).
	// Without ParentActivityID, the plan becomes a root-level sibling of the task,
	// causing it to render below all task children (team_stage/graph_stage) instead
	// of between the task and its children.
	rootTaskID := string(RootTaskActivityIDFromCtx(ctx))

	// spirit_plan_created: the canonical event for plan creation.
	ev := biz.ActivityEvent{
		Event: biz.ActivityEventCreated,
		Activity: biz.Activity{
			ID:               uuid.NewString(),
			Kind:             biz.ActivityKindPlan,
			Status:           biz.ActivityStatusPending,
			Stage:            "created",
			Timestamp:        time.Now().UTC(),
			SpiritSessionID:  spiritSessionID,
			SessionID:        sessionID,
			ParentActivityID: rootTaskID,
			AgentKey:         "task-planner",
			Meta: map[string]any{
				"plan_id":           plan.ID,
				"spirit_session_id": spiritSessionID,
				"complexity_level":  string(plan.ComplexityLevel),
				"complexity_score":  plan.ComplexityScore,
				"strategy":          string(plan.Strategy),
				"strategy_reason":   plan.StrategyReason,
				"topology_hint":     string(plan.TopologyHint),
				"subtask_count":     len(plan.SubTasks),
				"steps":             steps,
			},
		},
		Domain: biz.ActivityDomainChat,
	}
	impl.bus.Publish(ctx, ev)
}

// PublishV2Board 通过 v2 Sequencer 发布 PlanBoard + PlanStep + GraphStage + GraphNode
// 创建事件。这些事件会被 Sequencer 持久化到 v2 表 + 推送到 WS，前端 v2 store 收到后
// 渲染 PlanBoardCard 和 GraphStageBlock。
//
// 设计参考：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md
// §3.2.2 GraphStage / §3.5.2 执行流程
//
// 关键关系：
//   - PlanBoard 与 GraphStage 一对一（GraphStage.PlanBoardID = PlanBoard.ID）
//   - PlanStep 与 GraphNode 一对一（GraphNode.ID = PlanStep.ID，确定性派生）
//   - 所有实体共用同一个 TaskID（rootTaskID）和 SpiritSessionID
//
// 2026-07-05 Step 3 修复：从 publishPlanCreated 内部移到 spirit_tools.go Phase 2
// 之后调用，使 PlanStep.AgentKeys 能从 allocPlan.Allocations 填充。原先在 Phase 1
// 发布时 allocPlan 不存在，导致 RealTeamOrchestrator 无法获取 LLM 分配结果，
// 退回查 DB 取错 agent（所有 team 用同一 agent）。
//
// 注意：此处仅发布 created 事件（status=pending/running）。后续生命周期事件
// （completed/failed）由 spirit_team.go 在团队状态变更时发布。
func (impl *taskPlannerImpl) PublishV2Board(ctx context.Context, plan *biz.TaskPlan, allocPlan *biz.AllocationPlan, chatSessionID string) {
	if impl.seq == nil || plan == nil {
		return
	}
	spiritSessionID := plan.SpiritSessionID
	if spiritSessionID == "" {
		return
	}
	// 当 plan.SubTasks 为空时跳过发布，避免创建空 PlanBoard 与空 GraphStage。
	// 触发场景：PrePlanningGate force planning（complexity >= Moderate）
	// 时调用 Plan()，但任务分解失败或编排缓存命中导致 SubTasks 为空。
	if len(plan.SubTasks) == 0 {
		impl.lg.Info("PublishV2Board: SubTasks 为空，跳过发布 PlanBoard/GraphStage",
			loggateway.StepID(biz.SpiritStepPlannerPersist),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Str("plan_id", plan.ID),
		)
		return
	}
	// rootTaskID 从 ctx 读取（与 publishPlanCreated 一致）。
	rootTaskID := string(RootTaskActivityIDFromCtx(ctx))
	// SessionID 优先使用 chatSessionID（与 v1 Activity 一致），便于前端按 chat session
	// 过滤；fallback 到 spiritSessionID。
	sessionID := chatSessionID
	if sessionID == "" {
		sessionID = spiritSessionID
	}
	now := time.Now()
	pbID := "pb_" + uuid.NewString()
	// 构建 v2 PlanStep 列表（每个 SubTask 对应一个 PlanStep）。
	// 2026-07-05 Step 3 修复：从 allocPlan.Allocations 填充 AgentKeys，
	// 匹配规则：alloc.SubTaskID == SubTask.ID（== PlanStep.ID）。
	// 2026-07-05 FIX-A：DAG 模式下 alloc.TeamMemberKeys 也必须并入 AgentKeys，
	// 否则下游 RealTeamOrchestrator 只能组装 lead 单 agent team，
	// 导致前端左侧 agent 列表与实际 team 成员数不一致。
	planSteps := make([]biz.PlanStep, 0, len(plan.SubTasks))
	for i, st := range plan.SubTasks {
		ps := biz.PlanStep{
			ID:          st.ID,
			PlanID:      pbID,
			TaskID:      rootTaskID,
			Label:       st.Name,
			Description: st.Description,
			DependsOn:   append([]string(nil), st.DependsOn...),
			Status:      biz.PlanStepStatusPending,
			StartedAt:   now,
			Seq:         int64(i + 1),
			Version:     1,
		}
		if allocPlan != nil {
			for _, alloc := range allocPlan.Allocations {
				if alloc.SubTaskID != st.ID {
					continue
				}
				if alloc.AssignedKey != "" {
					ps.AgentKeys = append(ps.AgentKeys, alloc.AssignedKey)
				}
				for _, mk := range alloc.TeamMemberKeys {
					if mk != "" {
						ps.AgentKeys = append(ps.AgentKeys, mk)
					}
				}
			}
		}
		planSteps = append(planSteps, ps)
	}
	// 派生 GraphStage ID（确定性，基于 PlanBoard ID）。
	gsID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v2:"+pbID)).String()
	// 构建 GraphNode 列表（每个 PlanStep 对应一个 GraphNode）。
	graphNodes := make([]biz.GraphNode, 0, len(planSteps))
	for _, ps := range planSteps {
		gn := biz.GraphNode{
			ID:           ps.ID,
			GraphStageID: gsID,
			Label:        ps.Label,
			DagNodeID:    ps.ID,
			Status:       biz.MapPlanStepToGraphNodeStatus(ps.Status),
			DependsOn:    append([]string(nil), ps.DependsOn...),
		}
		graphNodes = append(graphNodes, gn)
	}
	// 构建 PlanBoard 实体。
	pb := biz.PlanBoard{
		ID:        pbID,
		TaskID:    rootTaskID,
		TurnID:    "", // TurnID 在 OnTurnStart 时关联；此处不持有 turn 信息
		SessionID: spiritSessionID,
		Strategy:  mapV1StrategyToV2(plan.Strategy),
		Status:    biz.PlanStatusPlanning,
		Steps:     planSteps,
		StartedAt: now,
		Version:   1,
	}
	// 构建 GraphStage 实体（1:1 关联 PlanBoard）。
	gs := biz.GraphStage{
		ID:          gsID,
		TaskID:      rootTaskID,
		TurnID:      "",
		SessionID:   spiritSessionID,
		PlanBoardID: pbID,
		Nodes:       graphNodes,
		Status:      biz.GraphStageStatusRunning,
		StartedAt:   now,
		Version:     1,
	}
	// 发布 PlanBoardCreatedEvent（先于 PlanStep 事件，保证前端先创建 PlanBoard）。
	impl.seq.Publish(ctx, biz.NewPlanBoardCreatedEvent(pb))
	// 发布 GraphStageCreatedEvent（先于 GraphNode 事件，保证前端先创建 GraphStage）。
	impl.seq.Publish(ctx, biz.NewGraphStageCreatedEvent(gs))
	// 发布 PlanStepStartedEvent（status=pending，使用 Started 状态表示已创建待执行）。
	for _, ps := range planSteps {
		impl.seq.Publish(ctx, biz.NewPlanStepStartedEvent(ps, spiritSessionID))
	}
	// 发布 GraphNodeUpdatedEvent（每个节点初始状态=pending）。
	for _, gn := range graphNodes {
		impl.seq.Publish(ctx, biz.NewGraphNodeUpdatedEvent(gn, rootTaskID, spiritSessionID))
	}
	_ = sessionID // 暂未使用 chatSessionID 派生其他字段；保留供未来扩展
}

// mapV1StrategyToV2 将 v1 biz.OrchestrationStrategy 映射为 v2 biz.PlanStrategy。
func mapV1StrategyToV2(s biz.OrchestrationStrategy) biz.PlanStrategy {
	switch s {
	case biz.StrategyDirect:
		return biz.PlanStrategySequential
	case biz.StrategyParallel:
		return biz.PlanStrategyParallel
	case biz.StrategyDAG:
		return biz.PlanStrategyDAG
	default:
		return biz.PlanStrategySequential
	}
}
