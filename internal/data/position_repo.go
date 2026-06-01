package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

type positionRepo struct {
	data *Data
}

var _ biz.PositionRepository = (*positionRepo)(nil)

func NewPositionRepo(d *Data) biz.PositionRepository {
	return &positionRepo{data: d}
}

const posCols = "id, key, name, department_key, description, responsibilities_json, skills_required, seniority_level, sort_order, created_at, updated_at, deleted_at"

type positionRow struct {
	ID                   string
	Key                  string
	Name                 string
	DepartmentKey        string
	Description          string
	ResponsibilitiesJSON string
	SkillsRequiredJSON   string
	SeniorityLevel       string
	SortOrder            int
	CreatedAt            string
	UpdatedAt            string
	DeletedAt            string
}

func scanPositionRow(row scanner) (positionRow, error) {
	var v positionRow
	err := row.Scan(&v.ID, &v.Key, &v.Name, &v.DepartmentKey, &v.Description, &v.ResponsibilitiesJSON, &v.SkillsRequiredJSON, &v.SeniorityLevel, &v.SortOrder, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt)
	return v, err
}

func posRowToBiz(lg loggateway.Logger, v positionRow) biz.Position {
	var skills []string
	if v.SkillsRequiredJSON != "" && v.SkillsRequiredJSON != "[]" {
		if err := json.Unmarshal([]byte(v.SkillsRequiredJSON), &skills); err != nil {
			lg.Warn("unmarshal position skills_required failed", loggateway.StepID("data.position"), loggateway.Err(err))
		}
	}
	return biz.Position{
		ID:                   v.ID,
		Key:                  v.Key,
		Name:                 v.Name,
		DepartmentKey:        v.DepartmentKey,
		Description:          v.Description,
		ResponsibilitiesJSON: v.ResponsibilitiesJSON,
		SkillsRequired:       skills,
		SeniorityLevel:       v.SeniorityLevel,
		SortOrder:            v.SortOrder,
		CreatedAt:            v.CreatedAt,
		UpdatedAt:            v.UpdatedAt,
		DeletedAt:            v.DeletedAt,
	}
}

func (r *positionRepo) ListPositions(ctx context.Context, q biz.PositionListQuery) (biz.PositionListResult, error) {
	where := " WHERE deleted_at = ''"
	args := []any{}
	if q.DepartmentKey != "" {
		where += " AND department_key = ?"
		args = append(args, q.DepartmentKey)
	}
	var total int
	if err := r.data.RawDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM positions"+where, args...).Scan(&total); err != nil {
		return biz.PositionListResult{}, kerrors.InternalServer("POSITION", err.Error())
	}
	rows, err := r.data.RawDB().QueryContext(ctx, "SELECT "+posCols+" FROM positions"+where+" ORDER BY sort_order ASC", args...)
	if err != nil {
		return biz.PositionListResult{}, kerrors.InternalServer("POSITION", err.Error())
	}
	defer rows.Close()
	items := make([]biz.Position, 0)
	for rows.Next() {
		v, err := scanPositionRow(rows)
		if err != nil {
			return biz.PositionListResult{}, kerrors.InternalServer("POSITION", err.Error())
		}
		items = append(items, posRowToBiz(r.data.lg, v))
	}
	return biz.PositionListResult{Items: items, Total: total}, nil
}

func (r *positionRepo) GetPositionByKey(ctx context.Context, key, departmentKey string) (biz.Position, error) {
	v, err := scanPositionRow(r.data.RawDB().QueryRowContext(ctx,
		"SELECT "+posCols+" FROM positions WHERE key = ? AND department_key = ? AND deleted_at = ''",
		key, departmentKey,
	))
	if err == sql.ErrNoRows {
		return biz.Position{}, kerrors.NotFound("POSITION", "position not found")
	}
	if err != nil {
		return biz.Position{}, kerrors.InternalServer("POSITION", err.Error())
	}
	return posRowToBiz(r.data.lg, v), nil
}

func (r *positionRepo) CreatePosition(ctx context.Context, p biz.Position) (biz.Position, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	skillsJSON, _ := json.Marshal(p.SkillsRequired)
	if len(skillsJSON) == 0 {
		skillsJSON = []byte("[]")
	}
	_, err := r.data.RawDB().ExecContext(ctx,
		"INSERT INTO positions ("+posCols+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')",
		id, p.Key, p.Name, p.DepartmentKey, p.Description, p.ResponsibilitiesJSON, string(skillsJSON), p.SeniorityLevel, p.SortOrder, now, now,
	)
	if err != nil {
		return biz.Position{}, kerrors.InternalServer("POSITION", err.Error())
	}
	p.ID = id
	p.CreatedAt = now
	p.UpdatedAt = now
	return p, nil
}

