package service

import (
	"context"

	v1 "aranea-agents/api/kratos/system_setting/v1"
	"aranea-agents/internal/biz"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SystemSettingService implements system_setting.v1.
type SystemSettingService struct {
	v1.UnimplementedSystemSettingServiceServer

	uc              *biz.SystemSettingUsecase
	a2aPublicBase   *A2APublicBaseReloader
}

func NewSystemSettingService(uc *biz.SystemSettingUsecase, a2aPublicBase *A2APublicBaseReloader) *SystemSettingService {
	return &SystemSettingService{uc: uc, a2aPublicBase: a2aPublicBase}
}

func (s *SystemSettingService) GetSystemSettings(ctx context.Context, _ *emptypb.Empty) (*v1.SystemSettings, error) {
	row, err := s.uc.Get(ctx)
	if err != nil {
		return nil, err
	}
	return toProtoSystemSettings(row), nil
}

func (s *SystemSettingService) UpdateSystemSettings(ctx context.Context, req *v1.UpdateSystemSettingsRequest) (*v1.SystemSettings, error) {
	row, err := s.uc.Update(ctx, req.GetRootDirectory(), req.GetWorkDirectory(), req.GetGlobalMonthlyMicroUsd(), req.GetA2APublicBaseUrl())
	if err != nil {
		return nil, err
	}
	if s.a2aPublicBase != nil {
		s.a2aPublicBase.Reload(row.A2APublicBaseURL)
	}
	return toProtoSystemSettings(row), nil
}

func toProtoSystemSettings(row biz.SystemSetting) *v1.SystemSettings {
	return &v1.SystemSettings{
		WorkDirectory:           row.WorkDirectory,
		UpdateTime:              timestamppb.New(row.UpdateTime),
		RootDirectory:           row.RootDirectory,
		GlobalMonthlyMicroUsd:   row.GlobalMonthlyMicroUSD,
		A2APublicBaseUrl:        row.A2APublicBaseURL,
	}
}
