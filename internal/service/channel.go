package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/channel/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/channel/slack"
	"aranea-agents/internal/channel/telegram"
	"aranea-agents/internal/channel/wechatilink"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// ChannelService implements kratos channel.v1.
type ChannelService struct {
	v1.UnimplementedChannelServiceServer

	uc       *biz.ChannelUsecase
	turnJobs *biz.ChannelTurnJobUsecase
	runtime  *ChannelRuntime
	testers  map[string]biz.ChannelLiveTester
	mon      *biz.MonitorUsecase
	lg       loggateway.Logger
}

func NewChannelService(uc *biz.ChannelUsecase, turnJobs *biz.ChannelTurnJobUsecase, runtime *ChannelRuntime, mon *biz.MonitorUsecase, lg loggateway.Logger) *ChannelService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	s := &ChannelService{uc: uc, turnJobs: turnJobs, runtime: runtime, mon: mon, lg: lg}
	s.testers = s.buildLiveTesters()
	return s
}

// buildLiveTesters registers platform-specific live testers.
func (s *ChannelService) buildLiveTesters() map[string]biz.ChannelLiveTester {
	return map[string]biz.ChannelLiveTester{
		"feishu":       biz.ChannelLiveTesterFunc(s.testFeishuLive),
		"slack":        biz.ChannelLiveTesterFunc(s.testSlackLive),
		"telegram":     biz.ChannelLiveTesterFunc(s.testTelegramLive),
		"wechat_ilink": biz.ChannelLiveTesterFunc(s.testWechatILinkLive),
	}
}

// reloadRuntime triggers an async runtime reconcile. Channel CRUD must not
// block the API response on connector shutdown — Manager.Reload waits up to
// runtimeReplaceShutdownWait (15s) for a stale connector to exit after
// cancel, which previously blocked PATCH for >15s. The reload runs in the
// background; the periodic reconciler (default 2m, channel_runtime.go) is
// the safety net if a triggered reload fails.
func (s *ChannelService) reloadRuntime(ctx context.Context) {
	if s == nil || s.runtime == nil {
		return
	}
	rt := s.runtime
	safego.Go(context.WithoutCancel(ctx), "channel.reloadRuntime", func() {
		rt.Reload(context.Background())
	})
}

// assertChannelAccess 校验 caller 是否可读取指定 channel（P2-B IDOR 防护）。
// 跨租户访问返回 NotFound（避免泄露 channel 存在性）。
// 共享 channel（workspace_id=""）对所有租户可读；变更须用 assertChannelMutateAccess。
func (s *ChannelService) assertChannelAccess(ctx context.Context, channelID string) error {
	return s.checkChannelAccess(ctx, channelID, false)
}

// assertChannelMutateAccess 校验 caller 是否可变更指定 channel。
// 共享 channel（workspace_id=""）对租户只读（fail-closed）。
func (s *ChannelService) assertChannelMutateAccess(ctx context.Context, channelID string) error {
	return s.checkChannelAccess(ctx, channelID, true)
}

func (s *ChannelService) checkChannelAccess(ctx context.Context, channelID string, mutate bool) error {
	if channelID == "" {
		return nil
	}
	c, err := s.uc.Get(ctx, channelID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound("CHANNEL", "channel not found")
		}
		return err
	}
	callerWS := workspace.IDFromContext(ctx)
	if mutate {
		err = workspace.AssertWorkspaceMutate(callerWS, c.WorkspaceID)
	} else {
		err = workspace.AssertWorkspaceOrShared(callerWS, c.WorkspaceID)
	}
	if err != nil {
		s.lg.Warn("channel access denied: workspace mismatch",
			loggateway.StepID("channel.idor"),
			loggateway.Str("channel_id", channelID),
			loggateway.Str("caller_ws", callerWS))
		return apierror.NotFound("CHANNEL", "channel not found")
	}
	return nil
}

func bizChannelToProto(c biz.Channel) *v1.Channel {
	return channelRowToProto(c, "")
}

