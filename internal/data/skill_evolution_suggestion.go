package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/skillevolutionsuggestion"
	"aranea-agents/pkg/loggateway"
)

type SkillEvolutionSuggestionRepo struct {
	data *Data
	lg   loggateway.Logger
}

var (
	_ biz.SkillEvolutionSuggestionReader = (*SkillEvolutionSuggestionRepo)(nil)
	_ biz.SkillEvolutionSuggestionWriter = (*SkillEvolutionSuggestionRepo)(nil)
)

// NewSkillEvolutionSuggestionRepo creates a new SkillEvolutionSuggestionRepo.
func NewSkillEvolutionSuggestionRepo(data *Data, lg loggateway.Logger) *SkillEvolutionSuggestionRepo {
	return &SkillEvolutionSuggestionRepo{data: data, lg: lg}
}

// ── SkillEvolutionSuggestionReader ────────────────────────────────────────────

func (r *SkillEvolutionSuggestionRepo) ListBySkill(ctx context.Context, skillID string, status biz.EvolutionSuggestionStatus, limit, offset int) ([]biz.SkillEvolutionSuggestion, error) {
	if limit <= 0 {
		limit = 50
	}
	q := r.data.RW().Read(ctx).SkillEvolutionSuggestion.Query().
		Where(skillevolutionsuggestion.SkillIDEQ(skillID)).
		Order(ent.Desc(skillevolutionsuggestion.FieldCreatedAt)).
		Limit(limit).
		Offset(offset)
	if status != "" {
		q = q.Where(skillevolutionsuggestion.StatusEQ(string(status)))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	return mapEntEvoSuggestions(rows), nil
}

func (r *SkillEvolutionSuggestionRepo) GetByID(ctx context.Context, id string) (*biz.SkillEvolutionSuggestion, error) {
	row, err := r.data.RW().Read(ctx).SkillEvolutionSuggestion.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s := mapEntEvoSuggestion(row)
	return &s, nil
}

func (r *SkillEvolutionSuggestionRepo) ListPending(ctx context.Context, limit, offset int) ([]biz.SkillEvolutionSuggestion, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.data.RW().Read(ctx).SkillEvolutionSuggestion.Query().
		Where(skillevolutionsuggestion.StatusEQ(string(biz.EvoSuggestionPending))).
		Order(ent.Desc(skillevolutionsuggestion.FieldCreatedAt)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapEntEvoSuggestions(rows), nil
}

func (r *SkillEvolutionSuggestionRepo) GetLatestBySkill(ctx context.Context, skillID string) (*biz.SkillEvolutionSuggestion, error) {
	rows, err := r.data.RW().Read(ctx).SkillEvolutionSuggestion.Query().
		Where(skillevolutionsuggestion.SkillIDEQ(skillID)).
		Order(ent.Desc(skillevolutionsuggestion.FieldCreatedAt)).
		Limit(1).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	s := mapEntEvoSuggestion(rows[0])
	return &s, nil
}

// ── SkillEvolutionSuggestionWriter ────────────────────────────────────────────

func (r *SkillEvolutionSuggestionRepo) Create(ctx context.Context, suggestion biz.SkillEvolutionSuggestion) error {
	builder := r.data.RW().Write(ctx).SkillEvolutionSuggestion.Create().
		SetID(suggestion.ID).
		SetSkillID(suggestion.SkillID).
		SetType(string(suggestion.Type)).
		SetStatus(string(suggestion.Status)).
		SetTriggerReason(suggestion.TriggerReason).
		SetDraftSkillBody(suggestion.DraftSkillBody).
		SetDraftVersionID(suggestion.DraftVersionID).
		SetSandboxPassed(suggestion.SandboxPassed).
		SetApprovedBy(suggestion.ApprovedBy).
		SetRejectedBy(suggestion.RejectedBy).
		SetRejectionReason(suggestion.RejectionReason).
		SetCreatedAt(suggestion.CreatedAt.UTC().Format(time.RFC3339))
	if len(suggestion.SourceReportIDs) > 0 {
		builder.SetSourceReportIds(suggestion.SourceReportIDs)
	}
	if suggestion.SandboxResult != nil {
		var m map[string]any
		if err := json.Unmarshal(suggestion.SandboxResult, &m); err == nil {
			builder.SetSandboxResult(m)
		}
	}
	if suggestion.PreVerifyResult != nil {
		var m map[string]any
		if err := json.Unmarshal(suggestion.PreVerifyResult, &m); err == nil {
			builder.SetPreVerifyResult(m)
		}
	}
	if suggestion.ResolvedAt != nil {
		builder.SetResolvedAt(suggestion.ResolvedAt.UTC().Format(time.RFC3339))
	}
	_, err := builder.Save(ctx)
	return err
}

func (r *SkillEvolutionSuggestionRepo) UpdateStatus(ctx context.Context, id string, status biz.EvolutionSuggestionStatus, resolvedBy string, reason string) error {
	builder := r.data.RW().Write(ctx).SkillEvolutionSuggestion.UpdateOneID(id).
		SetStatus(string(status)).
		SetResolvedAt(time.Now().UTC().Format(time.RFC3339))
	switch status {
	case biz.EvoSuggestionApproved:
		builder.SetApprovedBy(resolvedBy)
	case biz.EvoSuggestionRejected:
		builder.SetRejectedBy(resolvedBy)
		builder.SetRejectionReason(reason)
	}
	_, err := builder.Save(ctx)
	return err
}

func (r *SkillEvolutionSuggestionRepo) UpdateDraftBody(ctx context.Context, id string, draftBody string) error {
	_, err := r.data.RW().Write(ctx).SkillEvolutionSuggestion.UpdateOneID(id).
		SetDraftSkillBody(draftBody).
		Save(ctx)
	return err
}

func (r *SkillEvolutionSuggestionRepo) UpdateSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error {
	builder := r.data.RW().Write(ctx).SkillEvolutionSuggestion.UpdateOneID(id).
		SetSandboxPassed(passed)
	if result != nil {
		var m map[string]any
		if err := json.Unmarshal(result, &m); err == nil {
			builder.SetSandboxResult(m)
		}
	}
	_, err := builder.Save(ctx)
	return err
}

// ── Mapping helpers ───────────────────────────────────────────────────────────

func mapEntEvoSuggestion(row *ent.SkillEvolutionSuggestion) biz.SkillEvolutionSuggestion {
	s := biz.SkillEvolutionSuggestion{
		ID:              row.ID,
		SkillID:         row.SkillID,
		Type:            biz.EvolutionSuggestionType(row.Type),
		Status:          biz.EvolutionSuggestionStatus(row.Status),
		SourceReportIDs: row.SourceReportIds,
		TriggerReason:   row.TriggerReason,
		DraftSkillBody:  row.DraftSkillBody,
		DraftVersionID:  row.DraftVersionID,
		SandboxPassed:   row.SandboxPassed,
		ApprovedBy:      row.ApprovedBy,
		RejectedBy:      row.RejectedBy,
		RejectionReason: row.RejectionReason,
	}
	if row.SandboxResult != nil {
		if data, err := json.Marshal(row.SandboxResult); err == nil {
			s.SandboxResult = data
		}
	}
	if row.PreVerifyResult != nil {
		if data, err := json.Marshal(row.PreVerifyResult); err == nil {
			s.PreVerifyResult = data
		}
	}
	if row.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, row.CreatedAt); err == nil {
			s.CreatedAt = t
		}
	}
	if row.ResolvedAt != "" {
		if t, err := time.Parse(time.RFC3339, row.ResolvedAt); err == nil {
			s.ResolvedAt = &t
		}
	}
	return s
}

func mapEntEvoSuggestions(rows []*ent.SkillEvolutionSuggestion) []biz.SkillEvolutionSuggestion {
	result := make([]biz.SkillEvolutionSuggestion, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapEntEvoSuggestion(row))
	}
	return result
}
