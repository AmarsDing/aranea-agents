package service

import (
	"context"
	"encoding/hex"
	"os"
	"testing"

	"aranea-agents/internal/biz"
)

func withCredentialKey(t *testing.T) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 3)
	}
	_ = os.Setenv("ARANEA_CREDENTIAL_KEY", hex.EncodeToString(key))
	t.Cleanup(func() { _ = os.Unsetenv("ARANEA_CREDENTIAL_KEY") })
}

func TestResolveSecretRefEncRoundTrip(t *testing.T) {
	withCredentialKey(t)
	ctx := context.Background()
	ref, err := biz.EncryptChannelSecretRef(ctx, "token-123")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ResolveSecretRef(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if got != "token-123" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecretRefEnv(t *testing.T) {
	_ = os.Setenv("TEST_CHANNEL_SECRET", "abc")
	t.Cleanup(func() { _ = os.Unsetenv("TEST_CHANNEL_SECRET") })
	got, err := ResolveSecretRef(context.Background(), "env:TEST_CHANNEL_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecretRefLocalDeprecated(t *testing.T) {
	_, err := ResolveSecretRef(context.Background(), "local:deadbeef")
	if err == nil {
		t.Fatal("expected error")
	}
}
