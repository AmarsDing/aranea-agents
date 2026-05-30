package biz

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type IndustryReader interface {
	ListIndustries(ctx context.Context, q IndustryListQuery) (IndustryListResult, error)
	GetIndustryByKey(ctx context.Context, key string) (Industry, error)
}

type IndustryWriter interface {
	CreateIndustry(ctx context.Context, ind Industry) (Industry, error)
	UpdateIndustry(ctx context.Context, ind Industry) (Industry, error)
	UpsertIndustryByKey(ctx context.Context, ind Industry) (Industry, error)
}

type IndustryRepository interface {
	IndustryReader
	IndustryWriter
}

type IndustryUsecase struct {
	repo  IndustryRepository
	catUC *AgentCategoryUsecase
}

func NewIndustryUsecase(repo IndustryRepository, catUC *AgentCategoryUsecase) *IndustryUsecase {
	return &IndustryUsecase{repo: repo, catUC: catUC}
}

func (u *IndustryUsecase) List(ctx context.Context, q IndustryListQuery) (IndustryListResult, error) {
	return u.repo.ListIndustries(ctx, q)
}

func (u *IndustryUsecase) GetByKey(ctx context.Context, key string) (Industry, error) {
	if key == "" {
		return Industry{}, kerrors.BadRequest("INDUSTRY", "key is required")
	}
	return u.repo.GetIndustryByKey(ctx, key)
}

func (u *IndustryUsecase) UpsertByKey(ctx context.Context, ind Industry) (Industry, error) {
	if ind.Key == "" {
		return Industry{}, kerrors.BadRequest("INDUSTRY", "key is required")
	}
	return u.repo.UpsertIndustryByKey(ctx, ind)
}

func (u *IndustryUsecase) ListByLevel(ctx context.Context, level string) ([]AgentCategory, error) {
	return u.catUC.ListByLevel(ctx, level)
}

func (u *IndustryUsecase) GetCategoryByKey(ctx context.Context, key string) (AgentCategory, error) {
	return u.catUC.GetByKey(ctx, key)
}
