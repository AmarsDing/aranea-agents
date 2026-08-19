package biz

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	mcpmetadata "aranea-agents/internal/mcp/metadata"
	"aranea-agents/pkg/loggateway"
)

type stubMCPRepo struct {
	rows              []MCPServer
	fullUpdateCalled  bool
	configPatchCalled bool
}

func (s *stubMCPRepo) ListMCPServers(_ context.Context, _ MCPListQuery) ([]MCPServer, error) {
	return s.rows, nil
}

func (s *stubMCPRepo) ListMCPServersPaged(_ context.Context, q MCPListQuery) (MCPListResult, error) {
	return MCPListResult{Items: s.rows, Total: len(s.rows), Limit: q.Limit, Offset: q.Offset}, nil
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
	s.fullUpdateCalled = true
	for i := range s.rows {
		if s.rows[i].ID == m.ID {
			s.rows[i] = m
			return m, nil
		}
	}
	return m, nil
}

func (s *stubMCPRepo) UpdateMCPServerConfigJSON(_ context.Context, id string, configJSON string) error {
	s.configPatchCalled = true
	for i := range s.rows {
		if s.rows[i].ID == id {
			s.rows[i].ConfigJSON = configJSON
			return nil
		}
	}
	return nil
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
			// 与真实 data 层一致：status="" 表示不触碰行状态。
			if status != "" {
				s.rows[i].Status = status
			}
			return nil
		}
	}
	return nil
}

// stubMCPCredRepo is a stub for MCPServerUserCredentialRepo.
type stubMCPCredRepo struct {
	creds []MCPServerUserCredential
}

func (s stubMCPCredRepo) ListMCPServerUserCredentials(_ context.Context, _, _ string) ([]MCPServerUserCredential, error) {
	return s.creds, nil
}
func (stubMCPCredRepo) UpsertMCPServerUserCredential(_ context.Context, c MCPServerUserCredential) (MCPServerUserCredential, error) {
	return c, nil
}
func (stubMCPCredRepo) DeleteMCPServerUserCredential(_ context.Context, _, _, _ string) error {
	return nil
}

func TestMCPServerUsecase_PersistRotatedRefreshToken(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID:           "id1",
		Key:          "srv1",
		Status:       "active",
		MetadataJSON: `{"health_status":"ok","last_health_at":"2026-08-14T00:00:00Z"}`,
		ConfigJSON: `{"transport":"streamable_http","url":"https://mcp.example.com/mcp",` +
			`"auth":{"type":"oauth2_refresh","token_url":"https://auth.example.com/token","client_id":"cid","refresh_token":"old-refresh"}}`,
	}}}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, nil)

	if err := uc.PersistRotatedRefreshToken(context.Background(), "srv1", "new-refresh"); err != nil {
		t.Fatalf("PersistRotatedRefreshToken: %v", err)
	}
	var cfg struct {
		URL  string `json:"url"`
		Auth struct {
			Type         string `json:"type"`
			RefreshToken string `json:"refresh_token"`
		} `json:"auth"`
	}
	if err := json.Unmarshal([]byte(repo.rows[0].ConfigJSON), &cfg); err != nil {
		t.Fatalf("stored config not valid JSON: %v", err)
	}
	if cfg.Auth.RefreshToken != "new-refresh" {
		t.Fatalf("refresh_token not persisted: got %q", cfg.Auth.RefreshToken)
	}
	if cfg.URL != "https://mcp.example.com/mcp" || cfg.Auth.Type != "oauth2_refresh" {
		t.Fatalf("unrelated config fields changed: %+v", cfg)
	}
	// RV-01: token 回写必须走字段级写，禁止全行写（避免覆盖并发健康元数据）。
	if repo.fullUpdateCalled {
		t.Fatal("PersistRotatedRefreshToken must not use full-row UpdateMCPServer")
	}
	if !repo.configPatchCalled {
		t.Fatal("PersistRotatedRefreshToken must use field-level UpdateMCPServerConfigJSON")
	}
	if repo.rows[0].MetadataJSON != `{"health_status":"ok","last_health_at":"2026-08-14T00:00:00Z"}` {
		t.Fatalf("metadata_json clobbered: %s", repo.rows[0].MetadataJSON)
	}
	if repo.rows[0].Status != "active" {
		t.Fatalf("status clobbered: %s", repo.rows[0].Status)
	}
}

