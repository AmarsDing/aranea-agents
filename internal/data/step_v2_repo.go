package data

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/stepv2"
	"aranea-agents/pkg/loggateway"
)

// stepV2Repo implements biz.StepV2Repo.
// Stability:evolving
type stepV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.StepV2Repo = (*stepV2Repo)(nil)

// NewStepV2Repo creates a new StepV2Repo.
// Logger is preset with domain "STEP_V2" per loggateway convention.
func NewStepV2Repo(d *Data, lg loggateway.Logger) biz.StepV2Repo {
	return &stepV2Repo{data: d, lg: lg.With(loggateway.Domain("STEP_V2"))}
}

// GetStep returns the Step by ID.
func (r *stepV2Repo) GetStep(ctx context.Context, id string) (biz.Step, error) {
	if r == nil || r.data == nil {
		return biz.Step{}, fmt.Errorf("step v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).StepV2.Get(ctx, id)
	if err != nil {
		return biz.Step{}, entErrToBizErr(err, "STEP_V2")
	}
	return entStepV2ToBiz(row), nil
}

// ListStepsByTurn returns all steps for the given turn, ordered by seq asc.
func (r *stepV2Repo) ListStepsByTurn(ctx context.Context, turnID string) ([]biz.Step, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("step v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.TurnIDEQ(turnID)).
		Order(ent.Asc(stepv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "STEP_V2")
	}
	return entStepsV2ToBiz(rows), nil
}

// ListStepsByTask returns all steps for the given task, ordered by seq asc.
func (r *stepV2Repo) ListStepsByTask(ctx context.Context, taskID string) ([]biz.Step, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("step v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.TaskIDEQ(taskID)).
		Order(ent.Asc(stepv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "STEP_V2")
	}
	return entStepsV2ToBiz(rows), nil
}

// ListStepsBySession returns steps whose SessionID equals sessionID, ordered by started_at asc.
// SessionID is the owning chat session (spirit / member), not SpiritSessionID.
// Call with the spirit session ID for root-only history; call with a member
// session ID to lazy-load that member's steps (A.4.7).
func (r *stepV2Repo) ListStepsBySession(ctx context.Context, sessionID string) ([]biz.Step, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("step v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.SessionIDEQ(sessionID)).
		Order(ent.Asc(stepv2.FieldStartedAt)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "STEP_V2")
	}
	return entStepsV2ToBiz(rows), nil
}

// ListStepsBySessionPaged returns a page of steps for the session, always
// ordered by seq asc. Query plan: WHERE session_id=? [AND seq<before_seq]
// ORDER BY seq DESC LIMIT n+1 → hasMore = (len > n) → trim → reverse to ASC.
// Limit<=0 degrades to the legacy full list (started_at asc, hasMore=false).
func (r *stepV2Repo) ListStepsBySessionPaged(ctx context.Context, sessionID string, opts biz.StepListOptions) ([]biz.Step, bool, error) {
	if r == nil || r.data == nil {
		return nil, false, fmt.Errorf("step v2 repo: database not configured")
	}
	if opts.Limit <= 0 {
		rows, err := r.data.RW().Read(ctx).StepV2.Query().
			Where(stepv2.SessionIDEQ(sessionID)).
			Order(ent.Asc(stepv2.FieldStartedAt)).
			All(ctx)
		if err != nil {
			return nil, false, entErrToBizErr(err, "STEP_V2")
		}
		return entStepsV2ToBiz(rows), false, nil
	}
	q := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.SessionIDEQ(sessionID))
	if opts.BeforeSeq > 0 {
		q = q.Where(stepv2.SeqLT(opts.BeforeSeq))
	}
	rows, err := q.
		Order(ent.Desc(stepv2.FieldSeq)).
		Limit(opts.Limit + 1).
		All(ctx)
	if err != nil {
		return nil, false, entErrToBizErr(err, "STEP_V2")
	}
	hasMore := len(rows) > opts.Limit
	if hasMore {
		rows = rows[:opts.Limit]
	}
	// Reverse DESC → ASC so callers always receive ascending seq order.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return entStepsV2ToBiz(rows), hasMore, nil
}

// ListStepsBySpiritSession returns all steps under a spirit root session
// (spirit_session_id column), ordered by started_at asc. Includes member
// agent steps for StopGeneration cancel coverage.
func (r *stepV2Repo) ListStepsBySpiritSession(ctx context.Context, spiritSessionID string) ([]biz.Step, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("step v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.SpiritSessionID(spiritSessionID)).
		Order(ent.Asc(stepv2.FieldStartedAt)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "STEP_V2")
	}
	return entStepsV2ToBiz(rows), nil
}

// ListStepsBySessionID returns steps whose session_id equals the given session
// exactly (no tree expansion), ordered by started_at asc.
// Used by deliverable extraction to read a team main session's own reply step.
func (r *stepV2Repo) ListStepsBySessionID(ctx context.Context, sessionID string) ([]biz.Step, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("step v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.SessionIDEQ(sessionID)).
		Order(ent.Asc(stepv2.FieldStartedAt)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "STEP_V2")
	}
	return entStepsV2ToBiz(rows), nil
}

// MaxSeqBySpiritSession returns the highest Seq stored for the spirit session (0 if none).
func (r *stepV2Repo) MaxSeqBySpiritSession(ctx context.Context, spiritSessionID string) (int64, error) {
	if r == nil || r.data == nil {
		return 0, fmt.Errorf("step v2 repo: database not configured")
	}
	if spiritSessionID == "" {
		return 0, nil
	}
	row, err := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.SpiritSessionID(spiritSessionID)).
		Order(ent.Desc(stepv2.FieldSeq)).
		Select(stepv2.FieldSeq).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, nil
		}
		return 0, entErrToBizErr(err, "STEP_V2")
	}
	return row.Seq, nil
}

