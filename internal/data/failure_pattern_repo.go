package data

import (
	"context"
	"encoding/json"
	"time"

	bizmonitor "aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/apierror"
)

type failurePatternRepo struct {
	data *Data
}

var _ bizmonitor.FailurePatternReader = (*failurePatternRepo)(nil)
var _ bizmonitor.FailurePatternWriter = (*failurePatternRepo)(nil)
var _ bizmonitor.FailurePatternReader = (*FailurePatternReadWriter)(nil)
var _ bizmonitor.FailurePatternWriter = (*FailurePatternReadWriter)(nil)

// FailurePatternReadWriter combines FailurePatternReader and FailurePatternWriter.
// Used as a Wire binding target since the concrete repo implements both interfaces.
type FailurePatternReadWriter struct {
	*failurePatternRepo
}

// NewFailurePatternRepo creates a new FailurePatternReadWriter backed by raw SQL.
func NewFailurePatternRepo(d *Data) *FailurePatternReadWriter {
	return &FailurePatternReadWriter{failurePatternRepo: &failurePatternRepo{data: d}}
}

func (r *failurePatternRepo) Create(ctx context.Context, pattern bizmonitor.FailurePattern) error {
	if r == nil || r.data == nil {
		return apierror.Internal("FAILURE_PATTERN", "database not configured")
	}

	fixActionJSON, err := json.Marshal(pattern.FixAction)
	if err != nil {
		return entErrToBizErr(err, "FAILURE_PATTERN")
	}

	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO failure_pattern (id, source, type, pattern_hash, pattern_regex, fix_action,
			confidence, success_count, fail_count, version, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pattern.ID,
		string(pattern.Source),
		pattern.Type,
		pattern.PatternHash,
		pattern.PatternRegex,
		string(fixActionJSON),
		pattern.Confidence,
		pattern.SuccessCount,
		pattern.FailCount,
		pattern.Version,
		pattern.IsActive,
		pattern.CreatedAt.Format(time.RFC3339Nano),
		pattern.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *failurePatternRepo) Update(ctx context.Context, pattern bizmonitor.FailurePattern) error {
	if r == nil || r.data == nil {
		return apierror.Internal("FAILURE_PATTERN", "database not configured")
	}

	fixActionJSON, err := json.Marshal(pattern.FixAction)
	if err != nil {
		return entErrToBizErr(err, "FAILURE_PATTERN")
	}

	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE failure_pattern SET source=?, type=?, pattern_hash=?, pattern_regex=?, fix_action=?,
			confidence=?, success_count=?, fail_count=?, version=?, is_active=?, updated_at=?
		 WHERE id=?`,
		string(pattern.Source),
		pattern.Type,
		pattern.PatternHash,
		pattern.PatternRegex,
		string(fixActionJSON),
		pattern.Confidence,
		pattern.SuccessCount,
		pattern.FailCount,
		pattern.Version,
		pattern.IsActive,
		pattern.UpdatedAt.Format(time.RFC3339Nano),
		pattern.ID,
	)
	return err
}

func (r *failurePatternRepo) ListBySource(ctx context.Context, source bizmonitor.FailurePatternSource) ([]bizmonitor.FailurePattern, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("FAILURE_PATTERN", "database not configured")
	}

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, source, type, pattern_hash, pattern_regex, fix_action,
			confidence, success_count, fail_count, version, is_active, created_at, updated_at
		 FROM failure_pattern WHERE source = ? ORDER BY confidence DESC`,
		string(source),
	)
	if err != nil {
		return nil, entErrToBizErr(err, "FAILURE_PATTERN")
	}
	defer rows.Close()

	return scanFailurePatterns(rows)
}

func (r *failurePatternRepo) GetByPatternHash(ctx context.Context, hash string) (*bizmonitor.FailurePattern, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("FAILURE_PATTERN", "database not configured")
	}

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, source, type, pattern_hash, pattern_regex, fix_action,
			confidence, success_count, fail_count, version, is_active, created_at, updated_at
		 FROM failure_pattern WHERE pattern_hash = ? LIMIT 1`,
		hash,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "FAILURE_PATTERN")
	}
	defer rows.Close()

	patterns, err := scanFailurePatterns(rows)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return nil, nil
	}
	return &patterns[0], nil
}

func (r *failurePatternRepo) ListActive(ctx context.Context) ([]bizmonitor.FailurePattern, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("FAILURE_PATTERN", "database not configured")
	}

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, source, type, pattern_hash, pattern_regex, fix_action,
			confidence, success_count, fail_count, version, is_active, created_at, updated_at
		 FROM failure_pattern WHERE is_active = 1 ORDER BY confidence DESC`,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "FAILURE_PATTERN")
	}
	defer rows.Close()

	return scanFailurePatterns(rows)
}

func (r *failurePatternRepo) IncrementSuccess(ctx context.Context, id string) error {
	if r == nil || r.data == nil {
		return apierror.Internal("FAILURE_PATTERN", "database not configured")
	}

	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE failure_pattern SET success_count = success_count + 1, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

func (r *failurePatternRepo) IncrementFail(ctx context.Context, id string) error {
	if r == nil || r.data == nil {
		return apierror.Internal("FAILURE_PATTERN", "database not configured")
	}

	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE failure_pattern SET fail_count = fail_count + 1, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

func (r *failurePatternRepo) Deactivate(ctx context.Context, id string) error {
	if r == nil || r.data == nil {
		return apierror.Internal("FAILURE_PATTERN", "database not configured")
	}

	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE failure_pattern SET is_active = 0, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

func scanFailurePatterns(rows interface {
	Close() error
	Next() bool
	Scan(dest ...any) error
}) ([]bizmonitor.FailurePattern, error) {
	var patterns []bizmonitor.FailurePattern
	for rows.Next() {
		var (
			id, source, fpType, patternHash, patternRegex string
			fixActionJSON                                string
			confidence                                   float64
			successCount, failCount, version             int
			isActive                                     bool
			createdAtStr, updatedAtStr                   string
		)
		if err := rows.Scan(&id, &source, &fpType, &patternHash, &patternRegex, &fixActionJSON,
			&confidence, &successCount, &failCount, &version, &isActive, &createdAtStr, &updatedAtStr); err != nil {
			return nil, entErrToBizErr(err, "FAILURE_PATTERN")
		}

		var fixAction bizmonitor.FixAction
		if err := json.Unmarshal([]byte(fixActionJSON), &fixAction); err != nil {
			fixAction = bizmonitor.FixAction{Type: "log_only"}
		}

		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)

		patterns = append(patterns, bizmonitor.FailurePattern{
			ID: id, Source: bizmonitor.FailurePatternSource(source), Type: fpType,
			PatternHash: patternHash, PatternRegex: patternRegex, FixAction: fixAction,
			Confidence: confidence, SuccessCount: successCount, FailCount: failCount,
			Version: version, IsActive: isActive, CreatedAt: createdAt, UpdatedAt: updatedAt,
		})
	}
	return patterns, nil
}
