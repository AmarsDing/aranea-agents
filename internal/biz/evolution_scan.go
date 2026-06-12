package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

const (
	evolutionScanToolSuccessThreshold = 0.75
	evolutionScanRetrievalThreshold   = 0.60
	evolutionScanDefaultTimeRange     = "30d"
)

// ScanAll evaluates active agents with evolution suggestions enabled and may create pending suggestions.
func (uc *EvolutionUsecase) ScanAll(ctx context.Context) error {
	if uc == nil || uc.agents == nil {
		return nil
	}
	offset := 0
	const batchSize = 500
	var scanErrs []error
	for {
		page, err := uc.agents.SearchAgents(ctx, AgentListQuery{Limit: batchSize, Offset: offset, Status: "active"})
		if err != nil {
			return err
		}
		for i := range page.Items {
			if err := uc.ScanAgent(ctx, page.Items[i].ID); err != nil {
				scanErrs = append(scanErrs, apierror.Internal("EVOLUTION", "agent %s: %s", page.Items[i].ID, err.Error()))
				continue
			}
		}
		if len(page.Items) < batchSize {
			break
		}
		offset += batchSize
	}
	if len(scanErrs) > 0 {
		return errors.Join(scanErrs...)
	}
	return nil
}

// ScanAgent checks metrics against runtime thresholds and creates deduplicated pending suggestions.
func (uc *EvolutionUsecase) ScanAgent(ctx context.Context, agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}

	// Cross-pipeline dedup: skip if another pipeline already has a pending
	// suggestion for this agent. Prefer orchestrator over legacy coordinator.
	if uc.orchestrator != nil {
		hasPending, err := uc.orchestrator.HasPendingForTarget(ctx, "agent", agentID)
		if err == nil && hasPending {
			return nil
		}
	} else if uc.coordinator != nil && uc.coordinator.HasPendingEvolution(ctx, EvolutionTarget{Type: "agent", ID: agentID}) {
		return nil
	}

	settings, err := uc.agents.GetAgentRuntimeSettings(ctx, agentID)
	if err != nil {
		return err
	}
	if !settings.EvolutionSuggestionsEnabled && !settings.EvoEnabled {
		return nil
	}
	metrics, err := uc.GetEvolutionMetrics(ctx, agentID, evolutionScanDefaultTimeRange)
	if err != nil {
		return err
	}
	minEpisodes := settings.EvoMinEpisodes
	if minEpisodes <= 0 {
		minEpisodes = 3
	}
	minNeg := settings.EvoMinNegativeFeedback
	if minNeg <= 0 {
		minNeg = 2
	}
	if metrics.TotalEpisodes < minEpisodes && metrics.NegativeFeedback < minNeg {
		return nil
	}
	if metrics.ToolSuccessRate > 0 && metrics.ToolSuccessRate < evolutionScanToolSuccessThreshold {
		title := "工具成功率偏低"
		content := fmt.Sprintf(
			"近%s工具成功率 %.1f%%（阈值 %.0f%%）。建议检查工具 allow/deny 与 Skill 挂载策略。",
			evolutionScanDefaultTimeRange,
			metrics.ToolSuccessRate*100,
			evolutionScanToolSuccessThreshold*100,
		)
		if err := uc.ensurePendingSuggestion(ctx, agentID, "prompt", title, content); err != nil {
			return err
		}
	}
	if metrics.RetrievalQuality > 0 && metrics.RetrievalQuality < evolutionScanRetrievalThreshold {
		title := "检索质量偏低"
		content := fmt.Sprintf(
			"近%s检索质量 %.1f%%（阈值 %.0f%%）。建议调整记忆 L2/L3 召回参数或知识库覆盖。",
			evolutionScanDefaultTimeRange,
			metrics.RetrievalQuality*100,
			evolutionScanRetrievalThreshold*100,
		)
		if err := uc.ensurePendingSuggestion(ctx, agentID, "skill", title, content); err != nil {
			return err
		}
	}
	if metrics.NegativeFeedback >= minNeg {
		title := "负反馈累积"
		content := fmt.Sprintf(
			"近%s负反馈 %d 次（阈值 %d）。建议审阅 IDENTITY.md ## Persona 语气与工具策略。",
			evolutionScanDefaultTimeRange,
			metrics.NegativeFeedback,
			minNeg,
		)
		if err := uc.ensurePendingSuggestion(ctx, agentID, "persona", title, content); err != nil {
			return err
		}
	}
	return nil
}

func (uc *EvolutionUsecase) ensurePendingSuggestion(ctx context.Context, agentID, typ, title, content string) error {
	pending, err := uc.suggestionRepo.ListByAgent(ctx, agentID, EvolutionStatusPending)
	if err != nil {
		return err
	}
	for _, s := range pending {
		if strings.EqualFold(strings.TrimSpace(s.Type), typ) && strings.TrimSpace(s.Title) == title {
			return nil
		}
	}
	_, err = uc.suggestionRepo.Create(ctx, EvolutionSuggestion{
		ID:        newAgentCatalogID(),
		AgentID:   agentID,
		Type:      typ,
		Title:     title,
		Content:   content,
		Status:    EvolutionStatusPending,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	return err
}