// CreateStep inserts a new Step with the caller's claimed Version.
func (r *stepV2Repo) CreateStep(ctx context.Context, s biz.Step) (biz.Step, error) {
	if r == nil || r.data == nil {
		return biz.Step{}, fmt.Errorf("step v2 repo: database not configured")
	}
	row, err := r.data.RW().Write(ctx).StepV2.Create().
		SetID(s.ID).
		SetTurnID(s.TurnID).
		SetTaskID(s.TaskID).
		SetSessionID(s.SessionID).
		SetSpiritSessionID(s.SpiritSessionID).
		SetKind(string(s.Kind)).
		SetAuthorAgentKey(s.AuthorAgentKey).
		SetSeq(s.Seq).
		SetContent(s.Content).
		SetReasoning(s.Reasoning).
		SetToolName(s.ToolName).
		SetToolCallID(s.ToolCallID).
		SetToolArgs(string(s.ToolArgs)).
		SetToolResult(string(s.ToolResult)).
		SetToolDurationMs(s.ToolDurationMs).
		SetToolErrorCode(s.ToolErrorCode).
		SetNoticeType(s.NoticeType).
		SetStatus(string(s.Status)).
		SetIsFinal(s.IsFinal).
		SetStartedAt(s.StartedAt).
		SetVersion(s.Version).
		Save(ctx)
	if err != nil {
		return biz.Step{}, entErrToBizErr(err, "STEP_V2")
	}
	return entStepV2ToBiz(row), nil
}

