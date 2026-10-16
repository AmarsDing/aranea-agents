package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz/commerce"
	"aranea-agents/internal/data/ent"
	entsup "aranea-agents/internal/data/ent/suppliercandidate"
)

type supplierCandidateRepo struct {
	data *Data
}

var _ commerce.SupplierRepo = (*supplierCandidateRepo)(nil)

// NewSupplierCandidateRepo wires Ent-backed commerce.SupplierRepo.
func NewSupplierCandidateRepo(d *Data) commerce.SupplierRepo {
	return &supplierCandidateRepo{data: d}
}

func (r *supplierCandidateRepo) client(ctx context.Context) *ent.Client {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.data.RW().Write(ctx)
}

func (r *supplierCandidateRepo) Create(s commerce.SupplierCandidate) (commerce.SupplierCandidate, error) {
	ctx := context.Background()
	row, err := r.client(ctx).SupplierCandidate.Create().
		SetID(s.ID).
		SetOpportunityID(s.OpportunityID).
		SetName(s.Name).
		SetURL(s.URL).
		SetPriceRange(s.PriceRange).
		SetNotes(s.Notes).
		SetStatus(s.Status).
		SetCreatedAt(mvpTimeString(s.CreatedAt)).
		SetUpdatedAt(mvpTimeString(s.UpdatedAt)).
		Save(ctx)
	if err != nil {
		return commerce.SupplierCandidate{}, fmt.Errorf("commerce: supplier create: %w", err)
	}
	return entToSupplier(row), nil
}

func (r *supplierCandidateRepo) Get(id string) (commerce.SupplierCandidate, error) {
	ctx := context.Background()
	row, err := r.client(ctx).SupplierCandidate.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return commerce.SupplierCandidate{}, fmt.Errorf("commerce: supplier not found %s", id)
		}
		return commerce.SupplierCandidate{}, fmt.Errorf("commerce: supplier get: %w", err)
	}
	return entToSupplier(row), nil
}

func (r *supplierCandidateRepo) Update(s commerce.SupplierCandidate) (commerce.SupplierCandidate, error) {
	ctx := context.Background()
	row, err := r.client(ctx).SupplierCandidate.UpdateOneID(s.ID).
		SetOpportunityID(s.OpportunityID).
		SetName(s.Name).
		SetURL(s.URL).
		SetPriceRange(s.PriceRange).
		SetNotes(s.Notes).
		SetStatus(s.Status).
		SetUpdatedAt(mvpTimeString(s.UpdatedAt)).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return commerce.SupplierCandidate{}, fmt.Errorf("commerce: supplier not found %s", s.ID)
		}
		return commerce.SupplierCandidate{}, fmt.Errorf("commerce: supplier update: %w", err)
	}
	return entToSupplier(row), nil
}

func (r *supplierCandidateRepo) ListByOpportunity(opportunityID string) ([]commerce.SupplierCandidate, error) {
	ctx := context.Background()
	rows, err := r.client(ctx).SupplierCandidate.Query().
		Where(entsup.OpportunityIDEQ(opportunityID)).
		Order(ent.Asc(entsup.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("commerce: supplier list by opp: %w", err)
	}
	out := make([]commerce.SupplierCandidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToSupplier(row))
	}
	return out, nil
}

func (r *supplierCandidateRepo) List(limit, offset int) ([]commerce.SupplierCandidate, int, error) {
	ctx := context.Background()
	q := r.client(ctx).SupplierCandidate.Query()
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("commerce: supplier count: %w", err)
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := q.Offset(offset).Limit(limit).Order(ent.Desc(entsup.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("commerce: supplier list: %w", err)
	}
	out := make([]commerce.SupplierCandidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToSupplier(row))
	}
	return out, total, nil
}

func entToSupplier(row *ent.SupplierCandidate) commerce.SupplierCandidate {
	return commerce.SupplierCandidate{
		ID:            row.ID,
		OpportunityID: row.OpportunityID,
		Name:          row.Name,
		URL:           row.URL,
		PriceRange:    row.PriceRange,
		Notes:         row.Notes,
		Status:        row.Status,
		CreatedAt:     mvpParseTime(row.CreatedAt),
		UpdatedAt:     mvpParseTime(row.UpdatedAt),
	}
}
