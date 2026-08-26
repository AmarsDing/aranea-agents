package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/pkg/loggateway"
)

// decisionRepo is the raw-SQL implementation of decision.Repo (M80 Phase 1).
// Tables are created by DDL migrations 20261250/51; writes are idempotent on
// decision_key so worker retries and outbox replay never duplicate rows.
type decisionRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ decision.Repo = (*decisionRepo)(nil)

// NewDecisionRepo constructs the decision record repo; nil when DB is absent.
func NewDecisionRepo(data *Data, lg loggateway.Logger) decision.Repo {
	if data == nil || data.RWDB() == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &decisionRepo{data: data, lg: lg.With(loggateway.Domain("decision_repo"))}
}

// NewDecisionRepoFromData is the Wire-friendly constructor.
func NewDecisionRepoFromData(d *Data) decision.Repo {
	if d == nil {
		return nil
	}
	return NewDecisionRepo(d, d.lg)
}

const decisionRecordCols = "decision_key, category, scenario, reasoning, outcome, confidence, " +
	"actor_type, actor_key, parent_decision_id, related_entities, source_ref, metadata, " +
	"workspace_id, created_at, updated_at"

// InsertRecords batch-inserts rows inside one transaction, skipping duplicates
// on decision_key (replay/retry safe).
func (r *decisionRepo) InsertRecords(ctx context.Context, recs []decision.Record) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || len(recs) == 0 {
		return nil
	}
	d := r.data.Dialect()
	q := d.BuildInsertOrIgnore("decision_records", decisionRecordCols, d.Placeholders(15), "decision_key")

	tx, err := r.data.RWDB().WriteHandle().BeginTx(ctx, nil)
	if err != nil {
		return entErrToBizErr(err, "DECISION")
	}
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		_ = tx.Rollback()
		return entErrToBizErr(err, "DECISION")
	}
	defer stmt.Close()
	for i := range recs {
		if err := execDecisionRecordInsert(ctx, stmt, recs[i]); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return entErrToBizErr(err, "DECISION")
	}
	return nil
}

func execDecisionRecordInsert(ctx context.Context, stmt *sql.Stmt, rec decision.Record) error {
	entities, err := json.Marshal(rec.RelatedEntities)
	if err != nil {
		return entErrToBizErr(err, "DECISION")
	}
	sourceRef, err := json.Marshal(rec.SourceRef)
	if err != nil {
		return entErrToBizErr(err, "DECISION")
	}
	metadata := []byte("{}")
	if rec.Metadata != nil {
		if metadata, err = json.Marshal(rec.Metadata); err != nil {
			return entErrToBizErr(err, "DECISION")
		}
	}
	var confidence, parent any
	if rec.Confidence != nil {
		confidence = *rec.Confidence
	}
	if rec.ParentDecisionID != nil {
		parent = *rec.ParentDecisionID
	}
	_, err = stmt.ExecContext(ctx,
		rec.DecisionKey, string(rec.Category), rec.Scenario, rec.Reasoning, rec.Outcome,
		confidence, string(rec.ActorType), rec.ActorKey, parent,
		string(entities), string(sourceRef), string(metadata),
		rec.WorkspaceID, rec.CreatedAt, rec.UpdatedAt,
	)
	if err != nil {
		return entErrToBizErr(err, "DECISION")
	}
	return nil
}

// EnqueueOutbox persists records into the retry queue (idempotent on
// decision_key). Payload is the codec-stable JSON form.
func (r *decisionRepo) EnqueueOutbox(ctx context.Context, recs []decision.Record) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || len(recs) == 0 {
		return nil
	}
	d := r.data.Dialect()
	q := d.BuildInsertOrIgnore("decision_record_outbox",
		"decision_key, payload, created_at", d.Placeholders(3), "decision_key")

	tx, err := r.data.RWDB().WriteHandle().BeginTx(ctx, nil)
	if err != nil {
		return entErrToBizErr(err, "DECISION")
	}
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		_ = tx.Rollback()
		return entErrToBizErr(err, "DECISION")
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range recs {
		payload, err := marshalOutboxPayload(recs[i])
		if err != nil {
			_ = tx.Rollback()
			return entErrToBizErr(err, "DECISION")
		}
		createdAt := recs[i].CreatedAt
		if createdAt == "" {
			createdAt = now
		}
		if _, err := stmt.ExecContext(ctx, recs[i].DecisionKey, payload, createdAt); err != nil {
			_ = tx.Rollback()
			return entErrToBizErr(err, "DECISION")
		}
	}
	if err := tx.Commit(); err != nil {
		return entErrToBizErr(err, "DECISION")
	}
	return nil
}

