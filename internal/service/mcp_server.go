package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	v1 "aranea-agents/api/kratos/mcp_server/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	mcpconfig "aranea-agents/internal/mcp/config"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// MCPServerService implements kratos mcp_server.v1.
type MCPServerService struct {
	v1.UnimplementedMCPServerServiceServer

	uc         *biz.MCPServerUsecase
	mon        *biz.MonitorUsecase
	agents     *biz.AgentUsecase // 可选：用于 ListMCPServers 的 MCP 采纳汇总；nil 时跳过 summary
	lg         loggateway.Logger
	monitorBus contract.MonitorBus
}

func NewMCPServerService(uc *biz.MCPServerUsecase, mon *biz.MonitorUsecase, agents *biz.AgentUsecase, lg loggateway.Logger, monitorBus contract.MonitorBus) *MCPServerService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	// Wire the OAuth2 refresh-token rotation persistence hook: when a provider
	// rotates the refresh token, the agent runtime writes it back into the
	// server's stored config_json so a restart does not resurrect the revoked
	// token. The closure owns failure logging per the hook contract.
	chatagent.SetMCPRefreshTokenPersister(func(ctx context.Context, serverKey, refreshToken string) error {
		if err := uc.PersistRotatedRefreshToken(ctx, serverKey, refreshToken); err != nil {
			lg.Warn("MCP 轮换 refresh_token 回写失败",
				loggateway.StepID("mcp.server.token_rotate"),
				loggateway.Str("server_key", serverKey),
				loggateway.Err(err))
			return err
		}
		lg.Info("MCP 轮换 refresh_token 已回写",
			loggateway.StepID("mcp.server.token_rotate"),
			loggateway.Str("server_key", serverKey))
		return nil
	})
	return &MCPServerService{uc: uc, mon: mon, agents: agents, lg: lg, monitorBus: monitorBus}
}

// logMCPFlow emits a user-visible flow log (流程日志) for MCP server CRUD steps.
// err != nil → error phase; otherwise done phase. Nil-safe: skipped when the
// monitor bus is not wired (tests).
func (s *MCPServerService) logMCPFlow(ctx context.Context, step, message string, err error, pairs ...event.Pair) {
	if s == nil || s.monitorBus == nil {
		return
	}
	flow := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainSystem,
		LG:     s.lg,
		Infra:  event.NewInfraFromBus(s.monitorBus),
	})
	if err != nil {
		flow.LogError(step, message, append(pairs, event.P("error", err.Error()))...)
		return
	}
	flow.LogDone(step, message, pairs...)
}

// redactMCPConfigURL extracts the server URL from config_json and strips
// query params / userinfo / fragment so no credentials leak into flow logs
// (红线 #25). Returns "" when unavailable or unparseable.
func redactMCPConfigURL(configJSON string) string {
	cfg, err := mcpconfig.ParseServerConfigJSON(configJSON)
	if err != nil || cfg.URL == "" {
		return ""
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return ""
	}
	u.RawQuery = ""
	u.User = nil
	u.Fragment = ""
	return u.String()
}

// assertMCPServerAccess 校验 caller 是否可读取指定 mcp server（P2-B IDOR 防护）。
// 跨租户访问返回 NotFound（避免泄露 mcp server 存在性）。
// 共享 mcp server（workspace_id=""）对所有租户可读；变更须用 assertMCPServerMutateAccess。
func (s *MCPServerService) assertMCPServerAccess(ctx context.Context, serverID string) error {
	_, err := s.checkMCPServerAccess(ctx, serverID, false)
	return err
}

// assertMCPServerMutateAccess 校验 caller 是否可变更指定 mcp server。
// 共享 mcp server（workspace_id=""）对租户只读（fail-closed）。
func (s *MCPServerService) assertMCPServerMutateAccess(ctx context.Context, serverID string) error {
	_, err := s.checkMCPServerAccess(ctx, serverID, true)
	return err
}