func TestMCPServerUsecase_PersistRotatedRefreshToken_NotFound(t *testing.T) {
	repo := &stubMCPRepo{}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, nil)
	err := uc.PersistRotatedRefreshToken(context.Background(), "missing", "tok")
	if err == nil {
		t.Fatal("expected not-found error for unknown server key")
	}
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
func (mcpMetadataAdapter) ApplyToolDiscovery(m map[string]any, count int, names []string, at time.Time) map[string]any {
	return mcpmetadata.ApplyToolDiscovery(m, count, names, at)
}
func (mcpMetadataAdapter) ApplyToolDiscoveryError(m map[string]any, errMsg string, at time.Time) map[string]any {
	return mcpmetadata.ApplyToolDiscoveryError(m, errMsg, at)
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
		{"stdio shell metachar blocked", `{"transport":"stdio","command":"npx;rm -rf /"}`, true},
		{"stdio path traversal blocked", `{"transport":"stdio","command":"../bin/evil"}`, true},
		{"stdio args metachar blocked", `{"transport":"stdio","command":"npx","args":["foo;bar"]}`, true},
		{"public URL allowed", `{"transport":"sse","url":"https://mcp.example.com/sse"}`, false},
		{"localhost blocked", `{"transport":"sse","url":"http://localhost:8080/sse"}`, true},
		{"private IP blocked", `{"transport":"streamable_http","url":"http://10.0.0.1/mcp"}`, true},
		{"loopback blocked", `{"transport":"sse","url":"http://127.0.0.1/sse"}`, true},
		{"cloud metadata blocked", `{"transport":"sse","url":"http://169.254.169.254/meta"}`, true},
		{"invalid scheme blocked", `{"transport":"sse","url":"ftp://evil.com/payload"}`, true},
		{"invalid JSON", `{bad`, true},
		// OAuth2 token_url SSRF checks (B1 fix): token_url must pass the same
		// SSRF validation as the transport url, regardless of transport type.
		{"oauth2 token_url public allowed", `{"transport":"sse","url":"https://mcp.example.com/sse","auth":{"type":"oauth2_client_credentials","token_url":"https://auth.example.com/token"}}`, false},
		{"oauth2 token_url localhost blocked", `{"transport":"sse","url":"https://mcp.example.com/sse","auth":{"type":"oauth2_client_credentials","token_url":"http://localhost:8080/token"}}`, true},
		{"oauth2 token_url cloud metadata blocked", `{"transport":"streamable_http","url":"https://mcp.example.com/mcp","auth":{"type":"oauth2_refresh","token_url":"http://169.254.169.254/latest/meta-data/iam/security-credentials/"}}`, true},
		{"oauth2 token_url private IP blocked", `{"transport":"sse","url":"https://mcp.example.com/sse","auth":{"type":"oauth2_client_credentials","token_url":"http://10.0.0.1/token"}}`, true},
		{"oauth2 token_url empty allowed", `{"transport":"sse","url":"https://mcp.example.com/sse","auth":{"type":"oauth2_client_credentials","token_url":""}}`, false},
		{"oauth2 token_url missing auth allowed", `{"transport":"sse","url":"https://mcp.example.com/sse"}`, false},
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
		ID:         "m1",
		Key:        "my-server",
		Name:       "My Server",
		ConfigJSON: `{"transport":"stdio","command":"npx"}`,
	}}}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, NewCredentialCrypto(nil, loggateway.NewNoop()))
	ssrfURL := `{"transport":"sse","url":"http://192.168.1.1/mcp"}`
	_, err := uc.Update(context.Background(), "m1", MCPServerUpdate{ConfigJSON: &ssrfURL})
	if err == nil {
		t.Fatal("expected SSRF error on Update with private IP URL")
	}
}

