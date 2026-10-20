package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
	"fmt"
	"time"
)

// querySkillNames batch-resolves platform_skill IDs to display names.
// Returns an empty map (not error) on lookup failure — name backfill is best-effort.
func (r *SkillIntelligenceRepo) querySkillNames(ctx context.Context, idSet map[string]struct{}) map[string]string {
	if len(idSet) == 0 {
		return nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	skills, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDIn(ids...)).
		Select(platformskill.FieldID, platformskill.FieldName).
		All(ctx)
	if err != nil {
		r.lg.Warn("backfill skill names failed", loggateway.StepID("data.skill_intelligence"), loggateway.Err(err))
		return nil
	}
	names := make(map[string]string, len(skills))
	for _, s := range skills {
		names[s.ID] = s.Name
	}
	return names
}

func (r *SkillIntelligenceRepo) MarkAnalyzed(ctx context.Context, activationID string) error {
	if activationID == "" {
		return nil
	}

	n, err := r.data.RW().Write(ctx).SkillInvocation.Update().
		Where(skillinvocation.ActivationIDEQ(activationID)).
		SetAnalyzedAt(time.Now().UTC().Format(time.RFC3339)).
		Save(ctx)
	if err != nil {
		return entErrToBizErr(err, "SKILL_INTELLIGENCE")
	}
	if n == 0 {
		return apierror.NotFound("SKILL_INTELLIGENCE", fmt.Sprintf("skill invocation with activation_id %s not found", activationID))
	}
	return nil
}

func mapEntSkillInvocationToWrite(row *ent.SkillInvocation) biz.SkillInvocationWrite {
	return biz.SkillInvocationWrite{
		SkillID:         row.SkillID,
		SkillVersion:    row.SkillVersion,
		AgentID:         row.AgentID,
		UserID:          row.UserID,
		SessionID:       row.SessionID,
		Status:          row.Status,
		DurationMS:      row.DurationMs,
		StartedAt:       row.StartedAt,
		EndedAt:         row.EndedAt,
		InputPreview:    row.InputPreview,
		InputHash:       row.InputHash,
		OutputPreview:   row.OutputPreview,
		ErrorCode:       row.ErrorCode,
		ErrorMessage:    row.ErrorMessage,
		Source:          row.Source,
		ActivationID:    row.ActivationID,
		MessageID:       row.MessageID,
		SelectionReason: row.SelectionReason,
		Outcome:         row.Outcome,
		TokenUsage:      row.TokenUsage,
		RoutedSlugs:     row.RoutedSlugs,
		LoadedSlug:      row.LoadedSlug,
	}
}