// UpdateStep patches mutable fields without version guard.
// Use UpsertStep for concurrent-safe writes.
func (r *stepV2Repo) UpdateStep(ctx context.Context, s biz.Step) (biz.Step, error) {
	if r == nil || r.data == nil {
		return biz.Step{}, fmt.Errorf("step v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).StepV2.UpdateOneID(s.ID).
		SetContent(s.Content).
		SetReasoning(s.Reasoning).
		SetToolName(s.ToolName).
		SetToolCallID(s.ToolCallID).
		SetToolArgs(string(s.ToolArgs)).
		SetToolResult(string(s.ToolResult)).
		SetToolDurationMs(s.ToolDurationMs).
		SetToolErrorCode(s.ToolErrorCode).
		SetNoticeType(s.NoticeType).
		SetStatus(string(s.Status)).
		SetIsFinal(s.IsFinal).
		SetVersion(s.Version)
	if s.CompletedAt != nil {
		b.SetCompletedAt(*s.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Step{}, entErrToBizErr(err, "STEP_V2")
	}
	return entStepV2ToBiz(row), nil
}

// UpsertStep applies optimistic-concurrency upsert (see UpsertTask for semantics).
func (r *stepV2Repo) UpsertStep(ctx context.Context, s biz.Step) (biz.Step, error) {
	if r == nil || r.data == nil {
		return biz.Step{}, fmt.Errorf("step v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).StepV2.UpdateOneID(s.ID).
		Where(stepv2.VersionLT(s.Version)).
		SetTurnID(s.TurnID).
		SetTaskID(s.TaskID).
		SetSessionID(s.SessionID).
		SetSpiritSessionID(s.SpiritSessionID).
		SetKind(string(s.Kind)).
		SetAuthorAgentKey(s.AuthorAgentKey).
		SetSeq(s.Seq).
		SetContent(s.Content).
		SetReasoning(s.Reasoning).
		SetToolName(s.ToolName).
		SetToolCallID(s.ToolCallID).
		SetToolArgs(string(s.ToolArgs)).
		SetToolResult(string(s.ToolResult)).
		SetToolDurationMs(s.ToolDurationMs).
		SetToolErrorCode(s.ToolErrorCode).
		SetNoticeType(s.NoticeType).
		SetStatus(string(s.Status)).
		SetIsFinal(s.IsFinal).
		SetVersion(s.Version)
	if s.CompletedAt != nil {
		b.SetCompletedAt(*s.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).StepV2.Get(ctx, s.ID)
		if getErr != nil {
			return biz.Step{}, entErrToBizErr(getErr, "STEP_V2")
		}
		return entStepV2ToBiz(row), nil
	}
	// UPDATE failed. Two possible causes:
	//   1. Record doesn't exist yet → fall through to CREATE.
	//   2. Record exists but Version >= s.Version (WHERE didn't match) →
	//      return existing record (idempotent: a newer version is already
	//      persisted, e.g. sync persist wrote before the async event arrived).
	//      Without this check, the CREATE fallback would fail with CONFLICT
	//      and propagate an error to the v2 sequencer's retry loop.
	if existing, getErr := r.data.RW().Read(ctx).StepV2.Get(ctx, s.ID); getErr == nil {
		return entStepV2ToBiz(existing), nil
	}
	cb := r.data.RW().Write(ctx).StepV2.Create().
		SetID(s.ID).
		SetTurnID(s.TurnID).
		SetTaskID(s.TaskID).
		SetSessionID(s.SessionID).
		SetSpiritSessionID(s.SpiritSessionID).
		SetKind(string(s.Kind)).
		SetAuthorAgentKey(s.AuthorAgentKey).
		SetSeq(s.Seq).
		SetContent(s.Content).
		SetReasoning(s.Reasoning).
		SetToolName(s.ToolName).
		SetToolCallID(s.ToolCallID).
		SetToolArgs(string(s.ToolArgs)).
		SetToolResult(string(s.ToolResult)).
		SetToolDurationMs(s.ToolDurationMs).
		SetToolErrorCode(s.ToolErrorCode).
		SetNoticeType(s.NoticeType).
		SetStatus(string(s.Status)).
		SetIsFinal(s.IsFinal).
		SetStartedAt(s.StartedAt).
		SetVersion(s.Version)
	if s.CompletedAt != nil {
		cb.SetCompletedAt(*s.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) || isPgUniqueViolation(err) {
			existing, getErr := r.data.RW().Read(ctx).StepV2.Get(ctx, s.ID)
			if getErr != nil {
				return biz.Step{}, entErrToBizErr(getErr, "STEP_V2")
			}
			return entStepV2ToBiz(existing), nil
		}
		return biz.Step{}, entErrToBizErr(err, "STEP_V2")
	}
	return entStepV2ToBiz(row), nil
}

// entStepV2ToBiz converts an Ent StepV2 row to biz.Step.
// ToolArgs/ToolResult are stored as Text in Ent and converted to json.RawMessage.
func entStepV2ToBiz(row *ent.StepV2) biz.Step {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := *row.CompletedAt
		completedAt = &t
	}
	return biz.Step{
		ID:              row.ID,
		TurnID:          row.TurnID,
		TaskID:          row.TaskID,
		SessionID:       row.SessionID,
		SpiritSessionID: row.SpiritSessionID,
		Kind:            biz.StepKind(row.Kind),
		AuthorAgentKey:  row.AuthorAgentKey,
		Seq:             row.Seq,
		Content:         row.Content,
		Reasoning:       row.Reasoning,
		ToolName:        row.ToolName,
		ToolCallID:      row.ToolCallID,
		ToolArgs:        json.RawMessage(row.ToolArgs),
		ToolResult:      json.RawMessage(row.ToolResult),
		ToolDurationMs:  row.ToolDurationMs,
		ToolErrorCode:   row.ToolErrorCode,
		NoticeType:      row.NoticeType,
		Status:          biz.StepStatus(row.Status),
		IsFinal:         row.IsFinal,
		StartedAt:       row.StartedAt,
		CompletedAt:     completedAt,
		Version:         row.Version,
	}
}

func entStepsV2ToBiz(rows []*ent.StepV2) []biz.Step {
	out := make([]biz.Step, 0, len(rows))
	for _, r := range rows {
		out = append(out, entStepV2ToBiz(r))
	}
	return out
}
