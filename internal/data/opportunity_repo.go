package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz/commerce"
	"aranea-agents/internal/data/ent"
	entopp "aranea-agents/internal/data/ent/opportunity"
)

type opportunityRepo struct {
	data *Data
}

var _ commerce.OpportunityRepo = (*opportunityRepo)(nil)

// NewOpportunityRepo wires Ent-backed commerce.OpportunityRepo.
func NewOpportunityRepo(d *Data) commerce.OpportunityRepo {
	return &opportunityRepo{data: d}
}

func (r *opportunityRepo) client(ctx context.Context) *ent.Client {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.data.RW().Write(ctx)
}

func (r *opportunityRepo) Create(o commerce.Opportunity) (commerce.Opportunity, error) {
	ctx := context.Background()
	row, err := r.client(ctx).Opportunity.Create().
		SetID(o.ID).
		SetTitle(o.Title).
		SetSummary(o.Summary).
		SetEntitiesJSON(mvpEncodeStrings(o.Entities)).
		SetSignalsJSON(mvpEncodeStrings(o.Signals)).
		SetAudience(o.Audience).
		SetScore(o.Score).
		SetStatus(o.Status).
		SetCreatedAt(mvpTimeString(o.CreatedAt)).
		SetUpdatedAt(mvpTimeString(o.UpdatedAt)).
		Save(ctx)
	if err != nil {
		return commerce.Opportunity{}, fmt.Errorf("commerce: opportunity create: %w", err)
	}
	return entToOpportunity(row), nil
}

func (r *opportunityRepo) Get(id string) (commerce.Opportunity, error) {
	ctx := context.Background()
	row, err := r.client(ctx).Opportunity.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return commerce.Opportunity{}, fmt.Errorf("commerce: opportunity not found %s", id)
		}
		return commerce.Opportunity{}, fmt.Errorf("commerce: opportunity get: %w", err)
	}
	return entToOpportunity(row), nil
}

func (r *opportunityRepo) Update(o commerce.Opportunity) (commerce.Opportunity, error) {
	ctx := context.Background()
	row, err := r.client(ctx).Opportunity.UpdateOneID(o.ID).
		SetTitle(o.Title).
		SetSummary(o.Summary).
		SetEntitiesJSON(mvpEncodeStrings(o.Entities)).
		SetSignalsJSON(mvpEncodeStrings(o.Signals)).
		SetAudience(o.Audience).
		SetScore(o.Score).
		SetStatus(o.Status).
		SetUpdatedAt(mvpTimeString(o.UpdatedAt)).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return commerce.Opportunity{}, fmt.Errorf("commerce: opportunity not found %s", o.ID)
		}
		return commerce.Opportunity{}, fmt.Errorf("commerce: opportunity update: %w", err)
	}
	return entToOpportunity(row), nil
}

func (r *opportunityRepo) List(audience, status string, limit, offset int) ([]commerce.Opportunity, int, error) {
	ctx := context.Background()
	q := r.client(ctx).Opportunity.Query()
	if audience != "" {
		q = q.Where(entopp.Or(entopp.AudienceEQ(audience), entopp.AudienceEQ(commerce.AudienceBoth)))
	}
	if status != "" {
		q = q.Where(entopp.StatusEQ(status))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("commerce: opportunity count: %w", err)
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := q.Offset(offset).Limit(limit).Order(ent.Desc(entopp.FieldScore)).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("commerce: opportunity list: %w", err)
	}
	out := make([]commerce.Opportunity, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToOpportunity(row))
	}
	return out, total, nil
}

func entToOpportunity(row *ent.Opportunity) commerce.Opportunity {
	return commerce.Opportunity{
		ID:        row.ID,
		Title:     row.Title,
		Summary:   row.Summary,
		Entities:  mvpDecodeStrings(row.EntitiesJSON),
		Signals:   mvpDecodeStrings(row.SignalsJSON),
		Audience:  row.Audience,
		Score:     row.Score,
		Status:    row.Status,
		CreatedAt: mvpParseTime(row.CreatedAt),
		UpdatedAt: mvpParseTime(row.UpdatedAt),
	}
}
