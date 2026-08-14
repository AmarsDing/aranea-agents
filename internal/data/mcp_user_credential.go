package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformmcpusercredential"
	"aranea-agents/pkg/apierror"
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
	q := r.data.RW().Read(ctx).PlatformMCPUserCredential.Query().
		Where(
			platformmcpusercredential.McpServerIDEQ(strings.TrimSpace(mcpServerID)),
			platformmcpusercredential.DeletedAtEQ(""),
		)
	if uid := strings.TrimSpace(userID); uid != "" {
		q = q.Where(platformmcpusercredential.UserIDEQ(uid))
	}
	rows, err := q.Order(platformmcpusercredential.ByCredentialKey()).All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainMCP)
	}
	out := make([]biz.MCPServerUserCredential, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToMCPUserCred(e))
	}
	return out, nil
}

// UpsertMCPServerUserCredential atomically finds-or-creates a user credential
// inside a transaction. The read-then-write sequence is wrapped in ExecInTx so
// that two concurrent upserts for the same (server, user, key) tuple cannot
// both observe "not found" and race into a unique-constraint violation.
func (r *mcpServerRepo) UpsertMCPServerUserCredential(ctx context.Context, cred biz.MCPServerUserCredential) (biz.MCPServerUserCredential, error) {
	var out biz.MCPServerUserCredential
	txErr := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		existing, err := r.data.RW().Read(txCtx).PlatformMCPUserCredential.Query().
			Where(
				platformmcpusercredential.McpServerIDEQ(cred.MCPServerID),
				platformmcpusercredential.UserIDEQ(cred.UserID),
				platformmcpusercredential.CredentialKeyEQ(cred.CredentialKey),
				platformmcpusercredential.DeletedAtEQ(""),
			).
			Only(txCtx)
		now := nowRFC3339()
		status := cred.Status
		if status == "" {
			status = "active"
		}
		metadataJSON := cred.MetadataJSON
		if metadataJSON == "" {
			metadataJSON = "{}"
		}
		if ent.IsNotFound(err) {
			createdAt := cred.CreatedAt
			if createdAt == "" {
				createdAt = now
			}
			e, cerr := r.data.RW().Write(txCtx).PlatformMCPUserCredential.Create().
				SetID(cred.ID).
				SetMcpServerID(cred.MCPServerID).
				SetUserID(cred.UserID).
				SetCredentialKey(cred.CredentialKey).
				SetStatus(status).
				SetSecretRef(cred.SecretRef).
				SetMetadataJSON(metadataJSON).
				SetCreatedAt(createdAt).
				SetUpdatedAt(now).
				SetDeletedAt("").
				Save(txCtx)
			if cerr != nil {
				return cerr
			}
			out = entToMCPUserCred(e)
			return nil
		}
		if err != nil {
			return err
		}
		e, uerr := r.data.RW().Write(txCtx).PlatformMCPUserCredential.UpdateOneID(existing.ID).
			SetStatus(status).
			SetSecretRef(cred.SecretRef).
			SetMetadataJSON(metadataJSON).
			SetUpdatedAt(now).
			SetDeletedAt("").
			Save(txCtx)
		if uerr != nil {
			return uerr
		}
		out = entToMCPUserCred(e)
		return nil
	})
	if txErr != nil {
		return biz.MCPServerUserCredential{}, entErrToBizErr(txErr, apierror.DomainMCP)
	}
	return out, nil
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
	return entErrToBizErr(err, apierror.DomainMCP)
}

// Ensure mcpServerRepo implements user credential repo at compile time.
var _ biz.MCPServerUserCredentialRepo = (*mcpServerRepo)(nil)

// NewMCPServerUserCredentialRepo returns the same repo that implements
// both MCPServerRepo and MCPServerUserCredentialRepo.
func NewMCPServerUserCredentialRepo(d *Data) biz.MCPServerUserCredentialRepo {
	return &mcpServerRepo{data: d}
}

// ent field names use McpServerID from schema - verify after ent generate
