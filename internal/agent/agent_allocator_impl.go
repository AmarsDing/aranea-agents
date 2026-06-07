package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// agentAllocatorImpl implements biz.AgentAllocatorPort.
type agentAllocatorImpl struct {
	repo        biz.AllocationPlanRepository
	agentReader biz.AgentReader
	perfRepo    biz.AgentPerformanceRepository
	capBuilder  *AgentCapabilityBuilder
	catalog     *biz.LlmProviderModelUsecase
	httpClient  *http.Client
	bus         contract.Bus
	lg          loggateway.Logger
}

var _ biz.AgentAllocatorPort = (*agentAllocatorImpl)(nil)

// NewAgentAllocator creates a new AgentAllocator implementation.
func NewAgentAllocator(
	repo biz.AllocationPlanRepository,
	agentReader biz.AgentReader,
	perfRepo biz.AgentPerformanceRepository,
	capBuilder *AgentCapabilityBuilder,
	catalog *biz.LlmProviderModelUsecase,
	httpClient *http.Client,
	bus contract.Bus,
	lg loggateway.Logger,
) biz.AgentAllocatorPort {
	return &agentAllocatorImpl{
		repo:        repo,
		agentReader: agentReader,
		perfRepo:    perfRepo,
		capBuilder:  capBuilder,
		catalog:     catalog,
		httpClient:  httpClient,
		bus:         bus,
		lg:          lg,
	}
}

