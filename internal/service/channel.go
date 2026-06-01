package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/channel/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/channel/slack"
	"aranea-agents/internal/channel/telegram"
	"aranea-agents/pkg/loggateway"
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
	lg       loggateway.Logger
}

func NewChannelService(uc *biz.ChannelUsecase, turnJobs *biz.ChannelTurnJobUsecase, runtime *ChannelRuntime, lg loggateway.Logger) *ChannelService {
	s := &ChannelService{uc: uc, turnJobs: turnJobs, runtime: runtime, lg: lg}
	s.testers = s.buildLiveTesters()
	return s
}

// buildLiveTesters registers platform-specific live testers.
func (s *ChannelService) buildLiveTesters() map[string]biz.ChannelLiveTester {
	return map[string]biz.ChannelLiveTester{
		"feishu":   biz.ChannelLiveTesterFunc(s.testFeishuLive),
		"slack":    biz.ChannelLiveTesterFunc(s.testSlackLive),
		"telegram": biz.ChannelLiveTesterFunc(s.testTelegramLive),
	}
}

func (s *ChannelService) reloadRuntime(ctx context.Context) {
	if s != nil && s.runtime != nil {
		s.runtime.Reload(ctx)
	}
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

func bizCatalogItemToProto(it biz.ChannelCatalogItem) (*v1.ChannelCatalogItem, error) {
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
	return &v1.ChannelCatalogItem{
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

func (s *ChannelService) ListChannelCatalog(ctx context.Context, _ *emptypb.Empty) (*v1.ListChannelCatalogResponse, error) {
	items := s.uc.Catalog()
	protoItems := make([]*v1.ChannelCatalogItem, 0, len(items))
	for _, it := range items {
		p, err := bizCatalogItemToProto(it)
		if err != nil {
			return nil, err
		}
		protoItems = append(protoItems, p)
	}
	return &v1.ListChannelCatalogResponse{Items: protoItems}, nil
}

func (s *ChannelService) ListChannels(ctx context.Context, _ *emptypb.Empty) (*v1.ListChannelsResponse, error) {
	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.Channel, 0, len(items))
	for _, c := range items {
		out = append(out, channelRowToProto(c, s.runtimeMetadataPatch(c.ID)))
	}
	return &v1.ListChannelsResponse{Items: out}, nil
}

func (s *ChannelService) GetChannel(ctx context.Context, req *v1.GetChannelRequest) (*v1.Channel, error) {
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
	c, err := s.uc.Create(ctx, row, protoCredInputs(req.GetCredentials()))
	if err != nil {
		return nil, err
	}
	s.reloadRuntime(ctx)
	return bizChannelToProto(c), nil
}

func (s *ChannelService) UpdateChannel(ctx context.Context, req *v1.UpdateChannelRequest) (*v1.Channel, error) {
	current, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
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
	c, err := s.uc.Update(ctx, req.GetId(), row, protoCredInputs(req.GetCredentials()))
	if err != nil {
		return nil, err
	}
	if biz.RoutingTargetChanged(current.ConfigJSON, c.ConfigJSON) {
		if _, err := s.uc.DeletePeerBindingsByChannelID(ctx, c.ID); err != nil {
			s.lg.Warn("删除渠道 Peer 绑定失败",
				loggateway.StepID("channel.peer.delete_failed"),
				loggateway.Str("channel_id", c.ID),
				loggateway.Err(err),
			)
		}
	}
	s.reloadRuntime(ctx)
	return bizChannelToProto(c), nil
}

func (s *ChannelService) DeleteChannel(ctx context.Context, req *v1.DeleteChannelRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	s.reloadRuntime(ctx)
	return &emptypb.Empty{}, nil
}

func (s *ChannelService) ToggleChannel(ctx context.Context, req *v1.ToggleChannelRequest) (*v1.Channel, error) {
	c, err := s.uc.Toggle(ctx, req.GetId(), req.GetEnabled())
	if err != nil {
		return nil, err
	}
	s.reloadRuntime(ctx)
	return bizChannelToProto(c), nil
}

func (s *ChannelService) TestChannel(ctx context.Context, req *v1.TestChannelRequest) (*v1.ChannelTestResult, error) {
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
	_ = json.Unmarshal([]byte(row.ConfigJSON), &env)
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
	items, err := s.uc.UpsertCredentials(ctx, req.GetChannelId(), protoCredInputs(req.GetCredentials()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.ChannelCredential, 0, len(items))
	for _, c := range items {
		out = append(out, bizCredToProto(c))
	}
	s.reloadRuntime(ctx)
	return &v1.ListChannelCredentialsResponse{Items: out}, nil
}

func (s *ChannelService) DeleteChannelCredential(ctx context.Context, req *v1.DeleteChannelCredentialRequest) (*emptypb.Empty, error) {
	if err := s.uc.DeleteCredential(ctx, req.GetChannelId(), req.GetCredentialKey()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *ChannelService) ListChannelDeliveries(ctx context.Context, req *v1.ListChannelDeliveriesRequest) (*v1.ListChannelDeliveriesResponse, error) {
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
	token, terr := resolveCredentialPlain(ctx, s.uc, creds, "bot_token", s.lg)
	if terr != nil || strings.TrimSpace(token) == "" {
		return biz.ChannelTestResult{OK: false, Status: "pending_auth", Message: "bot_token not configured"}
	}
	if err := slack.AuthTest(ctx, lark.DefaultHTTPClient(), token); err != nil {
		return biz.ChannelTestResult{OK: false, Status: "error", Message: err.Error()}
	}
	return biz.ChannelTestResult{OK: true, Status: "ok", Message: "slack auth.test ok"}
}

// testTelegramLive performs a live telegram getMe test.
func (s *ChannelService) testTelegramLive(ctx context.Context, configJSON string, creds []biz.ChannelCredential) biz.ChannelTestResult {
	token, terr := resolveCredentialPlain(ctx, s.uc, creds, "bot_token", s.lg)
	if terr != nil || strings.TrimSpace(token) == "" {
		return biz.ChannelTestResult{OK: false, Status: "pending_auth", Message: "bot_token not configured"}
	}
	if err := telegram.GetMe(ctx, lark.DefaultHTTPClient(), token); err != nil {
		return biz.ChannelTestResult{OK: false, Status: "error", Message: err.Error()}
	}
	return biz.ChannelTestResult{OK: true, Status: "ok", Message: "telegram getMe ok"}
}
