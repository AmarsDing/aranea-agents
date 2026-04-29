package biz

import (
	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
)

// normalized page / page_size for API responses (defaults: page≥1, size default 20, max 100).
func PageToLimitOffset(page, pageSize int32) (limit, offset int, pageOut, pageSizeOut int32) {
	p := int(page)
	ps := int(pageSize)
	if p < 1 {
		p = 1
	}
	if ps < 1 {
		ps = 20
	}
	if ps > 100 {
		ps = 100
	}
	return ps, (p - 1) * ps, int32(p), int32(ps)
}

type ListOption func(*ListOptions)

type ListOptions struct {
	Filter  filtering.Filter
	OrderBy ordering.OrderBy
	Offset  int
	Limit   int
}

func ListFilter(filter filtering.Filter) ListOption {
	return func(o *ListOptions) {
		o.Filter = filter
	}
}

func ListOrderBy(orderBy ordering.OrderBy) ListOption {
	return func(o *ListOptions) {
		o.OrderBy = orderBy
	}
}

func ListOffset(offset int) ListOption {
	return func(o *ListOptions) {
		o.Offset = offset
	}
}

func ListLimit(limit int) ListOption {
	return func(o *ListOptions) {
		o.Limit = limit
	}
}
