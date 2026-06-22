package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── PatternTrigger（原 SkillEvolutionUsecase） ──

// PatternTrigger 从工具调用 Pattern 中检测新 Skill 需求
type PatternTrigger struct {
	patterns     PatternReader
	creator      SkillAutoCreator
	registrar    SkillRegistrationPort
	proposalRepo SkillProposalReadWriter // 用于检查已有 proposal
	lg           loggateway.Logger
}

func NewPatternTrigger(
	patterns PatternReader,
	creator SkillAutoCreator,
	registrar SkillRegistrationPort,
	proposalRepo SkillProposalReadWriter,
	lg loggateway.Logger,
) *PatternTrigger {
	return &PatternTrigger{
		patterns:     patterns,
		creator:      creator,
		registrar:    registrar,
		proposalRepo: proposalRepo,
		lg:           lg,
	}
}

func (t *PatternTrigger) TargetType() EvolutionTargetType { return EvolutionTargetAgent }
func (t *PatternTrigger) ActionType() EvolutionActionType { return EvolutionActionCreate }
func (t *PatternTrigger) TriggerSource() string           { return "pattern" }

func (t *PatternTrigger) Check(ctx context.Context, agentID string) ([]UnifiedEvolutionSuggestion, error) {
	if t.creator == nil || t.patterns == nil {
		return nil, nil
	}

	patterns, err := t.patterns.ListByAgent(ctx, agentID, string(PatternStatusDetected))
	if err != nil || len(patterns) == 0 {
		return nil, err
	}

	var suggestions []UnifiedEvolutionSuggestion
	for _, p := range patterns {
		if p.Kind != string(ObservationKindToolCall) || p.Confidence < skillPatternMinConfidence {
			continue
		}

		hash := patternHash(p.Description)
		existing, exErr := t.proposalRepo.GetByPatternHash(ctx, agentID, hash)
		if exErr != nil {
			t.lg.Warn("pattern trigger: GetByPatternHash failed", loggateway.Err(exErr))
		}
		if existing != nil {
			continue
		}

		suggestedName := inferSkillNameFromDesc(p.Description)
		if suggestedName != "" {
			exists, existErr := t.registrar.SkillExists(ctx, agentID, suggestedName)
			if existErr != nil {
				t.lg.Warn("pattern trigger: SkillExists check failed", loggateway.Err(existErr))
			} else if exists {
				continue
			}
		}

		toolHistory := extractToolHistoryFromPattern(p)
		name, content, genErr := t.creator.GenerateSKILLMD(ctx, p.Description, toolHistory)
		if genErr != nil {
			t.lg.Warn("pattern trigger: GenerateSKILLMD failed", loggateway.Err(genErr))
			continue
		}

		metadata, _ := json.Marshal(map[string]string{
			"pattern_hash": hash,
			"pattern_desc": p.Description,
		})

		suggestions = append(suggestions, UnifiedEvolutionSuggestion{
			ID:              newAgentCatalogID(),
			TargetType:      EvolutionTargetAgent,
			TargetID:        agentID,
			ActionType:      EvolutionActionCreate,
			TriggerSource:   "pattern",
			TriggerReason:   "detected tool call pattern: " + p.Description,
			Status:          "pending",
			Priority:        1,
			DraftBody:       content,
			DraftName:       name,
			LifecycleStatus: "draft",
			Metadata:        metadata,
			CreatedAt:       time.Now().UTC(),
		})
	}
	return suggestions, nil
}

// ── HealthTrigger（原 SkillIntelligenceUsecase.CheckEvolutionTriggers） ──

// SkillScorer is a narrow interface for scoring a skill, extracted to satisfy BA6
// (depend on abstractions, not concretions). SkillIntelligenceUsecase already implements it.
type SkillScorer interface {
	ScoreSkill(ctx context.Context, skillID string) (int, error)
}

// HealthTrigger 从健康指标中检测改进需求
type HealthTrigger struct {
	aggregator   SkillHealthAggregator
	scoreUsecase SkillScorer
	lg           loggateway.Logger
}

func NewHealthTrigger(
	aggregator SkillHealthAggregator,
	scoreUsecase SkillScorer,
	lg loggateway.Logger,
) *HealthTrigger {
	return &HealthTrigger{
		aggregator:   aggregator,
		scoreUsecase: scoreUsecase,
		lg:           lg,
	}
}

func (t *HealthTrigger) TargetType() EvolutionTargetType { return EvolutionTargetSkill }
func (t *HealthTrigger) ActionType() EvolutionActionType { return EvolutionActionImprove }
func (t *HealthTrigger) TriggerSource() string           { return "health" }