// TestResolveUserAuthHeaders_FallbackUsesServerHeaderName verifies the S2 fix:
// when falling back to a credential whose credential_key differs from the
// server-configured header name, the secret must be written to the
// server-configured header (keyName), not the credential's own key.
//
// Scenario: server expects "Authorization" header (keyName="authorization"),
// but the only configured credential has credential_key="x-api-key".
// Pass 1 (exact match) and Pass 2 (authorization fallback) both miss,
// so Pass 3 picks the x-api-key credential. The fix ensures the secret
// is written to "authorization" (keyName), not "x-api-key".
func TestResolveUserAuthHeaders_FallbackUsesServerHeaderName(t *testing.T) {
	// Set up a real CredentialCrypto with a test key.
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 20)
	}
	_ = os.Setenv(envCredentialKey, hex.EncodeToString(key))
	defer os.Unsetenv(envCredentialKey)

	crypto := NewCredentialCrypto(nil, loggateway.NewNoop())
	secretRef, err := crypto.EncryptChannelSecretRef(context.Background(), "my-token-value")
	if err != nil {
		t.Fatal(err)
	}

	repo := &stubMCPRepo{rows: []MCPServer{{
		ID:  "m1",
		Key: "srv",
	}}}
	credRepo := stubMCPCredRepo{creds: []MCPServerUserCredential{{
		ID:            "c1",
		MCPServerID:   "m1",
		UserID:        "u1",
		CredentialKey: "x-api-key",
		SecretRef:     secretRef,
		Configured:    true,
	}}}
	uc := NewMCPServerUsecase(repo, credRepo, nil, mcpMetadataAdapter{}, crypto)

	// Server expects Authorization header.
	sc := MCPServerConfig{
		Auth:                   MCPServerConfig{}.Auth,
		RequireUserCredentials: true,
	}
	sc.Auth.HeaderName = "authorization"

	headers, err := uc.ResolveUserAuthHeaders(context.Background(), "srv", "u1", sc)
	if err != nil {
		t.Fatal(err)
	}

	// S2 fix: secret should be in "authorization" (keyName), not "x-api-key".
	authVal, ok := headers["authorization"]
	if !ok {
		t.Fatalf("expected secret in 'authorization' header, got headers: %v", headers)
	}
	if !strings.HasPrefix(strings.ToLower(authVal), "bearer ") {
		t.Fatalf("expected Bearer prefix, got %q", authVal)
	}
	if strings.TrimPrefix(authVal, "Bearer ") != "my-token-value" {
		t.Fatalf("expected 'my-token-value', got %q", authVal)
	}

	// Ensure secret was NOT written to the credential's own key.
	if _, leaked := headers["x-api-key"]; leaked {
		t.Fatal("S2 bug: secret leaked to 'x-api-key' header instead of server-configured 'authorization'")
	}
}

// --- P2: tool discovery -------------------------------------------------

type stubMCPProber struct{ result MCPTestResult }

func (s stubMCPProber) Evaluate(_ context.Context, _ bool, _ string) MCPTestResult { return s.result }

type stubMCPDiscoverer struct {
	names      []string
	err        error
	called     int
	lastKey    string
	lastConfig string
}

func (s *stubMCPDiscoverer) DiscoverTools(_ context.Context, serverKey string, configJSON string) ([]string, error) {
	s.called++
	s.lastKey = serverKey
	s.lastConfig = configJSON
	return s.names, s.err
}

func TestTestMCPServer_MergesFullHandshakeDetails(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID: "m1", Key: "srv", Enabled: true, Status: "active",
		ConfigJSON: `{"transport":"stdio","command":"x","probe_mode":"full_handshake"}`,
	}}}
	prober := stubMCPProber{result: MCPTestResult{
		OK: true, Status: "ok", Message: "握手成功，发现 2 个工具",
		Details: map[string]any{"tool_count": 2, "tool_names": []string{"search", "fetch"}},
	}}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, prober, mcpMetadataAdapter{}, nil)

	res, err := uc.TestMCPServer(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Result.OK {
		t.Fatalf("result=%+v", res.Result)
	}
	meta := mcpmetadata.Parse(repo.rows[0].MetadataJSON)
	if meta["tool_count"] != float64(2) {
		t.Fatalf("tool_count=%v", meta["tool_count"])
	}
	if meta["tools_discovered_at"] == nil || meta["tools_discovered_at"] == "" {
		t.Fatal("tools_discovered_at missing")
	}
	if repo.rows[0].Status != "active" {
		t.Fatalf("status=%q", repo.rows[0].Status)
	}
}

