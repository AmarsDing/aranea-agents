package biz

import (
	stderrors "errors"

	"github.com/go-kratos/kratos/v2/errors"
)

// Data-layer sentinels (stdlib); mapped to kerrors in Usecase.
var (
	ErrUsageScopeRequired  = stderrors.New("usage scope required")
	ErrBudgetAlertNotFound = stderrors.New("budget alert not found after upsert")
	ErrQuotaNotFound       = stderrors.New("usage quota not configured")
)

var (
	// Admin
	ErrAdminNotFound = errors.NotFound("ADMIN", "admin not found")

	// General
	ErrNotFound = errors.NotFound("NOT_FOUND", "resource not found")

	// Graph
	ErrGraphSaveRun       = errors.InternalServer("GRAPH", "graph execute save run failed")
	ErrGraphInvalidStatus = errors.BadRequest("GRAPH", "cannot cancel execution in current status")
	ErrGraphResume        = errors.InternalServer("GRAPH", "graph resume failed")
	ErrGraphTemplateNotFound = errors.NotFound("GRAPH_TEMPLATE", "graph template not found")

	// Usage / Quota
	ErrQuotaUnsupportedScope = errors.BadRequest("USAGE_QUOTA", "unsupported quota scope_type")
)
