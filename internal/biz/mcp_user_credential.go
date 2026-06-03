package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// MCPAuthConfig is the domain-level representation of MCP server auth configuration.
type MCPAuthConfig struct {
	Type       string `json:"type"`
	HeaderName string `json:"header_name"`
}

// MCPServerConfig is the domain-level representation of MCP server config_json,
// containing only the fields needed for user credential resolution.
type MCPServerConfig struct {
	Headers                map[string]string `json:"headers"`
	Auth                   MCPAuthConfig     `json:"auth"`
	RequireUserCredentials bool              `json:"require_user_credentials"`
}

// MCPServerUserCredential is a per-user secret for an MCP server row.
type MCPServerUserCredential struct {
	ID            string
	MCPServerID   string
	UserID        string
	CredentialKey string
	Status        string
	SecretRef     string
	MetadataJSON  string
	CreatedAt     string
	UpdatedAt     string
	DeletedAt     string
	Configured    bool
	MaskedPreview string
}

type MCPServerUserCredentialInput struct {
	CredentialKey string
	Secret        string
	Status        string
}

type MCPServerUserCredentialRepo interface {
	ListMCPServerUserCredentials(ctx context.Context, mcpServerID, userID string) ([]MCPServerUserCredential, error)
	UpsertMCPServerUserCredential(ctx context.Context, cred MCPServerUserCredential) (MCPServerUserCredential, error)
	DeleteMCPServerUserCredential(ctx context.Context, mcpServerID, userID, credentialKey string) error
}

func (u *MCPServerUsecase) ListUserCredentials(ctx context.Context, mcpServerID, userID string) ([]MCPServerUserCredential, error) {
	repo, err := u.userCredRepo()
	if err != nil {
		return nil, err
	}
	return sanitizeMCPUserCredentials(repo.ListMCPServerUserCredentials(ctx, strings.TrimSpace(mcpServerID), strings.TrimSpace(userID)))
}

func (u *MCPServerUsecase) UpsertUserCredential(ctx context.Context, mcpServerID, userID string, in MCPServerUserCredentialInput) (MCPServerUserCredential, error) {
	repo, err := u.userCredRepo()
	if err != nil {
		return MCPServerUserCredential{}, err
	}
	mcpServerID = strings.TrimSpace(mcpServerID)
	userID = strings.TrimSpace(userID)
	key := strings.TrimSpace(in.CredentialKey)
	secret := strings.TrimSpace(in.Secret)
	if mcpServerID == "" || userID == "" || key == "" {
		return MCPServerUserCredential{}, errors.BadRequest("MCP_SERVER", "mcp_server_id, user_id and credential_key are required")
	}
	if secret == "" {
		return MCPServerUserCredential{}, errors.BadRequest("MCP_SERVER", "secret is required")
	}
	secretRef, err := u.crypto.EncryptChannelSecretRef(ctx, secret)
	if err != nil {
		return MCPServerUserCredential{}, err
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "active"
	}
	out, err := repo.UpsertMCPServerUserCredential(ctx, MCPServerUserCredential{
		ID:            uuid.NewString(),
		MCPServerID:   mcpServerID,
		UserID:        userID,
		CredentialKey: key,
		Status:        status,
		SecretRef:     secretRef,
		MetadataJSON:  "{}",
	})
	if err != nil {
		return MCPServerUserCredential{}, err
	}
	return sanitizeMCPUserCredential(out), nil
}

func (u *MCPServerUsecase) DeleteUserCredential(ctx context.Context, mcpServerID, userID, credentialKey string) error {
	repo, err := u.userCredRepo()
	if err != nil {
		return err
	}
	return repo.DeleteMCPServerUserCredential(ctx, strings.TrimSpace(mcpServerID), strings.TrimSpace(userID), strings.TrimSpace(credentialKey))
}

// ResolveUserAuthHeaders merges per-user credential into headers when require_user_credentials is set.
func (u *MCPServerUsecase) ResolveUserAuthHeaders(ctx context.Context, serverKey, userID string, sc MCPServerConfig) (map[string]string, error) {
	headers := make(map[string]string, len(sc.Headers)+1)
	for k, v := range sc.Headers {
		headers[k] = v
	}
	if !sc.RequireUserCredentials {
		return headers, nil
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return headers, errors.BadRequest("MCP_SERVER", "user credentials required but session user_id is empty")
	}
	repo, err := u.userCredRepo()
	if err != nil {
		return headers, err
	}
	serverKey = strings.TrimSpace(serverKey)
	row, err := u.findServerByKey(ctx, serverKey)
	if err != nil {
		return headers, err
	}
	creds, err := repo.ListMCPServerUserCredentials(ctx, row.ID, userID)
	if err != nil {
		return headers, err
	}
	keyName := strings.TrimSpace(sc.Auth.HeaderName)
	if keyName == "" {
		keyName = "authorization"
	}
	credKey := strings.ToLower(keyName)
	if credKey == "authorization" || credKey == "api_key" || credKey == "bearer" {
		// prefer explicit credential_key match, else first configured row
	}
	var picked *MCPServerUserCredential
	for i := range creds {
		if !creds[i].Configured {
			continue
		}
		if strings.EqualFold(creds[i].CredentialKey, keyName) || strings.EqualFold(creds[i].CredentialKey, "authorization") {
			picked = &creds[i]
			break
		}
		if picked == nil {
			picked = &creds[i]
		}
	}
	if picked == nil {
		return headers, errors.Forbidden("MCP_SERVER", "no user credential configured for this MCP server")
	}
	plain, err := u.crypto.DecryptChannelSecretRef(ctx, picked.SecretRef)
	if err != nil {
		return headers, err
	}
	headerName := strings.TrimSpace(picked.CredentialKey)
	if headerName == "" {
		headerName = "Authorization"
	}
	if strings.EqualFold(headerName, "Authorization") && !strings.HasPrefix(strings.ToLower(plain), "bearer ") {
		headers[headerName] = "Bearer " + plain
	} else {
		headers[headerName] = plain
	}
	return headers, nil
}

func (u *MCPServerUsecase) findServerByKey(ctx context.Context, serverKey string) (MCPServer, error) {
	row, err := u.repo.GetMCPServerByKey(ctx, serverKey)
	if err != nil {
		return MCPServer{}, err
	}
	return row, nil
}

func (u *MCPServerUsecase) userCredRepo() (MCPServerUserCredentialRepo, error) {
	if u == nil {
		return nil, errors.InternalServer("MCP_SERVER", "mcp usecase not configured")
	}
	repo, ok := u.repo.(MCPServerUserCredentialRepo)
	if !ok || repo == nil {
		return nil, errors.InternalServer("MCP_SERVER", "mcp user credential repo not configured")
	}
	return repo, nil
}

func sanitizeMCPUserCredentials(items []MCPServerUserCredential, err error) ([]MCPServerUserCredential, error) {
	if err != nil {
		return nil, err
	}
	out := make([]MCPServerUserCredential, len(items))
	for i := range items {
		out[i] = sanitizeMCPUserCredential(items[i])
	}
	return out, nil
}

func sanitizeMCPUserCredential(c MCPServerUserCredential) MCPServerUserCredential {
	c.Configured = strings.TrimSpace(c.SecretRef) != ""
	c.MaskedPreview = maskReference(c.SecretRef)
	c.SecretRef = ""
	return c
}
