package service

import (
	"context"
	"strconv"
	"strings"

	v1 "aranea-agents/api/kratos/credential/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/fleet"
	"aranea-agents/pkg/apierror"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CredentialService implements kratos credential.v1（E 阶段凭据托管）。
//
// 明文三不进：本层绝不把 request.Secret 写入任何日志；返回的 Credential
// proto 消息无 secret 字段（proto 层合同）。secret 只在 → biz.Create/Update
// 的一次调用中存在，随后由 biz 加密并丢弃。
type CredentialService struct {
	v1.UnimplementedCredentialServiceServer

	uc *biz.CredentialUsecase
}

func NewCredentialService(uc *biz.CredentialUsecase) *CredentialService {
	if uc != nil {
		fleet.SetKeyVault(toolKeyVault{uc: uc}) // 编码工具的 API Key 存在这里（不改 wire 依赖图）
	}
	return &CredentialService{uc: uc}
}

func parseCredentialID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, apierror.BadRequest("CREDENTIAL", "invalid credential id")
	}
	return id, nil
}

func protoSecretKindToBiz(k v1.SecretKind) string {
	switch k {
	case v1.SecretKind_SECRET_KIND_PASSWORD:
		return biz.CredentialKindPassword
	case v1.SecretKind_SECRET_KIND_TOKEN:
		return biz.CredentialKindToken
	case v1.SecretKind_SECRET_KIND_COOKIE:
		return biz.CredentialKindCookie
	default:
		return ""
	}
}

func bizSecretKindToProto(k string) v1.SecretKind {
	switch k {
	case biz.CredentialKindPassword:
		return v1.SecretKind_SECRET_KIND_PASSWORD
	case biz.CredentialKindToken:
		return v1.SecretKind_SECRET_KIND_TOKEN
	case biz.CredentialKindCookie:
		return v1.SecretKind_SECRET_KIND_COOKIE
	default:
		return v1.SecretKind_SECRET_KIND_UNSPECIFIED
	}
}

func protoSensitivityToBiz(s v1.Sensitivity) string {
	switch s {
	case v1.Sensitivity_SENSITIVITY_NORMAL:
		return biz.CredentialSensitivityNormal
	case v1.Sensitivity_SENSITIVITY_HIGH:
		return biz.CredentialSensitivityHigh
	default:
		return ""
	}
}

func bizSensitivityToProto(s string) v1.Sensitivity {
	switch s {
	case biz.CredentialSensitivityHigh:
		return v1.Sensitivity_SENSITIVITY_HIGH
	case biz.CredentialSensitivityNormal:
		return v1.Sensitivity_SENSITIVITY_NORMAL
	default:
		return v1.Sensitivity_SENSITIVITY_UNSPECIFIED
	}
}

func toProtoCredential(c biz.Credential) *v1.Credential {
	return &v1.Credential{
		Id:          strconv.FormatInt(c.ID, 10),
		WorkspaceId: c.WorkspaceID,
		Name:        c.Name,
		Site:        c.Site,
		Url:         c.URL,
		Username:    c.Username,
		SecretKind:  bizSecretKindToProto(c.SecretKind),
		Sensitivity: bizSensitivityToProto(c.Sensitivity),
		Note:        c.Note,
		CreatedAt:   timestamppb.New(c.CreatedAt),
		UpdatedAt:   timestamppb.New(c.UpdatedAt),
	}
}

func toProtoCredentialExecution(e biz.CredentialExecution) *v1.CredentialExecution {
	return &v1.CredentialExecution{
		Id:           strconv.FormatInt(e.ID, 10),
		CredentialId: strconv.FormatInt(e.CredentialID, 10),
		Site:         e.Site,
		Action:       e.Action,
		ActorType:    e.ActorType,
		ActorKey:     e.ActorKey,
		AgentId:      e.AgentID,
		SessionId:    e.SessionID,
		Verdict:      e.Verdict,
		Reason:       e.Reason,
		CreatedAt:    timestamppb.New(e.CreatedAt),
	}
}