func channelRowToProto(c biz.Channel, runtimeMetaJSON string) *v1.Channel {
	meta := c.MetadataJSON
	if strings.TrimSpace(runtimeMetaJSON) != "" {
		meta = mergeChannelMetadataJSON(c.MetadataJSON, runtimeMetaJSON)
	}
	return &v1.Channel{
		Id:           c.ID,
		Resource:     c.Resource,
		Key:          c.Key,
		Name:         c.Name,
		Description:  c.Description,
		Status:       c.Status,
		Enabled:      c.Enabled,
		SortOrder:    int32(c.SortOrder),
		ParentId:     c.ParentID,
		Level:        c.Level,
		AgentId:      c.AgentID,
		Provider:     c.Provider,
		Model:        c.Model,
		ConfigJson:   c.ConfigJSON,
		MetadataJson: meta,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
		DeletedAt:    c.DeletedAt,
	}
}

func mergeChannelMetadataJSON(base, patch string) string {
	var a, b map[string]any
	if json.Unmarshal([]byte(strings.TrimSpace(base)), &a) != nil || a == nil {
		a = map[string]any{}
	}
	if json.Unmarshal([]byte(strings.TrimSpace(patch)), &b) == nil {
		for k, v := range b {
			a[k] = v
		}
	}
	out, err := json.Marshal(a)
	if err != nil {
		return base
	}
	return string(out)
}

func (s *ChannelService) runtimeMetadataPatch(channelID string) string {
	if s == nil || s.runtime == nil {
		return ""
	}
	info := s.runtime.ConnectionInfo(channelID)
	if !info.Connected {
		return ""
	}
	patch := map[string]any{"runtime_connected": true}
	if !info.ConnectedSince.IsZero() {
		patch["runtime_connected_since"] = info.ConnectedSince.UTC().Format(time.RFC3339)
	}
	if !info.LastDisconnect.IsZero() {
		patch["runtime_last_disconnect"] = info.LastDisconnect.UTC().Format(time.RFC3339)
	}
	b, err := json.Marshal(patch)
	if err != nil {
		return ""
	}
	return string(b)
}

func bizTypeItemToProto(it biz.ChannelTypeItem) (*v1.ChannelTypeItem, error) {
	cfg, err := json.Marshal(it.ConfigSchema)
	if err != nil {
		return nil, err
	}
	cred, err := json.Marshal(it.CredentialSchema)
	if err != nil {
		return nil, err
	}
	ui, err := json.Marshal(it.UIHints)
	if err != nil {
		return nil, err
	}
	return &v1.ChannelTypeItem{
		Type:                 it.Type,
		Label:                it.Label,
		Description:          it.Description,
		Group:                it.Group,
		ReceiveModes:         append([]string(nil), it.ReceiveModes...),
		Icon:                 it.Icon,
		Bundled:              it.Bundled,
		SupportsTest:         it.SupportsTest,
		SupportsWebhook:      it.SupportsWebhook,
		ConfigSchemaJson:     string(cfg),
		CredentialSchemaJson: string(cred),
		UiHintsJson:          string(ui),
		SortOrder:            int32(it.SortOrder),
	}, nil
}

