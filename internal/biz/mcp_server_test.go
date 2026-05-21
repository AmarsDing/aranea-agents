package biz

import (
	"context"
	"encoding/json"
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

func TestRecordReconnectMetadata_PersistsCountAndTimestamp(t *testing.T) {
	repo := &stubMCPRepo{rows: []MCPServer{{
		ID:           "m1",
		Key:          "my-server",
		MetadataJSON: `{"reconnect_count":2}`,
	}}}
	uc := NewMCPServerUsecase(repo)
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
