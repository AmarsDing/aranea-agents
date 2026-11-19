package data

import (
	"context"

	"aranea-agents/internal/biz/fleet"
	"aranea-agents/pkg/apierror"
)

// ============================================================================
// Harness 多任务运行台状态存储（DDL 迁移 20261332：fleet_records 单表 JSON 文档）。
// 查询走 RWDB().ReadDB/WriteDB（事务感知），upsert 用 ON CONFLICT DO UPDATE。
// ============================================================================

type fleetStore struct {
	data *Data
}

var _ fleet.Store = (*fleetStore)(nil)

// NewFleetStore 装配运行台存储；d 为空返回 nil（运行台停用）。
func NewFleetStore(d *Data) fleet.Store {
	if d == nil {
		return nil
	}
	return &fleetStore{data: d}
}

func (s *fleetStore) Put(ctx context.Context, kind, id string, doc []byte) error {
	db := s.data.RWDB().WriteDB(ctx)
	if db == nil {
		return apierror.Internal("FLEET", "repository unavailable")
	}
	_, err := db.ExecContext(ctx, s.data.Dialect().RenumberPlaceholders(`INSERT INTO fleet_records (kind, id, data, created_at, updated_at)
		VALUES (?, ?, ?, NOW(), NOW())
		ON CONFLICT (kind, id) DO UPDATE SET data = EXCLUDED.data, updated_at = NOW()`), kind, id, string(doc))
	return entErrToBizErr(err, "FLEET")
}

func (s *fleetStore) Get(ctx context.Context, kind, id string) ([]byte, bool, error) {
	rows, err := s.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		s.data.Dialect().RenumberPlaceholders(`SELECT data FROM fleet_records WHERE kind = ? AND id = ?`), kind, id)
	if err != nil {
		return nil, false, entErrToBizErr(err, "FLEET")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, false, entErrToBizErr(rows.Err(), "FLEET")
	}
	var doc string
	if err := rows.Scan(&doc); err != nil {
		return nil, false, entErrToBizErr(err, "FLEET")
	}
	return []byte(doc), true, nil
}

func (s *fleetStore) List(ctx context.Context, kind string) ([][]byte, error) {
	rows, err := s.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		s.data.Dialect().RenumberPlaceholders(`SELECT data FROM fleet_records WHERE kind = ? ORDER BY created_at ASC`), kind)
	if err != nil {
		return nil, entErrToBizErr(err, "FLEET")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var doc string
		if err := rows.Scan(&doc); err != nil {
			return nil, entErrToBizErr(err, "FLEET")
		}
		out = append(out, []byte(doc))
	}
	return out, entErrToBizErr(rows.Err(), "FLEET")
}

func (s *fleetStore) Delete(ctx context.Context, kind, id string) error {
	db := s.data.RWDB().WriteDB(ctx)
	if db == nil {
		return apierror.Internal("FLEET", "repository unavailable")
	}
	_, err := db.ExecContext(ctx, s.data.Dialect().RenumberPlaceholders(`DELETE FROM fleet_records WHERE kind = ? AND id = ?`), kind, id)
	return entErrToBizErr(err, "FLEET")
}
