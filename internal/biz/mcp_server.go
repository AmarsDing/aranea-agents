package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/outboundguard"
)

var mcpIDRand uint64

func newMCPServerID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		// Fallback: mix counter + timestamp for unpredictability.
		n := atomic.AddUint64(&mcpIDRand, 1)
		ts := time.Now().UnixNano()
		fallback := make([]byte, 12)
		for i := 0; i < 8; i++ {
			fallback[i] = byte(n >> uint(i*8))
		}
		for i := 0; i < 4; i++ {
			fallback[8+i] = byte(ts >> uint(i*8))
		}
		return hex.EncodeToString(fallback)
	}
	return hex.EncodeToString(buf)
}

// MCPTestResult is the domain-level result of an MCP server connectivity test.
type MCPTestResult struct {
	OK      bool           `json:"ok"`
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// MCPProber abstracts MCP server connectivity testing so biz does not
// depend on internal/mcp/probe directly.
type MCPProber interface {
	Evaluate(ctx context.Context, enabled bool, configJSON string) MCPTestResult
}

// MCPMetadataEditor abstracts metadata_json manipulation so biz does not
// depend on internal/mcp/metadata directly.
type MCPMetadataEditor interface {
	Parse(raw string) map[string]any
	Marshal(m map[string]any) (string, error)
	ApplyHealth(m map[string]any, healthStatus string, ok bool, errMsg string, at time.Time) (map[string]any, string)
	ApplyReconnect(m map[string]any, at time.Time) map[string]any
	MarkHealthAlert(m map[string]any, at time.Time) map[string]any
}

// MCPServer matches legacy PlatformResource for mcp-servers.
type MCPServer struct {
	ID           string
	Key          string
	Name         string
	Description  string
	Status       string
	Enabled      bool
	SortOrder    int
	ConfigJSON   string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
}

type MCPServerReader interface {
	ListMCPServers(ctx context.Context) ([]MCPServer, error)
	GetMCPServer(ctx context.Context, id string) (MCPServer, error)
	GetMCPServerByKey(ctx context.Context, key string) (MCPServer, error)
}

type MCPServerWriter interface {
	CreateMCPServer(ctx context.Context, m MCPServer) (MCPServer, error)
	UpdateMCPServer(ctx context.Context, m MCPServer) (MCPServer, error)
	DeleteMCPServer(ctx context.Context, id string) error
}

type MCPServerMetadataWriter interface {
	UpdateMCPServerMetadata(ctx context.Context, id string, metadataJSON string, status string) error
}

type MCPServerRepo interface {
	MCPServerReader
	MCPServerWriter
	MCPServerMetadataWriter
}

type MCPServerUsecase struct {
	repo     MCPServerRepo
	credRepo MCPServerUserCredentialRepo
	prober   MCPProber
	metaEdit MCPMetadataEditor
	crypto   *CredentialCrypto
}

func NewMCPServerUsecase(repo MCPServerRepo, credRepo MCPServerUserCredentialRepo, prober MCPProber, metaEdit MCPMetadataEditor, crypto *CredentialCrypto) *MCPServerUsecase {
	return &MCPServerUsecase{repo: repo, credRepo: credRepo, prober: prober, metaEdit: metaEdit, crypto: crypto}
}

func (u *MCPServerUsecase) List(ctx context.Context) ([]MCPServer, error) {
	return u.repo.ListMCPServers(ctx)
}

func (u *MCPServerUsecase) Get(ctx context.Context, id string) (MCPServer, error) {
	if strings.TrimSpace(id) == "" {
		return MCPServer{}, apierror.BadRequest("MCP_SERVER", "id is required")
	}
	return u.repo.GetMCPServer(ctx, id)
}

func (u *MCPServerUsecase) Create(ctx context.Context, in MCPServer) (MCPServer, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	if in.Key == "" || in.Name == "" {
		return MCPServer{}, apierror.BadRequest("MCP_SERVER", "key and name are required")
	}
	if err := validateMCPConfigURLs(in.ConfigJSON); err != nil {
		return MCPServer{}, err
	}
	if in.ID == "" {
		in.ID = newMCPServerID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	return u.repo.CreateMCPServer(ctx, in)
}

// MCPServerUpdate is a partial-update DTO where nil fields mean "do not change".
// This solves proto3 zero-value ambiguity: bool false and int 0 cannot be
// distinguished from "field not set" in proto3, so pointer nil is used instead.
type MCPServerUpdate struct {
	Key          *string
	Name         *string
	Description  *string
	Status       *string
	Enabled      *bool
	SortOrder    *int
	ConfigJSON   *string
	MetadataJSON *string
}

func (u *MCPServerUsecase) Update(ctx context.Context, id string, patch MCPServerUpdate) (MCPServer, error) {
	if strings.TrimSpace(id) == "" {
		return MCPServer{}, apierror.BadRequest("MCP_SERVER", "id is required")
	}
	cur, err := u.repo.GetMCPServer(ctx, id)
	if err != nil {
		return MCPServer{}, err
	}
	merged := cur
	if patch.Key != nil {
		merged.Key = *patch.Key
	}
	if patch.Name != nil {
		merged.Name = *patch.Name
	}
	if patch.Description != nil {
		merged.Description = *patch.Description
	}
	if patch.Status != nil {
		merged.Status = *patch.Status
	}
	if patch.Enabled != nil {
		merged.Enabled = *patch.Enabled
	}
	if patch.SortOrder != nil {
		merged.SortOrder = *patch.SortOrder
	}
	if patch.ConfigJSON != nil {
		merged.ConfigJSON = *patch.ConfigJSON
	}
	if patch.MetadataJSON != nil {
		merged.MetadataJSON = *patch.MetadataJSON
	}
	if strings.TrimSpace(merged.Key) == "" {
		return MCPServer{}, apierror.BadRequest("MCP_SERVER", "key cannot be empty")
	}
	if strings.TrimSpace(merged.Name) == "" {
		return MCPServer{}, apierror.BadRequest("MCP_SERVER", "name cannot be empty")
	}
	if err := validateMCPConfigURLs(merged.ConfigJSON); err != nil {
		return MCPServer{}, err
	}
	return u.repo.UpdateMCPServer(ctx, merged)
}

func (u *MCPServerUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("MCP_SERVER", "id is required")
	}
	return u.repo.DeleteMCPServer(ctx, id)
}

// TestMCPServerResult combines the probe result with the updated server state.
type TestMCPServerResult struct {
	Result MCPTestResult
	Server MCPServer
}

// TestMCPServer runs probe and persists health_* metadata + row status.
func (u *MCPServerUsecase) TestMCPServer(ctx context.Context, id string) (TestMCPServerResult, error) {
	row, err := u.repo.GetMCPServer(ctx, id)
	if err != nil {
		return TestMCPServerResult{}, err
	}
	if u.prober == nil {
		return TestMCPServerResult{}, apierror.Internal("MCP_SERVER", "mcp prober not configured")
	}
	result := u.prober.Evaluate(ctx, row.Enabled, row.ConfigJSON)
	updated, persistErr := u.persistHealth(ctx, &row, result)
	// Return both result and updated server; caller can use updated metadata
	// without an extra DB read.
	return TestMCPServerResult{Result: result, Server: updated}, persistErr
}

// RecordReconnectMetadata updates last_reconnect_at and reconnect_count for the server key.
func (u *MCPServerUsecase) RecordReconnectMetadata(ctx context.Context, serverKey string, at time.Time) error {
	if u == nil || u.repo == nil {
		return nil
	}
	serverKey = strings.TrimSpace(serverKey)
	if serverKey == "" {
		return nil
	}
	row, err := u.repo.GetMCPServerByKey(ctx, serverKey)
	if err != nil {
		return err
	}
	if u.metaEdit == nil {
		return apierror.Internal("MCP_SERVER", "mcp metadata editor not configured")
	}
	meta := u.metaEdit.Parse(row.MetadataJSON)
	updatedMeta := u.metaEdit.ApplyReconnect(meta, at)
	raw, err := u.metaEdit.Marshal(updatedMeta)
	if err != nil {
		return err
	}
	return u.repo.UpdateMCPServerMetadata(ctx, row.ID, raw, "")
}

// MarkHealthAlertEmitted records last_health_alert_at in metadata_json to debounce monitor events.
func (u *MCPServerUsecase) MarkHealthAlertEmitted(ctx context.Context, id string, at time.Time) error {
	row, err := u.repo.GetMCPServer(ctx, id)
	if err != nil {
		return err
	}
	if u.metaEdit == nil {
		return apierror.Internal("MCP_SERVER", "mcp metadata editor not configured")
	}
	meta := u.metaEdit.Parse(row.MetadataJSON)
	updatedMeta := u.metaEdit.MarkHealthAlert(meta, at)
	raw, err := u.metaEdit.Marshal(updatedMeta)
	if err != nil {
		return err
	}
	return u.repo.UpdateMCPServerMetadata(ctx, row.ID, raw, "")
}

// ValidateConfig runs probe without persisting (pre-create URL check).
// Always passes enabled=true so the probe actually validates the config,
// regardless of the server's enabled state.
func (u *MCPServerUsecase) ValidateConfig(ctx context.Context, _ bool, configJSON string) MCPTestResult {
	if u.prober == nil {
		return MCPTestResult{OK: false, Status: "unknown", Message: "mcp prober not configured"}
	}
	return u.prober.Evaluate(ctx, true, configJSON)
}

func (u *MCPServerUsecase) persistHealth(ctx context.Context, row *MCPServer, result MCPTestResult) (MCPServer, error) {
	if u.metaEdit == nil {
		return MCPServer{}, apierror.Internal("MCP_SERVER", "mcp metadata editor not configured")
	}
	meta := u.metaEdit.Parse(row.MetadataJSON)
	at := time.Now().UTC()
	meta, status := u.metaEdit.ApplyHealth(meta, result.Status, result.OK, result.Message, at)
	raw, err := u.metaEdit.Marshal(meta)
	if err != nil {
		return MCPServer{}, err
	}
	if err := u.repo.UpdateMCPServerMetadata(ctx, row.ID, raw, status); err != nil {
		return MCPServer{}, err
	}
	// Return the in-memory updated server to avoid an extra DB read.
	row.MetadataJSON = raw
	row.Status = status
	return *row, nil
}

// validateMCPConfigURLs parses configJSON and validates that any HTTP
// transport URL passes SSRF checks. This prevents saving MCP server
// configurations that target internal/private networks.
func validateMCPConfigURLs(configJSON string) error {
	raw := strings.TrimSpace(configJSON)
	if raw == "" || raw == "{}" {
		return nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return apierror.BadRequest("MCP_SERVER", "config_json must be a valid JSON object")
	}
	transport, _ := cfg["transport"].(string)
	transport = strings.ToLower(strings.TrimSpace(transport))
	switch transport {
	case "stdio":
		cmd, _ := cfg["command"].(string)
		if strings.TrimSpace(cmd) == "" {
			return apierror.BadRequest("MCP_SERVER", "mcp stdio transport requires command")
		}
	case "sse", "streamable", "streamable_http":
		url, _ := cfg["url"].(string)
		if strings.TrimSpace(url) == "" {
			return apierror.BadRequest("MCP_SERVER", "mcp "+transport+" transport requires url")
		}
		if err := outboundguard.ValidateURL(url); err != nil {
			return apierror.BadRequest("MCP_SERVER", "mcp url failed SSRF check: "+err.Error())
		}
	}
	return nil
}
