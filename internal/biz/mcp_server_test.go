package biz

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	mcpmetadata "aranea-agents/internal/mcp/metadata"
	"aranea-agents/pkg/loggateway"
)

type stubMCPRepo struct {
	rows []MCPServer
}

func (s *stubMCPRepo) ListMCPServers(_ context.Context) ([]MCPServer, error) {
	return s.rows, nil
}

func (s *stubMCPRepo) GetMCPServer(_ context.Context, id string) (MCPServer, error) {
	for _, r := range s.rows {
		if r.ID == id {
			return r, nil
		}
	}
	return MCPServer{}, nil
}

func (s *stubMCPRepo) CreateMCPServer(_ context.Context, m MCPServer) (MCPServer, error) {
	return m, nil
}

func (s *stubMCPRepo) UpdateMCPServer(_ context.Context, m MCPServer) (MCPServer, error) {
	for i := range s.rows {
		if s.rows[i].ID == m.ID {
			s.rows[i] = m
			return m, nil
		}
	}
	return m, nil
}

func (s *stubMCPRepo) DeleteMCPServer(_ context.Context, _ string) error { return nil }

func (s *stubMCPRepo) GetMCPServerByKey(_ context.Context, key string) (MCPServer, error) {
	for _, r := range s.rows {
		if r.Key == key {
			return r, nil
		}
	}
	return MCPServer{}, nil
}

func (s *stubMCPRepo) UpdateMCPServerMetadata(_ context.Context, id string, metadataJSON string, status string) error {
	for i := range s.rows {
		if s.rows[i].ID == id {
			s.rows[i].MetadataJSON = metadataJSON
			s.rows[i].Status = status
			return nil
		}
	}
	return nil
}

// stubMCPCredRepo is a stub for MCPServerUserCredentialRepo.
type stubMCPCredRepo struct{}

func (stubMCPCredRepo) ListMCPServerUserCredentials(_ context.Context, _, _ string) ([]MCPServerUserCredential, error) {
	return nil, nil
}
func (stubMCPCredRepo) UpsertMCPServerUserCredential(_ context.Context, c MCPServerUserCredential) (MCPServerUserCredential, error) {
	return c, nil
}
func (stubMCPCredRepo) DeleteMCPServerUserCredential(_ context.Context, _, _, _ string) error {
	return nil
}

func TestRecordReconnectMetadata_PersistsCountAndTimestamp(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID:           "m1",
		Key:          "my-server",
		MetadataJSON: `{"reconnect_count":2}`,
	}}}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, NewCredentialCrypto(nil, loggateway.NewNoop()))
	at := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	if err := uc.RecordReconnectMetadata(context.Background(), "my-server", at); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(repo.rows[0].MetadataJSON), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["last_reconnect_at"] != at.Format(time.RFC3339) {
		t.Fatalf("last_reconnect_at = %v", meta["last_reconnect_at"])
	}
	if meta["reconnect_count"] != float64(3) {
		t.Fatalf("reconnect_count = %v", meta["reconnect_count"])
	}
}

// mcpMetadataAdapter delegates to the real metadata package, eliminating
// test code duplication while satisfying the MCPMetadataEditor interface.
type mcpMetadataAdapter struct{}

func (mcpMetadataAdapter) Parse(raw string) map[string]any          { return mcpmetadata.Parse(raw) }
func (mcpMetadataAdapter) Marshal(m map[string]any) (string, error) { return mcpmetadata.Marshal(m) }
func (mcpMetadataAdapter) ApplyHealth(m map[string]any, healthStatus string, ok bool, errMsg string, at time.Time) (map[string]any, string) {
	return mcpmetadata.ApplyHealth(m, healthStatus, ok, errMsg, at)
}
func (mcpMetadataAdapter) ApplyReconnect(m map[string]any, at time.Time) map[string]any {
	return mcpmetadata.ApplyReconnect(m, at)
}
func (mcpMetadataAdapter) MarkHealthAlert(m map[string]any, at time.Time) map[string]any {
	return mcpmetadata.MarkHealthAlert(m, at)
}

func TestValidateMCPConfigURLs(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{"empty allowed", "", false},
		{"empty object allowed", "{}", false},
		{"stdio allowed", `{"transport":"stdio","command":"npx"}`, false},
		{"stdio no command", `{"transport":"stdio","command":""}`, true},
		{"stdio missing command", `{"transport":"stdio"}`, true},
		{"public URL allowed", `{"transport":"sse","url":"https://mcp.example.com/sse"}`, false},
		{"localhost blocked", `{"transport":"sse","url":"http://localhost:8080/sse"}`, true},
		{"private IP blocked", `{"transport":"streamable_http","url":"http://10.0.0.1/mcp"}`, true},
		{"loopback blocked", `{"transport":"sse","url":"http://127.0.0.1/sse"}`, true},
		{"cloud metadata blocked", `{"transport":"sse","url":"http://169.254.169.254/meta"}`, true},
		{"invalid scheme blocked", `{"transport":"sse","url":"ftp://evil.com/payload"}`, true},
		{"invalid JSON", `{bad`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMCPConfigURLs(tc.json)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				// DNS-dependent test cases (e.g. "public URL allowed") may fail
				// in environments without network access. Skip instead of fail,
				// matching the pattern in pkg/outboundguard/guard_test.go.
				if isNetworkError(err) {
					t.Skipf("skipping DNS-dependent test case (no network): %v", err)
				}
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// isNetworkError reports whether err is caused by DNS resolution or network
// connectivity failure (as opposed to a validation error). Used to skip
// tests that depend on external DNS in air-gapped CI environments.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "lookup") ||
		strings.Contains(msg, "dial tcp") ||
		strings.Contains(msg, "network") ||
		strings.Contains(msg, "connection refused")
}

func TestMCPServerUsecase_Create_SSRFBlock(t *testing.T) {
	repo := &stubMCPRepo{}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, NewCredentialCrypto(nil, loggateway.NewNoop()))
	_, err := uc.Create(context.Background(), MCPServer{
		Key:        "ssrf-test",
		Name:       "SSRF Test",
		ConfigJSON: `{"transport":"sse","url":"http://127.0.0.1:3000/sse"}`,
	})
	if err == nil {
		t.Fatal("expected SSRF error on Create with localhost URL")
	}
}

func TestMCPServerUsecase_Update_SSRFBlock(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID:        "m1",
		Key:       "my-server",
		Name:      "My Server",
		ConfigJSON: `{"transport":"stdio","command":"npx"}`,
	}}}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, NewCredentialCrypto(nil, loggateway.NewNoop()))
	ssrfURL := `{"transport":"sse","url":"http://192.168.1.1/mcp"}`
	_, err := uc.Update(context.Background(), "m1", MCPServerUpdate{ConfigJSON: &ssrfURL})
	if err == nil {
		t.Fatal("expected SSRF error on Update with private IP URL")
	}
}
