package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz/approval"
	"aranea-agents/pkg/apierror"
)

// approvalRiskMatrixRepo implements approval.RiskMatrixRepo via raw SQL
// （approval_risk_matrix 为非 ent 配置表，迁移 20261323 建表）。
type approvalRiskMatrixRepo struct {
	data *Data
}

var _ approval.RiskMatrixRepo = (*approvalRiskMatrixRepo)(nil)

// NewApprovalRiskMatrixRepo returns the raw-SQL-backed RiskMatrixRepo.
func NewApprovalRiskMatrixRepo(d *Data) approval.RiskMatrixRepo {
	return &approvalRiskMatrixRepo{data: d}
}

func (r *approvalRiskMatrixRepo) ListRiskMatrix(ctx context.Context) ([]approval.RiskMatrixRow, error) {
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, apierror.Internal(apierror.DomainApproval, "repository unavailable")
	}
	rows, err := db.QueryContext(ctx,
		`SELECT matcher_key, risk, default_mode, auto_approve_times, updated_by, updated_at
		   FROM approval_risk_matrix ORDER BY matcher_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]approval.RiskMatrixRow, 0)
	for rows.Next() {
		var row approval.RiskMatrixRow
		var updatedAt string
		if err := rows.Scan(&row.MatcherKey, &row.Risk, &row.DefaultMode, &row.AutoApproveTimes, &row.UpdatedBy, &updatedAt); err != nil {
			return nil, err
		}
		row.UpdatedAt = parseRFC3339Time(updatedAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *approvalRiskMatrixRepo) ReplaceRiskMatrix(ctx context.Context, rows []approval.RiskMatrixRow, updatedBy string) error {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return apierror.Internal(apierror.DomainApproval, "repository unavailable")
	}
	now := nowRFC3339()
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		db := r.data.RWDB().WriteDB(txCtx)
		if _, err := db.ExecContext(txCtx, `DELETE FROM approval_risk_matrix`); err != nil {
			return err
		}
		for _, row := range rows {
			if _, err := db.ExecContext(txCtx, r.data.Dialect().RenumberPlaceholders(
				`INSERT INTO approval_risk_matrix (matcher_key, risk, default_mode, auto_approve_times, updated_by, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?)`),
				row.MatcherKey, row.Risk, row.DefaultMode, row.AutoApproveTimes, updatedBy, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// parseRFC3339Time 解析 RFC3339 文本时间戳（列存 TEXT，与 coding_tasks 同例）。
func parseRFC3339Time(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}
