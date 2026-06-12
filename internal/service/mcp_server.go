package service

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "aranea-agents/api/kratos/mcp_server/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// MCPServerService implements kratos mcp_server.v1.
type MCPServerService struct {
	v1.UnimplementedMCPServerServiceServer

	uc  *biz.MCPServerUsecase
	mon *biz.MonitorUsecase
}

func NewMCPServerService(uc *biz.MCPServerUsecase, mon *biz.MonitorUsecase) *MCPServerService {
	return &MCPServerService{uc: uc, mon: mon}
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
		ConfigJson:   m.ConfigJSON,
		MetadataJson: m.MetadataJSON,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		DeletedAt:    m.DeletedAt,
	}
}

// patchFromProtoMCPWithDiff builds an MCPServerUpdate by comparing proto values against
// the current persisted server. For string fields, non-empty proto values are included.
// For bool/int fields, only values that differ from current are included — this resolves
// proto3 zero-value ambiguity where false/0 could mean "not set" or "explicitly set to zero".
func patchFromProtoMCPWithDiff(pb *v1.MCPServer, cur biz.MCPServer) biz.MCPServerUpdate {
	if pb == nil {
		return biz.MCPServerUpdate{}
	}
	patch := biz.MCPServerUpdate{
		Key:          strPtrIfNonEmpty(pb.GetKey()),
		Name:         strPtrIfNonEmpty(pb.GetName()),
		Description:  strPtrIfNonEmpty(pb.GetDescription()),
		Status:       strPtrIfNonEmpty(pb.GetStatus()),
		ConfigJSON:   strPtrIfNonEmpty(pb.GetConfigJson()),
		MetadataJSON: strPtrIfNonEmpty(pb.GetMetadataJson()),
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

func (s *MCPServerService) ListMCPServers(ctx context.Context, _ *emptypb.Empty) (*v1.ListMCPServersResponse, error) {
	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListMCPServersResponse{Items: make([]*v1.MCPServer, 0, len(items))}
	for i := range items {
		resp.Items = append(resp.Items, toProtoMCP(items[i]))
	}
	return resp, nil
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
	out, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	s.mon.RecordAdminAudit(ctx, "mcp_server.create", "mcp_server", out.ID, fmt.Sprintf("key=%s", out.Key))
	return toProtoMCP(out), nil
}

func (s *MCPServerService) GetMCPServer(ctx context.Context, req *v1.GetMCPServerRequest) (*v1.MCPServer, error) {
	m, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("MCP_SERVER", "mcp server not found")
		}
		return nil, err
	}
	return toProtoMCP(m), nil
}

func (s *MCPServerService) UpdateMCPServer(ctx context.Context, req *v1.UpdateMCPServerRequest) (*v1.MCPServer, error) {
	if req.GetMcpServer() == nil {
		return nil, apierror.BadRequest("MCP_SERVER", "mcp_server body is required")
	}
	// Fetch current server to resolve proto3 zero-value ambiguity for bool/int fields.
	// Proto3 cannot distinguish "field not set" from "set to zero value" (false/0),
	// so we compare against current: only include a field in the patch when the proto
	// value differs from the current persisted value.
	current, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("MCP_SERVER", "mcp server not found")
		}
		return nil, err
	}
	patch := patchFromProtoMCPWithDiff(req.GetMcpServer(), current)
	out, err := s.uc.Update(ctx, req.GetId(), patch)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("MCP_SERVER", "mcp server not found")
		}
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	s.mon.RecordAdminAudit(ctx, "mcp_server.update", "mcp_server", out.ID, fmt.Sprintf("key=%s", out.Key))
	return toProtoMCP(out), nil
}

func (s *MCPServerService) DeleteMCPServer(ctx context.Context, req *v1.DeleteMCPServerRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	s.mon.RecordAdminAudit(ctx, "mcp_server.delete", "mcp_server", req.GetId(), "")
	return &emptypb.Empty{}, nil
}

func (s *MCPServerService) ValidateMCPServer(ctx context.Context, req *v1.ValidateMCPServerRequest) (*v1.ValidateMCPServerResponse, error) {
	res := s.uc.ValidateConfig(ctx, req.GetEnabled(), req.GetConfigJson())
	detailsJSON := "{}"
	if len(res.Details) > 0 {
		b, err := json.Marshal(res.Details)
		if err != nil {
			return nil, err
		}
		detailsJSON = string(b)
	}
	return &v1.ValidateMCPServerResponse{
		Ok:          res.OK,
		Status:      res.Status,
		Message:     res.Message,
		DetailsJson: detailsJSON,
	}, nil
}

func (s *MCPServerService) TestMCPServer(ctx context.Context, req *v1.TestMCPServerRequest) (*v1.MCPServerTestResponse, error) {
	res, err := s.uc.TestMCPServer(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("MCP_SERVER", "mcp server not found")
		}
		return nil, err
	}
	detailsJSON := "{}"
	if len(res.Details) > 0 {
		b, err := json.Marshal(res.Details)
		if err != nil {
			return nil, err
		}
		detailsJSON = string(b)
	}
	return &v1.MCPServerTestResponse{
		Ok:          res.OK,
		Status:      res.Status,
		Message:     res.Message,
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

func (s *MCPServerService) ListMCPServerUserCredentials(ctx context.Context, req *v1.ListMCPServerUserCredentialsRequest) (*v1.ListMCPServerUserCredentialsResponse, error) {
	items, err := s.uc.ListUserCredentials(ctx, req.GetMcpServerId(), req.GetUserId())
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
	out, err := s.uc.UpsertUserCredential(ctx, req.GetMcpServerId(), req.GetUserId(), biz.MCPServerUserCredentialInput{
		CredentialKey: req.GetCredentialKey(),
		Secret:        req.GetSecret(),
		Status:        req.GetStatus(),
	})
	if err != nil {
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	return toProtoMCPUserCred(out), nil
}

func (s *MCPServerService) DeleteMCPServerUserCredential(ctx context.Context, req *v1.DeleteMCPServerUserCredentialRequest) (*emptypb.Empty, error) {
	if err := s.uc.DeleteUserCredential(ctx, req.GetMcpServerId(), req.GetUserId(), req.GetCredentialKey()); err != nil {
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	return &emptypb.Empty{}, nil
}
