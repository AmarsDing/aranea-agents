package biz

import "aranea-agents/internal/biz/shared"

// Re-export error sentinels from shared sub-package for backward compatibility.
var (
	ErrUsageScopeRequired    = shared.ErrUsageScopeRequired
	ErrBudgetAlertNotFound   = shared.ErrBudgetAlertNotFound
	ErrQuotaNotFound         = shared.ErrQuotaNotFound
	ErrMessageDuplicate      = shared.ErrMessageDuplicate
	ErrAdminNotFound         = shared.ErrAdminNotFound
	ErrNotFound              = shared.ErrNotFound
	ErrGraphSaveRun          = shared.ErrGraphSaveRun
	ErrGraphInvalidStatus    = shared.ErrGraphInvalidStatus
	ErrGraphResume           = shared.ErrGraphResume
	ErrGraphTemplateNotFound = shared.ErrGraphTemplateNotFound
	ErrQuotaUnsupportedScope = shared.ErrQuotaUnsupportedScope
	ErrAgentKeyConflict      = shared.ErrAgentKeyConflict
)
