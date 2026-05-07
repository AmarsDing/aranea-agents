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

	uc *biz.SystemSettingUsecase
}

func NewSystemSettingService(uc *biz.SystemSettingUsecase) *SystemSettingService {
	return &SystemSettingService{uc: uc}
}

func (s *SystemSettingService) GetSystemSettings(ctx context.Context, _ *emptypb.Empty) (*v1.SystemSettings, error) {
	row, err := s.uc.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.SystemSettings{
		WorkDirectory: row.WorkDirectory,
		UpdateTime:    timestamppb.New(row.UpdateTime),
		RootDirectory: row.RootDirectory,
	}, nil
}

func (s *SystemSettingService) UpdateSystemSettings(ctx context.Context, req *v1.UpdateSystemSettingsRequest) (*v1.SystemSettings, error) {
	row, err := s.uc.Update(ctx, req.GetRootDirectory(), req.GetWorkDirectory())
	if err != nil {
		return nil, err
	}
	return &v1.SystemSettings{
		WorkDirectory: row.WorkDirectory,
		UpdateTime:    timestamppb.New(row.UpdateTime),
		RootDirectory: row.RootDirectory,
	}, nil
}
