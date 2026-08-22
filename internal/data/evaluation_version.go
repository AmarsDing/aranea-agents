package data

import (
	"context"

	"aranea-agents/internal/biz"
	beval "aranea-agents/internal/biz/evaluation"
)

func (r *evalRepo) InsertVersion(ctx context.Context, v biz.EvalDatasetVersion) (biz.EvalDatasetVersion, error) {
	if v.CreatedAt == "" {
		v.CreatedAt = now()
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO eval_dataset_versions
		 (id,dataset_id,version,hash,case_count,cases_json,created_at) VALUES (?,?,?,?,?,?,?)`),
		v.ID, v.DatasetID, v.Version, v.Hash, v.CaseCount, beval.MarshalVersionCases(v.Cases), v.CreatedAt)
	return v, entErrToBizErr(err, "EVAL")
}

func (r *evalRepo) GetVersion(ctx context.Context, id string) (biz.EvalDatasetVersion, error) {
	var v biz.EvalDatasetVersion
	var casesJSON string
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT id,dataset_id,version,hash,case_count,cases_json,created_at
			FROM eval_dataset_versions WHERE id=?`), []any{id},
		&v.ID, &v.DatasetID, &v.Version, &v.Hash, &v.CaseCount, &casesJSON, &v.CreatedAt)
	if err != nil {
		return biz.EvalDatasetVersion{}, entErrToBizErr(err, "EVAL")
	}
	v.Cases = beval.UnmarshalVersionCases(casesJSON)
	return v, nil
}

func (r *evalRepo) ListVersions(ctx context.Context, datasetID string, limit int) ([]biz.EvalDatasetVersion, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT id,dataset_id,version,hash,case_count,created_at
			FROM eval_dataset_versions WHERE dataset_id=? ORDER BY version DESC LIMIT ?`),
		datasetID, limit)
	if err != nil {
		return nil, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	out := make([]biz.EvalDatasetVersion, 0)
	for rows.Next() {
		var v biz.EvalDatasetVersion
		if err := rows.Scan(&v.ID, &v.DatasetID, &v.Version, &v.Hash, &v.CaseCount, &v.CreatedAt); err != nil {
			return nil, entErrToBizErr(err, "EVAL")
		}
		out = append(out, v)
	}
	return out, entErrToBizErr(rows.Err(), "EVAL")
}
