package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/plugin/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
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
	// monitorBus 流程日志总线；nil 时跳过流程日志发射（测试），进程日志不受影响。
	monitorBus contract.MonitorBus
}

func NewPluginService(uc *biz.PluginUsecase, runtime *plugintrpc.Runtime, lg loggateway.Logger, bus contract.MonitorBus) *PluginService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &PluginService{uc: uc, runtime: runtime, lg: lg, monitorBus: bus}
}

// Bootstrap seeds built-in plugins and hot-reloads runtime (call once at process start).
func (s *PluginService) Bootstrap(ctx context.Context) {
	if s == nil {
		return
	}
	// System context: full runtime snapshot across all workspaces (C-06).
	s.seedBuiltinPlugins(workspace.WithSystemWorkspace(ctx))
}

// NewPluginServiceWithBootstrap constructs PluginService and runs one-time bootstrap.
// TECH-DEBT(#plugin-bootstrap): constructor side-effect — should be called explicitly
// after Wire graph construction instead of inside a provider.
func NewPluginServiceWithBootstrap(uc *biz.PluginUsecase, runtime *plugintrpc.Runtime, lg loggateway.Logger, bus contract.MonitorBus) *PluginService {
	s := NewPluginService(uc, runtime, lg, bus)
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
		existing, err := s.uc.GetByKey(ctx, def.Key)
		if err == nil {
			s.syncBuiltinMeta(ctx, def, existing)
			continue
		}
		if !apierror.IsCode(err, apierror.CodeNotFound) {
			s.lg.Warn("插件种子查询失败",
				loggateway.StepID("plugin.seed_fail"),
				loggateway.Str("key", def.Key),
				loggateway.Err(err),
			)
			s.emitPluginFlowError(ctx, "system.plugin.seed_fail", "插件种子同步失败",
				event.P("key", def.Key), event.P("error", err.Error()))
			continue
		}
		if _, err := s.uc.Create(ctx, def.ToBizPlugin()); err != nil {
			if apierror.IsCode(err, apierror.CodeConflict) {
				// 并发 bootstrap（多实例同时启动）种子冲突属良性竞争：
				// 行已由另一实例创建，降级 Debug 且不发流程错误事件。
				s.lg.Debug("插件种子已存在（并发 bootstrap）",
					loggateway.StepID("plugin.seed_conflict"),
					loggateway.Str("key", def.Key),
				)
				continue
			}
			s.lg.Warn("插件种子创建失败",
				loggateway.StepID("plugin.seed_fail"),
				loggateway.Str("key", def.Key),
				loggateway.Err(err),
			)
			s.emitPluginFlowError(ctx, "system.plugin.seed_fail", "插件种子同步失败",
				event.P("key", def.Key), event.P("error", err.Error()))
		}
	}
	s.reloadRuntime(ctx)
}

// syncBuiltinMeta 将内置定义的平台自有元数据同步到已有行（ISSUE-009：
// 内置插件 schema/名称等演进后，存量行必须跟进，否则修复对既有安装不生效）。
// 仅在漂移时写库；管理员字段（enabled/config/sort_order/scope）由 data 层保留。
func (s *PluginService) syncBuiltinMeta(ctx context.Context, def plugintrpc.BuiltinPluginDef, existing biz.Plugin) {
	want := def.ToBizPlugin()
	if !biz.BuiltinMetaDrifted(existing, want) {
		return
	}
	if _, err := s.uc.SyncBuiltinMeta(ctx, want); err != nil {
		s.lg.Warn("内置插件元数据同步失败",
			loggateway.StepID("plugin.seed_meta_sync_fail"),
			loggateway.Str("key", def.Key),
			loggateway.Err(err),
		)
		s.emitPluginFlowError(ctx, "system.plugin.seed_meta_sync_fail", "内置插件元数据同步失败",
			event.P("key", def.Key), event.P("error", err.Error()))
		return
	}
	s.lg.Info("内置插件元数据已同步",
		loggateway.StepID("plugin.seed_meta_synced"),
		loggateway.Str("key", def.Key),
	)
}