func (r *positionRepo) UpsertPositionByKey(ctx context.Context, p biz.Position) (biz.Position, error) {
	existing, err := r.GetPositionByKey(ctx, p.Key, p.DepartmentKey)
	if err == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		skillsJSON, _ := json.Marshal(p.SkillsRequired)
		if len(skillsJSON) == 0 {
			skillsJSON = []byte("[]")
		}
		_, updateErr := r.data.RawDB().ExecContext(ctx,
			"UPDATE positions SET name=?, description=?, responsibilities_json=?, skills_required=?, seniority_level=?, sort_order=?, updated_at=? WHERE id=?",
			p.Name, p.Description, p.ResponsibilitiesJSON, string(skillsJSON), p.SeniorityLevel, p.SortOrder, now, existing.ID,
		)
		if updateErr != nil {
			return biz.Position{}, kerrors.InternalServer("POSITION", updateErr.Error())
		}
		p.ID = existing.ID
		p.UpdatedAt = now
		return p, nil
	}
	return r.CreatePosition(ctx, p)
}

func (r *positionRepo) GetPositionWithAncestors(ctx context.Context, positionKey string) (biz.PositionAncestors, error) {
	query := `SELECT
		p.id, p.key, p.name, p.department_key, p.description, p.responsibilities_json, p.skills_required, p.seniority_level, p.sort_order, p.created_at, p.updated_at, p.deleted_at,
		d.id, d.key, d.name, d.industry_key, d.description, d.responsibilities_json, d.sort_order, d.created_at, d.updated_at, d.deleted_at,
		i.id, i.key, i.name, i.icon, i.description, i.scenario_key, i.enabled, i.sort_order, i.created_at, i.updated_at, i.deleted_at
	FROM positions p
	JOIN departments d ON d.key = p.department_key AND d.deleted_at = ''
	JOIN industries i ON i.key = d.industry_key AND i.deleted_at = ''
	WHERE p.key = ? AND p.deleted_at = ''`
	row := r.data.RawDB().QueryRowContext(ctx, query, positionKey)

	var pRow positionRow
	var dID, dKey, dName, dIndKey, dDesc, dRespJSON string
	var dSort int
	var dCreatedAt, dUpdatedAt, dDeletedAt string
	var iID, iKey, iName, iIcon, iDesc, iScenarioKey string
	var iEnabled bool
	var iSort int
	var iCreatedAt, iUpdatedAt, iDeletedAt string

	err := row.Scan(
		&pRow.ID, &pRow.Key, &pRow.Name, &pRow.DepartmentKey, &pRow.Description, &pRow.ResponsibilitiesJSON, &pRow.SkillsRequiredJSON, &pRow.SeniorityLevel, &pRow.SortOrder, &pRow.CreatedAt, &pRow.UpdatedAt, &pRow.DeletedAt,
		&dID, &dKey, &dName, &dIndKey, &dDesc, &dRespJSON, &dSort, &dCreatedAt, &dUpdatedAt, &dDeletedAt,
		&iID, &iKey, &iName, &iIcon, &iDesc, &iScenarioKey, &iEnabled, &iSort, &iCreatedAt, &iUpdatedAt, &iDeletedAt,
	)
	if err == sql.ErrNoRows {
		return biz.PositionAncestors{}, kerrors.NotFound("POSITION", "position not found with ancestors")
	}
	if err != nil {
		return biz.PositionAncestors{}, kerrors.InternalServer("POSITION", err.Error())
	}
	return biz.PositionAncestors{
		Position:   posRowToBiz(r.data.lg, pRow),
		Department: biz.Department{ID: dID, Key: dKey, Name: dName, IndustryKey: dIndKey, Description: dDesc, ResponsibilitiesJSON: dRespJSON, SortOrder: dSort, CreatedAt: dCreatedAt, UpdatedAt: dUpdatedAt, DeletedAt: dDeletedAt},
		Industry:   biz.Industry{ID: iID, Key: iKey, Name: iName, Icon: iIcon, Description: iDesc, ScenarioKey: iScenarioKey, Enabled: iEnabled, SortOrder: iSort, CreatedAt: iCreatedAt, UpdatedAt: iUpdatedAt, DeletedAt: iDeletedAt},
	}, nil
}
