package biz

import (
	"context"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
)

// SystemSetting is the singleton platform configuration row.
type SystemSetting struct {
	RootDirectory         string
	WorkDirectory         string
	GlobalMonthlyMicroUSD int64
	A2APublicBaseURL      string
	UpdateTime            time.Time
}

type SystemSettingRepo interface {
	Get(ctx context.Context) (SystemSetting, error)
	Update(ctx context.Context, rootDir, workDir string, globalMonthlyMicroUSD int64, a2aPublicBaseURL string) (SystemSetting, error)
}

type SystemSettingUsecase struct {
	repo  SystemSettingRepo
	quota UsageQuotaRepo
}

func NewSystemSettingUsecase(repo SystemSettingRepo, quota UsageQuotaRepo) *SystemSettingUsecase {
	return &SystemSettingUsecase{repo: repo, quota: quota}
}

func (u *SystemSettingUsecase) Get(ctx context.Context) (SystemSetting, error) {
	s, err := u.repo.Get(ctx)
	if err != nil {
		return SystemSetting{}, err
	}
	if s.GlobalMonthlyMicroUSD <= 0 && u.quota != nil {
		q, qerr := u.quota.GetQuota(ctx, QuotaScopeGlobal, GlobalQuotaScopeID)
		if qerr == nil && q.MonthlyMicroUSD > 0 {
			s.GlobalMonthlyMicroUSD = q.MonthlyMicroUSD
		}
	}
	return s, nil
}

func (u *SystemSettingUsecase) Update(ctx context.Context, rootDir, workDir string, globalMonthlyMicroUSD int64, a2aPublicBaseURL string) (SystemSetting, error) {
	if globalMonthlyMicroUSD < 0 {
		return SystemSetting{}, errors.BadRequest("SYSTEM_SETTING", "global_monthly_micro_usd must be >= 0")
	}
	a2aPublicBaseURL = strings.TrimRight(strings.TrimSpace(a2aPublicBaseURL), "/")
	if a2aPublicBaseURL != "" && !strings.HasPrefix(a2aPublicBaseURL, "http://") && !strings.HasPrefix(a2aPublicBaseURL, "https://") {
		return SystemSetting{}, errors.BadRequest("SYSTEM_SETTING", "a2a_public_base_url must start with http:// or https://")
	}
	s, err := u.repo.Update(ctx, rootDir, workDir, globalMonthlyMicroUSD, a2aPublicBaseURL)
	if err != nil {
		return SystemSetting{}, err
	}
	if err := u.syncGlobalQuota(ctx, globalMonthlyMicroUSD); err != nil {
		return SystemSetting{}, err
	}
	s.GlobalMonthlyMicroUSD = globalMonthlyMicroUSD
	return s, nil
}

func (u *SystemSettingUsecase) syncGlobalQuota(ctx context.Context, monthlyMicroUSD int64) error {
	if u.quota == nil {
		return nil
	}
	_, err := u.quota.SetQuota(ctx, UsageQuota{
		ScopeType:       QuotaScopeGlobal,
		ScopeID:         GlobalQuotaScopeID,
		MonthlyMicroUSD: monthlyMicroUSD,
	})
	return mapUsageRepoErr(err)
}
