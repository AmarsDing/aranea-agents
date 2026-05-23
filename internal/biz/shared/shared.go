// Package shared contains cross-aggregate value objects, error sentinels, and
// generic helpers used throughout the biz layer. No Usecase or Repo lives here.
package shared

import (
	"encoding/json"
	stderrors "errors"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
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
	ErrGraphSaveRun          = errors.InternalServer("GRAPH", "graph execute save run failed")
	ErrGraphInvalidStatus    = errors.BadRequest("GRAPH", "cannot cancel execution in current status")
	ErrGraphResume           = errors.InternalServer("GRAPH", "graph resume failed")
	ErrGraphTemplateNotFound = errors.NotFound("GRAPH_TEMPLATE", "graph template not found")

	// Usage / Quota
	ErrQuotaUnsupportedScope = errors.BadRequest("USAGE_QUOTA", "unsupported quota scope_type")
)

// ── JSON helpers ──────────────────────────────────────────────────────────────

// JSONStringList parses a JSON string array from agent settings or API payloads.
func JSONStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "{}" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil
	}
	return list
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
		return errors.InternalServer(module, "schema validation error: "+err.Error())
	}
	if result.Valid() {
		return nil
	}
	var msgs []string
	for _, desc := range result.Errors() {
		msgs = append(msgs, desc.String())
	}
	return errors.BadRequest(module, "config does not match schema: "+strings.Join(msgs, "; "))
}
