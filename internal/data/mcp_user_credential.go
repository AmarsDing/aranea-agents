package data

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformmcpusercredential"
)

func entToMCPUserCred(e *ent.PlatformMCPUserCredential) biz.MCPServerUserCredential {
	if e == nil {
		return biz.MCPServerUserCredential{}
	}
	return biz.MCPServerUserCredential{
		ID:            e.ID,
		MCPServerID:   e.McpServerID,
		UserID:        e.UserID,
		CredentialKey: e.CredentialKey,
		Status:        e.Status,
		SecretRef:     e.SecretRef,
		MetadataJSON:  e.MetadataJSON,
		CreatedAt:     e.CreatedAt,
		UpdatedAt:     e.UpdatedAt,
		DeletedAt:     e.DeletedAt,
	}
}

func (r *mcpServerRepo) ListMCPServerUserCredentials(ctx context.Context, mcpServerID, userID string) ([]biz.MCPServerUserCredential, error) {
	mcpServerID = strings.TrimSpace(mcpServerID)
	if mcpServerID == "" {
		return nil, errors.New("mcp_server_id is required")
	}
	q := r.data.RW().Read(ctx).PlatformMCPUserCredential.Query().
		Where(
			platformmcpusercredential.McpServerIDEQ(mcpServerID),
			platformmcpusercredential.DeletedAtEQ(""),
		)
	if uid := strings.TrimSpace(userID); uid != "" {
		q = q.Where(platformmcpusercredential.UserIDEQ(uid))
	}
	rows, err := q.Order(platformmcpusercredential.ByCredentialKey()).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.MCPServerUserCredential, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToMCPUserCred(e))
	}
	return out, nil
}

func (r *mcpServerRepo) UpsertMCPServerUserCredential(ctx context.Context, cred biz.MCPServerUserCredential) (biz.MCPServerUserCredential, error) {
	cred.MCPServerID = strings.TrimSpace(cred.MCPServerID)
	cred.UserID = strings.TrimSpace(cred.UserID)
	cred.CredentialKey = strings.TrimSpace(cred.CredentialKey)
	if cred.ID == "" || cred.MCPServerID == "" || cred.UserID == "" || cred.CredentialKey == "" {
		return biz.MCPServerUserCredential{}, errors.New("id, mcp_server_id, user_id and credential_key are required")
	}
	existing, err := r.data.RW().Read(ctx).PlatformMCPUserCredential.Query().
		Where(
			platformmcpusercredential.McpServerIDEQ(cred.MCPServerID),
			platformmcpusercredential.UserIDEQ(cred.UserID),
			platformmcpusercredential.CredentialKeyEQ(cred.CredentialKey),
		).
		Only(ctx)
	now := nowRFC3339()
	if cred.Status == "" {
		cred.Status = "active"
	}
	if cred.MetadataJSON == "" {
		cred.MetadataJSON = "{}"
	}
	if ent.IsNotFound(err) {
		if cred.CreatedAt == "" {
			cred.CreatedAt = now
		}
		cred.UpdatedAt = now
		e, err := r.data.RW().Write(ctx).PlatformMCPUserCredential.Create().
			SetID(cred.ID).
			SetMcpServerID(cred.MCPServerID).
			SetUserID(cred.UserID).
			SetCredentialKey(cred.CredentialKey).
			SetStatus(cred.Status).
			SetSecretRef(cred.SecretRef).
			SetMetadataJSON(cred.MetadataJSON).
			SetCreatedAt(cred.CreatedAt).
			SetUpdatedAt(cred.UpdatedAt).
			SetDeletedAt("").
			Save(ctx)
		if err != nil {
			return biz.MCPServerUserCredential{}, err
		}
		return entToMCPUserCred(e), nil
	}
	if err != nil {
		return biz.MCPServerUserCredential{}, err
	}
	e, err := r.data.RW().Write(ctx).PlatformMCPUserCredential.UpdateOneID(existing.ID).
		SetStatus(cred.Status).
		SetSecretRef(cred.SecretRef).
		SetMetadataJSON(cred.MetadataJSON).
		SetUpdatedAt(now).
		SetDeletedAt("").
		Save(ctx)
	if err != nil {
		return biz.MCPServerUserCredential{}, err
	}
	return entToMCPUserCred(e), nil
}

func (r *mcpServerRepo) DeleteMCPServerUserCredential(ctx context.Context, mcpServerID, userID, credentialKey string) error {
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).PlatformMCPUserCredential.Update().
		Where(
			platformmcpusercredential.McpServerIDEQ(strings.TrimSpace(mcpServerID)),
			platformmcpusercredential.UserIDEQ(strings.TrimSpace(userID)),
			platformmcpusercredential.CredentialKeyEQ(strings.TrimSpace(credentialKey)),
			platformmcpusercredential.DeletedAtEQ(""),
		).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

// Ensure mcpServerRepo implements user credential repo at compile time.
var _ biz.MCPServerUserCredentialRepo = (*mcpServerRepo)(nil)

// ent field names use McpServerID from schema - verify after ent generate
