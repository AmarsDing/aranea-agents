package config

import (
	"fmt"
	"os"
	"path/filepath"

	"aranea-agents/internal/tools/preview"

	"github.com/pelletier/go-toml/v2"
)

// DefaultAgentKey is the default agent key used by the CLI chat command.
// This mirrors biz.SystemAdminAgentKey to avoid CLI importing internal/biz.
const DefaultAgentKey = "__system_admin__"

// CLIConfig is the root configuration struct for the Aranea CLI.
type CLIConfig struct {
	Backend   BackendConfig   `toml:"backend"`
	UI        UIConfig        `toml:"ui"`
	Skill     SkillConfig     `toml:"skill"`
	Chat      ChatConfig      `toml:"chat"`
	Telemetry TelemetryConfig `toml:"telemetry"`

	// path is where this config was loaded from; not serialized.
	path string
}

// BackendConfig holds connection settings.
type BackendConfig struct {
	BaseURL     string `toml:"base_url"`
	Token       string `toml:"token"`
	WorkspaceID string `toml:"workspace_id"` // reserved, not yet used
}

// UIConfig controls terminal output behaviour.
type UIConfig struct {
	Output string `toml:"output"` // text | json
	Color  string `toml:"color"`  // auto | always | never
}

// SkillConfig controls skill install behaviour.
type SkillConfig struct {
	DefaultDecision string `toml:"default_decision"` // ask|skip|keep|refine
	MaxZipMB        int    `toml:"max_zip_mb"`
	KeepTemp        bool   `toml:"keep_temp"`
}

// ChatConfig controls REPL / chat defaults (P1).
type ChatConfig struct {
	DefaultAgent string `toml:"default_agent"`
	AutoResume   bool   `toml:"auto_resume"`
}

// TelemetryConfig controls telemetry opt-in.
type TelemetryConfig struct {
	Enabled bool `toml:"enabled"`
}

// defaults returns a CLIConfig pre-filled with sensible defaults.
func defaults() *CLIConfig {
	return &CLIConfig{
		Backend: BackendConfig{
			BaseURL: "http://127.0.0.1:8080",
		},
		UI: UIConfig{
			Output: "text",
			Color:  "auto",
		},
		Skill: SkillConfig{
			DefaultDecision: "ask",
			MaxZipMB:        100,
		},
		Chat: ChatConfig{
			DefaultAgent: DefaultAgentKey,
			AutoResume:   true,
		},
	}
}

// Load reads the TOML config at path (or DefaultPath() if empty).
// If the file does not exist, a default config is returned without error.
func Load(path string) (*CLIConfig, error) {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return defaults(), nil
		}
	}

	cfg := defaults()
	cfg.path = path

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, &loadError{path: path, cause: err}
	}

	// Check permissions before exposing token.
	if permErr := EnsureSecurePerm(path); permErr != nil {
		// Return config but zero out token, return the permission error.
		if err2 := toml.Unmarshal(data, cfg); err2 == nil {
			cfg.Backend.Token = ""
		}
		return cfg, permErr
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, &configInvalidError{path: path, cause: err}
	}
	return cfg, nil
}

// Save writes the config to path (or the path it was loaded from if empty)
// and corrects the file permissions to 0600.
func (c *CLIConfig) Save(path string) error {
	if path == "" {
		if c.path != "" {
			path = c.path
		} else {
			var err error
			path, err = DefaultPath()
			if err != nil {
				return err
			}
		}
	}
	c.path = path

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("cannot create config dir: %w", err)
	}

	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("cannot marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("cannot write config: %w", err)
	}

	// Best-effort chmod (no-op on Windows if WriteFile already sets perms).
	if err := FixPerm(path); err != nil {
		// Non-fatal on Windows.
		fmt.Fprintf(os.Stderr, "warning: cannot set config file permissions to 0600: %v\n", err)
	}
	return nil
}

// Path returns the file path this config was loaded from.
func (c *CLIConfig) Path() string { return c.path }

// MustPath returns the default config path or panics.
func MustPath() string {
	p, err := DefaultPath()
	if err != nil {
		return "<unknown>"
	}
	return p
}

// OverrideFromEnv applies ARANEA_* environment variables on top of the loaded config.
func (c *CLIConfig) OverrideFromEnv() {
	if v := os.Getenv("ARANEA_BASE_URL"); v != "" {
		c.Backend.BaseURL = v
	}
	if v := os.Getenv("ARANEA_TOKEN"); v != "" {
		c.Backend.Token = v
	}
	if v := os.Getenv("ARANEA_OUTPUT"); v != "" {
		c.UI.Output = v
	}
	if v := os.Getenv("NO_COLOR"); v != "" {
		c.UI.Color = "never"
	}
	if v := os.Getenv("ARANEA_NO_COLOR"); v != "" && (v == "1" || v == "true") {
		c.UI.Color = "never"
	}
}

// OverrideFromFlags applies CLI flag values on top of env + file config.
// Empty strings are ignored (flags not set).
func (c *CLIConfig) OverrideFromFlags(baseURL, token, outputFmt string, noColor bool) {
	if baseURL != "" {
		c.Backend.BaseURL = baseURL
	}
	if token != "" {
		c.Backend.Token = token
	}
	if outputFmt != "" {
		c.UI.Output = outputFmt
	}
	if noColor {
		c.UI.Color = "never"
	}
}

// loadError wraps a file read error.
type loadError struct {
	path  string
	cause error
}

func (e *loadError) Error() string {
	return fmt.Sprintf("cannot read config %s: %v", e.path, e.cause)
}
func (e *loadError) Unwrap() error { return e.cause }

// configInvalidError wraps a parse error.
type configInvalidError struct {
	path  string
	cause error
}

func (e *configInvalidError) Error() string {
	return fmt.Sprintf("CONFIG_INVALID: cannot parse %s: %s", e.path, sanitizeConfigError(e.cause.Error()))
}
func (e *configInvalidError) Unwrap() error { return e.cause }

// sanitizeConfigError redacts sensitive values from configuration error
// messages. TOML parse errors may echo source lines containing API keys or
// tokens; this function ensures they are redacted before display.
func sanitizeConfigError(msg string) string {
	return preview.RedactAndTruncate(msg, 0)
}

// SanitizeConfigErrorForTest exposes sanitizeConfigError for external tests.
func SanitizeConfigErrorForTest(msg string) string {
	return sanitizeConfigError(msg)
}
