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
	crypto := biz.NewCredentialCrypto(nil, nil)
	ref, err := crypto.EncryptChannelSecretRef(ctx, "token-123")
	if err != nil {
		t.Fatal(err)
	}
	uc := biz.NewChannelUsecase(nil, nil, nil, nil, nil, crypto, nil)
	got, err := ResolveSecretRef(ctx, uc, ref)
	if err != nil {
		t.Fatal(err)
	}
	if got != "token-123" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecretRefEnv(t *testing.T) {
	crypto := biz.NewCredentialCrypto(nil, nil)
	uc := biz.NewChannelUsecase(nil, nil, nil, nil, nil, crypto, nil)
	_ = os.Setenv("TEST_CHANNEL_SECRET", "abc")
	t.Cleanup(func() { _ = os.Unsetenv("TEST_CHANNEL_SECRET") })
	got, err := ResolveSecretRef(context.Background(), uc, "env:TEST_CHANNEL_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecretRefLocalDeprecated(t *testing.T) {
	crypto := biz.NewCredentialCrypto(nil, nil)
	uc := biz.NewChannelUsecase(nil, nil, nil, nil, nil, crypto, nil)
	_, err := ResolveSecretRef(context.Background(), uc, "local:deadbeef")
	if err == nil {
		t.Fatal("expected error")
	}
}
