package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	v1 "aranea-agents/api/kratos/mcp_server/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

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

func patchFromProtoMCP(pb *v1.MCPServer) biz.MCPServer {
	if pb == nil {
		return biz.MCPServer{}
	}
	return biz.MCPServer{
		Key:          pb.GetKey(),
		Name:         pb.GetName(),
		Description:  pb.GetDescription(),
		Status:       pb.GetStatus(),
		Enabled:      pb.GetEnabled(),
		SortOrder:    int(pb.GetSortOrder()),
		ConfigJSON:   pb.GetConfigJson(),
		MetadataJSON: pb.GetMetadataJson(),
	}
}

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
	biz.RecordAdminAudit(ctx, s.mon, "mcp_server.create", "mcp_server", out.ID, fmt.Sprintf("key=%s", out.Key))
	return toProtoMCP(out), nil
}

func (s *MCPServerService) GetMCPServer(ctx context.Context, req *v1.GetMCPServerRequest) (*v1.MCPServer, error) {
	m, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("MCP_SERVER", "mcp server not found")
		}
		return nil, err
	}
	return toProtoMCP(m), nil
}

func (s *MCPServerService) UpdateMCPServer(ctx context.Context, req *v1.UpdateMCPServerRequest) (*v1.MCPServer, error) {
	if req.GetMcpServer() == nil {
		return nil, kerrors.BadRequest("MCP_SERVER", "mcp_server body is required")
	}
	out, err := s.uc.Update(ctx, req.GetId(), patchFromProtoMCP(req.GetMcpServer()))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("MCP_SERVER", "mcp server not found")
		}
		return nil, err
	}
	biz.RecordAdminAudit(ctx, s.mon, "mcp_server.update", "mcp_server", out.ID, fmt.Sprintf("key=%s", out.Key))
	return toProtoMCP(out), nil
}

func (s *MCPServerService) DeleteMCPServer(ctx context.Context, req *v1.DeleteMCPServerRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	biz.RecordAdminAudit(ctx, s.mon, "mcp_server.delete", "mcp_server", req.GetId(), "")
	return &emptypb.Empty{}, nil
}

func (s *MCPServerService) TestMCPServer(ctx context.Context, req *v1.TestMCPServerRequest) (*v1.MCPServerTestResponse, error) {
	res, err := s.uc.TestMCPServer(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("MCP_SERVER", "mcp server not found")
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
