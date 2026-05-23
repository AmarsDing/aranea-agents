package biz

// GraphValidationIssue is a single validation error or warning at the biz boundary.
type GraphValidationIssue struct {
	Code    string
	NodeID  string
	Field   string
	Message string
}

// GraphValidationResult is the biz-layer validation outcome (no trpc types).
type GraphValidationResult struct {
	Errors   []GraphValidationIssue
	Warnings []GraphValidationIssue
}

func (r *GraphValidationResult) HasErrors() bool {
	return r != nil && len(r.Errors) > 0
}

func (r *GraphValidationResult) HasWarnings() bool {
	return r != nil && len(r.Warnings) > 0
}
