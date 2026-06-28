package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistentDevSecret_GeneratesAndPersists(t *testing.T) {
	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "dev-jwt-secret")

	// First call: generates and persists
	secret1 := persistentDevSecret(secretPath)

	// Secret must be at least 32 chars (JWT signing key requirement)
	if len(secret1) < 32 {
		t.Fatalf("secret too short: %d chars", len(secret1))
	}

	// Must not be the old unstable placeholders
	if secret1 == "test-placeholder-secret-not-for-production" {
		t.Fatal("secret should not be the old test placeholder")
	}
	if secret1 == "dev-bypass-placeholder" {
		t.Fatal("secret should not be the bypass placeholder")
	}

	// File must have been created
	if _, err := os.Stat(secretPath); err != nil {
		t.Fatalf("secret file not created: %v", err)
	}

	// Second call: must return the same secret (stable across restarts)
	secret2 := persistentDevSecret(secretPath)
	if secret1 != secret2 {
		t.Fatalf("secret changed between calls (should be stable):\n  first=%q\n  second=%q", secret1, secret2)
	}
}

func TestPersistentDevSecret_LoadsExistingSecret(t *testing.T) {
	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "dev-jwt-secret")

	// Pre-write a known secret of sufficient length
	knownSecret := strings.Repeat("a", 40)
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte(knownSecret), 0o600); err != nil {
		t.Fatal(err)
	}

	// Should load the existing secret, not generate a new one
	loaded := persistentDevSecret(secretPath)
	if loaded != knownSecret {
		t.Fatalf("expected to load existing secret, got different value:\n  want=%q\n  got=%q", knownSecret, loaded)
	}
}

func TestPersistentDevSecret_RegeneratesIfTooShort(t *testing.T) {
	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "dev-jwt-secret")

	// Pre-write a too-short secret (< 32 chars)
	shortSecret := "too-short"
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte(shortSecret), 0o600); err != nil {
		t.Fatal(err)
	}

	// Should regenerate because the existing secret is too short
	regenerated := persistentDevSecret(secretPath)
	if regenerated == shortSecret {
		t.Fatal("should have regenerated too-short secret")
	}
	if len(regenerated) < 32 {
		t.Fatalf("regenerated secret too short: %d chars", len(regenerated))
	}
}

func TestPersistentDevSecret_DifferentPathsProduceDifferentSecrets(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	s1 := persistentDevSecret(filepath.Join(dir1, "dev-jwt-secret"))
	s2 := persistentDevSecret(filepath.Join(dir2, "dev-jwt-secret"))

	if s1 == s2 {
		t.Fatal("different paths should produce different secrets (random generation)")
	}
}

func TestPersistentDevSecret_TrimsWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "dev-jwt-secret")

	// Pre-write a secret with trailing newline (common when edited by hand)
	rawSecret := strings.Repeat("b", 40) + "\n"
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte(rawSecret), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded := persistentDevSecret(secretPath)
	want := strings.Repeat("b", 40)
	if loaded != want {
		t.Fatalf("whitespace not trimmed:\n  want=%q\n  got=%q", want, loaded)
	}
}
