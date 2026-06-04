package data

import (
	"context"
	_ "embed"

	"aranea-agents/internal/data/ent"
)

//go:embed sql/migrations/20260715_self_check_report_schema.sql
var selfCheckReportDDL string

// EnsureSelfCheckReportSchema creates self_check_reports if missing.
func EnsureSelfCheckReportSchema(ctx context.Context, client *ent.Client) error {
	return execDDLFile(ctx, client, selfCheckReportDDL, "self_check_report")
}
