package data

import (
	"context"
	"database/sql"
	"os"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// EnsureSessionTableSplit creates session_metrics and session_runtime tables
// and backfills data from the sessions table.
func EnsureSessionTableSplit(ctx context.Context, db *sql.DB, lg loggateway.Logger) error {
	if db == nil {
		return nil
	}
	sqlBytes, err := os.ReadFile("sql/migrations/20260708_session_table_split.sql")
	if err != nil {
		lg.Error("read session_table_split sql failed",
			loggateway.StepID("data.session_table_split.read"),
			loggateway.Err(err))
		return err
	}
	ddl := strings.TrimPrefix(string(sqlBytes), "\ufeff")
	for _, stmt := range splitDDLStatements(strings.TrimSpace(ddl)) {
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			lg.Error("session_table_split ddl failed",
				loggateway.StepID("data.session_table_split.exec"),
				loggateway.Err(err))
			return err
		}
	}
	return nil
}