// checkMCPServerAccess 校验访问权限并返回已读取的 server，调用方应复用该返回值，
// 禁止在校验通过后再次 Get（每个 RPC 只读一次）。
func (s *MCPServerService) checkMCPServerAccess(ctx context.Context, serverID string, mutate bool) (biz.MCPServer, error) {
	if serverID == "" {
		return biz.MCPServer{}, nil
	}
	srv, err := s.uc.Get(ctx, serverID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return biz.MCPServer{}, apierror.NotFound("MCP_SERVER", "mcp server not found")
		}
		return biz.MCPServer{}, err
	}
	callerWS := workspace.IDFromContext(ctx)
	if mutate {
		err = workspace.AssertWorkspaceMutate(callerWS, srv.WorkspaceID)
	} else {
		err = workspace.AssertWorkspaceOrShared(callerWS, srv.WorkspaceID)
	}
	if err != nil {
		// 进程日志记录 IDOR 拒绝（对外仍返回 NotFound，不泄露存在性）。
		s.lg.Warn("mcp.server.access_denied",
			loggateway.StepID("mcp.server.access_denied"),
			loggateway.Str("server_id", serverID),
			loggateway.Str("caller_workspace", callerWS),
			loggateway.Str("mutate", strconv.FormatBool(mutate)),
			loggateway.Err(err))
		return biz.MCPServer{}, apierror.NotFound("MCP_SERVER", "mcp server not found")
	}
	return srv, nil
}

func toProtoMCP(m biz.MCPServer) *v1.MCPServer {
	return &v1.MCPServer{
		Id:           m.ID,
		Key:          m.Key,
		Name:         m.Name,
		Description:  m.Description,
		Status:       m.Status,
		Enabled:      m.Enabled,
		SortOrder:    int32(m.SortOrder),
		ConfigJson:   mcpconfig.RedactConfigJSON(m.ConfigJSON),
		MetadataJson: m.MetadataJSON,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		DeletedAt:    m.DeletedAt,
		// Shared = system built-in (workspace_id==""). Exposed as a derived bool
		// (not the raw workspace ID) so the UI can disable mutation affordances
		// without leaking tenant topology.
		Shared: m.WorkspaceID == "",
	}
}

// patchFromProtoMCPWithDiff builds an MCPServerUpdate by comparing proto values against
// the current persisted server. For string fields, non-empty proto values are included.
// For bool/int fields, only values that differ from current are included — this resolves
// proto3 zero-value ambiguity where false/0 could mean "not set" or "explicitly set to zero".
//
// proto status/metadata_json are intentionally NOT mapped: both are system-managed
// (health runner, reconnect bookkeeping, delete) and the admin form round-trips a
// stale snapshot of them; writing them back would roll back concurrent health writes.
func patchFromProtoMCPWithDiff(pb *v1.MCPServer, cur biz.MCPServer) biz.MCPServerUpdate {
	if pb == nil {
		return biz.MCPServerUpdate{}
	}
	patch := biz.MCPServerUpdate{
		Key:         strPtrIfNonEmpty(pb.GetKey()),
		Name:        strPtrIfNonEmpty(pb.GetName()),
		Description: strPtrIfNonEmpty(pb.GetDescription()),
		ConfigJSON:  strPtrIfNonEmpty(pb.GetConfigJson()),
	}
	// For bool/int: only set if value differs from current (proto3 zero-value ambiguity).
	if pb.GetEnabled() != cur.Enabled {
		patch.Enabled = boolPtr(pb.GetEnabled())
	}
	if int(pb.GetSortOrder()) != cur.SortOrder {
		patch.SortOrder = intPtr(int(pb.GetSortOrder()))
	}
	return patch
}

func strPtrIfNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func boolPtr(b bool) *bool { return &b }

func intPtr(i int) *int { return &i }

