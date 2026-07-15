package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/plugin/v1"
	"aranea-agents/internal/biz"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// PluginService implements kratos plugin.v1.
type PluginService struct {
	v1.UnimplementedPluginServiceServer

	uc      *biz.PluginUsecase
	runtime *plugintrpc.Runtime
	lg      loggateway.Logger
}

func NewPluginService(uc *biz.PluginUsecase, runtime *plugintrpc.Runtime, lg loggateway.Logger) *PluginService {
	return &PluginService{uc: uc, runtime: runtime, lg: lg}
}

// Bootstrap seeds built-in plugins and hot-reloads runtime (call once at process start).
func (s *PluginService) Bootstrap(ctx context.Context) {
	if s == nil {
		return
	}
	s.seedBuiltinPlugins(ctx)
}

// NewPluginServiceWithBootstrap constructs PluginService and runs one-time bootstrap.
// TECH-DEBT(#plugin-bootstrap): constructor side-effect — should be called explicitly
// after Wire graph construction instead of inside a provider.
func NewPluginServiceWithBootstrap(uc *biz.PluginUsecase, runtime *plugintrpc.Runtime, lg loggateway.Logger) *PluginService {
	s := NewPluginService(uc, runtime, lg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.Bootstrap(ctx)
	return s
}

func (s *PluginService) seedBuiltinPlugins(ctx context.Context) {
	if s == nil || s.uc == nil {
		return
	}
	for _, def := range plugintrpc.BuiltinPluginDefs() {
		_, err := s.uc.GetByKey(ctx, def.Key)
		if err == nil {
			continue
		}
		if !apierror.IsCode(err, apierror.CodeNotFound) {
			s.lg.Warn("插件种子查询失败",
				loggateway.StepID("plugin.seed_fail"),
				loggateway.Str("key", def.Key),
				loggateway.Err(err),
			)
			continue
		}
		if _, err := s.uc.Create(ctx, def.ToBizPlugin()); err != nil {
			s.lg.Warn("插件种子创建失败",
				loggateway.StepID("plugin.seed_fail"),
				loggateway.Str("key", def.Key),
				loggateway.Err(err),
			)
		}
	}
	s.reloadRuntime(ctx)
}

// reloadRuntime fetches all enabled plugins and hot-reloads the plugin Runtime.
func (s *PluginService) reloadRuntime(ctx context.Context) {
	if s.runtime == nil {
		return
	}
	safego.Go(ctx, "plugin.reloadRuntime", func() {
		result, err := s.uc.List(context.Background(), biz.PluginListQuery{Enabled: "true", Limit: 200})
		if err != nil {
			s.lg.Warn("插件运行时重载列表失败",
				loggateway.StepID("plugin.reload_fail"),
				loggateway.Err(err),
			)
			return
		}
		s.runtime.Apply(context.Background(), result.Items)
	})
}

// assertPluginAccess 校验 caller 是否可读取指定 plugin（P2-B IDOR 防护）。
// 跨租户访问返回 NotFound（避免泄露 plugin 存在性）。
// 共享 plugin（workspace_id=""）对所有租户可读；变更须用 assertPluginMutateAccess。
func (s *PluginService) assertPluginAccess(ctx context.Context, pluginID string) error {
	return s.checkPluginAccess(ctx, pluginID, false)
}

// assertPluginMutateAccess 校验 caller 是否可变更指定 plugin。
// 共享 plugin（workspace_id=""）对租户只读（fail-closed）。
func (s *PluginService) assertPluginMutateAccess(ctx context.Context, pluginID string) error {
	return s.checkPluginAccess(ctx, pluginID, true)
}

func (s *PluginService) checkPluginAccess(ctx context.Context, pluginID string, mutate bool) error {
	if pluginID == "" {
		return nil
	}
	p, err := s.uc.Get(ctx, pluginID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound("PLUGIN", "plugin not found")
		}
		return err
	}
	callerWS := workspace.IDFromContext(ctx)
	if mutate {
		err = workspace.AssertWorkspaceMutate(callerWS, p.WorkspaceID)
	} else {
		err = workspace.AssertWorkspaceOrShared(callerWS, p.WorkspaceID)
	}
	if err != nil {
		s.lg.Warn("plugin access denied: workspace mismatch",
			loggateway.StepID("plugin.idor"),
			loggateway.Str("plugin_id", pluginID),
			loggateway.Str("caller_ws", callerWS))
		return apierror.NotFound("PLUGIN", "plugin not found")
	}
	return nil
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
	// P2-B: workspace visibility filter.
	// System caller (cron/admin) sees all; tenant caller sees shared + own.
	if !workspace.IsSystem(ctx) {
		q.WorkspaceID = workspace.IDFromContext(ctx)
	}
	result, err := s.uc.List(ctx, q)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListPluginsResponse{
		Total:    int32(result.Total),
		Page:     page,
		PageSize: pageSize,
		Items:    make([]*v1.Plugin, 0, len(result.Items)),
	}
	for i := range result.Items {
		resp.Items = append(resp.Items, toProtoPlugin(result.Items[i]))
	}
	return resp, nil
}

