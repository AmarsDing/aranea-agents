package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformmcpserver"
	"aranea-agents/internal/data/ent/platformmcpusercredential"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
)

type mcpServerRepo struct {
	data *Data
}

var _ biz.MCPServerRepo = (*mcpServerRepo)(nil)

func NewMCPServerRepo(d *Data) biz.MCPServerRepo {
	return &mcpServerRepo{data: d}
}

func entToBizMCP(e *ent.PlatformMCPServer) biz.MCPServer {
	if e == nil {
		return biz.MCPServer{}
	}
	return biz.MCPServer{
		ID:           e.ID,
		Key:          e.ServerKey,
		Name:         e.Name,
		Description:  e.Description,
		Status:       e.Status,
		Enabled:      e.Enabled,
		SortOrder:    e.SortOrder,
		ConfigJSON:   e.ConfigJSON,
		MetadataJSON: e.MetadataJSON,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
		DeletedAt:    e.DeletedAt,
	}
}

func (r *mcpServerRepo) ListMCPServers(ctx context.Context) ([]biz.MCPServer, error) {
	rows, err := r.data.RW().Read(ctx).PlatformMCPServer.Query().
		Where(platformmcpserver.DeletedAtEQ("")).
		Order(
			platformmcpserver.BySortOrder(),
			platformmcpserver.ByCreatedAt(entsql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.MCPServer, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizMCP(e))
	}
	return out, nil
}

func (r *mcpServerRepo) GetMCPServer(ctx context.Context, id string) (biz.MCPServer, error) {
	row, err := r.data.RW().Read(ctx).PlatformMCPServer.Query().
		Where(platformmcpserver.IDEQ(id), platformmcpserver.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.MCPServer{}, apierror.NotFound(apierror.DomainMCP, "not found")
		}
		return biz.MCPServer{}, err
	}
	return entToBizMCP(row), nil
}

func (r *mcpServerRepo) GetMCPServerByKey(ctx context.Context, key string) (biz.MCPServer, error) {
	row, err := r.data.RW().Read(ctx).PlatformMCPServer.Query().
		Where(platformmcpserver.ServerKeyEQ(key), platformmcpserver.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.MCPServer{}, apierror.NotFound(apierror.DomainMCP, "not found")
		}
		return biz.MCPServer{}, err
	}
	return entToBizMCP(row), nil
}

func (r *mcpServerRepo) UpdateMCPServerMetadata(ctx context.Context, id string, metadataJSON string, status string) error {
	update := r.data.RW().Write(ctx).PlatformMCPServer.UpdateOneID(id).
		SetMetadataJSON(metadataJSON).
		SetUpdatedAt(nowRFC3339())
	if status != "" {
		update = update.SetStatus(status)
	}
	return update.Exec(ctx)
}

func (r *mcpServerRepo) CreateMCPServer(ctx context.Context, m biz.MCPServer) (biz.MCPServer, error) {
	now := nowRFC3339()
	if m.CreatedAt == "" {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	saved, err := r.data.RW().Write(ctx).PlatformMCPServer.Create().
		SetID(m.ID).
		SetServerKey(m.Key).
		SetName(m.Name).
		SetDescription(m.Description).
		SetStatus(m.Status).
		SetEnabled(m.Enabled).
		SetSortOrder(m.SortOrder).
		SetConfigJSON(m.ConfigJSON).
		SetMetadataJSON(m.MetadataJSON).
		SetCreatedAt(m.CreatedAt).
		SetUpdatedAt(m.UpdatedAt).
		SetDeletedAt("").
		Save(ctx)
	if err != nil {
		return biz.MCPServer{}, err
	}
	return entToBizMCP(saved), nil
}

func (r *mcpServerRepo) UpdateMCPServer(ctx context.Context, m biz.MCPServer) (biz.MCPServer, error) {
	m.UpdatedAt = nowRFC3339()
	saved, err := r.data.RW().Write(ctx).PlatformMCPServer.UpdateOneID(m.ID).
		SetServerKey(m.Key).
		SetName(m.Name).
		SetDescription(m.Description).
		SetStatus(m.Status).
		SetEnabled(m.Enabled).
		SetSortOrder(m.SortOrder).
		SetConfigJSON(m.ConfigJSON).
		SetMetadataJSON(m.MetadataJSON).
		SetUpdatedAt(m.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.MCPServer{}, err
	}
	return entToBizMCP(saved), nil
}

func (r *mcpServerRepo) DeleteMCPServer(ctx context.Context, id string) error {
	now := nowRFC3339()
	// Cascade: soft-delete all user credentials belonging to this server.
	_, credErr := r.data.RW().Write(ctx).PlatformMCPUserCredential.Update().
		Where(
			platformmcpusercredential.McpServerIDEQ(id),
			platformmcpusercredential.DeletedAtEQ(""),
		).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if credErr != nil {
		return credErr
	}
	// Soft-delete the server itself.
	return r.data.RW().Write(ctx).PlatformMCPServer.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Exec(ctx)
}