func bizCredToProto(c biz.ChannelCredential) *v1.ChannelCredential {
	return &v1.ChannelCredential{
		Id:            c.ID,
		ChannelId:     c.ChannelID,
		CredentialKey: c.CredentialKey,
		Status:        c.Status,
		MetadataJson:  c.MetadataJSON,
		Configured:    c.Configured,
		MaskedPreview: c.MaskedPreview,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

func bizDeliveryToProto(d biz.ChannelDelivery) *v1.ChannelDelivery {
	return &v1.ChannelDelivery{
		Id:           d.ID,
		ChannelId:    d.ChannelID,
		AgentId:      d.AgentID,
		Status:       d.Status,
		PayloadJson:  d.PayloadJSON,
		ErrorMessage: d.ErrorMessage,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
	}
}

func bizTestToProto(t biz.ChannelTestResult) (*v1.ChannelTestResult, error) {
	out := &v1.ChannelTestResult{
		Ok:      t.OK,
		Status:  t.Status,
		Message: t.Message,
	}
	if len(t.Details) > 0 {
		b, err := json.Marshal(t.Details)
		if err != nil {
			return nil, err
		}
		out.DetailsJson = string(b)
	}
	return out, nil
}

func protoCredInputs(in []*v1.ChannelCredentialInput) []biz.ChannelCredentialInput {
	if len(in) == 0 {
		return nil
	}
	out := make([]biz.ChannelCredentialInput, 0, len(in))
	for _, x := range in {
		if x == nil {
			continue
		}
		out = append(out, biz.ChannelCredentialInput{
			CredentialKey: x.GetCredentialKey(),
			Secret:        x.GetSecret(),
			SecretRef:     x.GetSecretRef(),
			Status:        x.GetStatus(),
			MetadataJSON:  x.GetMetadataJson(),
		})
	}
	return out
}

func (s *ChannelService) ListChannelTypes(ctx context.Context, _ *emptypb.Empty) (*v1.ListChannelTypesResponse, error) {
	items := s.uc.ChannelTypes()
	protoItems := make([]*v1.ChannelTypeItem, 0, len(items))
	for _, it := range items {
		p, err := bizTypeItemToProto(it)
		if err != nil {
			return nil, err
		}
		protoItems = append(protoItems, p)
	}
	return &v1.ListChannelTypesResponse{Items: protoItems}, nil
}

func (s *ChannelService) ListChannels(ctx context.Context, _ *emptypb.Empty) (*v1.ListChannelsResponse, error) {
	search := searchQueryFromContext(ctx)
	if page, pageSize, ok := pageQueryFromContext(ctx); ok {
		limit, offset, page, pageSize := biz.PageToLimitOffset(page, pageSize)
		result, err := s.uc.ListPaged(ctx, biz.ChannelListQuery{
			Search: search,
			Type:   queryParamFromContext(ctx, "type"),
			Status: queryParamFromContext(ctx, "status"),
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		// Data ListPaged already applies workspace visibility via channelWorkspacePredicate.
		out := make([]*v1.Channel, 0, len(result.Items))
		for _, c := range result.Items {
			out = append(out, channelRowToProto(c, s.runtimeMetadataPatch(c.ID)))
		}
		return &v1.ListChannelsResponse{
			Items:    out,
			Total:    int32(result.Total),
			Page:     page,
			PageSize: pageSize,
		}, nil
	}

	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	// P2-B: workspace visibility filter (service layer — ChannelReader.List is
	// Stability:stable and RunHealthChecks needs all channels, so we filter here).
	// System caller (cron/admin) sees all; tenant caller sees shared (workspace_id="") + own.
	if !workspace.IsSystem(ctx) {
		ws := workspace.IDFromContext(ctx)
		filtered := make([]biz.Channel, 0, len(items))
		for _, c := range items {
			if c.WorkspaceID == "" || c.WorkspaceID == ws {
				filtered = append(filtered, c)
			}
		}
		items = filtered
	}
	if search != "" {
		needle := strings.ToLower(strings.TrimSpace(search))
		filtered := make([]biz.Channel, 0, len(items))
		for _, c := range items {
			if strings.Contains(strings.ToLower(c.Key), needle) ||
				strings.Contains(strings.ToLower(c.Name), needle) ||
				strings.Contains(strings.ToLower(c.Description), needle) {
				filtered = append(filtered, c)
			}
		}
		items = filtered
	}
	out := make([]*v1.Channel, 0, len(items))
	for _, c := range items {
		out = append(out, channelRowToProto(c, s.runtimeMetadataPatch(c.ID)))
	}
	return &v1.ListChannelsResponse{
		Items:    out,
		Total:    int32(len(out)),
		Page:     1,
		PageSize: int32(len(out)),
	}, nil
}

func (s *ChannelService) GetChannel(ctx context.Context, req *v1.GetChannelRequest) (*v1.Channel, error) {
	if err := s.assertChannelAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	c, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return channelRowToProto(c, s.runtimeMetadataPatch(c.ID)), nil
}

func (s *ChannelService) CreateChannel(ctx context.Context, req *v1.CreateChannelRequest) (*v1.Channel, error) {
	row := biz.Channel{
		Key:          req.GetKey(),
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		Status:       req.GetStatus(),
		Enabled:      req.GetEnabled(),
		SortOrder:    int(req.GetSortOrder()),
		ConfigJSON:   req.GetConfigJson(),
		MetadataJSON: req.GetMetadataJson(),
	}
	// P2-B: tenant isolation — tenant caller creates tenant-private channel;
	// system caller (cron/admin) creates shared channel (workspace_id="").
	if !workspace.IsSystem(ctx) {
		row.WorkspaceID = workspace.IDFromContext(ctx)
	}
	c, err := s.uc.Create(ctx, row, protoCredInputs(req.GetCredentials()))
	if err != nil {
		return nil, err
	}
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbCreate, "channel"),
		Resource:   "channel",
		ResourceID: c.ID,
		Summary:    fmt.Sprintf("key=%s", c.Key),
	})
	s.reloadRuntime(ctx)
	return bizChannelToProto(c), nil
}

func (s *ChannelService) UpdateChannel(ctx context.Context, req *v1.UpdateChannelRequest) (*v1.Channel, error) {
	if err := s.assertChannelMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	current, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	key := req.GetKey()
	name := req.GetName()
	description := req.GetDescription()
	enabled := req.GetEnabled()
	sortOrder := int(req.GetSortOrder())
	configJSON := req.GetConfigJson()
	metadataJSON := req.GetMetadataJson()
	c, err := s.uc.Update(ctx, req.GetId(), biz.ChannelUpdateOptions{
		Key:          &key,
		Name:         &name,
		Description:  &description,
		Enabled:      &enabled,
		SortOrder:    &sortOrder,
		ConfigJSON:   &configJSON,
		MetadataJSON: &metadataJSON,
	}, protoCredInputs(req.GetCredentials()))
	if err != nil {
		return nil, err
	}
	if biz.RoutingTargetChanged(current.ConfigJSON, c.ConfigJSON) {
		if _, err := s.uc.DeletePeerBindingsByChannelID(ctx, c.ID); err != nil {
			s.lg.Error("删除渠道 Peer 绑定失败，中止 runtime reload",
				loggateway.StepID("channel.peer.delete_failed"),
				loggateway.Str("channel_id", c.ID),
				loggateway.Err(err),
			)
			return nil, err
		}
	}
	s.reloadRuntime(ctx)
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbUpdate, "channel"),
		Resource:   "channel",
		ResourceID: c.ID,
		Summary:    fmt.Sprintf("key=%s", c.Key),
	})
	return bizChannelToProto(c), nil
}

