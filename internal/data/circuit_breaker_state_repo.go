package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz/tool"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/circuitbreakerstate"
	"aranea-agents/pkg/apierror"
)

type circuitBreakerStateRepo struct {
	data *Data
}

var _ tool.CircuitBreakerStateRepo = (*circuitBreakerStateRepo)(nil)

func NewCircuitBreakerStateRepo(d *Data) tool.CircuitBreakerStateRepo {
	return &circuitBreakerStateRepo{data: d}
}

func (r *circuitBreakerStateRepo) SaveState(ctx context.Context, key string, entry tool.CircuitBreakerStateEntry) error {
	if r == nil || r.data == nil {
		return apierror.Internal("CB_STATE", "database not configured")
	}
	now := time.Now()
	create := r.data.RW().Write(ctx).CircuitBreakerState.Create().
		SetKey(key).
		SetState(entry.State).
		SetFailureCount(entry.FailureCount).
		SetSuccessCount(entry.SuccessCount).
		SetUpdatedAt(now)
	if !entry.LastFailureTime.IsZero() {
		create = create.SetLastFailureTime(entry.LastFailureTime)
	}
	if !entry.LastStateChange.IsZero() {
		create = create.SetLastStateChange(entry.LastStateChange)
	}
	// Upsert: on key conflict, update all fields.
	err := create.
		OnConflictColumns(circuitbreakerstate.FieldKey).
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		return entErrToBizErr(err, "CB_STATE")
	}
	return nil
}

func (r *circuitBreakerStateRepo) LoadState(ctx context.Context, key string) (tool.CircuitBreakerStateEntry, error) {
	if r == nil || r.data == nil {
		return tool.CircuitBreakerStateEntry{}, apierror.Internal("CB_STATE", "database not configured")
	}
	row, err := r.data.RW().Read(ctx).CircuitBreakerState.Query().
		Where(circuitbreakerstate.KeyEQ(key)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return tool.CircuitBreakerStateEntry{}, nil
		}
		return tool.CircuitBreakerStateEntry{}, entErrToBizErr(err, "CB_STATE")
	}
	return entRowToCBEntry(row), nil
}

func (r *circuitBreakerStateRepo) LoadAllStates(ctx context.Context) (map[string]tool.CircuitBreakerStateEntry, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("CB_STATE", "database not configured")
	}
	rows, err := r.data.RW().Read(ctx).CircuitBreakerState.Query().All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "CB_STATE")
	}
	result := make(map[string]tool.CircuitBreakerStateEntry, len(rows))
	for _, row := range rows {
		result[row.Key] = entRowToCBEntry(row)
	}
	return result, nil
}

func entRowToCBEntry(row *ent.CircuitBreakerState) tool.CircuitBreakerStateEntry {
	entry := tool.CircuitBreakerStateEntry{
		State:        row.State,
		FailureCount: row.FailureCount,
		SuccessCount: row.SuccessCount,
		UpdatedAt:    row.UpdatedAt,
	}
	if row.LastFailureTime != nil {
		entry.LastFailureTime = *row.LastFailureTime
	}
	if row.LastStateChange != nil {
		entry.LastStateChange = *row.LastStateChange
	}
	return entry
}
