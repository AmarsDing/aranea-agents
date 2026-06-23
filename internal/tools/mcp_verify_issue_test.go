package tools

import (
	"testing"

	mcpconfig "aranea-agents/internal/mcp/config"
)

// TestMCPEnvPropagation_Regression verifies that the Env field from
// config_json is properly propagated through the full chain:
// config_json → ServerConfig.Env → MCPServerConfig.Env → ConnectionConfig.Env
//
// This is a regression test for a bug where Env was parsed and stored in
// MCPServerConfig but dropped by ToConnectionConfig(), causing stdio MCP
// servers to never receive environment variables.
func TestMCPEnvPropagation_Regression(t *testing.T) {
	configJSON := `{
		"transport": "stdio",
		"command": "npx",
		"args": ["-y", "@modelcontextprotocol/server-github"],
		"env": {"GITHUB_TOKEN": "ghp_secret_token_123", "API_KEY": "sk-xxx"}
	}`

	// Step 1: config_json → ServerConfig
	sc, err := mcpconfig.ParseServerConfigJSON(configJSON)
	if err != nil {
		t.Fatalf("ParseServerConfigJSON failed: %v", err)
	}
	if sc.Env["GITHUB_TOKEN"] != "ghp_secret_token_123" {
		t.Fatalf("Step1: Env.GITHUB_TOKEN = %q, want ghp_secret_token_123", sc.Env["GITHUB_TOKEN"])
	}

	// Step 2: ServerConfig → MCPServerConfig (as resolveMCPServers does)
	cfg := MCPServerConfig{
		Name:      "github",
		Transport: string(sc.Transport),
		Command:   sc.Command,
		Args:      sc.Args,
		Env:       sc.Env,
	}

	// Step 3: MCPServerConfig → ConnectionConfig (the previously broken step)
	connCfg := cfg.ToConnectionConfig()
	if connCfg.Env == nil {
		t.Fatal("Step3: ConnectionConfig.Env is nil — Env was dropped (regression!)")
	}
	if connCfg.Env["GITHUB_TOKEN"] != "ghp_secret_token_123" {
		t.Fatalf("Step3: Env.GITHUB_TOKEN = %q, want ghp_secret_token_123", connCfg.Env["GITHUB_TOKEN"])
	}
	if connCfg.Env["API_KEY"] != "sk-xxx" {
		t.Fatalf("Step3: Env.API_KEY = %q, want sk-xxx", connCfg.Env["API_KEY"])
	}
	if connCfg.Command != "npx" {
		t.Fatalf("Step3: Command = %q, want npx", connCfg.Command)
	}
	if connCfg.Transport != "stdio" {
		t.Fatalf("Step3: Transport = %q, want stdio", connCfg.Transport)
	}
}

// TestMCPEnvPropagation_HTTPNoEnv verifies that HTTP transports don't
// require Env (it's simply ignored for non-stdio transports).
func TestMCPEnvPropagation_HTTPNoEnv(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "remote",
		Transport: "streamable",
		ServerURL: "https://example.com/mcp",
		Env:       map[string]string{"FOO": "bar"}, // Env for HTTP is ignored but shouldn't cause errors
	}
	connCfg := cfg.ToConnectionConfig()
	if connCfg.Transport != "streamable" {
		t.Fatalf("Transport = %q, want streamable", connCfg.Transport)
	}
	// Env is still set in ConnectionConfig (harmless for HTTP, only used by stdio)
	if connCfg.Env["FOO"] != "bar" {
		t.Fatalf("Env.FOO = %q, want bar", connCfg.Env["FOO"])
	}
}
