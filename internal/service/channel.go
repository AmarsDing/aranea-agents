package service

import (
	"context"
	"encoding/json"
	"strings"

	v1 "aranea-agents/api/kratos/channel/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// ChannelService implements kratos channel.v1.
type ChannelService struct {
	v1.UnimplementedChannelServiceServer

	uc *biz.ChannelUsecase
}

func NewChannelService(uc *biz.ChannelUsecase) *ChannelService {
	return &ChannelService{uc: uc}
}

func bizChannelToProto(c biz.Channel) *v1.Channel {
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
		MetadataJson: c.MetadataJSON,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
		DeletedAt:    c.DeletedAt,
	}
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
		Type:                  it.Type,
		Label:                 it.Label,
		Description:           it.Description,
		Group:                 it.Group,
		ReceiveModes:          append([]string(nil), it.ReceiveModes...),
		Icon:                  it.Icon,
		Bundled:               it.Bundled,
		SupportsTest:          it.SupportsTest,
		SupportsWebhook:       it.SupportsWebhook,
		ConfigSchemaJson:      string(cfg),
		CredentialSchemaJson:  string(cred),
		UiHintsJson:           string(ui),
		SortOrder:             int32(it.SortOrder),
	}, nil
}

func bizCredToProto(c biz.ChannelCredential) *v1.ChannelCredential {
	return &v1.ChannelCredential{
		Id:             c.ID,
		ChannelId:      c.ChannelID,
		CredentialKey:  c.CredentialKey,
		Status:         c.Status,
		MetadataJson:   c.MetadataJSON,
		Configured:     c.Configured,
		MaskedPreview:  c.MaskedPreview,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
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
		out = append(out, bizChannelToProto(c))
	}
	return &v1.ListChannelsResponse{Items: out}, nil
}

func (s *ChannelService) GetChannel(ctx context.Context, req *v1.GetChannelRequest) (*v1.Channel, error) {
	c, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return bizChannelToProto(c), nil
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
	return bizChannelToProto(c), nil
}

func (s *ChannelService) UpdateChannel(ctx context.Context, req *v1.UpdateChannelRequest) (*v1.Channel, error) {
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
	return bizChannelToProto(c), nil
}

func (s *ChannelService) DeleteChannel(ctx context.Context, req *v1.DeleteChannelRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *ChannelService) ToggleChannel(ctx context.Context, req *v1.ToggleChannelRequest) (*v1.Channel, error) {
	c, err := s.uc.Toggle(ctx, req.GetId(), req.GetEnabled())
	if err != nil {
		return nil, err
	}
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
	if strings.EqualFold(strings.TrimSpace(env.Type), "feishu") && result.OK {
		region, appID, ferr := feishuAppAndRegion(row.ConfigJSON)
		if ferr != nil {
			result = biz.ChannelTestResult{OK: false, Status: "error", Message: ferr.Error()}
		} else {
			appRef, cerr := ChannelCredentialSecretRef(creds, "app_secret")
			if cerr != nil {
				result = biz.ChannelTestResult{OK: false, Status: "pending_auth", Message: cerr.Error()}
			} else {
				sec, serr := ResolveSecretRef(appRef)
				if serr != nil {
					result = biz.ChannelTestResult{OK: false, Status: "pending_auth", Message: serr.Error()}
				} else {
					_, _, terr := lark.FetchTenantAccessToken(ctx, lark.DefaultHTTPClient(), region, appID, sec)
					if terr != nil {
						result = biz.ChannelTestResult{OK: false, Status: "error", Message: terr.Error()}
					} else {
						result = biz.ChannelTestResult{OK: true, Status: "ok", Message: "tenant_access_token acquired"}
					}
				}
			}
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
