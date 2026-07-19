package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aranea-agents/internal/cli/config"
)

func TestLoad_FileNotExist(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("Load: expected no error for missing file, got %v", err)
	}
	if cfg == nil {
		t.Fatal("Load: expected non-nil config")
	}
	if cfg.Backend.BaseURL == "" {
		t.Error("Load: expected default base_url to be set")
	}
}

func TestSave_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aranea", "config.toml")

	cfg, _ := config.Load(path)
	cfg.Backend.BaseURL = "http://test:9090"
	cfg.Backend.Token = "tok-test"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Save: file not created: %v", err)
	}

	// On non-Windows, verify permissions.
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(path)
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("Save: expected 0600 perm, got %04o", fi.Mode().Perm())
		}
	}
}

func TestLoad_InsecurePerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission check not enforced on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write a config with loose permissions.
	content := `[backend]
base_url = "http://localhost:8080"
token = "secret-jwt-token"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err == nil {
		t.Fatal("Load: expected error for insecure permissions, got nil")
	}
	if cfg.Backend.Token != "" {
		t.Errorf("Load: token should be empty when permissions are insecure, got %q", cfg.Backend.Token)
	}
}

func TestOverrideFromEnv(t *testing.T) {
	t.Setenv("ARANEA_BASE_URL", "http://envhost:9000")
	t.Setenv("ARANEA_TOKEN", "env-token")

	cfg, _ := config.Load("")
	cfg.OverrideFromEnv()

	if cfg.Backend.BaseURL != "http://envhost:9000" {
		t.Errorf("expected %q, got %q", "http://envhost:9000", cfg.Backend.BaseURL)
	}
	if cfg.Backend.Token != "env-token" {
		t.Errorf("expected %q, got %q", "env-token", cfg.Backend.Token)
	}
}

func TestMaskToken(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"abc", "***"},
		{"abcd", "***"},
		{"eyJhbGciOiJIUzI1NiJ9", "***NiJ9"},
	}
	for _, c := range cases {
		got := config.MaskToken(c.input)
		if got != c.want {
			t.Errorf("MaskToken(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestDefaultPath(t *testing.T) {
	path, err := config.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if path == "" {
		t.Error("DefaultPath: returned empty string")
	}
	t.Logf("DefaultPath: %s", path)
}

// TestLoad_ConfigInvalidError_RedactsSecrets verifies that configInvalidError
// sanitizes its cause message via RedactAndTruncate, preventing secret leakage
// when toml.DecodeError echoes source lines containing tokens.
func TestLoad_ConfigInvalidError_RedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write a TOML config with a token that will cause a parse error.
	content := `[backend]
base_url = "http://localhost:8080"
token = "sk-livekey123456789abcdef" oops this is invalid toml
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load: expected error for invalid TOML, got nil")
	}

	errMsg := err.Error()
	// The error should mention the file path and that parsing failed.
	if !strings.Contains(errMsg, "CONFIG_INVALID") {
		t.Errorf("expected CONFIG_INVALID prefix, got: %s", errMsg)
	}
	// The error must NOT contain the raw API key.
	if strings.Contains(errMsg, "sk-livekey123456789abcdef") {
		t.Errorf("error message leaks API key: %s", errMsg)
	}
}

// TestLoad_ConfigInvalidError_UnwrapSafe verifies that Unwrap() still works
// (for errors.As compatibility) but the Error() string is redacted.
func TestLoad_ConfigInvalidError_UnwrapSafe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `[backend]
token = "sk-ant-api03-secretkey123" invalid toml here
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load: expected error, got nil")
	}

	// Error() should be redacted.
	errMsg := err.Error()
	if strings.Contains(errMsg, "sk-ant-api03-secretkey123") {
		t.Errorf("error message leaks Anthropic key: %s", errMsg)
	}

	// Unwrap should still return a non-nil cause (for errors.As compatibility).
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		if u.Unwrap() == nil {
			t.Error("Unwrap() returned nil, expected non-nil cause")
		}
	} else {
		t.Error("error does not implement Unwrap()")
	}
}

// TestSanitizeConfigError verifies the sanitizeConfigError helper directly.
func TestSanitizeConfigError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // substring that must be present
	}{
		{
			"OpenAI key in error",
			`toml: error on line 3: token = "sk-abc123def456ghi789"`,
			"[secret redacted]",
		},
		{
			"JWT in error",
			`error: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U is invalid`,
			"[secret redacted]",
		},
		{
			"Anthropic key in error",
			`error on line 2: key = "sk-ant-api03-abc123def456"`,
			"[secret redacted]",
		},
		{
			"password in error",
			`error: password=mysecret123 is too short`,
			"[secret redacted]",
		},
		{
			"no secrets",
			"toml: expected newline but got U+006F 'o'",
			"toml: expected newline but got U+006F 'o'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.SanitizeConfigErrorForTest(tt.input)
			if !strings.Contains(got, tt.want) {
				t.Errorf("sanitizeConfigError(%q) = %q, want to contain %q", tt.input, got, tt.want)
			}
		})
	}
}
