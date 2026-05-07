package biz

import (
	"context"
	"time"
)

// SystemSetting is the singleton platform configuration row.
type SystemSetting struct {
	RootDirectory string
	WorkDirectory string
	UpdateTime    time.Time
}

type SystemSettingRepo interface {
	Get(ctx context.Context) (SystemSetting, error)
	Update(ctx context.Context, rootDir, workDir string) (SystemSetting, error)
}

type SystemSettingUsecase struct {
	repo SystemSettingRepo
}

func NewSystemSettingUsecase(repo SystemSettingRepo) *SystemSettingUsecase {
	return &SystemSettingUsecase{repo: repo}
}

func (u *SystemSettingUsecase) Get(ctx context.Context) (SystemSetting, error) {
	return u.repo.Get(ctx)
}

func (u *SystemSettingUsecase) Update(ctx context.Context, rootDir, workDir string) (SystemSetting, error) {
	return u.repo.Update(ctx, rootDir, workDir)
}
