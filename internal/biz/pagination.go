package biz

import "aranea-agents/internal/biz/shared"

// Re-export pagination helpers from shared sub-package for backward compatibility.
var (
	PageToLimitOffset = shared.PageToLimitOffset

	ListFilter  = shared.ListFilter
	ListOrderBy = shared.ListOrderBy
	ListOffset  = shared.ListOffset
	ListLimit   = shared.ListLimit
)

// Re-export shared types for backward compatibility.
type (
	ListOption  = shared.ListOption
	ListOptions = shared.ListOptions
)
