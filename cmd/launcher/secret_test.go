package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAuthSecretFreshInstallGeneratesRandom(t *testing.T) {
	root := t.TempDir()
	s := resolveAuthSecret(root, func(string, ...any) {})
	if s == legacyAuthSecret {
		t.Fatal("fresh install must not use legacy dev secret")
	}
	if len(s) < 32 {
		t.Fatalf("secret too short: %d", len(s))
	}
	// persisted for subsequent runs
	b, err := os.ReadFile(filepath.Join(root, "configs", "auth.secret"))
	if err != nil {
		t.Fatalf("secret not persisted: %v", err)
	}
	if strings.TrimSpace(string(b)) != s {
		t.Fatal("persisted secret mismatch")
	}
	// second call returns the same secret
	s2 := resolveAuthSecret(root, func(string, ...any) {})
	if s2 != s {
		t.Fatal("secret not stable across calls")
	}
}

func TestResolveAuthSecretUpgradeKeepsLegacy(t *testing.T) {
	root := t.TempDir()
	// Simulate an upgraded install: existing bundled PG data means tokens may
	// already be issued under the legacy secret — do not invalidate them.
	if err := os.MkdirAll(filepath.Join(root, "postgres", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "postgres", "data", "PG_VERSION"), []byte("18"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := resolveAuthSecret(root, func(string, ...any) {})
	if s != legacyAuthSecret {
		t.Fatal("upgrade with existing data must keep legacy secret to preserve issued tokens")
	}
}

func TestResolveAuthSecretReadsPersistedFile(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(root, "configs")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("ab", 32)
	if err := os.WriteFile(filepath.Join(cfg, "auth.secret"), []byte(want+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Even with existing PG data, a persisted secret file wins.
	if err := os.MkdirAll(filepath.Join(root, "postgres", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "postgres", "data", "PG_VERSION"), []byte("18"), 0o644)
	if got := resolveAuthSecret(root, func(string, ...any) {}); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveAuthSecretRejectsShortPersisted(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(root, "configs")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "auth.secret"), []byte("tooshort"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Fresh install + short/garbage file → regenerate a strong one.
	s := resolveAuthSecret(root, func(string, ...any) {})
	if len(s) < 32 || s == "tooshort" {
		t.Fatalf("short persisted secret must be regenerated, got %q", s)
	}
}
