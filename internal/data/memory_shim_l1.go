package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// l1WorkingMemoryRepo delegates L1 working-memory operations to sessionmemory.Store.
// It implements biz.L1TaskWriter + biz.L1FieldWriter + biz.L1AdminReader + biz.L1IdleTaskReader.
type l1WorkingMemoryRepo struct {
	store *sessionmemory.Store
}

var _ biz.L1TaskWriter = (*l1WorkingMemoryRepo)(nil)
var _ biz.L1FieldWriter = (*l1WorkingMemoryRepo)(nil)
var _ biz.L1AdminReader = (*l1WorkingMemoryRepo)(nil)
var _ biz.L1IdleTaskReader = (*l1WorkingMemoryRepo)(nil)

func newL1WorkingMemoryRepo(store *sessionmemory.Store) *l1WorkingMemoryRepo {
	if store == nil {
		return nil
	}
	return &l1WorkingMemoryRepo{store: store}
}

// --- L1AdminReader ---

func (r *l1WorkingMemoryRepo) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	return r.store.ListL1TaskRows(ctx, sessionID, agentID, status, includeEnded)
}

func (r *l1WorkingMemoryRepo) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error) {
	return r.store.ListL1FieldRows(ctx, taskID, includeInternal)
}

func (r *l1WorkingMemoryRepo) GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	return r.store.GetL1TaskRow(ctx, sessionID, id)
}

func (r *l1WorkingMemoryRepo) GetL1FieldRow(ctx context.Context, taskID, fieldPath string) ([]byte, error) {
	return r.store.GetL1FieldRow(ctx, taskID, fieldPath)
}

// --- L1TaskWriter ---

func (r *l1WorkingMemoryRepo) StartL1Task(ctx context.Context, in biz.L1TaskInsert) ([]byte, error) {
	return r.store.StartL1Task(ctx, in)
}

func (r *l1WorkingMemoryRepo) EndL1Task(ctx context.Context, sessionID, taskID, status string) ([]byte, error) {
	return r.store.EndL1Task(ctx, sessionID, taskID, status)
}

func (r *l1WorkingMemoryRepo) ArchiveL1Task(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	return r.store.ArchiveL1Task(ctx, sessionID, taskID)
}

// --- L1FieldWriter ---

func (r *l1WorkingMemoryRepo) UpsertL1Field(ctx context.Context, in biz.L1FieldInsert) ([]byte, error) {
	return r.store.UpsertL1Field(ctx, in)
}

func (r *l1WorkingMemoryRepo) DeleteL1Field(ctx context.Context, taskID, fieldPath string) error {
	return r.store.DeleteL1Field(ctx, taskID, fieldPath)
}

func (r *l1WorkingMemoryRepo) PatchL1Fields(ctx context.Context, fields []biz.L1FieldInsert) ([][]byte, error) {
	return r.store.PatchL1Fields(ctx, fields)
}

// --- L1IdleTaskReader ---

func (r *l1WorkingMemoryRepo) ListIdleL1Tasks(ctx context.Context, cutoffRFC3339 string) ([][]byte, error) {
	return r.store.ListIdleL1Tasks(ctx, cutoffRFC3339)
}