func (s *ChannelService) DeleteChannel(ctx context.Context, req *v1.DeleteChannelRequest) (*emptypb.Empty, error) {
	if err := s.assertChannelMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	// 先取 key 供审计 summary 使用（best-effort，取不到不阻断删除）。
	summary := ""
	if c, err := s.uc.Get(ctx, req.GetId()); err == nil {
		summary = fmt.Sprintf("key=%s", c.Key)
	}
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	s.reloadRuntime(ctx)
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbDelete, "channel"),
		Resource:   "channel",
		ResourceID: req.GetId(),
		Summary:    summary,
	})
	return &emptypb.Empty{}, nil
}

func (s *ChannelService) ToggleChannel(ctx context.Context, req *v1.ToggleChannelRequest) (*v1.Channel, error) {
	if err := s.assertChannelMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	c, err := s.uc.Toggle(ctx, req.GetId(), req.GetEnabled())
	if err != nil {
		return nil, err
	}
	s.reloadRuntime(ctx)
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbToggle, "channel"),
		Resource:   "channel",
		ResourceID: c.ID,
		Summary:    fmt.Sprintf("key=%s enabled=%t", c.Key, c.Enabled),
	})
	return bizChannelToProto(c), nil
}

func (s *ChannelService) TestChannel(ctx context.Context, req *v1.TestChannelRequest) (*v1.ChannelTestResult, error) {
	if err := s.assertChannelMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	row, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	creds, err := s.uc.ListCredentialsRaw(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	result, err := biz.EvaluateChannelTest(row, creds)
	if err != nil {
		return nil, err
	}
	var env struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(row.ConfigJSON), &env); err != nil {
		s.lg.Warn("channel test config json unmarshal failed", loggateway.StepID("channel.test.config_parse"), loggateway.Err(err))
	}
	channelType := strings.TrimSpace(strings.ToLower(env.Type))
	if result.OK && channelType != "" {
		if tester, ok := s.testers[channelType]; ok {
			result = tester.TestLive(ctx, row.ConfigJSON, creds)
		}
	}
	final, err := s.uc.CommitChannelTest(ctx, row, creds, result)
	if err != nil {
		return nil, err
	}
	return bizTestToProto(final)
}

func (s *ChannelService) ListChannelCredentials(ctx context.Context, req *v1.GetChannelRequest) (*v1.ListChannelCredentialsResponse, error) {
	if err := s.assertChannelAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	items, err := s.uc.ListCredentials(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.ChannelCredential, 0, len(items))
	for _, c := range items {
		out = append(out, bizCredToProto(c))
	}
	return &v1.ListChannelCredentialsResponse{Items: out}, nil
}

