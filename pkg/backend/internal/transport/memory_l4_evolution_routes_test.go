package transport

import (
	mem "arenea/backend/internal/memory/domain"

	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"arenea/backend/internal/repository"
	"arenea/backend/internal/service"
)

func newMemoryParityHandler(t *testing.T) (http.Handler, repository.Store, *service.ChatService) {
	t.Helper()
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "memory-parity.db"))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	chat := service.NewChatService(repo, nil)
	handler := NewHTTPHandler(Services{
		Chat:  chat,
		Audit: service.NewAuditService(repo),
	})
	return handler, repo, chat
}

func TestAgentEvolutionSpecAliasRoutes(t *testing.T) {
	handler, _, _ := newMemoryParityHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-alias/identity", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("identity alias status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"agent_id":"agent-alias"`) {
		t.Fatalf("identity alias body missing agent id: %s", rec.Body.String())
	}

	body := bytes.NewBufferString(`{
		"proposal_kind":"tool_pref_update",
		"target_field":"strategy.tool_preference",
		"proposed_value":{"shell":0.8},
		"current_value":{},
		"rationale":"shell succeeds often",
		"source":"user"
	}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-alias/evolution/proposals", body)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("proposal alias status=%d body=%s", rec.Code, rec.Body.String())
	}
	proposalID := extractJSONID(rec.Body.String())
	if proposalID == "" {
		t.Fatalf("could not parse proposal id from %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-alias/evolution/proposals/"+proposalID, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), proposalID) {
		t.Fatalf("single proposal alias failed status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-alias/evolution/metrics?range=30d", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"proposals_total"`) {
		t.Fatalf("metrics alias failed status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMemoryL4SpecAliasRoutes(t *testing.T) {
	handler, _, chat := newMemoryParityHandler(t)
	entity, err := chat.MemoryL4().UpsertEntity(httptest.NewRequest(http.MethodPost, "/", nil).Context(), service.EntityUpsertInput{
		ScopeType:  mem.ScopeAgent,
		ScopeID:    "agent-l4-route",
		EntityType: mem.EntityFramework,
		Name:       "React",
	})
	if err != nil {
		t.Fatalf("upsert entity: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memory/l4/entities/"+entity.ID+"/neighborhood?hops=1&max=5", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"center"`) {
		t.Fatalf("entity neighborhood alias failed status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/memory/l4/entities:search", bytes.NewBufferString(`{"scope_type":"agent","scope_id":"agent-l4-route","query":"React"}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"React"`) {
		t.Fatalf("entities:search alias failed status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func extractJSONID(body string) string {
	marker := `"id":"`
	start := strings.Index(body, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := strings.Index(body[start:], `"`)
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}
