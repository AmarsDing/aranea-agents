package service

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestChannelCredentialSecretRef(t *testing.T) {
	t.Run("returns_ref_when_key_matched", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "enc:abc123"},
			{CredentialKey: "app_secret", SecretRef: "enc:xyz789"},
		}
		ref, err := ChannelCredentialSecretRef(creds, "bot_token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref != "enc:abc123" {
			t.Fatalf("expected enc:abc123, got %q", ref)
		}
	})

	t.Run("returns_ref_case_insensitive", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "App_Secret", SecretRef: "enc:xyz789"},
		}
		ref, err := ChannelCredentialSecretRef(creds, "app_secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref != "enc:xyz789" {
			t.Fatalf("expected enc:xyz789, got %q", ref)
		}
	})

	t.Run("trims_key_whitespace", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "enc:abc123"},
		}
		ref, err := ChannelCredentialSecretRef(creds, "  bot_token  ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref != "enc:abc123" {
			t.Fatalf("expected enc:abc123, got %q", ref)
		}
	})

	t.Run("trims_credential_key_whitespace", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "  bot_token  ", SecretRef: "enc:abc123"},
		}
		ref, err := ChannelCredentialSecretRef(creds, "bot_token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref != "enc:abc123" {
			t.Fatalf("expected enc:abc123, got %q", ref)
		}
	})

	t.Run("error_when_secret_ref_empty", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: ""},
		}
		_, err := ChannelCredentialSecretRef(creds, "bot_token")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("error_when_secret_ref_whitespace_only", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "   "},
		}
		_, err := ChannelCredentialSecretRef(creds, "bot_token")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("error_when_key_not_found", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "enc:abc123"},
		}
		_, err := ChannelCredentialSecretRef(creds, "app_secret")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("error_when_creds_empty", func(t *testing.T) {
		_, err := ChannelCredentialSecretRef(nil, "bot_token")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("error_when_key_empty", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "enc:abc123"},
		}
		_, err := ChannelCredentialSecretRef(creds, "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("error_when_key_whitespace_only", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "enc:abc123"},
		}
		_, err := ChannelCredentialSecretRef(creds, "   ")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("matches_first_occurrence", func(t *testing.T) {
		creds := []biz.ChannelCredential{
			{CredentialKey: "bot_token", SecretRef: "enc:first"},
			{CredentialKey: "bot_token", SecretRef: "enc:second"},
		}
		ref, err := ChannelCredentialSecretRef(creds, "bot_token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref != "enc:first" {
			t.Fatalf("expected enc:first, got %q", ref)
		}
	})
}