func TestTestMCPServer_HandshakeFailureRecordsToolsError(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID: "m1", Key: "srv", Enabled: true, Status: "active",
		ConfigJSON:   `{"transport":"stdio","command":"x","probe_mode":"full_handshake"}`,
		MetadataJSON: `{"tool_count":5}`,
	}}}
	prober := stubMCPProber{result: MCPTestResult{
		OK: false, Status: "error", Message: "MCP 握手失败: boom",
		Details: map[string]any{"phase": "handshake"},
	}}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, prober, mcpMetadataAdapter{}, nil)

	_, err := uc.TestMCPServer(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	meta := mcpmetadata.Parse(repo.rows[0].MetadataJSON)
	if meta["tools_error_message"] != "MCP 握手失败: boom" {
		t.Fatalf("tools_error_message=%v", meta["tools_error_message"])
	}
	// 上次成功数据保留
	if meta["tool_count"] != float64(5) {
		t.Fatalf("tool_count clobbered: %v", meta["tool_count"])
	}
	if repo.rows[0].Status != "error" {
		t.Fatalf("status=%q, want error (health flipped)", repo.rows[0].Status)
	}
}

func TestDiscoverMCPServerTools_Success(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID: "m1", Key: "srv", Enabled: true, Status: "active",
		ConfigJSON: `{"transport":"stdio","command":"x"}`,
	}}}
	disc := &stubMCPDiscoverer{names: []string{"a", "b", "c"}}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, nil)
	uc.SetToolDiscoverer(disc)

	res, err := uc.DiscoverMCPServerTools(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.ToolCount != 3 {
		t.Fatalf("res=%+v", res)
	}
	if disc.lastKey != "srv" {
		t.Fatalf("discoverer key=%q", disc.lastKey)
	}
	meta := mcpmetadata.Parse(repo.rows[0].MetadataJSON)
	if meta["tool_count"] != float64(3) {
		t.Fatalf("tool_count=%v", meta["tool_count"])
	}
	if repo.rows[0].Status != "active" {
		t.Fatalf("status clobbered: %q", repo.rows[0].Status)
	}
}

func TestDiscoverMCPServerTools_RequireUserCredentialsShortCircuits(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID: "m1", Key: "srv", Enabled: true, Status: "active",
		ConfigJSON: `{"transport":"streamable","url":"https://x","require_user_credentials":true}`,
	}}}
	disc := &stubMCPDiscoverer{names: []string{"a"}}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, nil)
	uc.SetToolDiscoverer(disc)

	res, err := uc.DiscoverMCPServerTools(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected OK=false for per-user credential server")
	}
	if disc.called != 0 {
		t.Fatal("discoverer must not be called for per-user credential server")
	}
	meta := mcpmetadata.Parse(repo.rows[0].MetadataJSON)
	if meta["tools_error_message"] == nil || meta["tools_error_message"] == "" {
		t.Fatal("expected tools_error_message recorded")
	}
}

func TestDiscoverMCPServerTools_FailurePreservesLastGood(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID: "m1", Key: "srv", Enabled: true, Status: "active",
		ConfigJSON:   `{"transport":"stdio","command":"x"}`,
		MetadataJSON: `{"tool_count":7,"tool_names":["a"]}`,
	}}}
	disc := &stubMCPDiscoverer{err: context.DeadlineExceeded}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, nil)
	uc.SetToolDiscoverer(disc)

	res, err := uc.DiscoverMCPServerTools(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected OK=false")
	}
	meta := mcpmetadata.Parse(repo.rows[0].MetadataJSON)
	if meta["tool_count"] != float64(7) {
		t.Fatalf("tool_count clobbered: %v", meta["tool_count"])
	}
	if meta["tools_error_message"] == nil {
		t.Fatal("tools_error_message missing")
	}
}

func TestDiscoverMCPServerTools_NoDiscovererConfigured(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID: "m1", Key: "srv", Enabled: true, ConfigJSON: `{"transport":"stdio","command":"x"}`,
	}}}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, nil)
	res, err := uc.DiscoverMCPServerTools(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected OK=false when discoverer not configured")
	}
}
