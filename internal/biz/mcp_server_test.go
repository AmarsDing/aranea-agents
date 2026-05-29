package biz

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
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

func TestRecordReconnectMetadata_PersistsCountAndTimestamp(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID:           "m1",
		Key:          "my-server",
		MetadataJSON: `{"reconnect_count":2}`,
	}}}
	uc := NewMCPServerUsecase(repo, NewCredentialCrypto(nil))
	uc.SetMetadataEditor(testMCPMetadataEditor{})
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

// testMCPMetadataEditor is a test double that delegates to the same logic as the real adapter.
type testMCPMetadataEditor struct{}

func (testMCPMetadataEditor) Parse(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return map[string]any{}
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (testMCPMetadataEditor) Marshal(m map[string]any) (string, error) {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (testMCPMetadataEditor) ApplyHealth(m map[string]any, healthStatus string, ok bool, errMsg string, at time.Time) string {
	if m == nil {
		m = map[string]any{}
	}
	m["health_status"] = healthStatus
	m["last_health_at"] = at.UTC().Format(time.RFC3339)
	if ok {
		m["last_error_message"] = ""
		delete(m, "health_error_since")
		return "active"
	}
	m["last_error_message"] = errMsg
	if _, exists := m["health_error_since"]; !exists {
		m["health_error_since"] = at.UTC().Format(time.RFC3339)
	}
	return "error"
}

func (testMCPMetadataEditor) ApplyReconnect(m map[string]any, at time.Time) {
	if m == nil {
		m = map[string]any{}
	}
	m["last_reconnect_at"] = at.UTC().Format(time.RFC3339)
	switch v := m["reconnect_count"].(type) {
	case float64:
		m["reconnect_count"] = int(v) + 1
	case int:
		m["reconnect_count"] = v + 1
	default:
		m["reconnect_count"] = 1
	}
}

func (testMCPMetadataEditor) MarkHealthAlert(m map[string]any, at time.Time) {
	if m == nil {
		return
	}
	m["last_health_alert_at"] = at.UTC().Format(time.RFC3339)
}
