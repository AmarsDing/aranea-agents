package data

import (
	"context"
	"database/sql"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
)

type systemSettingRepo struct {
	data *Data
}

// NewSystemSettingRepo implements biz.SystemSettingRepo.
func NewSystemSettingRepo(d *Data) biz.SystemSettingRepo {
	return &systemSettingRepo{data: d}
}

func entToBizSystemSetting(e *ent.SystemSetting) biz.SystemSetting {
	if e == nil {
		return biz.SystemSetting{}
	}
	return biz.SystemSetting{
		RootDirectory:         e.RootDirectory,
		WorkDirectory:         e.WorkDirectory,
		GlobalMonthlyMicroUSD: e.GlobalMonthlyMicroUsd,
		A2APublicBaseURL:      e.A2aPublicBaseURL,
		UpdateTime:            e.UpdateTime,
	}
}

func (r *systemSettingRepo) Get(ctx context.Context) (biz.SystemSetting, error) {
	row, err := r.data.entClient.SystemSetting.Get(ctx, systemSettingSingletonID)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.SystemSetting{}, sql.ErrNoRows
		}
		return biz.SystemSetting{}, err
	}
	return entToBizSystemSetting(row), nil
}

func (r *systemSettingRepo) Update(ctx context.Context, rootDir, workDir string, globalMonthlyMicroUSD int64, a2aPublicBaseURL string) (biz.SystemSetting, error) {
	row, err := r.data.entClient.SystemSetting.UpdateOneID(systemSettingSingletonID).
		SetRootDirectory(rootDir).
		SetWorkDirectory(workDir).
		SetGlobalMonthlyMicroUsd(globalMonthlyMicroUSD).
		SetA2aPublicBaseURL(a2aPublicBaseURL).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.SystemSetting{}, sql.ErrNoRows
		}
		return biz.SystemSetting{}, err
	}
	return entToBizSystemSetting(row), nil
}