func (s *MCPServerService) ListMCPServers(ctx context.Context, req *v1.ListMCPServersRequest) (*v1.ListMCPServersResponse, error) {
	// Prefer proto request params (typed clients); fall back to the raw HTTP
	// query for legacy clients (CLI, older generated TS client that silently
	// dropped page/pageSize/search when the rpc took google.protobuf.Empty).
	search := strings.TrimSpace(req.GetSearch())
	if search == "" {
		search = searchQueryFromContext(ctx)
	}
	q := biz.MCPListQuery{Search: search}
	// P2-B: workspace visibility filter.
	// System caller (cron/admin) sees all; tenant caller sees shared + own.
	if !workspace.IsSystem(ctx) {
		q.WorkspaceID = workspace.IDFromContext(ctx)
	}
	page, pageSize := req.GetPage(), req.GetPageSize()
	if page <= 0 && pageSize <= 0 {
		page, pageSize, _ = pageQueryFromContext(ctx)
	}
	if page > 0 || pageSize > 0 {
		limit, offset, page, pageSize := biz.PageToLimitOffset(page, pageSize)
		q.Limit, q.Offset = limit, offset
		result, err := s.uc.ListPaged(ctx, q)
		if err != nil {
			return nil, err
		}
		resp := &v1.ListMCPServersResponse{
			Items:    make([]*v1.MCPServer, 0, len(result.Items)),
			Total:    int32(result.Total),
			Page:     page,
			PageSize: pageSize,
			Summary:  s.usageSummary(ctx),
		}
		for i := range result.Items {
			resp.Items = append(resp.Items, toProtoMCP(result.Items[i]))
		}
		return resp, nil
	}
	items, err := s.uc.List(ctx, q)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListMCPServersResponse{
		Items:    make([]*v1.MCPServer, 0, len(items)),
		Total:    int32(len(items)),
		Page:     1,
		PageSize: int32(len(items)),
		Summary:  s.usageSummary(ctx),
	}
	for i := range items {
		resp.Items = append(resp.Items, toProtoMCP(items[i]))
	}
	return resp, nil
}

// usageSummary 计算 MCP 采纳汇总（使用方数量），供管理页提示「配置了但无 Agent 使用」。
// best-effort：agents 未注入或计算失败时返回 nil，绝不让汇总拖垮列表主流程。
func (s *MCPServerService) usageSummary(ctx context.Context) *v1.MCPUsageSummary {
	if s == nil || s.agents == nil {
		return nil
	}
	sum, err := s.agents.GetMCPUsageSummary(ctx)
	if err != nil {
		s.lg.Warn("MCP 采纳汇总计算失败（降级不返回 summary）",
			loggateway.StepID("mcp.server.usage_summary"),
			loggateway.Err(err))
		return nil
	}
	return &v1.MCPUsageSummary{
		EnabledAgentCount: int32(sum.EnabledAgentCount),
		TotalAgentCount:   int32(sum.TotalAgentCount),
	}
}

func (s *MCPServerService) CreateMCPServer(ctx context.Context, req *v1.CreateMCPServerRequest) (*v1.MCPServer, error) {
	in := biz.MCPServer{
		Key:          req.GetKey(),
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		Status:       req.GetStatus(),
		Enabled:      req.GetEnabled(),
		SortOrder:    int(req.GetSortOrder()),
		ConfigJSON:   req.GetConfigJson(),
		MetadataJSON: req.GetMetadataJson(),
	}
	// P2-B: stamp caller workspace on create (system caller = empty/shared).
	if !workspace.IsSystem(ctx) {
		in.WorkspaceID = workspace.IDFromContext(ctx)
	}
	out, err := s.uc.Create(ctx, in)
	if err != nil {
		s.logMCPFlow(ctx, "mcp.server.add", "MCP 服务器添加失败", err,
			event.P("server_key", in.Key),
			event.P("server_name", in.Name),
			event.P("url", redactMCPConfigURL(in.ConfigJSON)))
		return nil, err
	}
	s.logMCPFlow(ctx, "mcp.server.add", fmt.Sprintf("MCP 服务器已添加：%s", out.Name), nil,
		event.P("server_id", out.ID),
		event.P("server_key", out.Key),
		event.P("server_name", out.Name),
		event.P("url", redactMCPConfigURL(out.ConfigJSON)))
	// P0-2B：不再全量标脏。新增 server 只影响 effective 列表扩容的 agent，
	// 其 MCPVersionHash 变化 → 下一请求新 key miss → 热替换/全量兜底惰性生效；
	// 未引用该 server 的 agent 哈希不变、缓存照常命中，无需任何失效动作。
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbCreate, "mcp_server"),
		Resource:   "mcp_server",
		ResourceID: out.ID,
		Summary:    fmt.Sprintf("key=%s", out.Key),
	})
	return toProtoMCP(out), nil
}

