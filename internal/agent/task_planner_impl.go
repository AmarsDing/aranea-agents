package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// minSubTasksForParallel is the minimum number of subtasks to consider
// parallel or coordinator strategies instead of single-agent execution.
const minSubTasksForParallel = 3

// taskPlannerImpl implements biz.TaskPlannerPort.
type taskPlannerImpl struct {
	repo       biz.TaskPlanRepository
	catalog    *biz.LlmProviderModelUsecase
	httpClient *http.Client
	bus        contract.Bus
	orchCache  *biz.OrchestrationCache
	lg         loggateway.Logger
}

var _ biz.TaskPlannerPort = (*taskPlannerImpl)(nil)

// NewTaskPlanner creates a new TaskPlanner implementation.
func NewTaskPlanner(repo biz.TaskPlanRepository, catalog *biz.LlmProviderModelUsecase, httpClient *http.Client, bus contract.Bus, orchCache *biz.OrchestrationCache, lg loggateway.Logger) biz.TaskPlannerPort {
	return &taskPlannerImpl{
		repo:       repo,
		catalog:    catalog,
		httpClient: httpClient,
		bus:        bus,
		orchCache:  orchCache,
		lg:         lg,
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

		// Reuse historical topology as strategy hint
		strategy := biz.StrategyCoordinator
		topologyHint := biz.TopologyType(memoryHit.TopologyUsed)
		switch topologyHint {
		case biz.TopologyDirect:
			strategy = biz.StrategyDirect
		case biz.TopologyParallel:
			strategy = biz.StrategyParallel
		case biz.TopologyCoordinator:
			strategy = biz.StrategyCoordinator
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
			Status:          biz.PlanStatusDraft,
		}

		saved, err := impl.repo.Create(ctx, plan)
		if err != nil {
			impl.lg.Warn("TaskPlan 持久化失败",
				loggateway.StepID(biz.SpiritStepPlannerPersist),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
			return nil, kerrors.InternalServer("TASK_PLANNER", "persist plan: "+err.Error())
		}
		impl.publishPlanCreated(ctx, saved)
		return saved, nil
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

	// Step 3: Decompose task (only for complex)
	var subTasks []biz.SubTask
	var dag *biz.PlanTaskDAG
	var decomposeReason string
	if complexityLevel == biz.ComplexityComplex {
		var err error
		subTasks, dag, err = impl.decomposeTask(ctx, input.UserMessage, input.IntentArtifact)
		if err != nil {
			impl.lg.Warn("任务分解失败，降级为 coordinator 策略",
				loggateway.StepID(biz.SpiritStepPlannerDecompose),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
			strategy = biz.StrategyCoordinator
			strategyReason = "任务分解失败，降级为 coordinator"
			decomposeReason = "decompose_failed"
		} else if len(subTasks) > 0 {
			decomposeReason = fmt.Sprintf("分解为 %d 个子任务", len(subTasks))
			// Refine strategy based on DAG structure
			if dag != nil {
				if len(dag.RootIDs) == len(dag.Nodes) {
					// All independent
					if len(subTasks) >= minSubTasksForParallel {
						strategy = biz.StrategyParallel
						strategyReason = fmt.Sprintf("基于 DAG 分析: %d 个独立子任务，选择并行策略", len(subTasks))
					}
				} else if hasDependencies(dag) {
					strategy = biz.StrategyDAG
					strategyReason = fmt.Sprintf("基于 DAG 分析: %d 个子任务存在依赖关系，选择 DAG 策略", len(subTasks))
				}
				// Only fall back to coordinator when DAG has no clear structure
				if strategy != biz.StrategyParallel && strategy != biz.StrategyDAG && len(subTasks) >= minSubTasksForParallel {
					strategy = biz.StrategyCoordinator
					strategyReason = fmt.Sprintf("基于 DAG 分析: %d 个子任务需要协调，选择 coordinator 策略", len(subTasks))
				}
			}
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
		Status:             biz.PlanStatusDraft,
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
		return nil, kerrors.InternalServer("TASK_PLANNER", "persist plan: "+err.Error())
	}

	// Publish spirit_plan_created event.
	impl.publishPlanCreated(ctx, saved)

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

// ConfirmPlan applies adjustments and confirms the plan.
func (impl *taskPlannerImpl) ConfirmPlan(ctx context.Context, planID string, adjustments biz.PlanAdjustments) (*biz.TaskPlan, error) {
	plan, err := impl.repo.GetByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan.Status != biz.PlanStatusDraft {
		return nil, kerrors.BadRequest("TASK_PLANNER", "plan is not in draft status")
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

	plan.Status = biz.PlanStatusConfirmed

	impl.lg.Info("TaskPlan 确认",
		loggateway.StepID(biz.SpiritStepPlannerConfirm),
		loggateway.Str("plan_id", planID),
		loggateway.Str("strategy", string(plan.Strategy)),
	)

	saved, err := impl.repo.Update(ctx, plan)
	if err != nil {
		return nil, kerrors.InternalServer("TASK_PLANNER", "update plan: "+err.Error())
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

	// Semantic: from IntentArtifact.ComplexityScore (weight 0.25)
	if input.IntentArtifact != nil && input.IntentArtifact.ComplexityScore > 0 {
		dims.Semantic = input.IntentArtifact.ComplexityScore
	} else {
		// Fallback: estimate from message length
		runeCount := len([]rune(input.UserMessage))
		if runeCount > 500 {
			dims.Semantic = 0.7
		} else if runeCount > 200 {
			dims.Semantic = 0.4
		} else {
			dims.Semantic = 0.2
		}
	}

	// Structural: check if user message contains multiple questions/tasks (weight 0.15)
	dims.Structural = assessStructural(input.UserMessage)

	// Domain: count distinct domain signals from ComplexitySignals (weight 0.15)
	dims.Domain = assessDomain(input.IntentArtifact)

	// Tool: check if suggested_agents or tools are mentioned (weight 0.10)
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

// assessDomain evaluates domain complexity.
func assessDomain(artifact *biz.IntentArtifact) float64 {
	if artifact == nil || len(artifact.ComplexitySignals) == 0 {
		return 0.1
	}
	// Count distinct domain signals
	domainSignals := 0
	for _, sig := range artifact.ComplexitySignals {
		switch sig {
		case "multi_domain", "requires_database", "needs_research":
			domainSignals++
		}
	}
	if domainSignals >= 2 {
		return 0.8
	} else if domainSignals >= 1 {
		return 0.5
	}
	return 0.2
}

// assessTool evaluates tool complexity.
func assessTool(artifact *biz.IntentArtifact) float64 {
	if artifact == nil {
		return 0.1
	}
	agentCount := len(artifact.SuggestedAgents)
	if agentCount >= 3 {
		return 0.8
	} else if agentCount >= 2 {
		return 0.5
	} else if agentCount >= 1 {
		return 0.3
	}
	return 0.1
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

// determineStrategy selects the orchestration strategy based on complexity.
func (impl *taskPlannerImpl) determineStrategy(level biz.ComplexityLevel, score float64, input biz.PlanInput) (biz.OrchestrationStrategy, string, biz.TopologyType) {
	switch level {
	case biz.ComplexitySimple:
		return biz.StrategyDirect, "简单任务，Spirit 直接回答", biz.TopologyDirect
	case biz.ComplexityModerate:
		return biz.StrategySingleAgent, "中等复杂度，使用 Agent-as-Tool", biz.TopologyCoordinator
	case biz.ComplexityComplex:
		// Check memory hit for historical topology
		if input.HistoryDQScore > 0.7 && input.IntentArtifact != nil && input.IntentArtifact.SuggestedTopology != "" {
			topology := biz.TopologyType(input.IntentArtifact.SuggestedTopology)
			return biz.StrategyCoordinator, "基于历史编排缓存推荐策略", topology
		}
		// Use intent artifact's suggested topology if available
		if input.IntentArtifact != nil && input.IntentArtifact.SuggestedTopology != "" {
			topology := biz.TopologyType(input.IntentArtifact.SuggestedTopology)
			switch topology {
			case biz.TopologyParallel:
				return biz.StrategyParallel, "基于意图分析推荐并行策略", topology
			case "dag":
				return biz.StrategyDAG, "基于意图分析推荐 DAG 策略", topology
			default:
				return biz.StrategyCoordinator, "基于意图分析推荐协调策略", topology
			}
		}
		return biz.StrategyCoordinator, "复杂任务，默认使用 coordinator 策略", biz.TopologyCoordinator
	default:
		return biz.StrategyDirect, "未知复杂度，默认直接回答", biz.TopologyDirect
	}
}

// decomposeTask uses LLM to decompose a complex task into subtasks (T1.6).
func (impl *taskPlannerImpl) decomposeTask(ctx context.Context, userMessage string, artifact *biz.IntentArtifact) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return nil, nil, kerrors.InternalServer("TASK_PLANNER", "LLM catalog or HTTP client not configured")
	}

	prompt := buildDecompositionPrompt(userMessage, artifact)

	// Use the default provider/model from catalog (same pattern as intent pass)
	provider, model := resolvePlannerProviderModel()
	if provider == "" || model == "" {
		// Fallback: use the first available model from catalog
		provider, model = resolveFallbackProviderModelFromCatalog(ctx, impl.catalog, impl.lg, biz.SpiritStepPlannerAssess, "TaskPlanner")
	}
	if provider == "" || model == "" {
		return nil, nil, kerrors.InternalServer("TASK_PLANNER", "no provider/model configured for task decomposition (set ARANEA_PLANNER_PROVIDER/ARANEA_PLANNER_MODEL env vars or add models in system settings)")
	}

	row, err := impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return nil, nil, kerrors.InternalServer("TASK_PLANNER", "get provider config: "+err.Error())
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
		return nil, nil, kerrors.InternalServer("TASK_PLANNER", "LLM call failed: "+err.Error())
	}

	text = stripDecompositionFences(text)
	subTasks, err := parseDecompositionOutput(text)
	if err != nil {
		return nil, nil, kerrors.InternalServer("TASK_PLANNER", "parse decomposition: "+err.Error())
	}

	if len(subTasks) == 0 {
		return nil, nil, nil
	}

	// Validate: no cycles, all depends_on references exist
	if err := validateSubTaskDAG(subTasks); err != nil {
		return nil, nil, kerrors.InternalServer("TASK_PLANNER", "invalid DAG: "+err.Error())
	}

	dag := buildDAGFromSubTasks(subTasks)
	return subTasks, dag, nil
}

// buildDecompositionPrompt creates the system prompt for task decomposition.
func buildDecompositionPrompt(userMessage string, artifact *biz.IntentArtifact) string {
	intentContext := ""
	if artifact != nil {
		intentContext = fmt.Sprintf("\nIntent analysis:\n- Refined goal: %s\n- Intent kind: %s\n- Complexity signals: %v\n- Suggested agents: %v",
			artifact.RefinedGoal,
			artifact.IntentKind,
			artifact.ComplexitySignals,
			artifact.SuggestedAgents,
		)
	}

	return `You are a task decomposition specialist. Break down complex tasks into 2-6 subtasks.

Rules:
- Each subtask must have: id (st_1, st_2, etc.), name, description, depends_on (array of other subtask IDs), required_capabilities (from the predefined list), priority (1-5, 1=highest), estimated_complexity (0.0-1.0)
- Output ONLY a JSON array, no markdown fences, no commentary
- required_capabilities must use these predefined tags: go-backend, go-kratos, vue3-frontend, quasar-ui, devops, database, architecture, testing, security, research, documentation, api-design
- depends_on must only reference IDs of other subtasks in the array
- No circular dependencies allowed
- Subtasks should be independently executable where possible` + intentContext
}

// resolvePlannerProviderModel returns the provider and model for task decomposition.
// Uses environment variables or defaults to the same model as intent pass.
func resolvePlannerProviderModel() (string, string) {
	// TODO: Make configurable via system settings
	// For now, use environment variables with sensible defaults
	provider := strings.TrimSpace(getEnvOrDefault("ARANEA_PLANNER_PROVIDER", ""))
	model := strings.TrimSpace(getEnvOrDefault("ARANEA_PLANNER_MODEL", ""))
	return provider, model
}

func getEnvOrDefault(key, defaultVal string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	return v
}

// resolveFallbackProviderModelFromCatalog attempts to find the first available
// provider/model from the catalog when environment variables are not configured.
// Shared by TaskPlanner and AgentAllocator.
func resolveFallbackProviderModelFromCatalog(ctx context.Context, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger, stepID, component string) (string, string) {
	if catalog == nil {
		return "", ""
	}
	models, err := catalog.List(ctx)
	if err != nil || len(models) == 0 {
		return "", ""
	}
	for _, m := range models {
		if m.Provider != "" && m.Model != "" {
			lg.Info(component+": using fallback provider/model from catalog",
				loggateway.StepID(stepID),
				loggateway.Str("provider", m.Provider),
				loggateway.Str("model", m.Model),
			)
			return m.Provider, m.Model
		}
	}
	return "", ""
}

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
		return nil, kerrors.InternalServer("TASK_PLANNER", "json unmarshal: "+err.Error())
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
		subTasks = append(subTasks, biz.SubTask{
			ID:                   rt.ID,
			Name:                 rt.Name,
			Description:          rt.Description,
			DependsOn:            rt.DependsOn,
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
				return kerrors.BadRequest("TASK_PLANNER", fmt.Sprintf("subtask %s depends on non-existent subtask %s", t.ID, depID))
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
					return kerrors.BadRequest("TASK_PLANNER", fmt.Sprintf("cycle detected: %s → %s", id, depID))
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

// hasDependencies checks if the DAG has any dependency edges.
func hasDependencies(dag *biz.PlanTaskDAG) bool {
	if dag == nil {
		return false
	}
	for _, node := range dag.Nodes {
		if len(node.DependsOn) > 0 {
			return true
		}
	}
	return false
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
// For dual consumption (REQ-SO-04), also publishes spirit_team_assembled (old equivalent).
func (impl *taskPlannerImpl) publishPlanCreated(ctx context.Context, plan *biz.TaskPlan) {
	if impl.bus == nil || plan == nil {
		return
	}
	spiritSessionID := plan.SpiritSessionID

	// New event: spirit_plan_created
	env := contract.NewEnvelope(contract.EnvelopeTypeSpiritPlanCreated, "task-planner", spiritSessionID)
	env.Metadata = map[string]any{
		"plan_id":           plan.ID,
		"spirit_session_id": spiritSessionID,
		"complexity_level":  string(plan.ComplexityLevel),
		"complexity_score":  plan.ComplexityScore,
		"strategy":          string(plan.Strategy),
		"strategy_reason":   plan.StrategyReason,
		"topology_hint":     string(plan.TopologyHint),
		"subtask_count":     len(plan.SubTasks),
	}
	impl.bus.Publish(ctx, env)

	// Dual consumption: also publish spirit_team_assembled (old equivalent)
	// so that the existing frontend continues to work during migration.
	dualEnv := contract.NewEnvelope(contract.EnvelopeTypeSpiritTeamAssembled, "task-planner", spiritSessionID)
	dualEnv.Metadata = map[string]any{
		"team_id":           plan.ID,
		"team_name":         "Task Plan: " + biz.TruncateRunes(plan.UserMessage, 50),
		"task_summary":      biz.TruncateRunes(plan.UserMessage, 200),
		"mode":              string(plan.Strategy),
		"total_steps":       len(plan.SubTasks),
		"spirit_session_id": spiritSessionID,
	}
	impl.bus.Publish(ctx, dualEnv)
}
