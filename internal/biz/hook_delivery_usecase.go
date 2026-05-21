package biz

import (
	"context"
)

// HookDeliveryUsecase lists persisted Hook notify deliveries.
type HookDeliveryUsecase struct {
	repo HookDeliveryRepo
}

// NewHookDeliveryUsecase creates a delivery query usecase.
func NewHookDeliveryUsecase(repo HookDeliveryRepo) *HookDeliveryUsecase {
	return &HookDeliveryUsecase{repo: repo}
}

// List returns paginated hook_deliveries rows.
func (u *HookDeliveryUsecase) List(ctx context.Context, q HookDeliveryQuery, page, pageSize int32) (HookDeliveryListResult, error) {
	if u == nil || u.repo == nil {
		return HookDeliveryListResult{}, nil
	}
	limit, offset, _, _ := PageToLimitOffset(page, pageSize)
	q.Limit = int32(limit)
	q.Offset = int32(offset)
	return u.repo.List(ctx, q)
}
