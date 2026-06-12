package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/evolutionsuggestion"
)

type evolutionSuggestionRepo struct {
	data *Data
}

var _ biz.EvolutionSuggestionRepo = (*evolutionSuggestionRepo)(nil)

func NewEvolutionSuggestionRepo(data *Data) biz.EvolutionSuggestionRepo {
	return &evolutionSuggestionRepo{data: data}
}

func entSuggestionToBiz(row *ent.EvolutionSuggestion) biz.EvolutionSuggestion {
	if row == nil {
		return biz.EvolutionSuggestion{}
	}
	return biz.EvolutionSuggestion{
		ID:               row.ID,
		AgentID:          row.AgentID,
		Type:             row.Type,
		Title:            row.Title,
		Content:          row.Content,
		Status:           row.Status,
		DiffPreview:      row.DiffPreview,
		PreApplySnapshot: row.PreApplySnapshot,
		CreatedAt:        row.CreatedAt,
		AppliedAt:        row.AppliedAt,
	}
}

func (r *evolutionSuggestionRepo) ListByAgent(ctx context.Context, agentID string, status string) ([]biz.EvolutionSuggestion, error) {
	query := r.data.RW().Read(ctx).EvolutionSuggestion.Query().
		Where(evolutionsuggestion.AgentIDEQ(agentID))
	if status != "" {
		query = query.Where(evolutionsuggestion.StatusEQ(status))
	}
	rows, err := query.Order(ent.Desc(evolutionsuggestion.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.EvolutionSuggestion, 0, len(rows))
	for _, row := range rows {
		out = append(out, entSuggestionToBiz(row))
	}
	return out, nil
}

func (r *evolutionSuggestionRepo) GetByID(ctx context.Context, id string) (biz.EvolutionSuggestion, error) {
	row, err := r.data.RW().Read(ctx).EvolutionSuggestion.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.EvolutionSuggestion{}, fmt.Errorf("suggestion not found")
		}
		return biz.EvolutionSuggestion{}, err
	}
	return entSuggestionToBiz(row), nil
}

func (r *evolutionSuggestionRepo) Create(ctx context.Context, s biz.EvolutionSuggestion) (biz.EvolutionSuggestion, error) {
	now := nowRFC3339()
	row, err := r.data.RW().Write(ctx).EvolutionSuggestion.Create().
		SetID(s.ID).
		SetAgentID(s.AgentID).
		SetType(s.Type).
		SetTitle(s.Title).
		SetContent(s.Content).
		SetStatus(s.Status).
		SetDiffPreview(s.DiffPreview).
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.EvolutionSuggestion{}, err
	}
	return entSuggestionToBiz(row), nil
}

func (r *evolutionSuggestionRepo) UpdateStatus(ctx context.Context, id string, status string) (biz.EvolutionSuggestion, error) {
	update := r.data.RW().Write(ctx).EvolutionSuggestion.UpdateOneID(id).
		SetStatus(status)
	if status == "applied" {
		update = update.SetAppliedAt(nowRFC3339())
	}
	row, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.EvolutionSuggestion{}, fmt.Errorf("suggestion not found")
		}
		return biz.EvolutionSuggestion{}, err
	}
	return entSuggestionToBiz(row), nil
}

func (r *evolutionSuggestionRepo) UpdateSnapshot(ctx context.Context, id string, snapshot string) error {
	_, err := r.data.RW().Write(ctx).EvolutionSuggestion.UpdateOneID(id).
		SetPreApplySnapshot(snapshot).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("suggestion not found")
		}
		return err
	}
	return nil
}