// Allocate matches each SubTask in the TaskPlan to the best Agent or Team.
func (impl *agentAllocatorImpl) Allocate(ctx context.Context, taskPlan *biz.TaskPlan) (*biz.AllocationPlan, error) {
	if taskPlan == nil {
		return nil, kerrors.BadRequest("ALLOCATOR", "task plan is required")
	}

	traceID := taskPlan.TraceID
	if traceID == "" {
		traceID, _ = biz.SpiritTraceIDFromContext(ctx)
	}

	impl.lg.Info("AgentAllocator.Allocate 开始",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("task_plan_id", taskPlan.ID),
	)

	// Build agent capabilities from catalog
	capabilities, err := impl.capBuilder.BuildAll(ctx)
	if err != nil {
		impl.lg.Warn("构建 Agent 能力列表失败",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return nil, kerrors.InternalServer("ALLOCATOR", "build capabilities: "+err.Error())
	}

	// Match each subtask
	var allocations []biz.TaskAllocation
	for _, subTask := range taskPlan.SubTasks {
		allocation, err := impl.matchSubTask(ctx, subTask, capabilities, traceID)
		if err != nil {
			impl.lg.Warn("子任务匹配失败，使用降级策略",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("sub_task_id", subTask.ID),
				loggateway.Err(err),
			)
			// Fallback: assign to first available agent
			allocation = impl.fallbackAllocation(subTask, capabilities)
		}
		allocations = append(allocations, allocation)
	}

	// If no subtasks (simple/moderate), allocate the whole plan to a single agent
	if len(taskPlan.SubTasks) == 0 {
		allocation, err := impl.matchWholePlan(ctx, taskPlan, capabilities, traceID)
		if err != nil {
			allocation = impl.fallbackWholePlanAllocation(taskPlan, capabilities)
		}
		allocations = append(allocations, allocation)
	}

	impl.lg.Info("子任务匹配完成",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.Int("allocation_count", len(allocations)),
	)

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

	saved, err := impl.repo.Create(ctx, plan)
	if err != nil {
		impl.lg.Warn("AllocationPlan 持久化失败",
			loggateway.StepID(biz.SpiritStepAllocatorPersist),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return nil, kerrors.InternalServer("ALLOCATOR", "persist allocation plan: "+err.Error())
	}

	// Publish spirit_allocation_created event.
	impl.publishAllocationCreated(ctx, saved)

	return saved, nil
}

// GetAllocation retrieves an allocation plan by ID.
func (impl *agentAllocatorImpl) GetAllocation(ctx context.Context, allocationID string) (*biz.AllocationPlan, error) {
	return impl.repo.GetByID(ctx, allocationID)
}

// matchSubTask matches a single subtask to the best agent/team.
func (impl *agentAllocatorImpl) matchSubTask(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string) (biz.TaskAllocation, error) {
	// Determine assigned type based on estimated complexity
	assignedType := "agent"
	if subTask.EstimatedComplexity >= 0.5 {
		assignedType = "team"
	}

	// Priority: use AgentPerformance.GetBestForTaskType if performance data exists.
	if impl.perfRepo != nil && len(subTask.RequiredCapabilities) > 0 {
		taskType := subTask.RequiredCapabilities[0]
		bestPerfs, err := impl.perfRepo.GetBestForTaskType(ctx, taskType, 1)
		if err == nil && len(bestPerfs) > 0 {
			bestAgentKey := bestPerfs[0].AgentKey
			// Find the matching capability for display name
			for _, cap := range capabilities {
				if cap.AgentKey == bestAgentKey {
					impl.lg.Info("AgentPerformance.GetBestForTaskType 命中",
						loggateway.StepID(biz.SpiritStepAllocatorMatch),
						loggateway.Str("trace_id", traceID),
						loggateway.Str("sub_task_id", subTask.ID),
						loggateway.Str("agent_key", bestAgentKey),
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

	// Layer 2: Semantic match — keyword-based similarity between task and agent capabilities
	semCap, semScore, semReason := impl.matchLayer2(ctx, subTask, capabilities, traceID)
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

	// If LLM cold start also fails, use the best exact match (even if score < 0.5)
	if bestMatch.AgentKey != "" {
		return biz.TaskAllocation{
			SubTaskID:    subTask.ID,
			SubTaskName:  subTask.Name,
			AssignedType: assignedType,
			AssignedKey:  bestMatch.AgentKey,
			AssignedName: bestMatch.DisplayName,
			MatchScore:   bestScore,
			MatchLayer:   "exact",
			MatchReason:  matchReason + " (低分匹配)",
		}, nil
	}

	return biz.TaskAllocation{}, kerrors.NotFound("ALLOCATOR", "no agent found for subtask "+subTask.ID)
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
// Uses keyword-based TF-IDF-like scoring as a placeholder; TODO: integrate pgvector for true embedding similarity.
func (impl *agentAllocatorImpl) matchLayer2(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string) (biz.AgentCapability, float64, string) {
	if len(capabilities) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

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
		if cap.AgentKey == biz.SpiritAgentKey {
			continue
		}
		score := computeSemanticScore(taskText, cap)
		// Combine with historical success rate if available
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
		if cap.AgentKey == biz.SpiritAgentKey {
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
// TODO(embedding-upgrade): The project has an Embedder (internal/knowledge/embedder.go)
// supporting OpenAI/Ollama/Gemini/HuggingFace backends, but it is not wired into the
// allocator. To upgrade Layer 2 from TF-IDF to true embedding cosine similarity:
//  1. Add knowledge.BatchEmbedder as a dependency of agentAllocatorImpl
//  2. Persist agent capability vectors (pre-computed embeddings of role/domain/tool/skill text)
//  3. Replace this function with embedding cosine similarity via pgvector or in-memory comparison
//
// The existing Embedder is only available in the knowledge pipeline, not the agent allocation pipeline.
// Until wired, TF-IDF remains the Layer 2 strategy.
func computeSemanticScore(taskDesc string, cap biz.AgentCapability) float64 {
	// Build a text corpus from the agent's capability profile
	agentText := cap.DisplayName + " " + cap.Description
	for _, r := range cap.Roles {
		agentText += " " + r
	}
	for _, d := range cap.Domains {
		agentText += " " + d
	}
	for _, t := range cap.Tools {
		agentText += " " + t
	}
	for _, s := range cap.Skills {
		agentText += " " + s
	}

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

// llmColdStart performs Layer 3 matching: use LLM to select the best agent.
func (impl *agentAllocatorImpl) llmColdStart(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string) (string, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return "", kerrors.InternalServer("ALLOCATOR", "LLM catalog or HTTP client not configured")
	}

	prompt := buildAllocatorColdStartPrompt(subTask, capabilities)

	provider, model := resolvePlannerProviderModel()
	if provider == "" || model == "" {
		provider, model = resolveFallbackProviderModelFromCatalog(ctx, impl.catalog, impl.lg, "allocator.fallback_model", "AgentAllocator")
	}
	if provider == "" || model == "" {
		return "", kerrors.InternalServer("ALLOCATOR", "no provider/model configured for agent allocation (set ARANEA_PLANNER_PROVIDER/ARANEA_PLANNER_MODEL env vars or add models in system settings)")
	}

	row, err := impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return "", kerrors.InternalServer("ALLOCATOR", "get provider config: "+err.Error())
	}

	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: fmt.Sprintf("Select the best agent for this subtask:\n\nName: %s\nDescription: %s\nRequired Capabilities: %v", subTask.Name, subTask.Description, subTask.RequiredCapabilities)},
	}

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	text, _, _, _, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)
	if err != nil {
		return "", kerrors.InternalServer("ALLOCATOR", "LLM call failed: "+err.Error())
	}

	// Parse the agent_key from the response
	return parseAllocatorColdStartResponse(text), nil
}

// matchWholePlan handles allocation for plans without subtasks (simple/moderate).
func (impl *agentAllocatorImpl) matchWholePlan(ctx context.Context, taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability, traceID string) (biz.TaskAllocation, error) {
	// For simple/moderate plans, use the strategy to determine assignment
	assignedType := "agent"
	if taskPlan.Strategy == biz.StrategyCoordinator || taskPlan.Strategy == biz.StrategyParallel || taskPlan.Strategy == biz.StrategyDAG {
		assignedType = "team"
	}

	// Priority: use AgentPerformance.GetBestForTaskType if performance data exists.
	if impl.perfRepo != nil {
		capHints := extractCapabilityHints(taskPlan.UserMessage)
		if len(capHints) > 0 {
			bestPerfs, err := impl.perfRepo.GetBestForTaskType(ctx, capHints[0], 1)
			if err == nil && len(bestPerfs) > 0 {
				bestAgentKey := bestPerfs[0].AgentKey
				for _, cap := range capabilities {
					if cap.AgentKey == bestAgentKey {
						impl.lg.Info("AgentPerformance.GetBestForTaskType 命中 (whole plan)",
							loggateway.StepID(biz.SpiritStepAllocatorMatch),
							loggateway.Str("trace_id", traceID),
							loggateway.Str("agent_key", bestAgentKey),
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

	return biz.TaskAllocation{}, kerrors.NotFound("ALLOCATOR", "no agent found for plan")
}

// llmColdStartForPlan uses LLM to select an agent for a whole plan (no subtasks).
func (impl *agentAllocatorImpl) llmColdStartForPlan(ctx context.Context, taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability, traceID string) (string, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return "", kerrors.InternalServer("ALLOCATOR", "LLM catalog or HTTP client not configured")
	}

	prompt := buildAllocatorColdStartPromptForPlan(taskPlan, capabilities)

	provider, model := resolvePlannerProviderModel()
	if provider == "" || model == "" {
		provider, model = resolveFallbackProviderModelFromCatalog(ctx, impl.catalog, impl.lg, "allocator.fallback_model", "AgentAllocator")
	}
	if provider == "" || model == "" {
		return "", kerrors.InternalServer("ALLOCATOR", "no provider/model configured for agent allocation (set ARANEA_PLANNER_PROVIDER/ARANEA_PLANNER_MODEL env vars or add models in system settings)")
	}

	row, err := impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return "", kerrors.InternalServer("ALLOCATOR", "get provider config: "+err.Error())
	}

	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: fmt.Sprintf("Select the best agent for this task:\n\n%s", taskPlan.UserMessage)},
	}

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	text, _, _, _, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)
	if err != nil {
		return "", kerrors.InternalServer("ALLOCATOR", "LLM call failed: "+err.Error())
	}

	return parseAllocatorColdStartResponse(text), nil
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
		line := fmt.Sprintf("- agent_key: %s, display_name: %s, roles: %v, description: %s",
			cap.AgentKey, cap.DisplayName, cap.Roles, cap.Description)
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

	env := contract.NewEnvelope(contract.EnvelopeTypeSpiritAllocationCreated, "agent-allocator", spiritSessionID)
	env.Metadata = map[string]any{
		"allocation_id":     plan.ID,
		"task_plan_id":      plan.TaskPlanID,
		"spirit_session_id": spiritSessionID,
		"allocation_count":  len(plan.Allocations),
		"status":            string(plan.Status),
	}
	impl.bus.Publish(ctx, env)
}
