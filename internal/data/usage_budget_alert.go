package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	entpkg "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/budgetalert"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

func entBudgetAlertToBiz(row *entpkg.BudgetAlert) biz.BudgetAlert {
	if row == nil {
		return biz.BudgetAlert{}
	}
	return biz.BudgetAlert{
		ID:          row.ID,
		ScopeType:   row.ScopeType,
		ScopeID:     row.ScopeID,
		AlertRatio:  row.AlertRatio,
		Enabled:     row.Enabled,
		LastFiredAt: row.LastFiredAt,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func (r *usageRepo) ListBudgetAlerts(ctx context.Context, scopeType, scopeID string) ([]biz.BudgetAlert, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	q := r.readClient(ctx).BudgetAlert.Query()
	if scopeType != "" {
		q = q.Where(budgetalert.ScopeTypeEQ(scopeType))
	}
	if scopeID != "" {
		q = q.Where(budgetalert.ScopeIDEQ(scopeID))
	}
	rows, err := q.Order(budgetalert.ByAlertRatio()).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.BudgetAlert, 0, len(rows))
	for _, row := range rows {
		out = append(out, entBudgetAlertToBiz(row))
	}
	return out, nil
}

func (r *usageRepo) SetBudgetAlert(ctx context.Context, alert biz.BudgetAlert) (biz.BudgetAlert, error) {
	scopeType := strings.TrimSpace(alert.ScopeType)
	scopeID := strings.TrimSpace(alert.ScopeID)
	if scopeType == "" || scopeID == "" {
		return biz.BudgetAlert{}, biz.ErrUsageScopeRequired
	}
	now := nowRFC3339()
	id := strings.TrimSpace(alert.ID)
	if id == "" {
		id = uuid.NewString()
	}
	err := r.ent().BudgetAlert.Create().
		SetID(id).
		SetScopeType(scopeType).
		SetScopeID(scopeID).
		SetAlertRatio(alert.AlertRatio).
		SetEnabled(alert.Enabled).
		SetLastFiredAt(strings.TrimSpace(alert.LastFiredAt)).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		OnConflict(
			entsql.ConflictColumns(budgetalert.FieldScopeType, budgetalert.FieldScopeID, budgetalert.FieldAlertRatio),
			entsql.ResolveWithNewValues(),
		).
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		return biz.BudgetAlert{}, err
	}
	alerts, err := r.ListBudgetAlerts(ctx, scopeType, scopeID)
	if err != nil {
		return biz.BudgetAlert{}, err
	}
	for _, a := range alerts {
		if a.AlertRatio == alert.AlertRatio {
			return a, nil
		}
	}
	return biz.BudgetAlert{}, biz.ErrBudgetAlertNotFound
}

func (r *usageRepo) UpdateBudgetAlertLastFired(ctx context.Context, id, firedAt string) error {
	return r.ent().BudgetAlert.UpdateOneID(strings.TrimSpace(id)).
		SetLastFiredAt(firedAt).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
}
