package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformmcpserver"
	"aranea-agents/internal/data/ent/platformmcpusercredential"
	"aranea-agents/internal/data/ent/predicate"
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
		WorkspaceID:  e.WorkspaceID, // P2-B: tenant isolation
	}
}

func (r *mcpServerRepo) mcpListQuery(ctx context.Context, q biz.MCPListQuery) *ent.PlatformMCPServerQuery {
	pq := r.data.RW().Read(ctx).PlatformMCPServer.Query().
		Where(platformmcpserver.DeletedAtEQ(""))
	// P2-B: workspace visibility filter.
	// empty WorkspaceID = system caller (see all); non-empty = tenant caller (shared + own).
	if ws := strings.TrimSpace(q.WorkspaceID); ws != "" {
		pq = pq.Where(platformmcpserver.Or(
			platformmcpserver.WorkspaceIDEQ(""),
			platformmcpserver.WorkspaceIDEQ(ws),
		))
	}
	if s := strings.TrimSpace(q.Search); s != "" {
		pq = pq.Where(platformmcpserver.Or(
			platformmcpserver.ServerKeyContainsFold(s),
			platformmcpserver.NameContainsFold(s),
			platformmcpserver.DescriptionContainsFold(s),
		))
	}
	return pq
}

func (r *mcpServerRepo) ListMCPServers(ctx context.Context, q biz.MCPListQuery) ([]biz.MCPServer, error) {
	rows, err := r.mcpListQuery(ctx, q).Order(
		platformmcpserver.BySortOrder(),
		platformmcpserver.ByCreatedAt(entsql.OrderDesc()),
	).All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainMCP)
	}
	out := make([]biz.MCPServer, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizMCP(e))
	}
	return out, nil
}

func (r *mcpServerRepo) ListMCPServersPaged(ctx context.Context, q biz.MCPListQuery) (biz.MCPListResult, error) {
	total, err := r.mcpListQuery(ctx, q).Count(ctx)
	if err != nil {
		return biz.MCPListResult{}, entErrToBizErr(err, apierror.DomainMCP)
	}
	rows, err := r.mcpListQuery(ctx, q).
		Order(
			platformmcpserver.BySortOrder(),
			platformmcpserver.ByCreatedAt(entsql.OrderDesc()),
		).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.MCPListResult{}, entErrToBizErr(err, apierror.DomainMCP)
	}
	out := make([]biz.MCPServer, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizMCP(e))
	}
	return biz.MCPListResult{Items: out, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

// mcpWorkspacePredicate returns shared-or-own visibility for mcp_server (C-25).
func mcpWorkspacePredicate(ctx context.Context) predicate.PlatformMCPServer {
	ids := workspaceSharedOrOwnIDs(ctx)
	if ids == nil {
		return nil
	}
	return platformmcpserver.WorkspaceIDIn(ids...)
}

func (r *mcpServerRepo) GetMCPServer(ctx context.Context, id string) (biz.MCPServer, error) {
	preds := []predicate.PlatformMCPServer{
		platformmcpserver.IDEQ(id),
		platformmcpserver.DeletedAtEQ(""),
	}
	if wsPred := mcpWorkspacePredicate(ctx); wsPred != nil {
		preds = append(preds, wsPred)
	}
	row, err := r.data.RW().Read(ctx).PlatformMCPServer.Query().
		Where(preds...).
		Only(ctx)
	if err != nil {
		return biz.MCPServer{}, entErrToBizErr(err, apierror.DomainMCP)
	}
	return entToBizMCP(row), nil
}

func (r *mcpServerRepo) GetMCPServerByKey(ctx context.Context, key string) (biz.MCPServer, error) {
	preds := []predicate.PlatformMCPServer{
		platformmcpserver.ServerKeyEQ(key),
		platformmcpserver.DeletedAtEQ(""),
	}
	if wsPred := mcpWorkspacePredicate(ctx); wsPred != nil {
		preds = append(preds, wsPred)
	}
	row, err := r.data.RW().Read(ctx).PlatformMCPServer.Query().
		Where(preds...).
		Only(ctx)
	if err != nil {
		return biz.MCPServer{}, entErrToBizErr(err, apierror.DomainMCP)
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
	return entErrToBizErr(update.Exec(ctx), apierror.DomainMCP)
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
		SetWorkspaceID(m.WorkspaceID). // P2-B: tenant isolation
		Save(ctx)
	if err != nil {
		return biz.MCPServer{}, entErrToBizErr(err, apierror.DomainMCP)
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
		return biz.MCPServer{}, entErrToBizErr(err, apierror.DomainMCP)
	}
	return entToBizMCP(saved), nil
}

// DeleteMCPServer soft-deletes the server and cascades to its user credentials
// in a single transaction. If either write fails the whole operation rolls
// back, preventing the inconsistent state where credentials are deleted but
// the server remains (or vice versa).
func (r *mcpServerRepo) DeleteMCPServer(ctx context.Context, id string) error {
	now := nowRFC3339()
	return entErrToBizErr(r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		// Cascade: soft-delete all user credentials belonging to this server.
		if _, err := r.data.RW().Write(txCtx).PlatformMCPUserCredential.Update().
			Where(
				platformmcpusercredential.McpServerIDEQ(id),
				platformmcpusercredential.DeletedAtEQ(""),
			).
			SetDeletedAt(now).
			SetUpdatedAt(now).
			Save(txCtx); err != nil {
			return err
		}
		// Soft-delete the server itself.
		return r.data.RW().Write(txCtx).PlatformMCPServer.UpdateOneID(id).
			SetDeletedAt(now).
			SetStatus("deleted").
			SetUpdatedAt(now).
			Exec(txCtx)
	}), apierror.DomainMCP)
}
