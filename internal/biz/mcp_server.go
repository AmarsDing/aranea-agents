package biz

import (
	"aranea-agents/internal/mcp/probe"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

func defaultMetaJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
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

type MCPServerRepo interface {
	ListMCPServers(ctx context.Context) ([]MCPServer, error)
	GetMCPServer(ctx context.Context, id string) (MCPServer, error)
	CreateMCPServer(ctx context.Context, m MCPServer) (MCPServer, error)
	UpdateMCPServer(ctx context.Context, m MCPServer) (MCPServer, error)
	DeleteMCPServer(ctx context.Context, id string) error
}

type MCPServerUsecase struct {
	repo MCPServerRepo
}

func NewMCPServerUsecase(repo MCPServerRepo) *MCPServerUsecase {
	return &MCPServerUsecase{repo: repo}
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

// TestMCPServer runs probe and persists health_* metadata + row status (legacy TestMCPServer).
func (u *MCPServerUsecase) TestMCPServer(ctx context.Context, id string) (probe.TestResult, error) {
	row, err := u.repo.GetMCPServer(ctx, id)
	if err != nil {
		return probe.TestResult{}, err
	}
	result := probe.Evaluate(row.Enabled, row.ConfigJSON)
	if err := u.persistHealth(ctx, &row, result); err != nil {
		return result, err
	}
	return result, nil
}

func (u *MCPServerUsecase) PersistHealth(ctx context.Context, id string, result probe.TestResult) error {
	row, err := u.repo.GetMCPServer(ctx, id)
	if err != nil {
		return err
	}
	return u.persistHealth(ctx, &row, result)
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
	rows, err := u.repo.ListMCPServers(ctx)
	if err != nil {
		return err
	}
	var row *MCPServer
	for i := range rows {
		if strings.TrimSpace(rows[i].Key) == serverKey {
			row = &rows[i]
			break
		}
	}
	if row == nil {
		return nil
	}
	var metadata map[string]any
	if json.Unmarshal([]byte(defaultMetaJSON(row.MetadataJSON)), &metadata) != nil {
		metadata = map[string]any{}
	}
	metadata["last_reconnect_at"] = at.UTC().Format(time.RFC3339)
	switch v := metadata["reconnect_count"].(type) {
	case float64:
		metadata["reconnect_count"] = int(v) + 1
	case int:
		metadata["reconnect_count"] = v + 1
	default:
		metadata["reconnect_count"] = 1
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row.MetadataJSON = string(raw)
	_, err = u.repo.UpdateMCPServer(ctx, *row)
	return err
}

func (u *MCPServerUsecase) persistHealth(ctx context.Context, row *MCPServer, result probe.TestResult) error {
	var metadata map[string]any
	if json.Unmarshal([]byte(defaultMetaJSON(row.MetadataJSON)), &metadata) != nil {
		metadata = map[string]any{}
	}
	metadata["health_status"] = result.Status
	metadata["last_health_at"] = time.Now().UTC().Format(time.RFC3339)
	if result.OK {
		metadata["last_error_message"] = ""
		row.Status = "active"
	} else {
		metadata["last_error_message"] = result.Message
		row.Status = "error"
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row.MetadataJSON = string(raw)
	_, err = u.repo.UpdateMCPServer(ctx, *row)
	return err
}
