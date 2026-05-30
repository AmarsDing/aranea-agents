package service_test

import (
	"testing"

	gwv1 "aranea-agents/api/kratos/gateway/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func ptrBool(v bool) *bool { return &v }

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
		{"tab and space", "\t ", ""},
		{"normal secret", "my-secret-key", "••••••••"},
		{"short secret", "a", "••••••••"},
		{"long secret", "very-long-secret-key-with-many-chars", "••••••••"},
		{"secret with spaces", " my secret ", "••••••••"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.MaskSecret(tt.input)
			if got != tt.expected {
				t.Fatalf("maskSecret(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestWebhookToProto(t *testing.T) {
	w := biz.WebhookConfig{
		ID:             "wh-1",
		Name:           "My Webhook",
		URL:            "https://example.com/webhook",
		EventTypesJSON: `["run.completed","run.failed"]`,
		Secret:         "super-secret-key",
		Headers:        map[string]string{"Authorization": "Bearer token"},
		Enabled:        true,
		CreatedAt:      "2024-01-01",
		UpdatedAt:      "2024-06-01",
	}
	pb := service.WebhookToProto(w)
	if pb.GetId() != "wh-1" || pb.GetName() != "My Webhook" {
		t.Fatalf("id/name mismatch: id=%q name=%q", pb.GetId(), pb.GetName())
	}
	if pb.GetUrl() != "https://example.com/webhook" {
		t.Fatalf("url mismatch: %q", pb.GetUrl())
	}
	if pb.GetEventTypesJson() != `["run.completed","run.failed"]` {
		t.Fatalf("event_types mismatch: %q", pb.GetEventTypesJson())
	}
	if pb.GetSecret() != "••••••••" {
		t.Fatalf("secret should be masked: %q", pb.GetSecret())
	}
	if pb.GetHeaders()["Authorization"] != "Bearer token" {
		t.Fatalf("headers mismatch: %+v", pb.GetHeaders())
	}
	if !pb.GetEnabled() {
		t.Fatalf("enabled mismatch: %v", pb.GetEnabled())
	}
}

func TestWebhookToProtoWithSecret(t *testing.T) {
	w := biz.WebhookConfig{
		ID:             "wh-2",
		Name:           "Secret Webhook",
		URL:            "https://example.com/hook",
		EventTypesJSON: `["run.completed"]`,
		Secret:         "plaintext-secret",
		Headers:        nil,
		Enabled:        false,
		CreatedAt:      "2024-02-01",
		UpdatedAt:      "2024-07-01",
	}
	pb := service.WebhookToProtoWithSecret(w)
	if pb.GetId() != "wh-2" || pb.GetName() != "Secret Webhook" {
		t.Fatalf("id/name mismatch: id=%q name=%q", pb.GetId(), pb.GetName())
	}
	if pb.GetSecret() != "plaintext-secret" {
		t.Fatalf("secret should NOT be masked: %q", pb.GetSecret())
	}
	if pb.GetEnabled() {
		t.Fatalf("enabled should be false")
	}
}

func TestWebhookToProto_EmptySecret(t *testing.T) {
	w := biz.WebhookConfig{
		ID:     "wh-3",
		Name:   "No Secret",
		Secret: "",
	}
	pb := service.WebhookToProto(w)
	if pb.GetSecret() != "" {
		t.Fatalf("empty secret should remain empty: %q", pb.GetSecret())
	}
}

func TestWebhookToProtoWithSecret_EmptySecret(t *testing.T) {
	w := biz.WebhookConfig{
		ID:     "wh-4",
		Name:   "No Secret",
		Secret: "",
	}
	pb := service.WebhookToProtoWithSecret(w)
	if pb.GetSecret() != "" {
		t.Fatalf("empty secret should remain empty: %q", pb.GetSecret())
	}
}

func TestWebhookFromCreate_Nil(t *testing.T) {
	got := service.WebhookFromCreate(nil)
	if !got.Enabled {
		t.Fatalf("default enabled should be true, got %v", got.Enabled)
	}
	if got.Name != "" || got.URL != "" {
		t.Fatalf("expected zero values: %+v", got)
	}
}

func TestWebhookFromCreate(t *testing.T) {
	disabled := false
	req := &gwv1.CreateWebhookRequest{
		Name:           "New Webhook",
		Url:            "https://new.example.com",
		EventTypesJson: `["run.completed"]`,
		Secret:         "new-secret",
		Headers:        map[string]string{"X-Custom": "value"},
		Enabled:        &disabled,
	}
	w := service.WebhookFromCreate(req)
	if w.Name != "New Webhook" || w.URL != "https://new.example.com" {
		t.Fatalf("name/url mismatch: name=%q url=%q", w.Name, w.URL)
	}
	if w.EventTypesJSON != `["run.completed"]` {
		t.Fatalf("event_types mismatch: %q", w.EventTypesJSON)
	}
	if w.Secret != "new-secret" {
		t.Fatalf("secret mismatch: %q", w.Secret)
	}
	if w.Headers["X-Custom"] != "value" {
		t.Fatalf("headers mismatch: %+v", w.Headers)
	}
	if w.Enabled {
		t.Fatalf("enabled should be false when explicitly set")
	}
}

func TestWebhookFromCreate_DefaultEnabled(t *testing.T) {
	req := &gwv1.CreateWebhookRequest{
		Name: "Default Enabled",
		Url:  "https://default.example.com",
	}
	w := service.WebhookFromCreate(req)
	if !w.Enabled {
		t.Fatalf("enabled should default to true when not set")
	}
}

func TestWebhookFromCreate_EnabledTrue(t *testing.T) {
	req := &gwv1.CreateWebhookRequest{
		Name:    "Explicit Enabled",
		Url:     "https://explicit.example.com",
		Enabled: ptrBool(true),
	}
	w := service.WebhookFromCreate(req)
	if !w.Enabled {
		t.Fatalf("enabled should be true when explicitly set")
	}
}

func TestWebhookToProto_AllFields(t *testing.T) {
	w := biz.WebhookConfig{
		ID:             "wh-full",
		Name:           "Full Webhook",
		URL:            "https://full.example.com",
		EventTypesJSON: `["a","b"]`,
		Secret:         "secret123",
		Headers:        map[string]string{"H1": "V1", "H2": "V2"},
		Enabled:        true,
		CreatedAt:      "2024-01-01",
		UpdatedAt:      "2024-12-01",
	}
	pb := service.WebhookToProto(w)
	if pb.GetId() != "wh-full" {
		t.Fatalf("id mismatch: %q", pb.GetId())
	}
	if pb.GetUrl() != "https://full.example.com" {
		t.Fatalf("url mismatch: %q", pb.GetUrl())
	}
	if len(pb.GetHeaders()) != 2 {
		t.Fatalf("headers count mismatch: %d", len(pb.GetHeaders()))
	}
	if pb.GetCreatedAt() != "2024-01-01" || pb.GetUpdatedAt() != "2024-12-01" {
		t.Fatalf("timestamps mismatch: created=%q updated=%q", pb.GetCreatedAt(), pb.GetUpdatedAt())
	}
}

func TestWebhookToProtoWithSecret_AllFields(t *testing.T) {
	w := biz.WebhookConfig{
		ID:             "wh-full-secret",
		Name:           "Full Secret Webhook",
		URL:            "https://secret.example.com",
		EventTypesJSON: `["c"]`,
		Secret:         "real-secret",
		Headers:        nil,
		Enabled:        false,
		CreatedAt:      "2024-03-01",
		UpdatedAt:      "2024-09-01",
	}
	pb := service.WebhookToProtoWithSecret(w)
	if pb.GetSecret() != "real-secret" {
		t.Fatalf("secret should be plaintext: %q", pb.GetSecret())
	}
	if pb.GetEnabled() {
		t.Fatalf("enabled should be false")
	}
}
