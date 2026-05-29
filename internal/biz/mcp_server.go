package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
)

var mcpIDRand uint64

func newMCPServerID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&mcpIDRand, 1)
		return hex.EncodeToString([]byte{byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
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
	ApplyHealth(m map[string]any, healthStatus string, ok bool, errMsg string, at time.Time) string
	ApplyReconnect(m map[string]any, at time.Time)
	MarkHealthAlert(m map[string]any, at time.Time)
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
	prober   MCPProber
	metaEdit MCPMetadataEditor
	crypto   *CredentialCrypto
}

func NewMCPServerUsecase(repo MCPServerRepo, crypto *CredentialCrypto) *MCPServerUsecase {
	return &MCPServerUsecase{repo: repo, crypto: crypto}
}

// SetProber injects the MCP probe implementation after construction.
func (u *MCPServerUsecase) SetProber(prober MCPProber) {
	if u != nil {
		u.prober = prober
	}
}

// SetMetadataEditor injects the MCP metadata editor after construction.
func (u *MCPServerUsecase) SetMetadataEditor(editor MCPMetadataEditor) {
	if u != nil {
		u.metaEdit = editor
	}
}

func (u *MCPServerUsecase) List(ctx context.Context) ([]MCPServer, error) {
	return u.repo.ListMCPServers(ctx)
}

func (u *MCPServerUsecase) Get(ctx context.Context, id string) (MCPServer, error) {
	if strings.TrimSpace(id) == "" {
		return MCPServer{}, errors.BadRequest("MCP_SERVER", "id is required")
	}
	return u.repo.GetMCPServer(ctx, id)
}

func (u *MCPServerUsecase) Create(ctx context.Context, in MCPServer) (MCPServer, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	if in.Key == "" || in.Name == "" {
		return MCPServer{}, errors.BadRequest("MCP_SERVER", "key and name are required")
	}
	if in.ID == "" {
		in.ID = newMCPServerID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	return u.repo.CreateMCPServer(ctx, in)
}

func (u *MCPServerUsecase) Update(ctx context.Context, id string, patch MCPServer) (MCPServer, error) {
	if strings.TrimSpace(id) == "" {
		return MCPServer{}, errors.BadRequest("MCP_SERVER", "id is required")
	}
	cur, err := u.repo.GetMCPServer(ctx, id)
	if err != nil {
		return MCPServer{}, err
	}
	merged := cur
	if patch.Key != "" {
		merged.Key = patch.Key
	}
	if patch.Name != "" {
		merged.Name = patch.Name
	}
	if patch.Status != "" {
		merged.Status = patch.Status
	}
	merged.Description = patch.Description
	merged.Enabled = patch.Enabled
	merged.SortOrder = patch.SortOrder
	merged.ConfigJSON = patch.ConfigJSON
	merged.MetadataJSON = patch.MetadataJSON
	if merged.Key == "" {
		merged.Key = cur.Key
	}
	if merged.Name == "" {
		merged.Name = cur.Name
	}
	if merged.Status == "" {
		merged.Status = cur.Status
	}
	return u.repo.UpdateMCPServer(ctx, merged)
}

func (u *MCPServerUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("MCP_SERVER", "id is required")
	}
	return u.repo.DeleteMCPServer(ctx, id)
}

// TestMCPServer runs probe and persists health_* metadata + row status.
func (u *MCPServerUsecase) TestMCPServer(ctx context.Context, id string) (MCPTestResult, error) {
	row, err := u.repo.GetMCPServer(ctx, id)
	if err != nil {
		return MCPTestResult{}, err
	}
	if u.prober == nil {
		return MCPTestResult{}, errors.InternalServer("MCP_SERVER", "mcp prober not configured")
	}
	result := u.prober.Evaluate(ctx, row.Enabled, row.ConfigJSON)
	if err := u.persistHealth(ctx, &row, result); err != nil {
		return result, err
	}
	return result, nil
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
		return errors.InternalServer("MCP_SERVER", "mcp metadata editor not configured")
	}
	meta := u.metaEdit.Parse(row.MetadataJSON)
	u.metaEdit.ApplyReconnect(meta, at)
	raw, err := u.metaEdit.Marshal(meta)
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
		return errors.InternalServer("MCP_SERVER", "mcp metadata editor not configured")
	}
	meta := u.metaEdit.Parse(row.MetadataJSON)
	u.metaEdit.MarkHealthAlert(meta, at)
	raw, err := u.metaEdit.Marshal(meta)
	if err != nil {
		return err
	}
	return u.repo.UpdateMCPServerMetadata(ctx, row.ID, raw, "")
}

// ValidateConfig runs probe without persisting (pre-create URL check).
func (u *MCPServerUsecase) ValidateConfig(ctx context.Context, enabled bool, configJSON string) MCPTestResult {
	if u.prober == nil {
		return MCPTestResult{OK: false, Status: "unknown", Message: "mcp prober not configured"}
	}
	return u.prober.Evaluate(ctx, enabled, configJSON)
}

func (u *MCPServerUsecase) persistHealth(ctx context.Context, row *MCPServer, result MCPTestResult) error {
	if u.metaEdit == nil {
		return errors.InternalServer("MCP_SERVER", "mcp metadata editor not configured")
	}
	meta := u.metaEdit.Parse(row.MetadataJSON)
	at := time.Now().UTC()
	status := u.metaEdit.ApplyHealth(meta, result.Status, result.OK, result.Message, at)
	raw, err := u.metaEdit.Marshal(meta)
	if err != nil {
		return err
	}
	return u.repo.UpdateMCPServerMetadata(ctx, row.ID, raw, status)
}