func (s *CredentialService) ListCredentials(ctx context.Context, req *v1.ListCredentialsRequest) (*v1.ListCredentialsResponse, error) {
	items, total, err := s.uc.List(ctx, biz.CredentialListQuery{
		Site:     strings.TrimSpace(req.GetSite()),
		Page:     int(req.GetPage()),
		PageSize: int(req.GetPageSize()),
	})
	if err != nil {
		return nil, err
	}
	out := &v1.ListCredentialsResponse{Total: total}
	for _, c := range items {
		out.Items = append(out.Items, toProtoCredential(c))
	}
	return out, nil
}

func (s *CredentialService) CreateCredential(ctx context.Context, req *v1.CreateCredentialRequest) (*v1.Credential, error) {
	kind := protoSecretKindToBiz(req.GetSecretKind())
	if kind == "" {
		return nil, apierror.BadRequest("CREDENTIAL", "secret_kind is required")
	}
	c, err := s.uc.Create(ctx, biz.CredentialCreateInput{
		Name:        req.GetName(),
		Site:        req.GetSite(),
		URL:         req.GetUrl(),
		Username:    req.GetUsername(),
		SecretKind:  kind,
		Sensitivity: protoSensitivityToBiz(req.GetSensitivity()),
		Note:        req.GetNote(),
		Secret:      req.GetSecret(),
	})
	if err != nil {
		return nil, err
	}
	return toProtoCredential(c), nil
}

func (s *CredentialService) GetCredential(ctx context.Context, req *v1.GetCredentialRequest) (*v1.Credential, error) {
	id, err := parseCredentialID(req.GetId())
	if err != nil {
		return nil, err
	}
	c, err := s.uc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return toProtoCredential(c), nil
}

func (s *CredentialService) UpdateCredential(ctx context.Context, req *v1.UpdateCredentialRequest) (*v1.Credential, error) {
	id, err := parseCredentialID(req.GetId())
	if err != nil {
		return nil, err
	}
	in := biz.CredentialUpdateInput{}
	if req.Name != nil {
		in.Name = req.Name
	}
	if req.Url != nil {
		in.URL = req.Url
	}
	if req.Username != nil {
		in.Username = req.Username
	}
	if req.Sensitivity != nil {
		v := protoSensitivityToBiz(req.GetSensitivity())
		if v == "" {
			return nil, apierror.BadRequest("CREDENTIAL", "sensitivity must be one of normal|high")
		}
		in.Sensitivity = &v
	}
	if req.Note != nil {
		in.Note = req.Note
	}
	if req.Secret != nil && strings.TrimSpace(req.GetSecret()) != "" {
		in.Secret = req.Secret
	}
	c, err := s.uc.Update(ctx, id, in)
	if err != nil {
		return nil, err
	}
	return toProtoCredential(c), nil
}

func (s *CredentialService) DeleteCredential(ctx context.Context, req *v1.DeleteCredentialRequest) (*emptypb.Empty, error) {
	id, err := parseCredentialID(req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.uc.Delete(ctx, id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *CredentialService) ImportBitwarden(ctx context.Context, req *v1.ImportBitwardenRequest) (*v1.ImportBitwardenResponse, error) {
	res, err := s.uc.ImportBitwarden(ctx, req.GetVaultJson())
	if err != nil {
		return nil, err
	}
	return &v1.ImportBitwardenResponse{
		ImportedCount: int32(res.Imported),
		SkippedCount:  int32(res.Skipped),
	}, nil
}

func (s *CredentialService) ListExecutions(ctx context.Context, req *v1.ListExecutionsRequest) (*v1.ListExecutionsResponse, error) {
	var credID int64
	if raw := strings.TrimSpace(req.GetCredentialId()); raw != "" {
		id, err := parseCredentialID(raw)
		if err != nil {
			return nil, err
		}
		credID = id
	}
	items, total, err := s.uc.ListExecutions(ctx, biz.CredentialExecutionQuery{
		CredentialID: credID,
		Site:         strings.TrimSpace(req.GetSite()),
		Page:         int(req.GetPage()),
		PageSize:     int(req.GetPageSize()),
	})
	if err != nil {
		return nil, err
	}
	out := &v1.ListExecutionsResponse{Total: total}
	for _, e := range items {
		out.Items = append(out.Items, toProtoCredentialExecution(e))
	}
	return out, nil
}
