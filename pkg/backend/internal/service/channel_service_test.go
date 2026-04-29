package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

func newTestChannelService(t *testing.T) *ChannelService {
	t.Helper()
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewChannelService(repo)
}

func TestChannelCatalogHasTwentyPlusItems(t *testing.T) {
	svc := newTestChannelService(t)
	items := svc.Catalog()
	if len(items) < 20 {
		t.Fatalf("expected at least 20 channel catalog items, got %d", len(items))
	}
	if !catalogHasType("telegram") || !catalogHasType("feishu") || !catalogHasType("voice-call") {
		t.Fatalf("expected catalog to include common channel types")
	}
}

func TestChannelCreateCredentialsAreSanitized(t *testing.T) {
	svc := newTestChannelService(t)
	row, err := svc.Create(domain.PlatformResource{
		Key:        "telegram_support",
		Name:       "Telegram Support",
		Enabled:    true,
		ConfigJSON: `{"type":"telegram","receive_mode":"webhook","webhook":{"path":"/webhooks/telegram_support"},"config":{}}`,
	}, []domain.ChannelCredentialInput{{CredentialKey: "bot_token", Secret: "123456:secret-token"}})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	credentials, err := svc.ListCredentials(row.ID)
	if err != nil {
		t.Fatalf("list credentials: %v", err)
	}
	if len(credentials) != 1 {
		t.Fatalf("expected one credential, got %d", len(credentials))
	}
	if credentials[0].SecretRef != "" {
		t.Fatalf("secret_ref should not be returned to API callers")
	}
	if !credentials[0].Configured || credentials[0].MaskedPreview == "" {
		t.Fatalf("expected configured credential with masked preview")
	}
}

func TestChannelToggleOnlyChangesEnabled(t *testing.T) {
	svc := newTestChannelService(t)
	row, err := svc.Create(domain.PlatformResource{
		Key:        "qa",
		Name:       "QA",
		Enabled:    true,
		ConfigJSON: `{"type":"qa-channel","receive_mode":"plugin","config":{"foo":"bar"}}`,
	}, nil)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	updated, err := svc.Toggle(row.ID, false)
	if err != nil {
		t.Fatalf("toggle channel: %v", err)
	}
	if updated.Enabled {
		t.Fatalf("expected channel to be disabled")
	}
	if !strings.Contains(updated.ConfigJSON, `"foo":"bar"`) {
		t.Fatalf("toggle should preserve config_json, got %s", updated.ConfigJSON)
	}
}

func TestChannelRejectsInvalidConfigJSON(t *testing.T) {
	svc := newTestChannelService(t)
	_, err := svc.Create(domain.PlatformResource{
		Key:        "bad",
		Name:       "Bad",
		Enabled:    true,
		ConfigJSON: `{bad json`,
	}, nil)
	if err == nil {
		t.Fatalf("expected invalid config_json to be rejected")
	}
}

func TestChannelTestReturnsStructuredResult(t *testing.T) {
	svc := newTestChannelService(t)
	row, err := svc.Create(domain.PlatformResource{
		Key:        "telegram_support",
		Name:       "Telegram Support",
		Enabled:    true,
		ConfigJSON: `{"type":"telegram","receive_mode":"webhook","webhook":{"path":"/webhooks/telegram_support"},"config":{}}`,
	}, []domain.ChannelCredentialInput{{CredentialKey: "bot_token", Secret: "123456:secret-token"}})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	result, err := svc.Test(context.Background(), row.ID)
	if err != nil {
		t.Fatalf("test channel: %v", err)
	}
	if !result.OK || result.Status == "" || result.Message == "" {
		t.Fatalf("expected successful structured test result, got %+v", result)
	}
}
