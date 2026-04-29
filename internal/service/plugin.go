package service

import (
	"context"
	"database/sql"
	"errors"

	v1 "aranea-agents/api/kratos/plugin/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// PluginService implements kratos plugin.v1.
type PluginService struct {
	v1.UnimplementedPluginServiceServer

	uc *biz.PluginUsecase
}

func NewPluginService(uc *biz.PluginUsecase) *PluginService {
	return &PluginService{uc: uc}
}

func toProtoPlugin(p biz.Plugin) *v1.Plugin {
	return &v1.Plugin{
		Id:                p.ID,
		Key:               p.Key,
		Name:              p.Name,
		Description:       p.Description,
		Category:          p.Category,
		RiskLevel:         p.RiskLevel,
		Enabled:           p.Enabled,
		Scope:             p.Scope,
		CallbackPoints:    p.CallbackPoints,
		SortOrder:         int32(p.SortOrder),
		ConfigSchemaJson:  p.ConfigSchemaJSON,
		ConfigJson:        p.ConfigJSON,
		DefaultConfigJson: p.DefaultConfigJSON,
		InvokeCount:       int32(p.InvokeCount),
		BlockCount:        int32(p.BlockCount),
		ErrorCount:        int32(p.ErrorCount),
		LastInvokedAt:     p.LastInvokedAt,
		LastStatus:        p.LastStatus,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
		Permissions: &v1.PluginPermissions{
			CanView:       p.Permissions.CanView,
			CanToggle:     p.Permissions.CanToggle,
			CanEditConfig: p.Permissions.CanEditConfig,
			CanViewLogs:   p.Permissions.CanViewLogs,
		},
	}
}

func (s *PluginService) ListPlugins(ctx context.Context, req *v1.ListPluginsRequest) (*v1.ListPluginsResponse, error) {
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	q := biz.PluginListQuery{
		Search:        req.GetSearch(),
		Category:      req.GetCategory(),
		Enabled:       req.GetEnabled(),
		CallbackPoint: req.GetCallbackPoint(),
		Limit:         limit,
		Offset:        offset,
	}
	result, err := s.uc.List(ctx, q)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListPluginsResponse{
		Total:    int32(result.Total),
		Page:     page,
		PageSize: pageSize,
		Items:      make([]*v1.Plugin, 0, len(result.Items)),
	}
	for i := range result.Items {
		resp.Items = append(resp.Items, toProtoPlugin(result.Items[i]))
	}
	return resp, nil
}

func (s *PluginService) TogglePluginEnabled(ctx context.Context, req *v1.TogglePluginEnabledRequest) (*v1.Plugin, error) {
	out, err := s.uc.ToggleEnabled(ctx, req.GetId(), req.GetEnabled())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("PLUGIN", "plugin not found")
		}
		return nil, err
	}
	return toProtoPlugin(out), nil
}

func (s *PluginService) UpdatePluginConfig(ctx context.Context, req *v1.UpdatePluginConfigRequest) (*v1.Plugin, error) {
	out, err := s.uc.UpdateConfig(ctx, req.GetId(), req.GetConfigJson())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("PLUGIN", "plugin not found")
		}
		return nil, err
	}
	return toProtoPlugin(out), nil
}
