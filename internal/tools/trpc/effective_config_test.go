package trpc

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestToolsetConfigFromEffectiveKeys_filesystemGroup(t *testing.T) {
	cfg := ToolsetConfigFromEffectiveKeys(map[string]bool{"read_file": true})
	if !cfg.Filesystem || cfg.ShellExec {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestToolsetConfigFromEffectiveKeys_integrationKeys(t *testing.T) {
	cfg := ToolsetConfigFromEffectiveKeys(map[string]bool{
		biz.ToolKeyKnowledgeSearch: true,
		biz.ToolKeyCallAgent:       true,
	})
	if !cfg.KnowledgeSearch || !cfg.CallAgent {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestToolsetConfigFromEffectiveKeys_allMappings(t *testing.T) {
	tests := []struct {
		key      string
		field    string
		expected bool
	}{
		{"shell_exec", "ShellExec", true},
		{"web_fetch", "WebFetch", true},
		{"duckduckgo_search", "WebSearch", true},
		{"gemini_web_fetch", "GeminiFetch", true},
		{"google_search", "GoogleSearch", true},
		{"arxiv_search", "ArxivSearch", true},
		{"wikipedia_search", "Wikipedia", true},
		{"send_email", "Email", true},
		{"todo_write", "Todo", true},
		{"await_user_reply", "AwaitReply", true},
		{"claude_code", "ClaudeCode", true},
		{"workspace_exec", "WorkspaceExec", true},
		{biz.ToolKeyKanban, "Kanban", true},
		{biz.ToolKeyWebResearch, "WebResearch", true},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			cfg := ToolsetConfigFromEffectiveKeys(map[string]bool{tt.key: true})
			checkField(t, cfg, tt.field, tt.expected)
		})
	}
}

func TestToolsetConfigFromEffectiveKeys_filesystemSubkeys(t *testing.T) {
	subkeys := []string{"read_file", "read_multiple_files", "save_file", "list_file",
		"search_file", "search_content", "replace_content", "diff_edit", "patch_file"}
	for _, k := range subkeys {
		t.Run(k, func(t *testing.T) {
			cfg := ToolsetConfigFromEffectiveKeys(map[string]bool{k: true})
			if !cfg.Filesystem {
				t.Errorf("key %q should enable Filesystem", k)
			}
		})
	}
}

func TestToolsetConfigFromEffectiveKeys_empty(t *testing.T) {
	cfg := ToolsetConfigFromEffectiveKeys(nil)
	if cfg.Filesystem || cfg.ShellExec || cfg.WebFetch {
		t.Fatalf("empty map should produce zero config: %+v", cfg)
	}
}

func TestToolsetConfigHasAny(t *testing.T) {
	tests := []struct {
		name   string
		cfg    ToolsetConfig
		expect bool
	}{
		{"empty", ToolsetConfig{}, false},
		{"filesystem", ToolsetConfig{Filesystem: true}, true},
		{"shell", ToolsetConfig{ShellExec: true}, true},
		{"webfetch", ToolsetConfig{WebFetch: true}, true},
		{"websearch", ToolsetConfig{WebSearch: true}, true},
		{"gemini", ToolsetConfig{GeminiFetch: true}, true},
		{"google", ToolsetConfig{GoogleSearch: true}, true},
		{"arxiv", ToolsetConfig{ArxivSearch: true}, true},
		{"wiki", ToolsetConfig{Wikipedia: true}, true},
		{"email", ToolsetConfig{Email: true}, true},
		{"todo", ToolsetConfig{Todo: true}, true},
		{"await", ToolsetConfig{AwaitReply: true}, true},
		{"claude", ToolsetConfig{ClaudeCode: true}, true},
		{"workspace", ToolsetConfig{WorkspaceExec: true}, true},
		{"knowledge", ToolsetConfig{KnowledgeSearch: true}, true},
		{"reflect", ToolsetConfig{KnowledgeReflect: true}, true},
		{"callagent", ToolsetConfig{CallAgent: true}, true},
		{"kanban", ToolsetConfig{Kanban: true}, true},
		{"memory", ToolsetConfig{MemoryEnabled: true}, true},
		{"webresearch", ToolsetConfig{WebResearch: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToolsetConfigHasAny(tt.cfg)
			if got != tt.expect {
				t.Errorf("ToolsetConfigHasAny(%+v) = %v, want %v", tt.cfg, got, tt.expect)
			}
		})
	}
}

func TestToolsetConfigHasAny_collections(t *testing.T) {
	if !ToolsetConfigHasAny(ToolsetConfig{MCPServers: []MCPServerConfig{{Name: "x"}}}) {
		t.Error("MCPServers should make HasAny true")
	}
	if !ToolsetConfigHasAny(ToolsetConfig{MCPBroker: &MCPBrokerConfig{}}) {
		t.Error("MCPBroker should make HasAny true")
	}
}

func checkField(t *testing.T, cfg ToolsetConfig, field string, expected bool) {
	t.Helper()
	got := fieldByName(cfg, field)
	if got != expected {
		t.Errorf("field %s = %v, want %v", field, got, expected)
	}
}

func fieldByName(cfg ToolsetConfig, name string) bool {
	switch name {
	case "ShellExec":
		return cfg.ShellExec
	case "WebFetch":
		return cfg.WebFetch
	case "WebSearch":
		return cfg.WebSearch
	case "GeminiFetch":
		return cfg.GeminiFetch
	case "GoogleSearch":
		return cfg.GoogleSearch
	case "ArxivSearch":
		return cfg.ArxivSearch
	case "Wikipedia":
		return cfg.Wikipedia
	case "Email":
		return cfg.Email
	case "Todo":
		return cfg.Todo
	case "AwaitReply":
		return cfg.AwaitReply
	case "ClaudeCode":
		return cfg.ClaudeCode
	case "WorkspaceExec":
		return cfg.WorkspaceExec
	case "KnowledgeSearch":
		return cfg.KnowledgeSearch
	case "CallAgent":
		return cfg.CallAgent
	case "Kanban":
		return cfg.Kanban
	case "WebResearch":
		return cfg.WebResearch
	default:
		return false
	}
}
