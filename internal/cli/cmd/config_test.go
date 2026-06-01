package cmd

import (
	"errors"
	"testing"

	"aranea-agents/internal/cli"
	"aranea-agents/internal/cli/config"
)

func newTestCfg() *config.CLIConfig {
	return &config.CLIConfig{
		Backend: config.BackendConfig{
			BaseURL:     "http://localhost:8080",
			Token:       "eyJhbGciOiJIUzI1NiJ9.payload.sig",
			WorkspaceID: "ws-123",
		},
		UI: config.UIConfig{
			Output: "text",
			Color:  "auto",
		},
		Skill: config.SkillConfig{
			DefaultDecision: "ask",
		},
	}
}

func TestGetConfigValue_BackendBaseURL(t *testing.T) {
	cfg := newTestCfg()
	val, err := getConfigValue(cfg, "backend.base_url", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "http://localhost:8080" {
		t.Errorf("got %q, want %q", val, "http://localhost:8080")
	}
}

func TestGetConfigValue_BackendWorkspaceID(t *testing.T) {
	cfg := newTestCfg()
	val, err := getConfigValue(cfg, "backend.workspace_id", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "ws-123" {
		t.Errorf("got %q, want %q", val, "ws-123")
	}
}

func TestGetConfigValue_UIOutput(t *testing.T) {
	cfg := newTestCfg()
	val, err := getConfigValue(cfg, "ui.output", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "text" {
		t.Errorf("got %q, want %q", val, "text")
	}
}

func TestGetConfigValue_UIColor(t *testing.T) {
	cfg := newTestCfg()
	val, err := getConfigValue(cfg, "ui.color", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "auto" {
		t.Errorf("got %q, want %q", val, "auto")
	}
}

func TestGetConfigValue_SkillDefaultDecision(t *testing.T) {
	cfg := newTestCfg()
	val, err := getConfigValue(cfg, "skill.default_decision", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "ask" {
		t.Errorf("got %q, want %q", val, "ask")
	}
}

func TestGetConfigValue_TokenMasked(t *testing.T) {
	cfg := newTestCfg()
	val, err := getConfigValue(cfg, "backend.token", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := config.MaskToken(cfg.Backend.Token)
	if val != want {
		t.Errorf("got %q, want masked %q", val, want)
	}
	if val == cfg.Backend.Token {
		t.Error("token should be masked but was returned in plaintext")
	}
}

func TestGetConfigValue_TokenShowToken(t *testing.T) {
	cfg := newTestCfg()
	val, err := getConfigValue(cfg, "backend.token", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != cfg.Backend.Token {
		t.Errorf("got %q, want plaintext %q", val, cfg.Backend.Token)
	}
}

func TestGetConfigValue_UnknownKey(t *testing.T) {
	cfg := newTestCfg()
	_, err := getConfigValue(cfg, "unknown.key", false)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cli.CLIError, got %T: %v", err, err)
	}
	if ce.Code != "CONFIG_KEY_UNKNOWN" {
		t.Errorf("code: got %q, want %q", ce.Code, "CONFIG_KEY_UNKNOWN")
	}
}

func TestSetConfigValue_BackendBaseURL(t *testing.T) {
	cfg := newTestCfg()
	if err := setConfigValue(cfg, "backend.base_url", "http://new:9090"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Backend.BaseURL != "http://new:9090" {
		t.Errorf("got %q, want %q", cfg.Backend.BaseURL, "http://new:9090")
	}
}

func TestSetConfigValue_BackendToken(t *testing.T) {
	cfg := newTestCfg()
	if err := setConfigValue(cfg, "backend.token", "new-token"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Backend.Token != "new-token" {
		t.Errorf("got %q, want %q", cfg.Backend.Token, "new-token")
	}
}

func TestSetConfigValue_BackendWorkspaceID(t *testing.T) {
	cfg := newTestCfg()
	if err := setConfigValue(cfg, "backend.workspace_id", "ws-456"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Backend.WorkspaceID != "ws-456" {
		t.Errorf("got %q, want %q", cfg.Backend.WorkspaceID, "ws-456")
	}
}

func TestSetConfigValue_UIOutputText(t *testing.T) {
	cfg := newTestCfg()
	cfg.UI.Output = "json"
	if err := setConfigValue(cfg, "ui.output", "text"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UI.Output != "text" {
		t.Errorf("got %q, want %q", cfg.UI.Output, "text")
	}
}

func TestSetConfigValue_UIOutputJSON(t *testing.T) {
	cfg := newTestCfg()
	if err := setConfigValue(cfg, "ui.output", "json"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UI.Output != "json" {
		t.Errorf("got %q, want %q", cfg.UI.Output, "json")
	}
}

func TestSetConfigValue_UIColorAuto(t *testing.T) {
	cfg := newTestCfg()
	cfg.UI.Color = "never"
	if err := setConfigValue(cfg, "ui.color", "auto"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UI.Color != "auto" {
		t.Errorf("got %q, want %q", cfg.UI.Color, "auto")
	}
}

func TestSetConfigValue_UIColorAlways(t *testing.T) {
	cfg := newTestCfg()
	if err := setConfigValue(cfg, "ui.color", "always"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UI.Color != "always" {
		t.Errorf("got %q, want %q", cfg.UI.Color, "always")
	}
}

func TestSetConfigValue_UIColorNever(t *testing.T) {
	cfg := newTestCfg()
	if err := setConfigValue(cfg, "ui.color", "never"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UI.Color != "never" {
		t.Errorf("got %q, want %q", cfg.UI.Color, "never")
	}
}

func TestSetConfigValue_UnknownKey(t *testing.T) {
	cfg := newTestCfg()
	err := setConfigValue(cfg, "unknown.key", "val")
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cli.CLIError, got %T: %v", err, err)
	}
	if ce.Code != "CONFIG_KEY_UNKNOWN" {
		t.Errorf("code: got %q, want %q", ce.Code, "CONFIG_KEY_UNKNOWN")
	}
}

func TestSetConfigValue_InvalidUIOutput(t *testing.T) {
	cfg := newTestCfg()
	err := setConfigValue(cfg, "ui.output", "xml")
	if err == nil {
		t.Fatal("expected error for invalid ui.output, got nil")
	}
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cli.CLIError, got %T: %v", err, err)
	}
	if ce.Code != "CONFIG_VALUE_INVALID" {
		t.Errorf("code: got %q, want %q", ce.Code, "CONFIG_VALUE_INVALID")
	}
}

func TestSetConfigValue_InvalidUIColor(t *testing.T) {
	cfg := newTestCfg()
	err := setConfigValue(cfg, "ui.color", "sometimes")
	if err == nil {
		t.Fatal("expected error for invalid ui.color, got nil")
	}
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cli.CLIError, got %T: %v", err, err)
	}
	if ce.Code != "CONFIG_VALUE_INVALID" {
		t.Errorf("code: got %q, want %q", ce.Code, "CONFIG_VALUE_INVALID")
	}
}
