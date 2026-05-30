package biz

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type DepartmentReader interface {
	ListDepartments(ctx context.Context, q DepartmentListQuery) (DepartmentListResult, error)
	GetDepartmentByKey(ctx context.Context, key, industryKey string) (Department, error)
}

type DepartmentWriter interface {
	CreateDepartment(ctx context.Context, d Department) (Department, error)
	UpsertDepartmentByKey(ctx context.Context, d Department) (Department, error)
}

type DepartmentRepository interface {
	DepartmentReader
	DepartmentWriter
}

type DepartmentUsecase struct {
	repo  DepartmentRepository
	catUC *AgentCategoryUsecase
}

func NewDepartmentUsecase(repo DepartmentRepository, catUC *AgentCategoryUsecase) *DepartmentUsecase {
	return &DepartmentUsecase{repo: repo, catUC: catUC}
}

func (u *DepartmentUsecase) ListByIndustry(ctx context.Context, industryKey string) (DepartmentListResult, error) {
	if industryKey == "" {
		return DepartmentListResult{}, kerrors.BadRequest("DEPARTMENT", "industry_key is required")
	}
	return u.repo.ListDepartments(ctx, DepartmentListQuery{IndustryKey: industryKey})
}

func (u *DepartmentUsecase) UpsertByKey(ctx context.Context, d Department) (Department, error) {
	if d.Key == "" || d.IndustryKey == "" {
		return Department{}, kerrors.BadRequest("DEPARTMENT", "key and industry_key are required")
	}
	return u.repo.UpsertDepartmentByKey(ctx, d)
}

func (u *DepartmentUsecase) ListByParentID(ctx context.Context, parentID string) ([]AgentCategory, error) {
	return u.catUC.ListByParentID(ctx, parentID)
}