// marshalOutboxPayload mirrors decision's recordPayload wire form without
// importing the unexported type: the data layer must not depend on biz
// internals, and the wire contract is stable JSON keyed by column names.
func marshalOutboxPayload(rec decision.Record) ([]byte, error) {
	return json.Marshal(map[string]any{
		"decision_key":       rec.DecisionKey,
		"category":           string(rec.Category),
		"scenario":           rec.Scenario,
		"reasoning":          rec.Reasoning,
		"outcome":            rec.Outcome,
		"confidence":         rec.Confidence,
		"actor_type":         string(rec.ActorType),
		"actor_key":          rec.ActorKey,
		"parent_decision_id": rec.ParentDecisionID,
		"related_entities":   rec.RelatedEntities,
		"source_ref":         rec.SourceRef,
		"metadata":           rec.Metadata,
		"workspace_id":       rec.WorkspaceID,
		"created_at":         rec.CreatedAt,
		"updated_at":         rec.UpdatedAt,
	})
}

// ListPendingOutbox returns pending retry-queue rows oldest-first.
func (r *decisionRepo) ListPendingOutbox(ctx context.Context, limit int) ([]decision.OutboxRow, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`SELECT id, decision_key, payload, status, attempts, last_error, created_at, published_at
		 FROM decision_record_outbox WHERE status=? ORDER BY created_at ASC LIMIT ?`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, decision.OutboxStatusPending, limit)
	if err != nil {
		return nil, entErrToBizErr(err, "DECISION")
	}
	defer rows.Close()
	out := make([]decision.OutboxRow, 0, limit)
	for rows.Next() {
		var row decision.OutboxRow
		var published sql.NullString
		var created string
		if err := rows.Scan(&row.ID, &row.DecisionKey, &row.Payload, &row.Status,
			&row.Attempts, &row.LastError, &created, &published); err != nil {
			return nil, entErrToBizErr(err, "DECISION")
		}
		row.CreatedAt = parseRFC3339Loose(created)
		if published.Valid && published.String != "" {
			if t := parseRFC3339Loose(published.String); !t.IsZero() {
				row.PublishedAt = &t
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "DECISION")
	}
	return out, nil
}

// MarkOutboxPublished marks replayed rows published.
func (r *decisionRepo) MarkOutboxPublished(ctx context.Context, ids []int64, publishedAt time.Time) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || len(ids) == 0 {
		return nil
	}
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}
	marks := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, decision.OutboxStatusPublished, publishedAt.UTC().Format(time.RFC3339Nano))
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id)
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`UPDATE decision_record_outbox SET status=?, published_at=? WHERE id IN (` +
			strings.Join(marks, ",") + `)`)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, args...)
	if err != nil {
		return entErrToBizErr(err, "DECISION")
	}
	return nil
}

// MarkOutboxAttempt records one failed replay attempt.
func (r *decisionRepo) MarkOutboxAttempt(ctx context.Context, id int64, lastError string) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || id <= 0 {
		return nil
	}
	if len(lastError) > 2000 {
		lastError = lastError[:2000]
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`UPDATE decision_record_outbox SET attempts=attempts+1, last_error=? WHERE id=?`)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, lastError, id)
	if err != nil {
		return entErrToBizErr(err, "DECISION")
	}
	return nil
}

// parseRFC3339Loose parses the TEXT timestamp convention (RFC3339 or
// RFC3339Nano); zero on unparseable input (callers treat as absent).
func parseRFC3339Loose(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
