package data

import (
	"context"
	"database/sql"
	"time"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

type industryRepo struct {
	data *Data
}

var _ biz.IndustryRepository = (*industryRepo)(nil)

func NewIndustryRepo(d *Data) biz.IndustryRepository {
	return &industryRepo{data: d}
}

func (r *industryRepo) ListIndustries(ctx context.Context, q biz.IndustryListQuery) (biz.IndustryListResult, error) {
	where := " WHERE deleted_at = ''"
	args := []any{}
	if q.Enabled != nil {
		where += " AND enabled = ?"
		args = append(args, *q.Enabled)
	}
	var total int
	countSQL := "SELECT COUNT(*) FROM industries" + where
	if err := r.data.RawDB().QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return biz.IndustryListResult{}, kerrors.InternalServer("INDUSTRY", err.Error())
	}
	listSQL := "SELECT id, key, name, icon, description, scenario_key, enabled, sort_order, created_at, updated_at, deleted_at FROM industries" + where + " ORDER BY sort_order ASC"
	rows, err := r.data.RawDB().QueryContext(ctx, listSQL, args...)
	if err != nil {
		return biz.IndustryListResult{}, kerrors.InternalServer("INDUSTRY", err.Error())
	}
	defer rows.Close()
	items := make([]biz.Industry, 0)
	for rows.Next() {
		var v biz.Industry
		if err := rows.Scan(&v.ID, &v.Key, &v.Name, &v.Icon, &v.Description, &v.ScenarioKey, &v.Enabled, &v.SortOrder, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt); err != nil {
			return biz.IndustryListResult{}, kerrors.InternalServer("INDUSTRY", err.Error())
		}
		items = append(items, v)
	}
	return biz.IndustryListResult{Items: items, Total: total}, nil
}

func (r *industryRepo) GetIndustryByKey(ctx context.Context, key string) (biz.Industry, error) {
	var v biz.Industry
	err := r.data.RawDB().QueryRowContext(ctx,
		"SELECT id, key, name, icon, description, scenario_key, enabled, sort_order, created_at, updated_at, deleted_at FROM industries WHERE key = ? AND deleted_at = ''",
		key,
	).Scan(&v.ID, &v.Key, &v.Name, &v.Icon, &v.Description, &v.ScenarioKey, &v.Enabled, &v.SortOrder, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt)
	if err == sql.ErrNoRows {
		return biz.Industry{}, kerrors.NotFound("INDUSTRY", "industry not found")
	}
	if err != nil {
		return biz.Industry{}, kerrors.InternalServer("INDUSTRY", err.Error())
	}
	return v, nil
}

func (r *industryRepo) CreateIndustry(ctx context.Context, ind biz.Industry) (biz.Industry, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	_, err := r.data.RawDB().ExecContext(ctx,
		"INSERT INTO industries (id, key, name, icon, description, scenario_key, enabled, sort_order, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')",
		id, ind.Key, ind.Name, ind.Icon, ind.Description, ind.ScenarioKey, ind.Enabled, ind.SortOrder, now, now,
	)
	if err != nil {
		return biz.Industry{}, kerrors.InternalServer("INDUSTRY", err.Error())
	}
	ind.ID = id
	ind.CreatedAt = now
	ind.UpdatedAt = now
	return ind, nil
}

func (r *industryRepo) UpdateIndustry(ctx context.Context, ind biz.Industry) (biz.Industry, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.data.RawDB().ExecContext(ctx,
		"UPDATE industries SET name=?, icon=?, description=?, enabled=?, sort_order=?, updated_at=? WHERE id=?",
		ind.Name, ind.Icon, ind.Description, ind.Enabled, ind.SortOrder, now, ind.ID,
	)
	if err != nil {
		return biz.Industry{}, kerrors.InternalServer("INDUSTRY", err.Error())
	}
	ind.UpdatedAt = now
	return ind, nil
}

func (r *industryRepo) UpsertIndustryByKey(ctx context.Context, ind biz.Industry) (biz.Industry, error) {
	existing, err := r.GetIndustryByKey(ctx, ind.Key)
	if err == nil {
		ind.ID = existing.ID
		return r.UpdateIndustry(ctx, ind)
	}
	return r.CreateIndustry(ctx, ind)
}