func (s *MCPServerService) GetMCPServer(ctx context.Context, req *v1.GetMCPServerRequest) (*v1.MCPServer, error) {
	// P2-B: IDOR guard — 校验与读取合并为一次 DB 查询。
	srv, err := s.checkMCPServerAccess(ctx, req.GetId(), false)
	if err != nil {
		return nil, err
	}
	return toProtoMCP(srv), nil
}

func (s *MCPServerService) UpdateMCPServer(ctx context.Context, req *v1.UpdateMCPServerRequest) (*v1.MCPServer, error) {
	if req.GetMcpServer() == nil {
		return nil, apierror.BadRequest("MCP_SERVER", "mcp_server body is required")
	}
	// P2-B: IDOR guard + 取当前值合并为一次 DB 查询。
	// Proto3 cannot distinguish "field not set" from "set to zero value" (false/0),
	// so we compare against current: only include a field in the patch when the proto
	// value differs from the current persisted value.
	current, err := s.checkMCPServerAccess(ctx, req.GetId(), true)
	if err != nil {
		return nil, err
	}
	patch := patchFromProtoMCPWithDiff(req.GetMcpServer(), current)
	out, err := s.uc.Update(ctx, req.GetId(), patch)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("MCP_SERVER", "mcp server not found")
		}
		s.logMCPFlow(ctx, "mcp.server.update", "MCP 服务器更新失败", err,
			event.P("server_id", req.GetId()),
			event.P("server_key", current.Key))
		return nil, err
	}
	s.logMCPFlow(ctx, "mcp.server.update", fmt.Sprintf("MCP 服务器已更新：%s", out.Name), nil,
		event.P("server_id", out.ID),
		event.P("server_key", out.Key),
		event.P("server_name", out.Name),
		event.P("url", redactMCPConfigURL(out.ConfigJSON)))
	// P0-2B：不再全量标脏。config_json/server_key/enabled 等全部构建有效字段
	// 均已折入 MCPVersionHash（见 agent.ComputeMCPVersionHash 契约），变更经
	// 新 key miss + 热替换惰性传播；纯元数据编辑（name/description）正确地不触发重建。
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbUpdate, "mcp_server"),
		Resource:   "mcp_server",
		ResourceID: out.ID,
		Summary:    fmt.Sprintf("key=%s", out.Key),
	})
	return toProtoMCP(out), nil
}

func (s *MCPServerService) DeleteMCPServer(ctx context.Context, req *v1.DeleteMCPServerRequest) (*emptypb.Empty, error) {
	// P2-B: IDOR guard + 审计摘要取值合并为一次 DB 查询。
	srv, err := s.checkMCPServerAccess(ctx, req.GetId(), true)
	if err != nil {
		return nil, err
	}
	summary := fmt.Sprintf("key=%s", srv.Key)
	serverURL := redactMCPConfigURL(srv.ConfigJSON)
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		s.logMCPFlow(ctx, "mcp.server.remove", "MCP 服务器移除失败", err,
			event.P("server_id", req.GetId()),
			event.P("server_key", srv.Key))
		return nil, err
	}
	s.logMCPFlow(ctx, "mcp.server.remove", fmt.Sprintf("MCP 服务器已移除：%s", srv.Name), nil,
		event.P("server_id", req.GetId()),
		event.P("server_key", srv.Key),
		event.P("server_name", srv.Name),
		event.P("url", serverURL))
	// P0-2B：不再全量标脏。删除使 server 从引用方 effective 列表消失 →
	// MCPVersionHash 变化 → 下一请求新 key miss → 热替换/全量兜底惰性生效。
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbDelete, "mcp_server"),
		Resource:   "mcp_server",
		ResourceID: req.GetId(),
		Summary:    summary,
	})
	return &emptypb.Empty{}, nil
}