func (t *HealthTrigger) Check(ctx context.Context, skillID string) ([]UnifiedEvolutionSuggestion, error) {
	if t.aggregator == nil {
		return nil, nil
	}

	since30d := time.Now().UTC().Add(-30 * 24 * time.Hour)
	metrics, err := t.aggregator.GetHealthMetrics(ctx, skillID, since30d)
	if err != nil || metrics.InvocationCount < EvoTriggerMinInvocations {
		return nil, err
	}

	var triggerTypes []EvolutionActionType
	var triggerReasons []string
	var priority int

	// Condition 1: 30d failure rate > 30%
	failureRate := 1.0 - metrics.SuccessRate
	if failureRate > EvoTriggerFailureRate {
		triggerTypes = append(triggerTypes, EvolutionActionImprove)
		triggerReasons = append(triggerReasons, fmt.Sprintf("30d failure rate %.1f%% exceeds threshold %.1f%%",
			failureRate*100, EvoTriggerFailureRate*100))
		priority = 2
	}

	// Condition 2: 7d success rate < 60%
	since7d := time.Now().UTC().Add(-7 * 24 * time.Hour)
	metrics7d, err7d := t.aggregator.GetHealthMetrics(ctx, skillID, since7d)
	if err7d != nil {
		t.lg.Warn("HealthTrigger.Check: GetHealthMetrics(7d) failed",
			loggateway.StepID("skill_intelligence.evo_trigger"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err7d))
	} else if metrics7d.InvocationCount >= EvoTrigger7dMinInvocations && metrics7d.SuccessRate < EvoTrigger7dSuccessRate {
		triggerTypes = append(triggerTypes, EvolutionActionImprove)
		triggerReasons = append(triggerReasons, fmt.Sprintf("7d success rate %.1f%% below threshold %.1f%% (%d invocations)",
			metrics7d.SuccessRate*100, EvoTrigger7dSuccessRate*100, metrics7d.InvocationCount))
		if priority < 2 {
			priority = 2
		}
	}

	// Condition 3: Same failure tag >= 5 times in 7d
	if t.aggregator != nil {
		tagCounts, tagErr := t.aggregator.GetFailureTagCounts(ctx, skillID, since7d)
		if tagErr != nil {
			t.lg.Warn("HealthTrigger.Check: GetFailureTagCounts failed",
				loggateway.StepID("skill_intelligence.evo_trigger"),
				loggateway.Str("skill_id", skillID),
				loggateway.Err(tagErr))
		} else {
			for _, tc := range tagCounts {
				if tc.Count >= EvoTriggerSameTagThreshold {
					triggerTypes = append(triggerTypes, EvolutionActionImprove)
					triggerReasons = append(triggerReasons, fmt.Sprintf("failure tag %q appears %d times in 7d (threshold %d)",
						tc.Tag, tc.Count, EvoTriggerSameTagThreshold))
					if priority < 2 {
						priority = 2
					}
					break // one matching tag is enough
				}
			}
		}
	}

	// Condition 4: Skill score < 60
	if t.scoreUsecase != nil {
		if score, scoreErr := t.scoreUsecase.ScoreSkill(ctx, skillID); scoreErr == nil && score < EvoTriggerScoreThreshold {
			triggerTypes = append(triggerTypes, EvolutionActionImprove)
			triggerReasons = append(triggerReasons, fmt.Sprintf("Skill score %d below threshold %d", score, EvoTriggerScoreThreshold))
			if priority < 1 {
				priority = 1
			}
		}
	}

	if len(triggerTypes) == 0 {
		return nil, nil
	}

	// Encode all triggered action types in metadata for traceability.
	var triggeredActionStrs []string
	for _, at := range triggerTypes {
		triggeredActionStrs = append(triggeredActionStrs, string(at))
	}

	// Determine primary ActionType by priority: failure > efficiency.
	primaryAction := triggerTypes[0]

	metadata, _ := json.Marshal(map[string]any{
		"success_rate":      metrics.SuccessRate,
		"invocation_count":  metrics.InvocationCount,
		"avg_duration_ms":   metrics.AvgDurationMS,
		"triggered_actions": triggeredActionStrs,
	})

	return []UnifiedEvolutionSuggestion{
		{
			ID:              newAgentCatalogID(),
			TargetType:      EvolutionTargetSkill,
			TargetID:        skillID,
			ActionType:      primaryAction,
			TriggerSource:   "health",
			TriggerReason:   strings.Join(triggerReasons, "; "),
			Status:          "pending",
			Priority:        priority,
			LifecycleStatus: "draft",
			Metadata:        metadata,
			CreatedAt:       time.Now().UTC(),
		},
	}, nil
}

// ── AgentConfigTrigger（原 EvolutionUsecase） ──

// AgentConfigTrigger 从 Agent 配置中检测进化需求
type AgentConfigTrigger struct {
	metricsRepo EvolutionMetricsRepo
	lg          loggateway.Logger
}

func NewAgentConfigTrigger(
	metricsRepo EvolutionMetricsRepo,
	lg loggateway.Logger,
) *AgentConfigTrigger {
	return &AgentConfigTrigger{metricsRepo: metricsRepo, lg: lg}
}

func (t *AgentConfigTrigger) TargetType() EvolutionTargetType { return EvolutionTargetAgent }
func (t *AgentConfigTrigger) ActionType() EvolutionActionType { return EvolutionActionEvolve }
func (t *AgentConfigTrigger) TriggerSource() string           { return "agent_config" }

// Check is a reserved entry point for agent-level evolution triggers.
// Currently returns nil because agent-level evolution requires explicit external
// invocation (e.g. user manual trigger or admin API call). This placeholder
// preserves the trigger's registration in the orchestrator without generating
// suggestions automatically, ensuring the trigger is available for future
// auto-detection logic without requiring registration changes.
func (t *AgentConfigTrigger) Check(ctx context.Context, agentID string) ([]UnifiedEvolutionSuggestion, error) {
	return nil, nil
}

// ── Helpers ──

// inferSkillNameFromDesc attempts to derive a concise skill name from the pattern
// description (e.g. "web_search(query)" → "web_search").
func inferSkillNameFromDesc(desc string) string {
	parts := strings.Split(desc, ",")
	if len(parts) == 0 {
		return ""
	}
	first := strings.TrimSpace(parts[0])
	if idx := strings.Index(first, "("); idx > 0 {
		return strings.TrimSpace(first[:idx])
	}
	return ""
}

func extractToolHistoryFromPattern(p Pattern) []ToolCallRecord {
	var records []ToolCallRecord
	toolNames := extractToolNamesFromDesc(p.Description)
	for _, name := range toolNames {
		records = append(records, ToolCallRecord{
			ToolName: name,
			Success:  p.Confidence >= skillPatternMinConfidence,
			CalledAt: p.DetectedAt,
		})
	}
	return records
}