func (s *ChannelService) UpsertChannelCredentials(ctx context.Context, req *v1.UpsertChannelCredentialsRequest) (*v1.ListChannelCredentialsResponse, error) {
	if err := s.assertChannelMutateAccess(ctx, req.GetChannelId()); err != nil {
		return nil, err
	}
	items, err := s.uc.UpsertCredentials(ctx, req.GetChannelId(), protoCredInputs(req.GetCredentials()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.ChannelCredential, 0, len(items))
	for _, c := range items {
		out = append(out, bizCredToProto(c))
	}
	s.reloadRuntime(ctx)
	// 凭据变更属高危操作：仅记录数量，严禁记录 secret 内容。
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbCredentials, "channel"),
		Resource:   "channel",
		ResourceID: req.GetChannelId(),
		Summary:    fmt.Sprintf("upsert count=%d", len(req.GetCredentials())),
	})
	return &v1.ListChannelCredentialsResponse{Items: out}, nil
}

func (s *ChannelService) DeleteChannelCredential(ctx context.Context, req *v1.DeleteChannelCredentialRequest) (*emptypb.Empty, error) {
	if err := s.assertChannelMutateAccess(ctx, req.GetChannelId()); err != nil {
		return nil, err
	}
	if err := s.uc.DeleteCredential(ctx, req.GetChannelId(), req.GetCredentialKey()); err != nil {
		return nil, err
	}
	s.reloadRuntime(ctx)
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbCredentials, "channel"),
		Resource:   "channel",
		ResourceID: req.GetChannelId(),
		Summary:    fmt.Sprintf("delete key=%s", req.GetCredentialKey()),
	})
	return &emptypb.Empty{}, nil
}

func (s *ChannelService) ListChannelDeliveries(ctx context.Context, req *v1.ListChannelDeliveriesRequest) (*v1.ListChannelDeliveriesResponse, error) {
	if err := s.assertChannelAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	items, err := s.uc.ListDeliveries(ctx, req.GetId(), limit)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.ChannelDelivery, 0, len(items))
	for _, d := range items {
		out = append(out, bizDeliveryToProto(d))
	}
	return &v1.ListChannelDeliveriesResponse{Items: out}, nil
}

func bizTurnJobToProto(j biz.ChannelTurnJob) *v1.ChannelTurnJob {
	return &v1.ChannelTurnJob{
		Id:               strutil.ValidUTF8(j.ID),
		ChannelId:        strutil.ValidUTF8(j.ChannelID),
		SessionId:        strutil.ValidUTF8(j.SessionID),
		PeerId:           strutil.ValidUTF8(j.PeerID),
		PeerKey:          strutil.ValidUTF8(j.PeerKey),
		IdempotencyKey:   strutil.ValidUTF8(j.IdempotencyKey),
		Status:           strutil.ValidUTF8(j.Status),
		PreviewMessageId: strutil.ValidUTF8(j.PreviewMessageID),
		ContentPreview:   strutil.ValidUTF8(j.ContentPreview),
		AsyncTargetType:  strutil.ValidUTF8(j.AsyncTargetType),
		AsyncTargetId:    strutil.ValidUTF8(j.AsyncTargetID),
		ErrorMessage:     strutil.ValidUTF8(j.ErrorMessage),
		StartedAt:        strutil.ValidUTF8(j.StartedAt),
		FinishedAt:       strutil.ValidUTF8(j.FinishedAt),
		CreatedAt:        strutil.ValidUTF8(j.CreatedAt),
		UpdatedAt:        strutil.ValidUTF8(j.UpdatedAt),
	}
}

func (s *ChannelService) ListChannelTurnJobs(ctx context.Context, req *v1.ListChannelTurnJobsRequest) (*v1.ListChannelTurnJobsResponse, error) {
	if s == nil || s.turnJobs == nil {
		return &v1.ListChannelTurnJobsResponse{}, nil
	}
	if err := s.assertChannelAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	items, err := s.turnJobs.ListByChannel(ctx, req.GetId(), limit)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.ChannelTurnJob, 0, len(items))
	for _, job := range items {
		out = append(out, bizTurnJobToProto(job))
	}
	return &v1.ListChannelTurnJobsResponse{Items: out}, nil
}

