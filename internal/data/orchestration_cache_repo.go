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
	// Use ON CONFLICT upsert (works for both SQLite 3.24+ and Postgres).
	// 2026-09-04：system_settings.update_time 是 NOT NULL 且仅 Go 侧默认
	// （schema 无 entsql.Default，PG 侧列无 DB 默认）——裸 SQL 绕过 ent，
	// 首行 INSERT 在 PG 必炸 23502。显式写 CURRENT_TIMESTAMP（SQL 标准，
	// SQLite/PG 双兼容；不可用 NOW()——SQLite 无此函数）。
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO system_settings (id, orchestration_cache_json, update_time)
		 VALUES (1, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET orchestration_cache_json=excluded.orchestration_cache_json, update_time=CURRENT_TIMESTAMP`),
		jsonStr,
	)
	return entErrToBizErr(err, "ORCHESTRATION_CACHE")
}
