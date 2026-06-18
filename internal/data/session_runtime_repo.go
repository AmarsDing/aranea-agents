package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	entsession "aranea-agents/internal/data/ent/session"
	"aranea-agents/internal/data/ent/sessionruntime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type sessionRuntimeRepo struct {
	data *Data
}

var (
	_ biz.SessionRuntimeReader = (*sessionRuntimeRepo)(nil)
	_ biz.SessionRuntimeWriter = (*sessionRuntimeRepo)(nil)
)

func NewSessionRuntimeRepo(data *Data) biz.SessionRuntimeWriter {
	return &sessionRuntimeRepo{data: data}
}

// NewSessionRuntimeReader provides a SessionRuntimeReader from the same sessionRuntimeRepo.
// The returned value also implements SessionRuntimeWriter.
func NewSessionRuntimeReader(data *Data) biz.SessionRuntimeReader {
	return &sessionRuntimeRepo{data: data}
}

func entSessionRuntimeToBiz(e *ent.SessionRuntime) *biz.SessionRuntime {
	if e == nil {
		return nil
	}
	return &biz.SessionRuntime{
		SessionID:          e.ID,
		SessionRevision:    e.SessionRevision,
		StateJSON:          e.StateJSON,
		RunnerSnapshotJSON: e.RunnerSnapshotJSON,
		MetadataJSON:       e.MetadataJSON,
		CompressVersion:    e.CompressVersion,
		UpdatedAt:          e.UpdatedAt,
	}
}

func (r *sessionRuntimeRepo) GetSessionRuntime(ctx context.Context, sessionID string) (*biz.SessionRuntime, error) {
	c := r.data.RW().Read(ctx)
	row, err := c.SessionRuntime.Get(ctx, sessionID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, entErrToBizErr(err, "SESSION_RUNTIME")
	}
	return entSessionRuntimeToBiz(row), nil
}

func (r *sessionRuntimeRepo) UpsertSessionRuntime(ctx context.Context, sessionID string, runtime *biz.SessionRuntime) error {
	if runtime == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}

	c := r.data.RW().Write(ctx)
	now := nowRFC3339()

	builder := c.SessionRuntime.Create().
		SetID(sessionID).
		SetSessionRevision(runtime.SessionRevision).
		SetStateJSON(runtime.StateJSON).
		SetRunnerSnapshotJSON(runtime.RunnerSnapshotJSON).
		SetMetadataJSON(runtime.MetadataJSON).
		SetCompressVersion(runtime.CompressVersion).
		SetUpdatedAt(now)

	err := builder.
		OnConflictColumns(sessionruntime.FieldID).
		Update(func(u *ent.SessionRuntimeUpsert) {
			u.SetSessionRevision(runtime.SessionRevision)
			u.SetStateJSON(runtime.StateJSON)
			u.SetRunnerSnapshotJSON(runtime.RunnerSnapshotJSON)
			u.SetMetadataJSON(runtime.MetadataJSON)
			u.SetCompressVersion(runtime.CompressVersion)
			u.SetUpdatedAt(now)
		}).
		Exec(ctx)
	if err != nil {
		r.data.lg.Warn("upsert session runtime failed", loggateway.StepID("data.session_runtime.upsert"), loggateway.Err(err))
	}
	return entErrToBizErr(err, "SESSION_RUNTIME")
}

// TransitionSessionStatus updates the session status in the sessions table.
// Although hosted on sessionRuntimeRepo, status transitions are part of the runtime
// management domain (lifecycle control), not the metrics domain.
// The sessions table is the single source of truth for status; session_runtime
// holds extended runtime state.
// currentStatus is used as a WHERE condition to prevent TOCTOU races:
// the UPDATE only succeeds if the row still has the expected current status.
// TODO(test): Add unit tests for TransitionSessionStatus covering: normal transition, empty sessionID, DB error, concurrent status conflict.
func (r *sessionRuntimeRepo) TransitionSessionStatus(ctx context.Context, sessionID string, currentStatus string, newStatus string, statusReason string, statusChangedAt string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	c := r.data.RW().Write(ctx)
	now := nowRFC3339()
	n, err := c.Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.StatusEQ(currentStatus)).
		SetStatus(newStatus).
		SetStatusReason(statusReason).
		SetStatusChangedAt(statusChangedAt).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		r.data.lg.Warn("transition session status failed", loggateway.StepID("data.session_runtime.transition_status"), loggateway.Err(err))
		return entErrToBizErr(err, "SESSION_RUNTIME")
	}
	if n == 0 {
		r.data.lg.Warn("transition session status: no rows affected (status already changed?)", loggateway.StepID("data.session_runtime.transition_status_noop"), loggateway.Str("session_id", sessionID), loggateway.Str("current_status", currentStatus), loggateway.Str("new_status", newStatus))
		return apierror.Conflict("SESSION", "session status was concurrently modified")
	}
	return nil
}