// emitPluginFlowError 发射系统域流程日志错误事件；monitorBus 未注入时跳过
// （对应进程 Warn 已单独记录，避免重复进程日志）。
func (s *PluginService) emitPluginFlowError(ctx context.Context, stepID, message string, pairs ...event.Pair) {
	if s == nil || s.monitorBus == nil {
		return
	}
	flow := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainSystem,
		LG:     s.lg,
		Infra:  event.NewInfraFromBus(s.monitorBus),
	})
	flow.LogError(stepID, message, pairs...)
}

// reloadRuntime fetches enabled plugins and hot-reloads the plugin Runtime (C-06).
// Preserves request workspace (or system when absent) — never uses bare Background
// without workspace for List/Apply.
func (s *PluginService) reloadRuntime(ctx context.Context) {
	if s.runtime == nil {
		return
	}
	reloadCtx := workspace.WithSystemWorkspace(context.Background())
	if ws, ok := workspace.FromContext(ctx); ok {
		reloadCtx = workspace.WithContext(context.Background(), ws)
	}
	safego.Go(ctx, "plugin.reloadRuntime", func() {
		q := biz.PluginListQuery{Enabled: "true", Limit: 200}
		if !workspace.IsSystem(reloadCtx) {
			q.WorkspaceID = workspace.IDFromContext(reloadCtx)
		}
		result, err := s.uc.List(reloadCtx, q)
		if err != nil {
			s.lg.Warn("插件运行时重载列表失败",
				loggateway.StepID("plugin.reload_fail"),
				loggateway.Err(err),
			)
			s.emitPluginFlowError(reloadCtx, "system.plugin.reload_fail", "插件运行时重载失败",
				event.P("error", err.Error()))
			return
		}
		s.runtime.Apply(reloadCtx, result.Items)
	})
}

// assertPluginMutateAccess 校验 caller 是否可变更指定 plugin（P2-B IDOR 防护）。
// 跨租户访问返回 NotFound（避免泄露 plugin 存在性）。
// 内置/共享 plugin（workspace_id=""）是平台级配置，登录管理员可变更
// （需求 22-plugin §0.3：管理员启停/排序/改配置）；仅租户私有 plugin 做 workspace 写隔离。
func (s *PluginService) assertPluginMutateAccess(ctx context.Context, pluginID string) error {
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
	// 租户私有 plugin：仅同 workspace 可写。共享 plugin（workspace_id=""）
	// 为内置平台级配置，不套用共享资源 fail-closed（否则管理页写功能全灭）。
	if p.WorkspaceID != "" {
		callerWS := workspace.IDFromContext(ctx)
		if err := workspace.AssertWorkspaceMutate(callerWS, p.WorkspaceID); err != nil {
			s.lg.Warn("plugin access denied: workspace mismatch",
				loggateway.StepID("plugin.idor"),
				loggateway.Str("plugin_id", pluginID),
				loggateway.Str("caller_ws", callerWS))
			return apierror.NotFound("PLUGIN", "plugin not found")
		}
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
	q := biz.PluginRunQuery{
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
	}
	// N-B5: workspace 可见性过滤（与 ListPlugins 同语义），由服务端注入，不接受客户端输入。
	// 系统调用看全部；租户调用看共享行（workspace_id=''）+ 自身行。
	if !workspace.IsSystem(ctx) {
		q.WorkspaceID = workspace.IDFromContext(ctx)
	}
	result, err := s.uc.ListRuns(ctx, q)
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
	// N-B5: 系统调用全删；租户调用只删可见行（共享行 + 自身行）。
	workspaceID := ""
	if !workspace.IsSystem(ctx) {
		workspaceID = workspace.IDFromContext(ctx)
	}
	count, err := s.uc.DeleteAllRuns(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return &v1.DeleteAllPluginRunsResponse{DeletedCount: count}, nil
}
