package data

import (
	"context"

	"aranea-agents/internal/biz"
)

type memoryFactReader struct {
	data *Data
}

var _ biz.MemoryFactReader = (*memoryFactReader)(nil)

func NewMemoryFactReader(data *Data) biz.MemoryFactReader {
	if data == nil {
		return nil
	}
	return &memoryFactReader{data: data}
}

func (r *memoryFactReader) ReadSessionMemoryFacts(ctx context.Context, sessionID string) ([]biz.MemoryFactEntry, error) {
	if r == nil || r.data == nil {
		return nil, nil
	}
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		SELECT statement, fact_kind, confidence
		FROM memory_facts
		WHERE source_session_id = ? AND deleted_at = '' AND status = 'active'
		ORDER BY importance DESC
		LIMIT 50`), sessionID)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var result []biz.MemoryFactEntry
	for rows.Next() {
		var e biz.MemoryFactEntry
		if err := rows.Scan(&e.Statement, &e.Scope, &e.Confidence); err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L3")
		}
		result = append(result, e)
	}
	return result, entErrToBizErr(rows.Err(), "MEMORY_L3")
}