func (s *PluginService) TogglePluginEnabled(ctx context.Context, req *v1.TogglePluginEnabledRequest) (*v1.Plugin, error) {
	if err := s.assertPluginMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	out, err := s.uc.ToggleEnabled(ctx, req.GetId(), req.GetEnabled())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("PLUGIN", "plugin not found")
		}
		return nil, err
	}
	s.reloadRuntime(ctx)
	return toProtoPlugin(out), nil
}

func (s *PluginService) UpdatePluginConfig(ctx context.Context, req *v1.UpdatePluginConfigRequest) (*v1.Plugin, error) {
	if err := s.assertPluginMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	out, err := s.uc.UpdateConfig(ctx, req.GetId(), req.GetConfigJson())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("PLUGIN", "plugin not found")
		}
		return nil, err
	}
	s.reloadRuntime(ctx)
	return toProtoPlugin(out), nil
}

func (s *PluginService) UpdatePluginSortOrder(ctx context.Context, req *v1.UpdatePluginSortOrderRequest) (*v1.Plugin, error) {
	if err := s.assertPluginMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	out, err := s.uc.UpdateSortOrder(ctx, req.GetId(), int(req.GetSortOrder()))
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("PLUGIN", "plugin not found")
		}
		return nil, err
	}
	s.reloadRuntime(ctx)
	return toProtoPlugin(out), nil
}

func (s *PluginService) UpdatePluginScope(ctx context.Context, req *v1.UpdatePluginScopeRequest) (*v1.Plugin, error) {
	if err := s.assertPluginMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	out, err := s.uc.UpdateScope(ctx, req.GetId(), req.GetScope())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("PLUGIN", "plugin not found")
		}
		return nil, err
	}
	s.reloadRuntime(ctx)
	return toProtoPlugin(out), nil
}

func toProtoPluginRun(r biz.PluginRun) *v1.PluginRun {
	return &v1.PluginRun{
		Id:            r.ID,
		PluginKey:     r.PluginKey,
		PluginId:      r.PluginID,
		SessionId:     r.SessionID,
		AgentId:       r.AgentID,
		CallbackPoint: r.CallbackPoint,
		Status:        r.Status,
		DurationMs:    int32(r.DurationMS),
		DetailJson:    r.DetailJSON,
		CreatedAt:     r.CreatedAt,
	}
}

func (s *PluginService) ListPluginRuns(ctx context.Context, req *v1.ListPluginRunsRequest) (*v1.ListPluginRunsResponse, error) {
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	result, err := s.uc.ListRuns(ctx, biz.PluginRunQuery{
		PluginKey:     req.GetPluginKey(),
		PluginID:      req.GetPluginId(),
		SessionID:     req.GetSessionId(),
		AgentID:       req.GetAgentId(),
		CallbackPoint: req.GetCallbackPoint(),
		Status:        req.GetStatus(),
		From:          req.GetFrom(),
		To:            req.GetTo(),
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		return nil, err
	}
	resp := &v1.ListPluginRunsResponse{
		Total:    result.Total,
		Page:     page,
		PageSize: pageSize,
		Items:    make([]*v1.PluginRun, 0, len(result.Items)),
	}
	for i := range result.Items {
		resp.Items = append(resp.Items, toProtoPluginRun(result.Items[i]))
	}
	return resp, nil
}

func (s *PluginService) DeleteAllPluginRuns(ctx context.Context, _ *v1.DeleteAllPluginRunsRequest) (*v1.DeleteAllPluginRunsResponse, error) {
	count, err := s.uc.DeleteAllRuns(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.DeleteAllPluginRunsResponse{DeletedCount: count}, nil
}