func (s *MCPServerService) ValidateMCPServer(ctx context.Context, req *v1.ValidateMCPServerRequest) (*v1.ValidateMCPServerResponse, error) {
	res := s.uc.ValidateConfig(ctx, req.GetConfigJson())
	// P2: 表单校验通过后，best-effort 追加一次真实握手工具发现，让配置对话
	// 框即时反馈「发现 N 个工具」。不持久化（配置可能尚未保存）。
	message := res.Message
	details := res.Details
	if res.OK && !detailsHasToolCount(details) {
		if disc := s.uc.DiscoverToolsForConfig(ctx, req.GetConfigJson()); disc.OK {
			details, message = mergeDiscoveryResult(details, disc)
		} else if disc.Message != "" {
			details = mergeDiscoveryError(details, disc)
		}
	}
	detailsJSON := "{}"
	if len(details) > 0 {
		b, err := json.Marshal(details)
		if err != nil {
			return nil, err
		}
		detailsJSON = string(b)
	}
	return &v1.ValidateMCPServerResponse{
		Ok:          res.OK,
		Status:      res.Status,
		Message:     message,
		DetailsJson: detailsJSON,
	}, nil
}

// detailsHasToolCount reports whether probe Details already carry a discovery
// result (full_handshake mode merged it in biz already).
func detailsHasToolCount(details map[string]any) bool {
	if details == nil {
		return false
	}
	_, ok := details["tool_count"]
	return ok
}

// mergeDiscoveryResult folds a successful discovery into response details and
// returns the user-facing message suffix appended.
func mergeDiscoveryResult(details map[string]any, disc biz.MCPToolDiscoveryResult) (map[string]any, string) {
	out := make(map[string]any, len(details)+2)
	for k, v := range details {
		out[k] = v
	}
	out["tool_count"] = disc.ToolCount
	if len(disc.ToolNames) > 0 {
		names := disc.ToolNames
		if len(names) > 50 {
			names = names[:50]
		}
		out["tool_names"] = names
	}
	return out, fmt.Sprintf("，发现 %d 个工具", disc.ToolCount)
}

// mergeDiscoveryError records a failed discovery in details without changing
// the probe verdict (connectivity OK stands; discovery is orthogonal).
func mergeDiscoveryError(details map[string]any, disc biz.MCPToolDiscoveryResult) map[string]any {
	out := make(map[string]any, len(details)+1)
	for k, v := range details {
		out[k] = v
	}
	out["tools_error"] = disc.Message
	return out
}

func (s *MCPServerService) TestMCPServer(ctx context.Context, req *v1.TestMCPServerRequest) (*v1.MCPServerTestResponse, error) {
	// P2-B: IDOR guard — read-level is sufficient here. Probing does not mutate
	// tenant-owned config; it only refreshes system health bookkeeping metadata
	// (health_status/last_health_at), which the background health runner already
	// writes for shared servers under the system workspace. Using the mutate
	// guard here fail-closed on shared/built-in servers (workspace_id="") and
	// made the UI 测试连接 button permanently 404 for exactly the servers an
	// operator most needs to probe.
	if err := s.assertMCPServerAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	res, err := s.uc.TestMCPServer(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("MCP_SERVER", "mcp server not found")
		}
		return nil, err
	}
	// P2: 手动「测试连接」在连通性通过后追加一次真实握手发现（非
	// full_handshake 模式时 Details 无 tool_count），结果持久化并即时反馈
	// 「发现 N 个工具」。发现失败不改变连通性结论，仅以 tools_error 呈现。
	message := res.Result.Message
	details := res.Result.Details
	if res.Result.OK && !detailsHasToolCount(details) {
		if disc, derr := s.uc.DiscoverMCPServerTools(ctx, req.GetId()); derr != nil {
			s.lg.Warn("MCP 手动测试后工具发现持久化失败",
				loggateway.StepID("mcp.server.tools_discovery"),
				loggateway.Str("server_id", req.GetId()),
				loggateway.Err(derr))
		} else if disc.OK {
			details, message = mergeDiscoveryResult(details, disc)
		} else if disc.Message != "" {
			details = mergeDiscoveryError(details, disc)
		}
	}
	detailsJSON := "{}"
	if len(details) > 0 {
		b, err := json.Marshal(details)
		if err != nil {
			return nil, err
		}
		detailsJSON = string(b)
	}
	return &v1.MCPServerTestResponse{
		Ok:          res.Result.OK,
		Status:      res.Result.Status,
		Message:     message,
		DetailsJson: detailsJSON,
	}, nil
}

