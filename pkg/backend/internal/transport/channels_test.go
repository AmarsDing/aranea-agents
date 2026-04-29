package transport

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"arenea/backend/internal/repository"
	"arenea/backend/internal/service"
)

func TestChannelCatalogRoute(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	channelSvc := service.NewChannelService(repo)
	handler := NewHTTPHandler(Services{Channel: channelSvc})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/catalog", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !containsAll(body, `"type":"telegram"`, `"type":"feishu"`, `"type":"voice-call"`) {
		t.Fatalf("catalog response missing expected channel types: %s", body)
	}
}

func containsAll(body string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(body, needle) {
			return false
		}
	}
	return true
}
