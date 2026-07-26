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

// AgentEvolutionSettingsReader is the narrow settings dependency shared by
// agent-scoped triggers for per-agent opt-in gating (A6: the orchestrator
// worker scans the union of L1/L3 opted-in agents, so each trigger re-checks
// its own opt-in flag).
// Stability:evolving
type AgentEvolutionSettingsReader interface {
	GetAgentRuntimeSettings(ctx context.Context, agentID string) (AgentRuntimeSettings, error)
}

// PatternTrigger 从工具调用 Pattern 中检测新 Skill 需求
type PatternTrigger struct {
	settings      AgentEvolutionSettingsReader  // L1 opt-in gate (nil = no gate, tests)
	patterns      PatternReader
	creator       SkillAutoCreator
	registrar     SkillRegistrationPort
	patternReader UnifiedEvolutionPatternReader // pattern_hash 去重（A6 unified）
	lg            loggateway.Logger
}

func NewPatternTrigger(
	settings AgentEvolutionSettingsReader,
	patterns PatternReader,
	creator SkillAutoCreator,
	registrar SkillRegistrationPort,
	patternReader UnifiedEvolutionPatternReader,
	lg loggateway.Logger,
) *PatternTrigger {
	return &PatternTrigger{
		settings:      settings,
		patterns:      patterns,
		creator:       creator,
		registrar:     registrar,
		patternReader: patternReader,
		lg:            lg,
	}
}

func (t *PatternTrigger) TargetType() EvolutionTargetType { return EvolutionTargetAgent }
func (t *PatternTrigger) ActionType() EvolutionActionType { return EvolutionActionCreate }
func (t *PatternTrigger) TriggerSource() string           { return "pattern" }