func toProtoMCPUserCred(c biz.MCPServerUserCredential) *v1.MCPServerUserCredential {
	return &v1.MCPServerUserCredential{
		Id:            c.ID,
		McpServerId:   c.MCPServerID,
		UserId:        c.UserID,
		CredentialKey: c.CredentialKey,
		Status:        c.Status,
		Configured:    c.Configured,
		MaskedPreview: c.MaskedPreview,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

// resolveMCPCredentialUserID binds credential RPCs to the authenticated principal.
// Non-admins may only act on their own user_id; admins may manage any user.
func resolveMCPCredentialUserID(ctx context.Context, requested string) (string, error) {
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return "", auth.ErrUnauthorized
	}
	self := strconv.FormatInt(a.UserID, 10)
	reqID := strings.TrimSpace(requested)
	if a.HasAdminAccess() {
		if reqID == "" {
			return self, nil
		}
		return reqID, nil
	}
	if reqID != "" && reqID != self {
		return "", apierror.Forbidden("MCP_SERVER", "cannot manage credentials for another user")
	}
	return self, nil
}

func (s *MCPServerService) ListMCPServerUserCredentials(ctx context.Context, req *v1.ListMCPServerUserCredentialsRequest) (*v1.ListMCPServerUserCredentialsResponse, error) {
	// P2-B: IDOR guard — verify caller workspace on parent mcp server.
	if err := s.assertMCPServerAccess(ctx, req.GetMcpServerId()); err != nil {
		return nil, err
	}
	userID, err := resolveMCPCredentialUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	items, err := s.uc.ListUserCredentials(ctx, req.GetMcpServerId(), userID)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListMCPServerUserCredentialsResponse{Items: make([]*v1.MCPServerUserCredential, 0, len(items))}
	for i := range items {
		resp.Items = append(resp.Items, toProtoMCPUserCred(items[i]))
	}
	return resp, nil
}

func (s *MCPServerService) UpsertMCPServerUserCredential(ctx context.Context, req *v1.UpsertMCPServerUserCredentialRequest) (*v1.MCPServerUserCredential, error) {
	// P2-B: IDOR guard — read-level is sufficient here. A user credential is the
	// caller's own data (resolveMCPCredentialUserID binds non-admins to their own
	// user_id), not the server's config; the mutate-level guard would fail-close
	// on shared/built-in servers (workspace_id="") and make per-user credentials
	// unusable for exactly the rows that require them (require_user_credentials).
	// Cross-tenant private servers stay hidden: the workspace-scoped Get inside
	// the guard returns NotFound for them.
	if err := s.assertMCPServerAccess(ctx, req.GetMcpServerId()); err != nil {
		return nil, err
	}
	userID, err := resolveMCPCredentialUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	out, err := s.uc.UpsertUserCredential(ctx, req.GetMcpServerId(), userID, biz.MCPServerUserCredentialInput{
		CredentialKey: req.GetCredentialKey(),
		Secret:        req.GetSecret(),
		Status:        req.GetStatus(),
	})
	if err != nil {
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbCredentials, "mcp_server"),
		Resource:   "mcp_server",
		ResourceID: req.GetMcpServerId(),
		Summary:    fmt.Sprintf("user=%s credential_key=%s op=upsert", userID, req.GetCredentialKey()),
	})
	return toProtoMCPUserCred(out), nil
}

func (s *MCPServerService) DeleteMCPServerUserCredential(ctx context.Context, req *v1.DeleteMCPServerUserCredentialRequest) (*emptypb.Empty, error) {
	// P2-B: IDOR guard — read-level, same rationale as UpsertMCPServerUserCredential.
	if err := s.assertMCPServerAccess(ctx, req.GetMcpServerId()); err != nil {
		return nil, err
	}
	userID, err := resolveMCPCredentialUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err := s.uc.DeleteUserCredential(ctx, req.GetMcpServerId(), userID, req.GetCredentialKey()); err != nil {
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbCredentials, "mcp_server"),
		Resource:   "mcp_server",
		ResourceID: req.GetMcpServerId(),
		Summary:    fmt.Sprintf("user=%s credential_key=%s op=delete", userID, req.GetCredentialKey()),
	})
	return &emptypb.Empty{}, nil
}
