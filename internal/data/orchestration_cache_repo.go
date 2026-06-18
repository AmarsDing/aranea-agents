package data

import (
	"context"

	"aranea-agents/internal/biz"
)

var _ biz.OrchestrationCacheRepo = (*orchestrationCacheRepo)(nil)

type orchestrationCacheRepo struct {
	data *Data
}

func NewOrchestrationCacheRepo(data *Data) biz.OrchestrationCacheRepo {
	return &orchestrationCacheRepo{data: data}
}

func (r *orchestrationCacheRepo) LoadCacheJSON(ctx context.Context) (string, error) {
	var jsonStr string
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT orchestration_cache_json FROM system_settings WHERE id = 1 LIMIT 1`,
	)
	if err != nil {
		return "", entErrToBizErr(err, "ORCHESTRATION_CACHE")
	}
	defer rows.Close()
	if !rows.Next() {
		return "", nil
	}
	if err := rows.Scan(&jsonStr); err != nil {
		return "", entErrToBizErr(err, "ORCHESTRATION_CACHE")
	}
	return jsonStr, nil
}

func (r *orchestrationCacheRepo) SaveCacheJSON(ctx context.Context, jsonStr string) error {
	// Use INSERT OR REPLACE to handle both initial insert and subsequent updates.
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		`INSERT OR REPLACE INTO system_settings (id, orchestration_cache_json) VALUES (1, ?)`,
		jsonStr,
	)
	return entErrToBizErr(err, "ORCHESTRATION_CACHE")
}
