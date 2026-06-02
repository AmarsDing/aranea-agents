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
	rows, err := r.data.ReadDB().QueryContext(ctx,
		`SELECT orchestration_cache_json FROM system_settings WHERE id = 1 LIMIT 1`,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", nil
	}
	if err := rows.Scan(&jsonStr); err != nil {
		return "", err
	}
	return jsonStr, nil
}

func (r *orchestrationCacheRepo) SaveCacheJSON(ctx context.Context, jsonStr string) error {
	_, err := r.data.entClient.ExecContext(ctx,
		`UPDATE system_settings SET orchestration_cache_json = ? WHERE id = 1`,
		jsonStr,
	)
	return err
}