func (t *PatternTrigger) Check(ctx context.Context, agentID string) ([]UnifiedEvolutionSuggestion, error) {
	if t.creator == nil || t.patterns == nil {
		return nil, nil
	}
	// L1 opt-in gate: mirrors the legacy SkillEvolutionScanner flag check.
	if t.settings != nil {
		settings, err := t.settings.GetAgentRuntimeSettings(ctx, agentID)
		if err != nil {
			t.lg.Warn("pattern trigger: GetAgentRuntimeSettings failed", loggateway.Err(err))
			return nil, nil
		}
		if !settings.EvolutionSkillEvolve {
			return nil, nil
		}
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
		if t.patternReader != nil {
			existing, exErr := t.patternReader.GetLatestByPatternHash(ctx, agentID, hash)
			if exErr != nil {
				t.lg.Warn("pattern trigger: GetLatestByPatternHash failed", loggateway.Err(exErr))
			}
			if existing != nil {
				continue
			}
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

// ── AgentConfigTrigger（原 EvolutionUsecase.ScanAgent） ──

const (
	agentConfigScanToolSuccessThreshold = 0.75
	agentConfigScanRetrievalThreshold   = 0.60
	agentConfigScanDefaultTimeRange     = "30d"
)

// AgentConfigTrigger 从 Agent 运行指标中检测 L3 进化需求（工具成功率 /
// 检索质量 / 负反馈），移植自 legacy EvolutionUsecase.ScanAgent（A6）。
// 建议落库为 unified 行：action_type=evolve_agent, trigger_source=agent_config，
// legacy type/title 存 metadata（EvoMetaLegacyType / EvoMetaTitle）。
type AgentConfigTrigger struct {
	settings    AgentEvolutionSettingsReader // L3 opt-in gate (nil = no gate, tests)
	metricsRepo EvolutionMetricsRepo         // 指标采集
	queryReader UnifiedEvolutionQueryReader  // pending type+title 去重 (nil = skip dedup)
	lg          loggateway.Logger
}

func NewAgentConfigTrigger(
	settings AgentEvolutionSettingsReader,
	metricsRepo EvolutionMetricsRepo,
	queryReader UnifiedEvolutionQueryReader,
	lg loggateway.Logger,
) *AgentConfigTrigger {
	return &AgentConfigTrigger{settings: settings, metricsRepo: metricsRepo, queryReader: queryReader, lg: lg}
}

func (t *AgentConfigTrigger) TargetType() EvolutionTargetType { return EvolutionTargetAgent }
func (t *AgentConfigTrigger) ActionType() EvolutionActionType { return EvolutionActionEvolve }
func (t *AgentConfigTrigger) TriggerSource() string           { return "agent_config" }

// Check evaluates the agent's 30d metrics against the legacy scan thresholds
// and returns deduplicated pending suggestions (tool success / retrieval
// quality / negative feedback). Returns nil when the agent has not opted into
// L3 evolution or metrics are below the minimum-signal floor.
func (t *AgentConfigTrigger) Check(ctx context.Context, agentID string) ([]UnifiedEvolutionSuggestion, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || t.metricsRepo == nil {
		return nil, nil
	}

	// L3 opt-in gate: mirrors legacy ScanAgent flag check.
	var settings AgentRuntimeSettings
	if t.settings != nil {
		var err error
		settings, err = t.settings.GetAgentRuntimeSettings(ctx, agentID)
		if err != nil {
			return nil, err
		}
		if !settings.EvolutionSuggestionsEnabled && !settings.EvoEnabled {
			return nil, nil
		}
	}

	metrics := collectEvolutionMetrics(ctx, t.metricsRepo, t.lg, agentID, agentConfigScanDefaultTimeRange)
	minEpisodes := settings.EvoMinEpisodes
	if minEpisodes <= 0 {
		minEpisodes = 3
	}
	minNeg := settings.EvoMinNegativeFeedback
	if minNeg <= 0 {
		minNeg = 2
	}
	if metrics.TotalEpisodes < minEpisodes && metrics.NegativeFeedback < minNeg {
		return nil, nil
	}

	type candidate struct {
		typ     string
		title   string
		content string
	}
	var candidates []candidate
	if metrics.ToolSuccessRate > 0 && metrics.ToolSuccessRate < agentConfigScanToolSuccessThreshold {
		candidates = append(candidates, candidate{
			typ:   "prompt",
			title: "工具成功率偏低",
			content: fmt.Sprintf(
				"近%s工具成功率 %.1f%%（阈值 %.0f%%）。建议检查工具 allow/deny 与 Skill 挂载策略。",
				agentConfigScanDefaultTimeRange,
				metrics.ToolSuccessRate*100,
				agentConfigScanToolSuccessThreshold*100,
			),
		})
	}
	if metrics.RetrievalQuality > 0 && metrics.RetrievalQuality < agentConfigScanRetrievalThreshold {
		candidates = append(candidates, candidate{
			typ:   "skill",
			title: "检索质量偏低",
			content: fmt.Sprintf(
				"近%s检索质量 %.1f%%（阈值 %.0f%%）。建议调整记忆 L2/L3 召回参数或知识库覆盖。",
				agentConfigScanDefaultTimeRange,
				metrics.RetrievalQuality*100,
				agentConfigScanRetrievalThreshold*100,
			),
		})
	}
	if metrics.NegativeFeedback >= minNeg {
		candidates = append(candidates, candidate{
			typ:   "persona",
			title: "负反馈累积",
			content: fmt.Sprintf(
				"近%s负反馈 %d 次（阈值 %d）。建议审阅 IDENTITY.md ## Persona 语气与工具策略。",
				agentConfigScanDefaultTimeRange,
				metrics.NegativeFeedback,
				minNeg,
			),
		})
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Dedup against pending suggestions with the same legacy type + title
	// (mirrors legacy ensurePendingSuggestion).
	pendingKeys := map[string]struct{}{}
	if t.queryReader != nil {
		pending, err := t.queryReader.ListByTargetAndAction(ctx,
			string(EvolutionTargetAgent), agentID, string(EvolutionActionEvolve),
			string(UnifiedEvolutionStatePending), 100, 0)
		if err != nil {
			t.lg.Warn("agent_config trigger: pending list failed, skip dedup", loggateway.Err(err))
		} else {
			for i := range pending {
				key := strings.ToLower(strings.TrimSpace(pending[i].MetaString(EvoMetaLegacyType))) + "|" + strings.TrimSpace(pending[i].MetaString(EvoMetaTitle))
				pendingKeys[key] = struct{}{}
			}
		}
	}

	now := time.Now().UTC()
	var suggestions []UnifiedEvolutionSuggestion
	for _, c := range candidates {
		key := strings.ToLower(strings.TrimSpace(c.typ)) + "|" + strings.TrimSpace(c.title)
		if _, dup := pendingKeys[key]; dup {
			continue
		}
		metadata, _ := json.Marshal(map[string]string{
			EvoMetaLegacyType: c.typ,
			EvoMetaTitle:      c.title,
		})
		suggestions = append(suggestions, UnifiedEvolutionSuggestion{
			ID:              newAgentCatalogID(),
			TargetType:      EvolutionTargetAgent,
			TargetID:        agentID,
			ActionType:      EvolutionActionEvolve,
			TriggerSource:   "agent_config",
			TriggerReason:   c.title,
			Status:          string(UnifiedEvolutionStatePending),
			Priority:        1,
			DraftBody:       c.content,
			LifecycleStatus: "draft",
			Metadata:        metadata,
			CreatedAt:       now,
		})
	}
	return suggestions, nil
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
