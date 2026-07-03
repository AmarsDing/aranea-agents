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
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// agentAllocatorImpl implements biz.AgentAllocatorPort.
type agentAllocatorImpl struct {
	repo           biz.AllocationPlanRepository
	agentReader    biz.AgentReader
	perfRepo       biz.AgentPerformanceRepository
	capBuilder     *AgentCapabilityBuilder
	catalog        *biz.LlmProviderModelUsecase
	httpClient     *http.Client
	bus            biz.EventBus // Phase 3b-D: v2 EventBus
	lg             loggateway.Logger
	embedder       knowledge.Embedder
	agentFactory   biz.AgentFactory
	plannerSetting PlannerModelLookup
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

// Allocate matches each SubTask in the TaskPlan to the best Agent or Team.
func (impl *agentAllocatorImpl) Allocate(ctx context.Context, taskPlan *biz.TaskPlan) (*biz.AllocationPlan, error) {
	if taskPlan == nil {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "task plan is required")
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
		return nil, apierror.Internal(apierror.DomainSpirit, "build capabilities").WithCause(err)
	}

	// Match each subtask
	isDAG := taskPlan.Strategy == biz.StrategyDAG
	var allocations []biz.TaskAllocation
	for _, subTask := range taskPlan.SubTasks {
		allocation, err := impl.matchSubTask(ctx, subTask, capabilities, traceID)
		if err != nil {
			impl.lg.Warn("子任务匹配失败，尝试 AgentFactory",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("sub_task_id", subTask.ID),
				loggateway.Err(err),
			)
			// P1-4: 4-layer matching failed → try AgentFactory before fallback.
			if factoryAlloc, ok := impl.tryAgentFactoryForSubTask(ctx, subTask, traceID); ok {
				allocations = append(allocations, factoryAlloc)
				continue
			}
			// AgentFactory unavailable/failed → fallback to first available agent
			allocation = impl.fallbackAllocation(subTask, capabilities)
		}
		// For dag mode: each subtask becomes a multi-member team (≥2 members).
		// The primary agent (AssignedKey) is the team lead; selectAdditionalMembers
		// picks additional agents from the capability pool to fill out the team.
		if isDAG && allocation.AssignedKey != "" {
			additional := impl.selectAdditionalMembers(allocation.AssignedKey, capabilities, 1)
			if len(additional) > 0 {
				allocation.TeamMemberKeys = additional
				allocation.AssignedType = "team"
				impl.lg.Info("DAG 模式：为子任务分配多成员团队",
					loggateway.StepID(biz.SpiritStepAllocatorMatch),
					loggateway.Str("trace_id", traceID),
					loggateway.Str("sub_task_id", subTask.ID),
					loggateway.Str("lead", allocation.AssignedKey),
					loggateway.Str("members", strings.Join(additional, ",")),
				)
			}
		}
		allocations = append(allocations, allocation)
	}

	// If no subtasks (simple/moderate), allocate the whole plan to a single agent
	if len(taskPlan.SubTasks) == 0 {
		allocation, err := impl.matchWholePlan(ctx, taskPlan, capabilities, traceID)
		if err != nil {
			// P1-4: 4-layer matching failed → try AgentFactory before fallback.
			if factoryAlloc, ok := impl.tryAgentFactoryForPlan(ctx, taskPlan, traceID); ok {
				allocations = append(allocations, factoryAlloc)
			} else {
				allocation = impl.fallbackWholePlanAllocation(taskPlan, capabilities)
				allocations = append(allocations, allocation)
			}
		} else {
			allocations = append(allocations, allocation)
		}
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
		return nil, apierror.Internal(apierror.DomainSpirit, "persist allocation plan").WithCause(err)
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

	return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for subtask %s", subTask.ID)
}

// selectAdditionalMembers picks `count` additional agent keys from the
// capability pool, excluding the primary key. Used for dag mode where each
// team must have ≥2 members. The selection is capability-agnostic — it just
// picks the first available agents that aren't the primary. This is
// intentionally simple; smarter matching (e.g., complementary capabilities)
// can be added later.
func (impl *agentAllocatorImpl) selectAdditionalMembers(primaryKey string, capabilities []biz.AgentCapability, count int) []string {
	if count <= 0 || len(capabilities) <= 1 {
		return nil
	}
	result := make([]string, 0, count)
	for _, cap := range capabilities {
		if cap.AgentKey == "" || cap.AgentKey == primaryKey {
			continue
		}
		result = append(result, cap.AgentKey)
		if len(result) >= count {
			break
		}
	}
	return result
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
//
// When an embedder is configured, it uses in-memory embedding cosine similarity (the embedder is
// typically backed by OpenAI/Gemini/Ollama text-embedding APIs) for true semantic matching. When
// the embedder is nil or fails, it gracefully falls back to the existing TF-IDF keyword-based
// matching — no hard error is surfaced to the caller.
//
// Note: pgvector SQL similarity (`<=>` operator on the `vector_embeddings` table) is used by the
// memory domain (internal/data/vector/pgvector_fact.go) for memory retrieval, not by the allocator.
// The allocator computes cosine in Go memory to avoid a Postgres round-trip on the hot allocation path.
func (impl *agentAllocatorImpl) matchLayer2(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string) (biz.AgentCapability, float64, string) {
	if len(capabilities) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	// Try embedding-based matching first; fall back to TF-IDF on failure or nil embedder.
	if impl.embedder != nil {
		if cap, score, reason, ok := impl.matchLayer2Embedding(ctx, subTask, capabilities, traceID); ok {
			return cap, score, reason
		}
	}

	return impl.matchLayer2TFIDF(ctx, subTask, capabilities, traceID)
}

// matchLayer2Embedding uses embedding cosine similarity for semantic matching.
// Returns ok=false when the embedder fails or produces unusable vectors; the caller falls back to TF-IDF.
func (impl *agentAllocatorImpl) matchLayer2Embedding(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string) (biz.AgentCapability, float64, string, bool) {
	taskText := subTask.Name + " " + subTask.Description
	for _, cap := range subTask.RequiredCapabilities {
		taskText += " " + cap
	}

	// Collect non-Spirit candidates and their capability text for batch embedding.
	var agentCaps []biz.AgentCapability
	var agentTexts []string
	for _, cap := range capabilities {
		if cap.AgentKey == biz.SpiritAgentKey {
			continue
		}
		agentCaps = append(agentCaps, cap)
		agentTexts = append(agentTexts, buildAgentCapabilityText(cap))
	}
	if len(agentCaps) == 0 {
		return biz.AgentCapability{}, 0, "", false
	}

	// Batch embed: [taskText, agentText1, agentText2, ...].
	allTexts := append([]string{taskText}, agentTexts...)
	vectors, err := impl.embedder.Embed(ctx, allTexts)
	if err != nil {
		impl.lg.Warn("Layer 2 embedding 失败，降级为 TF-IDF",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return biz.AgentCapability{}, 0, "", false
	}
	if len(vectors) != len(allTexts) || len(vectors[0]) == 0 {
		impl.lg.Warn("Layer 2 embedding 维度不匹配，降级为 TF-IDF",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Int("expected", len(allTexts)),
			loggateway.Int("got", len(vectors)),
		)
		return biz.AgentCapability{}, 0, "", false
	}

	taskVec := vectors[0]

	type scored struct {
		cap   biz.AgentCapability
		score float64
	}

	var candidates []scored
	for i, cap := range agentCaps {
		sim := cosineSimilarity32(taskVec, vectors[i+1])
		if sim <= 0 {
			continue
		}
		// Blend with historical success rate (same weighting as TF-IDF path).
		score := sim
		if impl.perfRepo != nil {
			perf, err := impl.perfRepo.Get(ctx, cap.AgentKey, "general")
			if err == nil && perf != nil {
				score = score*0.6 + perf.SuccessRate*0.4
			}
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
func (impl *agentAllocatorImpl) matchLayer2TFIDF(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string) (biz.AgentCapability, float64, string) {
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
// This function powers the TF-IDF fallback path in matchLayer2TFIDF. The
// primary Layer 2 path now uses embedding cosine similarity via matchLayer2Embedding;
// this keyword scorer remains as the graceful-degradation fallback when the
// embedder is unavailable or fails.
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

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: fmt.Sprintf("Select the best agent for this subtask:\n\nName: %s\nDescription: %s\nRequired Capabilities: %v", subTask.Name, subTask.Description, subTask.RequiredCapabilities)},
	}

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	text, _, _, _, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)
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

	return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for plan")
}

// llmColdStartForPlan uses LLM to select an agent for a whole plan (no subtasks).
func (impl *agentAllocatorImpl) llmColdStartForPlan(ctx context.Context, taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability, traceID string) (string, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return "", apierror.Internal(apierror.DomainSpirit, "LLM catalog or HTTP client not configured")
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

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: fmt.Sprintf("Select the best agent for this task:\n\n%s", taskPlan.UserMessage)},
	}

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	text, _, _, _, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)
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
func (impl *agentAllocatorImpl) tryAgentFactoryForSubTask(ctx context.Context, subTask biz.SubTask, traceID string) (biz.TaskAllocation, bool) {
	if impl.agentFactory == nil {
		return biz.TaskAllocation{}, false
	}
	desc := strings.TrimSpace(subTask.Description)
	if desc == "" {
		desc = subTask.Name
	}
	profile := biz.TaskProfile{
		RequiredCapabilities: subTask.RequiredCapabilities,
		Domain:               "engineering",
		TaskDescription:      desc,
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
func (impl *agentAllocatorImpl) tryAgentFactoryForPlan(ctx context.Context, taskPlan *biz.TaskPlan, traceID string) (biz.TaskAllocation, bool) {
	if impl.agentFactory == nil {
		return biz.TaskAllocation{}, false
	}
	profile := biz.TaskProfile{
		RequiredCapabilities: extractCapabilityHints(taskPlan.UserMessage),
		Domain:               "engineering",
		TaskDescription:      taskPlan.UserMessage,
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
	meta := map[string]any{
		"allocation_id":     plan.ID,
		"task_plan_id":      plan.TaskPlanID,
		"spirit_session_id": spiritSessionID,
		"allocation_count":  len(plan.Allocations),
		"status":             string(plan.Status),
		"agent_key":          "agent-allocator",
	}
	impl.bus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "allocation_created", "", meta))
}
