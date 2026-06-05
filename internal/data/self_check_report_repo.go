package data

import (
	"context"
	"encoding/json"
	"time"

	bizmonitor "aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/types"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type selfCheckReportRepo struct {
	data *Data
}

var _ bizmonitor.SelfCheckReportRepo = (*selfCheckReportRepo)(nil)

// NewSelfCheckReportRepo creates a new SelfCheckReportRepo backed by raw SQL.
func NewSelfCheckReportRepo(d *Data) bizmonitor.SelfCheckReportRepo {
	return &selfCheckReportRepo{data: d}
}

func (r *selfCheckReportRepo) InsertSelfCheckReport(ctx context.Context, report bizmonitor.SelfCheckReport) error {
	if r == nil || r.data == nil {
		return kerrors.InternalServer("SELF_CHECK_REPORT", "database not configured")
	}

	checkResultsJSON, err := json.Marshal(report.CheckResults)
	if err != nil {
		return kerrors.InternalServer("SELF_CHECK_REPORT", "marshal check_results failed").WithCause(err)
	}
	repairActionsJSON, err := json.Marshal(report.RepairActions)
	if err != nil {
		return kerrors.InternalServer("SELF_CHECK_REPORT", "marshal repair_actions failed").WithCause(err)
	}

	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO self_check_reports (id, check_results_json, overall_status, repair_actions_json, started_at, finished_at, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		report.ID,
		string(checkResultsJSON),
		string(report.OverallStatus),
		string(repairActionsJSON),
		report.StartedAt.Format(time.RFC3339Nano),
		report.FinishedAt.Format(time.RFC3339Nano),
		report.DurationMs,
		report.StartedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *selfCheckReportRepo) ListSelfCheckReports(ctx context.Context, limit, offset int) ([]bizmonitor.SelfCheckReport, int, error) {
	if r == nil || r.data == nil {
		return nil, 0, kerrors.InternalServer("SELF_CHECK_REPORT", "database not configured")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		`SELECT COUNT(*) FROM self_check_reports`, nil, &total)
	if err != nil {
		return nil, 0, kerrors.InternalServer("SELF_CHECK_REPORT", "count failed").WithCause(err)
	}

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, check_results_json, overall_status, repair_actions_json, started_at, finished_at, duration_ms
		 FROM self_check_reports ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, 0, kerrors.InternalServer("SELF_CHECK_REPORT", "query failed").WithCause(err)
	}
	defer rows.Close()

	var reports []bizmonitor.SelfCheckReport
	for rows.Next() {
		var (
			id, overallStatus   string
			checkResultsJSON    string
			repairActionsJSON   string
			startedAtStr        string
			finishedAtStr       string
			durationMs          int64
		)
		if err := rows.Scan(&id, &checkResultsJSON, &overallStatus, &repairActionsJSON, &startedAtStr, &finishedAtStr, &durationMs); err != nil {
			return nil, 0, kerrors.InternalServer("SELF_CHECK_REPORT", "scan failed").WithCause(err)
		}

		var checkResults []types.SelfCheckResult
		if err := json.Unmarshal([]byte(checkResultsJSON), &checkResults); err != nil {
			checkResults = nil
		}

		var repairActions []bizmonitor.RepairOutcome
		if err := json.Unmarshal([]byte(repairActionsJSON), &repairActions); err != nil {
			repairActions = nil
		}

		startedAt, _ := time.Parse(time.RFC3339Nano, startedAtStr)
		finishedAt, _ := time.Parse(time.RFC3339Nano, finishedAtStr)

		reports = append(reports, bizmonitor.SelfCheckReport{
			ID:            id,
			CheckResults:  checkResults,
			OverallStatus: types.SelfCheckStatus(overallStatus),
			RepairActions: repairActions,
			StartedAt:     startedAt,
			FinishedAt:    finishedAt,
			DurationMs:    durationMs,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, kerrors.InternalServer("SELF_CHECK_REPORT", "rows error").WithCause(err)
	}

	return reports, total, nil
}

func (r *selfCheckReportRepo) DeleteSelfCheckReportsOlderThan(ctx context.Context, olderThan time.Time) (int, error) {
	if r == nil || r.data == nil {
		return 0, kerrors.InternalServer("SELF_CHECK_REPORT", "database not configured")
	}

	cutoff := olderThan.Format(time.RFC3339Nano)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`DELETE FROM self_check_reports WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, kerrors.InternalServer("SELF_CHECK_REPORT", "delete failed").WithCause(err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
