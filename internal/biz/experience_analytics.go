package biz

import (
	"context"
	"math"
	"sort"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── Result types ──────────────────────────────────────────────────────────────

// ToolWeightAnalysis is the result of AnalyzeToolWeights.
type ToolWeightAnalysis struct {
	AgentID string
	Items   []ToolWeightItem
}

// ToolWeightItem represents one tool's weight analysis.
type ToolWeightItem struct {
	ToolKey        string
	CallCount      int
	SuccessCount   int
	SuccessRate    float64
	AvgDurationMS  float64
	WeightScore    float64
	Recommendation string

	// internal normalization fields
	normSR     float64
	normCount  float64
	normInvDur float64
}

// SkillHealthAnalysis is the result of AnalyzeSkillHealth.
type SkillHealthAnalysis struct {
	AgentID string
	Items   []SkillHealthItem
}

// SkillHealthItem represents one skill's health analysis.
type SkillHealthItem struct {
	SkillID        string
	SkillName      string
	InvokeCount    int
	SuccessCount   int
	FailureCount   int
	SuccessRate    float64
	AvgDurationMS  float64
	HealthStatus   string
	Recommendation string
}

// OrchestrationAnalysis is the result of AnalyzeOrchestration.
type OrchestrationAnalysis struct {
	AgentID string
	Items   []OrchestrationModeItem
}

// OrchestrationModeItem represents one orchestration mode's analysis.
type OrchestrationModeItem struct {
	Mode         string
	RunCount     int
	SuccessCount int
	SuccessRate  float64
	DQScore      float64
	Validity     float64
	Specificity  float64
	Correctness  float64
}

// MemoryQualityAnalysis is the result of AnalyzeMemoryQuality.
type MemoryQualityAnalysis struct {
	AgentID              string
	FactCount            int
	RetrievalQuality     float64
	NegativeFeedback     int
	HealthScore          float64
	Recommendation       string
}

// AgentCapabilityAnalysis is the combined result of AnalyzeAgentCapability.
type AgentCapabilityAnalysis struct {
	AgentID       string
	ToolWeights   ToolWeightAnalysis
	SkillHealth   SkillHealthAnalysis
	Orchestration OrchestrationAnalysis
	MemoryQuality MemoryQualityAnalysis
	CostSummary   CostSummary
}

// CostSummary is a lightweight cost overview for an agent.
type CostSummary struct {
	TotalCostMicroUSD int64
	TotalTokens       int
	CallCount         int
}

// ── Usecase ───────────────────────────────────────────────────────────────────

// ExperienceAnalyticsUsecase provides data-driven analysis for memory and skills butlers.
type ExperienceAnalyticsUsecase struct {
	metricsRepo  EvolutionMetricsRepo
	skillRepo    SkillQueryReader
	teamReader   TeamReader
	runReader    TeamRunReader
	usageRepo    UsageAnalyticsRepo
	memoryAdmin  *MemoryAdminUsecase
	sessionRepo  SessionReader
	toolInvData  ToolInvocationReader
	lg           loggateway.Logger
}

// NewExperienceAnalyticsUsecase creates a new ExperienceAnalyticsUsecase.
func NewExperienceAnalyticsUsecase(
	metricsRepo EvolutionMetricsRepo,
	skillRepo SkillQueryReader,
	teamReader TeamReader,
	runReader TeamRunReader,
	usageRepo UsageAnalyticsRepo,
	memoryAdmin *MemoryAdminUsecase,
	sessionRepo SessionReader,
	toolInvData ToolInvocationReader,
	lg loggateway.Logger,
) *ExperienceAnalyticsUsecase {
	return &ExperienceAnalyticsUsecase{
		metricsRepo:  metricsRepo,
		skillRepo:    skillRepo,
		teamReader:   teamReader,
		runReader:    runReader,
		usageRepo:    usageRepo,
		memoryAdmin:  memoryAdmin,
		sessionRepo:  sessionRepo,
		toolInvData:  toolInvData,
		lg:           lg,
	}
}

// ── AnalyzeToolWeights ────────────────────────────────────────────────────────

// AnalyzeToolWeights queries tool invocations for an agent, aggregates by tool_key,
// and computes a weight_score = normalize(success_rate)*0.5 + normalize(call_count)*0.3 + normalize(1/duration)*0.2.
func (uc *ExperienceAnalyticsUsecase) AnalyzeToolWeights(ctx context.Context, agentID string, since time.Time) (ToolWeightAnalysis, error) {
	agentID, err := requireNonEmpty(agentID, "XP_ANALYTICS", "agent_id")
	if err != nil {
		return ToolWeightAnalysis{}, err
	}

	fromStr := since.UTC().Format(time.RFC3339)
	result, err := uc.toolInvData.SearchToolInvocations(ctx, ToolRunQuery{
		AgentID: agentID,
		From:    fromStr,
		Limit:   500,
	})
	if err != nil {
		return ToolWeightAnalysis{}, err
	}

	agg := make(map[string]*toolAgg)
	for _, inv := range result.Items {
		a, ok := agg[inv.ToolKey]
		if !ok {
			a = &toolAgg{ToolKey: inv.ToolKey}
			agg[inv.ToolKey] = a
		}
		a.CallCount++
		a.TotalDurationMS += inv.DurationMS
		if inv.Status == "success" {
			a.SuccessCount++
		}
	}

	items := make([]ToolWeightItem, 0, len(agg))
	for _, a := range agg {
		sr := float64(0)
		if a.CallCount > 0 {
			sr = float64(a.SuccessCount) / float64(a.CallCount)
		}
		avgDur := float64(0)
		if a.CallCount > 0 {
			avgDur = float64(a.TotalDurationMS) / float64(a.CallCount)
		}
		items = append(items, ToolWeightItem{
			ToolKey:      a.ToolKey,
			CallCount:    a.CallCount,
			SuccessCount: a.SuccessCount,
			SuccessRate:  sr,
			AvgDurationMS: avgDur,
		})
	}

	normalizeToolWeights(items)

	for i := range items {
		items[i].WeightScore = items[i].normSR*0.5 + items[i].normCount*0.3 + items[i].normInvDur*0.2
		items[i].Recommendation = toolWeightRecommendation(items[i].WeightScore, items[i].SuccessRate)
	}

	sort.Slice(items, func(i, j int) bool { return items[i].WeightScore > items[j].WeightScore })

	return ToolWeightAnalysis{AgentID: agentID, Items: items}, nil
}

type toolAgg struct {
	ToolKey        string
	CallCount      int
	SuccessCount   int
	TotalDurationMS int
}

func normalizeToolWeights(items []ToolWeightItem) {
	if len(items) == 0 {
		return
	}
	var maxCount, maxInvDur float64
	for _, it := range items {
		if float64(it.CallCount) > maxCount {
			maxCount = float64(it.CallCount)
		}
		invDur := safeInvDuration(it.AvgDurationMS)
		if invDur > maxInvDur {
			maxInvDur = invDur
		}
	}
	for i := range items {
		if maxCount > 0 {
			items[i].normCount = float64(items[i].CallCount) / maxCount
		}
		if maxInvDur > 0 {
			items[i].normInvDur = safeInvDuration(items[i].AvgDurationMS) / maxInvDur
		}
		items[i].normSR = items[i].SuccessRate
	}
}

func safeInvDuration(avgMS float64) float64 {
	if avgMS <= 0 {
		return 0
	}
	return 1.0 / avgMS
}

func toolWeightRecommendation(score, successRate float64) string {
	switch {
	case score >= 0.7 && successRate >= 0.9:
		return "keep"
	case score >= 0.5:
		return "monitor"
	case successRate < 0.5:
		return "disable"
	default:
		return "review"
	}
}

// ── AnalyzeSkillHealth ────────────────────────────────────────────────────────

// AnalyzeSkillHealth queries skill invocations for an agent, aggregates by skill,
// and computes health status based on success rate and usage patterns.
func (uc *ExperienceAnalyticsUsecase) AnalyzeSkillHealth(ctx context.Context, agentID string, since time.Time) (SkillHealthAnalysis, error) {
	agentID, err := requireNonEmpty(agentID, "XP_ANALYTICS", "agent_id")
	if err != nil {
		return SkillHealthAnalysis{}, err
	}

	fromStr := since.UTC().Format(time.RFC3339)
	result, err := uc.skillRepo.SearchSkillInvocations(ctx, SkillRunQuery{
		AgentID: agentID,
		From:    fromStr,
		Limit:   500,
	})
	if err != nil {
		return SkillHealthAnalysis{}, err
	}

	agg := make(map[string]*skillAgg)
	for _, inv := range result.Items {
		a, ok := agg[inv.SkillID]
		if !ok {
			a = &skillAgg{SkillID: inv.SkillID, SkillName: inv.SkillName}
			agg[inv.SkillID] = a
		}
		a.InvokeCount++
		a.TotalDurationMS += inv.DurationMS
		if inv.Status == "success" {
			a.SuccessCount++
		} else if inv.Status == "failure" {
			a.FailureCount++
		}
	}

	items := make([]SkillHealthItem, 0, len(agg))
	for _, a := range agg {
		sr := float64(0)
		if a.InvokeCount > 0 {
			sr = float64(a.SuccessCount) / float64(a.InvokeCount)
		}
		avgDur := float64(0)
		if a.InvokeCount > 0 {
			avgDur = float64(a.TotalDurationMS) / float64(a.InvokeCount)
		}
		items = append(items, SkillHealthItem{
			SkillID:       a.SkillID,
			SkillName:     a.SkillName,
			InvokeCount:   a.InvokeCount,
			SuccessCount:  a.SuccessCount,
			FailureCount:  a.FailureCount,
			SuccessRate:   sr,
			AvgDurationMS: avgDur,
		})
	}

	for i := range items {
		items[i].HealthStatus, items[i].Recommendation = skillHealthStatus(items[i].SuccessRate, items[i].InvokeCount)
	}

	sort.Slice(items, func(i, j int) bool { return items[i].SuccessRate < items[j].SuccessRate })

	return SkillHealthAnalysis{AgentID: agentID, Items: items}, nil
}

type skillAgg struct {
	SkillID        string
	SkillName      string
	InvokeCount    int
	SuccessCount   int
	FailureCount   int
	TotalDurationMS int
}

func skillHealthStatus(successRate float64, invokeCount int) (status, recommendation string) {
	switch {
	case invokeCount == 0:
		return "unused", "consider_removing"
	case successRate >= 0.9:
		return "healthy", "keep"
	case successRate >= 0.7:
		return "degraded", "review_errors"
	case successRate >= 0.5:
		return "unstable", "investigate_failures"
	default:
		return "critical", "disable_or_rewrite"
	}
}

// ── AnalyzeOrchestration ──────────────────────────────────────────────────────

// AnalyzeOrchestration queries team runs, aggregates by mode, and computes
// DQ score = 0.4*Validity + 0.3*Specificity + 0.3*Correctness.
func (uc *ExperienceAnalyticsUsecase) AnalyzeOrchestration(ctx context.Context, agentID string, since time.Time) (OrchestrationAnalysis, error) {
	agentID, err := requireNonEmpty(agentID, "XP_ANALYTICS", "agent_id")
	if err != nil {
		return OrchestrationAnalysis{}, err
	}

	teams, err := uc.teamReader.ListTeams(ctx)
	if err != nil {
		uc.lg.Warn("list teams for orchestration analysis", loggateway.StepID("xp_analytics.orchestration"), loggateway.Err(err))
		return OrchestrationAnalysis{AgentID: agentID}, nil
	}

	agg := make(map[string]*orchAgg)
	for _, team := range teams {
		runs, runErr := uc.runReader.ListTeamRuns(ctx, team.ID, 100)
		if runErr != nil {
			uc.lg.Warn("list team runs", loggateway.StepID("xp_analytics.orchestration"), loggateway.Err(runErr))
			continue
		}
		for _, run := range runs {
			if run.StartedAt != "" {
				if t, pErr := time.Parse(time.RFC3339, run.StartedAt); pErr == nil && t.Before(since) {
					continue
				}
			}
			mode := run.Mode
			if mode == "" {
				mode = "unknown"
			}
			a, ok := agg[mode]
			if !ok {
				a = &orchAgg{Mode: mode}
				agg[mode] = a
			}
			a.RunCount++
			if run.Status == "completed" {
				a.SuccessCount++
			}
			if run.ErrorMessage != "" {
				a.ErrorCount++
			}
			a.TotalDurationMS += run.DurationMS
		}
	}

	items := make([]OrchestrationModeItem, 0, len(agg))
	for _, a := range agg {
		sr := float64(0)
		if a.RunCount > 0 {
			sr = float64(a.SuccessCount) / float64(a.RunCount)
		}
		validity := sr
		specificity := orchSpecificity(a)
		correctness := orchCorrectness(a)
		dq := 0.4*validity + 0.3*specificity + 0.3*correctness
		items = append(items, OrchestrationModeItem{
			Mode:         a.Mode,
			RunCount:     a.RunCount,
			SuccessCount: a.SuccessCount,
			SuccessRate:  sr,
			DQScore:      math.Round(dq*1000) / 1000,
			Validity:     math.Round(validity*1000) / 1000,
			Specificity:  math.Round(specificity*1000) / 1000,
			Correctness:  math.Round(correctness*1000) / 1000,
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].DQScore > items[j].DQScore })

	return OrchestrationAnalysis{AgentID: agentID, Items: items}, nil
}

type orchAgg struct {
	Mode           string
	RunCount       int
	SuccessCount   int
	ErrorCount     int
	TotalDurationMS int
}

func orchSpecificity(a *orchAgg) float64 {
	if a.RunCount == 0 {
		return 0
	}
	return float64(a.RunCount-a.ErrorCount) / float64(a.RunCount)
}

func orchCorrectness(a *orchAgg) float64 {
	if a.RunCount == 0 {
		return 0
	}
	return float64(a.SuccessCount) / float64(a.RunCount)
}

// ── AnalyzeMemoryQuality ─────────────────────────────────────────────────────

// AnalyzeMemoryQuality uses MemoryAdminUsecase.ListFactRows + EvolutionMetricsRepo
// retrieval quality and negative feedback counts to compute a HealthScore.
func (uc *ExperienceAnalyticsUsecase) AnalyzeMemoryQuality(ctx context.Context, agentID string, since time.Time) (MemoryQualityAnalysis, error) {
	agentID, err := requireNonEmpty(agentID, "XP_ANALYTICS", "agent_id")
	if err != nil {
		return MemoryQualityAnalysis{}, err
	}

	var factCount int
	if uc.memoryAdmin != nil {
		_, total, _, _, listErr := uc.memoryAdmin.ListFactRows(ctx, "agent", agentID, "", "", "", 1, 0)
		if listErr != nil {
			uc.lg.Warn("list fact rows for memory quality", loggateway.StepID("xp_analytics.memory_quality"), loggateway.Err(listErr))
		} else {
			factCount = int(total)
		}
	}

	retrievalQuality := float64(0)
	rq, _, rqErr := uc.metricsRepo.GetRetrievalQuality(ctx, agentID, since)
	if rqErr != nil {
		uc.lg.Warn("get retrieval quality", loggateway.StepID("xp_analytics.memory_quality"), loggateway.Err(rqErr))
	} else {
		retrievalQuality = rq
	}

	negFeedback := 0
	nf, nfErr := uc.metricsRepo.GetNegativeFeedbackCount(ctx, agentID, since)
	if nfErr != nil {
		uc.lg.Warn("get negative feedback count", loggateway.StepID("xp_analytics.memory_quality"), loggateway.Err(nfErr))
	} else {
		negFeedback = nf
	}

	healthScore := computeMemoryHealthScore(factCount, retrievalQuality, negFeedback)
	recommendation := memoryQualityRecommendation(healthScore, factCount)

	return MemoryQualityAnalysis{
		AgentID:          agentID,
		FactCount:        factCount,
		RetrievalQuality: math.Round(retrievalQuality*1000) / 1000,
		NegativeFeedback: negFeedback,
		HealthScore:      math.Round(healthScore*1000) / 1000,
		Recommendation:   recommendation,
	}, nil
}

func computeMemoryHealthScore(factCount int, retrievalQuality float64, negFeedback int) float64 {
	coverageScore := math.Min(float64(factCount)/100.0, 1.0)
	penalty := math.Min(float64(negFeedback)/10.0, 1.0)
	return 0.4*coverageScore + 0.4*retrievalQuality + 0.2*(1.0-penalty)
}

func memoryQualityRecommendation(healthScore float64, factCount int) string {
	switch {
	case healthScore >= 0.8:
		return "healthy"
	case healthScore >= 0.6:
		return "review_facts"
	case factCount == 0:
		return "seed_memory"
	default:
		return "prune_and_enrich"
	}
}

// ── AnalyzeAgentCapability ───────────────────────────────────────────────────

// AnalyzeAgentCapability combines tool, skill, orchestration, and cost data
// into a unified agent capability report.
func (uc *ExperienceAnalyticsUsecase) AnalyzeAgentCapability(ctx context.Context, agentID string, timeRange string) (AgentCapabilityAnalysis, error) {
	agentID, err := requireNonEmpty(agentID, "XP_ANALYTICS", "agent_id")
	if err != nil {
		return AgentCapabilityAnalysis{}, err
	}

	since := timeRangeToSince(timeRange)

	toolWeights, toolErr := uc.AnalyzeToolWeights(ctx, agentID, since)
	if toolErr != nil {
		return AgentCapabilityAnalysis{}, toolErr
	}

	skillHealth, skillErr := uc.AnalyzeSkillHealth(ctx, agentID, since)
	if skillErr != nil {
		return AgentCapabilityAnalysis{}, skillErr
	}

	orchestration, orchErr := uc.AnalyzeOrchestration(ctx, agentID, since)
	if orchErr != nil {
		return AgentCapabilityAnalysis{}, orchErr
	}

	memoryQuality, memErr := uc.AnalyzeMemoryQuality(ctx, agentID, since)
	if memErr != nil {
		return AgentCapabilityAnalysis{}, memErr
	}

	costSummary := uc.computeCostSummary(ctx, agentID, since)

	return AgentCapabilityAnalysis{
		AgentID:       agentID,
		ToolWeights:   toolWeights,
		SkillHealth:   skillHealth,
		Orchestration: orchestration,
		MemoryQuality: memoryQuality,
		CostSummary:   costSummary,
	}, nil
}

func (uc *ExperienceAnalyticsUsecase) computeCostSummary(ctx context.Context, agentID string, since time.Time) CostSummary {
	startDate := since.UTC().Format("2006-01-02")
	endDate := time.Now().UTC().Format("2006-01-02")
	summary, err := uc.usageRepo.GetModelUsageSummary(ctx, UsageQuery{
		AgentID:   agentID,
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		uc.lg.Warn("get model usage summary for cost", loggateway.StepID("xp_analytics.cost"), loggateway.Err(err))
		return CostSummary{}
	}
	return CostSummary{
		TotalCostMicroUSD: summary.TotalCostMicroUSD,
		TotalTokens:       summary.TotalTokens,
		CallCount:         summary.CallCount,
	}
}
