package data

import (
	"context"
	"strings"

	bridge "aranea-agents/internal/biz/agentbridge"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/codingproject"
	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

// codingProjectRepo implements bridge.ProjectRepo via Ent.
type codingProjectRepo struct {
	data *Data
}

var _ bridge.ProjectRepo = (*codingProjectRepo)(nil)

// NewCodingProjectRepo returns the Ent-backed ProjectRepo.
func NewCodingProjectRepo(d *Data) bridge.ProjectRepo {
	return &codingProjectRepo{data: d}
}

func entCodingProjectToBiz(e *dataent.CodingProject) *bridge.CodingProject {
	return &bridge.CodingProject{
		ID:          e.ID,
		Workspace:   e.Workspace,
		Name:        e.Name,
		Path:        e.Path,
		Description: e.Description,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

func (r *codingProjectRepo) GetByName(ctx context.Context, workspace, name string) (*bridge.CodingProject, error) {
	row, err := r.data.RW().Read(ctx).CodingProject.Query().
		Where(codingproject.WorkspaceEQ(workspace), codingproject.NameEQ(name)).
		Only(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	return entCodingProjectToBiz(row), nil
}

// Match 按 精确 → 前缀 → 包含 排序返回候选（大小写不敏感）。
// 项目注册表规模小（本机目录白名单），内存分类排序足够。
func (r *codingProjectRepo) Match(ctx context.Context, workspace, query string) ([]*bridge.CodingProject, error) {
	rows, err := r.data.RW().Read(ctx).CodingProject.Query().
		Where(codingproject.WorkspaceEQ(workspace)).
		Order(dataent.Asc(codingproject.FieldName)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	var exact, prefix, contains []*bridge.CodingProject
	for _, row := range rows {
		name := strings.ToLower(row.Name)
		switch {
		case name == q:
			exact = append(exact, entCodingProjectToBiz(row))
		case strings.HasPrefix(name, q):
			prefix = append(prefix, entCodingProjectToBiz(row))
		case strings.Contains(name, q):
			contains = append(contains, entCodingProjectToBiz(row))
		}
	}
	if len(exact) > 0 {
		return exact, nil
	}
	out := make([]*bridge.CodingProject, 0, len(prefix)+len(contains))
	out = append(out, prefix...)
	out = append(out, contains...)
	return out, nil
}

func (r *codingProjectRepo) List(ctx context.Context, workspace string) ([]*bridge.CodingProject, error) {
	rows, err := r.data.RW().Read(ctx).CodingProject.Query().
		Where(codingproject.WorkspaceEQ(workspace)).
		Order(dataent.Asc(codingproject.FieldName)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	out := make([]*bridge.CodingProject, 0, len(rows))
	for _, row := range rows {
		out = append(out, entCodingProjectToBiz(row))
	}
	return out, nil
}

func (r *codingProjectRepo) Upsert(ctx context.Context, p *bridge.CodingProject) error {
	now := nowRFC3339()
	if p.Workspace == "" {
		p.Workspace = "default"
	}
	create := r.data.RW().Write(ctx).CodingProject.Create().
		SetWorkspace(p.Workspace).
		SetName(p.Name).
		SetPath(p.Path).
		SetDescription(p.Description).
		SetCreatedAt(now).
		SetUpdatedAt(now)
	if p.ID != "" {
		create = create.SetID(p.ID)
	} else {
		create = create.SetID("codingproj_" + uuid.NewString())
	}
	err := create.
		OnConflictColumns(codingproject.FieldWorkspace, codingproject.FieldName).
		Update(func(u *dataent.CodingProjectUpsert) {
			u.UpdatePath()
			u.UpdateDescription()
			u.UpdateUpdatedAt()
		}).
		Exec(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	row, err := r.data.RW().Read(ctx).CodingProject.Query().
		Where(codingproject.WorkspaceEQ(p.Workspace), codingproject.NameEQ(p.Name)).
		Only(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	*p = *entCodingProjectToBiz(row)
	return nil
}

func (r *codingProjectRepo) Delete(ctx context.Context, id string) error {
	n, err := r.data.RW().Write(ctx).CodingProject.Delete().
		Where(codingproject.ID(id)).
		Exec(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	if n == 0 {
		return apierror.NotFound(apierror.DomainAgentBridge, "coding project not found: "+id)
	}
	return nil
}
