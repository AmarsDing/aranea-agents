package biz

import (
	"context"
)

type PositionAncestors struct {
	Industry   Industry
	Department Department
	Position   Position
}

type PositionReader interface {
	ListPositions(ctx context.Context, q PositionListQuery) (PositionListResult, error)
	GetPositionByKey(ctx context.Context, key, departmentKey string) (Position, error)
	GetPositionWithAncestors(ctx context.Context, positionKey string) (PositionAncestors, error)
}

type PositionWriter interface {
	CreatePosition(ctx context.Context, p Position) (Position, error)
	UpsertPositionByKey(ctx context.Context, p Position) (Position, error)
}

type PositionRepository interface {
	PositionReader
	PositionWriter
}

type PositionUsecase struct {
	repo  PositionRepository
	catUC *AgentCategoryUsecase
}

func NewPositionUsecase(repo PositionRepository, catUC *AgentCategoryUsecase) *PositionUsecase {
	return &PositionUsecase{repo: repo, catUC: catUC}
}

func (u *PositionUsecase) ListByDepartment(ctx context.Context, departmentKey string) (PositionListResult, error) {
	if departmentKey == "" {
		return PositionListResult{}, ErrCategoryBadRequest("department_key is required")
	}
	return u.repo.ListPositions(ctx, PositionListQuery{DepartmentKey: departmentKey})
}

func (u *PositionUsecase) UpsertByKey(ctx context.Context, p Position) (Position, error) {
	if p.Key == "" || p.DepartmentKey == "" {
		return Position{}, ErrCategoryBadRequest("key and department_key are required")
	}
	return u.repo.UpsertPositionByKey(ctx, p)
}

func (u *PositionUsecase) GetWithAncestors(ctx context.Context, positionKey string) (PositionAncestors, error) {
	if positionKey == "" {
		return PositionAncestors{}, ErrCategoryBadRequest("position_key is required")
	}
	return u.repo.GetPositionWithAncestors(ctx, positionKey)
}

func (u *PositionUsecase) GetPositionPrompt(ctx context.Context, industryKey, positionKey, variant string) (PositionPromptResult, error) {
	return u.catUC.GetPositionPrompt(ctx, industryKey, positionKey, variant)
}

func (u *PositionUsecase) ListPositionVariants(ctx context.Context, industryKey, positionKey string) ([]VariantInfo, error) {
	return u.catUC.ListPositionVariants(ctx, industryKey, positionKey)
}

func (u *PositionUsecase) GetAncestors(ctx context.Context, positionID string) (CategoryAncestors, error) {
	return u.catUC.GetAncestors(ctx, positionID)
}
