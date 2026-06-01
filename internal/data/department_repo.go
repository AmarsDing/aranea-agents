package data

import (
	"context"
	"database/sql"
	"time"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

type departmentRepo struct {
	data *Data
}

var _ biz.DepartmentRepository = (*departmentRepo)(nil)

func NewDepartmentRepo(d *Data) biz.DepartmentRepository {
	return &departmentRepo{data: d}
}

const deptCols = "id, key, name, industry_key, description, responsibilities_json, sort_order, created_at, updated_at, deleted_at"

func scanDepartment(row scanner) (biz.Department, error) {
	var v biz.Department
	err := row.Scan(&v.ID, &v.Key, &v.Name, &v.IndustryKey, &v.Description, &v.ResponsibilitiesJSON, &v.SortOrder, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt)
	return v, err
}

func (r *departmentRepo) ListDepartments(ctx context.Context, q biz.DepartmentListQuery) (biz.DepartmentListResult, error) {
	where := " WHERE deleted_at = ''"
	args := []any{}
	if q.IndustryKey != "" {
		where += " AND industry_key = ?"
		args = append(args, q.IndustryKey)
	}
	var total int
	if err := r.data.RawDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM departments"+where, args...).Scan(&total); err != nil {
		return biz.DepartmentListResult{}, kerrors.InternalServer("DEPARTMENT", err.Error())
	}
	rows, err := r.data.RawDB().QueryContext(ctx, "SELECT "+deptCols+" FROM departments"+where+" ORDER BY sort_order ASC", args...)
	if err != nil {
		return biz.DepartmentListResult{}, kerrors.InternalServer("DEPARTMENT", err.Error())
	}
	defer rows.Close()
	items := make([]biz.Department, 0)
	for rows.Next() {
		v, err := scanDepartment(rows)
		if err != nil {
			return biz.DepartmentListResult{}, kerrors.InternalServer("DEPARTMENT", err.Error())
		}
		items = append(items, v)
	}
	return biz.DepartmentListResult{Items: items, Total: total}, nil
}

func (r *departmentRepo) GetDepartmentByKey(ctx context.Context, key, industryKey string) (biz.Department, error) {
	v, err := scanDepartment(r.data.RawDB().QueryRowContext(ctx,
		"SELECT "+deptCols+" FROM departments WHERE key = ? AND industry_key = ? AND deleted_at = ''",
		key, industryKey,
	))
	if err == sql.ErrNoRows {
		return biz.Department{}, kerrors.NotFound("DEPARTMENT", "department not found")
	}
	if err != nil {
		return biz.Department{}, kerrors.InternalServer("DEPARTMENT", err.Error())
	}
	return v, nil
}

func (r *departmentRepo) CreateDepartment(ctx context.Context, d biz.Department) (biz.Department, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	_, err := r.data.RawDB().ExecContext(ctx,
		"INSERT INTO departments ("+deptCols+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '')",
		id, d.Key, d.Name, d.IndustryKey, d.Description, d.ResponsibilitiesJSON, d.SortOrder, now, now,
	)
	if err != nil {
		return biz.Department{}, kerrors.InternalServer("DEPARTMENT", err.Error())
	}
	d.ID = id
	d.CreatedAt = now
	d.UpdatedAt = now
	return d, nil
}

func (r *departmentRepo) UpsertDepartmentByKey(ctx context.Context, d biz.Department) (biz.Department, error) {
	existing, err := r.GetDepartmentByKey(ctx, d.Key, d.IndustryKey)
	if err == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		_, updateErr := r.data.RawDB().ExecContext(ctx,
			"UPDATE departments SET name=?, description=?, responsibilities_json=?, sort_order=?, updated_at=? WHERE id=?",
			d.Name, d.Description, d.ResponsibilitiesJSON, d.SortOrder, now, existing.ID,
		)
		if updateErr != nil {
			return biz.Department{}, kerrors.InternalServer("DEPARTMENT", updateErr.Error())
		}
		d.ID = existing.ID
		d.UpdatedAt = now
		return d, nil
	}
	return r.CreateDepartment(ctx, d)
}


