// Package shared contains cross-aggregate value objects, error sentinels, and
// generic helpers used throughout the biz layer. No Usecase or Repo lives here.
package shared

import (
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"
	"github.com/xeipuuv/gojsonschema"
	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
)

// ── Pagination ────────────────────────────────────────────────────────────────

// PageToLimitOffset normalizes page / page_size for API responses.
// Defaults: page≥1, size default 20, max 100.
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

// ListOption configures a ListOptions.
type ListOption func(*ListOptions)

// ListOptions holds filter, ordering, and pagination for list queries.
type ListOptions struct {
	Filter  filtering.Filter
	OrderBy ordering.OrderBy
	Offset  int
	Limit   int
}

// ListFilter sets the AIP filter.
func ListFilter(filter filtering.Filter) ListOption {
	return func(o *ListOptions) { o.Filter = filter }
}

// ListOrderBy sets the AIP order-by.
func ListOrderBy(orderBy ordering.OrderBy) ListOption {
	return func(o *ListOptions) { o.OrderBy = orderBy }
}

// ListOffset sets the offset.
func ListOffset(offset int) ListOption {
	return func(o *ListOptions) { o.Offset = offset }
}

// ListLimit sets the limit.
func ListLimit(limit int) ListOption {
	return func(o *ListOptions) { o.Limit = limit }
}

// ── Error sentinels ───────────────────────────────────────────────────────────

// Cross-aggregate error sentinels (apierror). All new sentinels must use
// apierror constructors so that APIToKratos middleware can translate them
// automatically without mapXxxError functions in the service layer.
var (
	// Usage / Quota
	ErrUsageScopeRequired  = apierror.BadRequest(apierror.DomainUsageQuota, "usage scope required")
	ErrBudgetAlertNotFound = apierror.NotFound(apierror.DomainUsageQuota, "budget alert not found after upsert")
	ErrQuotaNotFound       = apierror.NotFound(apierror.DomainUsageQuota, "usage quota not configured")

	// Data constraints
	ErrMessageDuplicate = apierror.Conflict(apierror.DomainData, "message duplicate constraint")
	ErrAgentKeyConflict = apierror.Conflict(apierror.DomainAgent, "agent_key unique constraint violation")
)

var (
	// Admin
	ErrAdminNotFound = apierror.NotFound(apierror.DomainAdmin, "admin not found")

	// General
	ErrNotFound = apierror.NotFound(apierror.DomainShared, "resource not found")

	// Graph
	ErrGraphSaveRun          = apierror.Internal(apierror.DomainGraph, "graph execute save run failed")
	ErrGraphInvalidStatus    = apierror.BadRequest(apierror.DomainGraph, "cannot cancel execution in current status")
	ErrGraphResume           = apierror.Internal(apierror.DomainGraph, "graph resume failed")
	ErrGraphTemplateNotFound = apierror.NotFound(apierror.DomainGraphTemplate, "graph template not found")
	// ErrGraphCheckpointMissing 崩溃续跑安全闸（83-长时运行韧性）：lineage 无可用
	// checkpoint 时拒绝恢复，避免 BuildAndResume 静默 fresh-start 造成副作用重跑。
	ErrGraphCheckpointMissing = apierror.NotFound(apierror.DomainGraph, "graph checkpoint missing for lineage; cannot crash-resume")

	// Usage / Quota
	ErrQuotaUnsupportedScope = apierror.BadRequest(apierror.DomainUsageQuota, "unsupported quota scope_type")
)

// ── JSON helpers ──────────────────────────────────────────────────────────────

// JSONStringList parses a JSON string array from agent settings or API payloads.
func JSONStringList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "{}" {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, apierror.BadRequest("SHARED", "json string list parse: "+err.Error())
	}
	return list, nil
}

// ValidateDocumentAgainstSchema validates a JSON document against a JSON Schema.
func ValidateDocumentAgainstSchema(module, schemaJSON, docJSON string) error {
	schemaJSON = strings.TrimSpace(schemaJSON)
	docJSON = strings.TrimSpace(docJSON)
	if schemaJSON == "" || schemaJSON == "{}" {
		return nil
	}
	if docJSON == "" {
		docJSON = "{}"
	}
	result, err := gojsonschema.Validate(
		gojsonschema.NewStringLoader(schemaJSON),
		gojsonschema.NewStringLoader(docJSON),
	)
	if err != nil {
		return apierror.Internal(module, "schema validation error: "+err.Error())
	}
	if result.Valid() {
		return nil
	}
	var msgs []string
	for _, desc := range result.Errors() {
		msgs = append(msgs, desc.String())
	}
	return apierror.BadRequest(module, "config does not match schema: "+strings.Join(msgs, "; "))
}