// testFeishuLive performs a live feishu/lark tenant_access_token test.
func (s *ChannelService) testFeishuLive(ctx context.Context, configJSON string, creds []biz.ChannelCredential) biz.ChannelTestResult {
	region, appID, ferr := feishuAppAndRegion(configJSON)
	if ferr != nil {
		return biz.ChannelTestResult{OK: false, Status: "error", Message: ferr.Error()}
	}
	appRef, cerr := ChannelCredentialSecretRef(creds, "app_secret")
	if cerr != nil {
		return biz.ChannelTestResult{OK: false, Status: "pending_auth", Message: cerr.Error()}
	}
	sec, serr := ResolveSecretRef(ctx, s.uc, appRef)
	if serr != nil {
		return biz.ChannelTestResult{OK: false, Status: "pending_auth", Message: serr.Error()}
	}
	_, _, terr := lark.FetchTenantAccessToken(ctx, lark.DefaultHTTPClient(), region, appID, sec)
	if terr != nil {
		msg := terr.Error()
		if strings.Contains(msg, "code=10003") {
			msg += " — 请检查 App ID、App Secret 是否与飞书开放平台一致，并确认 region（国内飞书 / 国际 Lark）"
		}
		return biz.ChannelTestResult{OK: false, Status: "error", Message: msg}
	}
	return biz.ChannelTestResult{OK: true, Status: "ok", Message: "tenant_access_token acquired"}
}

// testSlackLive performs a live slack auth.test.
func (s *ChannelService) testSlackLive(ctx context.Context, configJSON string, creds []biz.ChannelCredential) biz.ChannelTestResult {
	token, terr := resolveCredentialPlain(ctx, s.uc, creds, "bot_token")
	if terr != nil || strings.TrimSpace(token) == "" {
		return biz.ChannelTestResult{OK: false, Status: "pending_auth", Message: "bot_token not configured"}
	}
	if err := slack.AuthTest(ctx, lark.DefaultHTTPClient(), token, s.lg); err != nil {
		return biz.ChannelTestResult{OK: false, Status: "error", Message: err.Error()}
	}
	return biz.ChannelTestResult{OK: true, Status: "ok", Message: "slack auth.test ok"}
}

// testTelegramLive performs a live telegram getMe test.
func (s *ChannelService) testTelegramLive(ctx context.Context, configJSON string, creds []biz.ChannelCredential) biz.ChannelTestResult {
	token, terr := resolveCredentialPlain(ctx, s.uc, creds, "bot_token")
	if terr != nil || strings.TrimSpace(token) == "" {
		return biz.ChannelTestResult{OK: false, Status: "pending_auth", Message: "bot_token not configured"}
	}
	if err := telegram.GetMe(ctx, lark.DefaultHTTPClient(), token, s.lg); err != nil {
		return biz.ChannelTestResult{OK: false, Status: "error", Message: err.Error()}
	}
	return biz.ChannelTestResult{OK: true, Status: "ok", Message: "telegram getMe ok"}
}

// testWechatILinkLive performs a live wechat_ilink getconfig probe (read-only).
// getconfig 必传 ilink_user_id（iLink 网关未文档化要求），取自扫码登录时写入的凭据。
func (s *ChannelService) testWechatILinkLive(ctx context.Context, configJSON string, creds []biz.ChannelCredential) biz.ChannelTestResult {
	token, terr := resolveCredentialPlain(ctx, s.uc, creds, "bot_token")
	if terr != nil || strings.TrimSpace(token) == "" {
		return biz.ChannelTestResult{OK: false, Status: "pending_auth", Message: "bot_token not configured，请先扫码登录"}
	}
	ilinkUserID, _ := resolveCredentialPlain(ctx, s.uc, creds, "ilink_user_id")
	baseURL, _ := resolveCredentialPlain(ctx, s.uc, creds, "baseurl")
	if err := wechatilink.TestConnection(ctx, lark.DefaultHTTPClient(), baseURL, token, ilinkUserID, s.lg); err != nil {
		s.lg.Warn("wechat_ilink live test failed", loggateway.Err(err), loggateway.Str("step_id", "channel.wechat_ilink.test_fail"))
		return biz.ChannelTestResult{OK: false, Status: "error", Message: err.Error()}
	}
	return biz.ChannelTestResult{OK: true, Status: "ok", Message: "wechat_ilink getconfig ok"}
}
