package trpc

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	webresearchpkg "aranea-agents/internal/tools/webresearch"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestIsFilesystemToolKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"read_file", true},
		{"read_multiple_files", true},
		{"save_file", true},
		{"list_file", true},
		{"search_file", true},
		{"search_content", true},
		{"replace_content", true},
		{"diff_edit", true},
		{"patch_file", true},
		{"read_lints", true},
		{"delete_file", true},
		{"shell_exec", false},
		{"web_fetch", false},
		{"", false},
		{"Read_File", false},
		{"read_file_extra", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isFilesystemToolKey(tt.key); got != tt.want {
				t.Errorf("isFilesystemToolKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestIsGeminiLikeProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     bool
	}{
		{"gemini", "gemini", true},
		{"gemini_upper", "Gemini", true},
		{"google", "google", true},
		{"google_ai", "google-ai", true},
		{"google_spaced", "  google  ", true},
		{"openai", "openai", false},
		{"empty", "", false},
		{"deepseek", "deepseek", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGeminiLikeProvider(tt.provider); got != tt.want {
				t.Errorf("isGeminiLikeProvider(%q) = %v, want %v", tt.provider, got, tt.want)
			}
		})
	}
}

func TestApplyRuntimeConfigMaps_nilConfig(t *testing.T) {
	ApplyRuntimeConfigMaps(nil, map[string]map[string]any{
		"read_file": {"filesystem_dir": "/data"},
	})
}

func TestApplyRuntimeConfigMaps_emptyMap(t *testing.T) {
	cfg := &ToolsetConfig{Filesystem: true}
	ApplyRuntimeConfigMaps(cfg, nil)
	if cfg.FilesystemDir != "" {
		t.Errorf("FilesystemDir should be empty, got %q", cfg.FilesystemDir)
	}
}

func TestApplyRuntimeConfigMaps_emptyToolConfig(t *testing.T) {
	cfg := &ToolsetConfig{}
	ApplyRuntimeConfigMaps(cfg, map[string]map[string]any{
		"read_file": {},
	})
	if cfg.FilesystemDir != "" {
		t.Errorf("FilesystemDir should be empty for empty config map")
	}
}

func TestApplyRuntimeConfigMaps_claudeCodeDir(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want string
	}{
		{"base_dir", map[string]any{"base_dir": "/cc"}, "/cc"},
		{"claude_code_dir", map[string]any{"claude_code_dir": "/cc2"}, "/cc2"},
		{"working_dir", map[string]any{"working_dir": "/cc3"}, "/cc3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ToolsetConfig{}
			ApplyRuntimeConfigMaps(cfg, map[string]map[string]any{
				"claude_code": tt.m,
			})
			if cfg.ClaudeCodeDir != tt.want {
				t.Errorf("ClaudeCodeDir = %q, want %q", cfg.ClaudeCodeDir, tt.want)
			}
		})
	}
}

func TestApplyRuntimeConfigMaps_shellExecDirAliases(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want string
	}{
		{"base_dir", map[string]any{"base_dir": "/sh"}, "/sh"},
		{"shell_root", map[string]any{"shell_root": "/sh2"}, "/sh2"},
		{"filesystem_dir", map[string]any{"filesystem_dir": "/sh3"}, "/sh3"},
		{"working_dir", map[string]any{"working_dir": "/sh4"}, "/sh4"},
		{"root_dir", map[string]any{"root_dir": "/sh5"}, "/sh5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ToolsetConfig{}
			ApplyRuntimeConfigMaps(cfg, map[string]map[string]any{
				"shell_exec": tt.m,
			})
			if cfg.ShellExecDir != tt.want {
				t.Errorf("ShellExecDir = %q, want %q", cfg.ShellExecDir, tt.want)
			}
		})
	}
}

func TestApplyRuntimeConfigMaps_filesystemDirAliases(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want string
	}{
		{"filesystem_dir", map[string]any{"filesystem_dir": "/fs"}, "/fs"},
		{"base_dir", map[string]any{"base_dir": "/fs2"}, "/fs2"},
		{"working_dir", map[string]any{"working_dir": "/fs3"}, "/fs3"},
		{"root_dir", map[string]any{"root_dir": "/fs4"}, "/fs4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ToolsetConfig{}
			ApplyRuntimeConfigMaps(cfg, map[string]map[string]any{
				"read_file": tt.m,
			})
			if cfg.FilesystemDir != tt.want {
				t.Errorf("FilesystemDir = %q, want %q", cfg.FilesystemDir, tt.want)
			}
		})
	}
}

func TestApplyRuntimeConfigMaps_googleSearchAliases(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		cx   string
	}{
		{"api_key", map[string]any{"api_key": "k1"}, "k1", ""},
		{"google_api_key", map[string]any{"google_api_key": "k2"}, "k2", ""},
		{"cx", map[string]any{"api_key": "k", "cx": "c1"}, "k", "c1"},
		{"engine_id", map[string]any{"api_key": "k", "engine_id": "c2"}, "k", "c2"},
		{"google_cx", map[string]any{"api_key": "k", "google_cx": "c3"}, "k", "c3"},
		{"search_engine_id", map[string]any{"api_key": "k", "search_engine_id": "c4"}, "k", "c4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ToolsetConfig{}
			ApplyRuntimeConfigMaps(cfg, map[string]map[string]any{
				"google_search": tt.m,
			})
			if cfg.GoogleAPIKey != tt.key {
				t.Errorf("GoogleAPIKey = %q, want %q", cfg.GoogleAPIKey, tt.key)
			}
			if cfg.GoogleCX != tt.cx {
				t.Errorf("GoogleCX = %q, want %q", cfg.GoogleCX, tt.cx)
			}
		})
	}
}

func TestApplyRuntimeConfigMaps_geminiModelAliases(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want string
	}{
		{"model", map[string]any{"model": "gemini-2.0"}, "gemini-2.0"},
		{"gemini_model", map[string]any{"gemini_model": "gemini-2.5"}, "gemini-2.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ToolsetConfig{}
			ApplyRuntimeConfigMaps(cfg, map[string]map[string]any{
				"gemini_web_fetch": tt.m,
			})
			if cfg.GeminiModel != tt.want {
				t.Errorf("GeminiModel = %q, want %q", cfg.GeminiModel, tt.want)
			}
		})
	}
}

func TestApplyRuntimeConfigMaps_unknownToolKey(t *testing.T) {
	cfg := &ToolsetConfig{}
	ApplyRuntimeConfigMaps(cfg, map[string]map[string]any{
		"unknown_tool": {"some_key": "some_value"},
	})
	if cfg.FilesystemDir != "" || cfg.GoogleAPIKey != "" || cfg.GeminiModel != "" {
		t.Error("unknown tool key should not modify any config field")
	}
}

func TestResolveGeminiFetchModel_nilConfig(t *testing.T) {
	ResolveGeminiFetchModel(nil, "google", "gemini-2.5-flash")
}

func TestResolveGeminiFetchModel_notEnabled(t *testing.T) {
	cfg := &ToolsetConfig{GeminiFetch: false}
	ResolveGeminiFetchModel(cfg, "google", "gemini-2.5-flash")
	if cfg.GeminiModel != "" {
		t.Errorf("GeminiModel should remain empty when GeminiFetch is disabled")
	}
}

func TestResolveGeminiFetchModel_nonGeminiProvider(t *testing.T) {
	cfg := &ToolsetConfig{GeminiFetch: true}
	ResolveGeminiFetchModel(cfg, "openai", "gpt-4")
	if cfg.GeminiModel != "" {
		t.Errorf("GeminiModel should remain empty for non-Gemini provider")
	}
}

func TestResolveGeminiFetchModel_emptyModel(t *testing.T) {
	cfg := &ToolsetConfig{GeminiFetch: true}
	ResolveGeminiFetchModel(cfg, "google", "")
	if cfg.GeminiModel != "" {
		t.Errorf("GeminiModel should remain empty when agent model is empty")
	}
}

func TestResolveGeminiFetchModel_whitespaceModel(t *testing.T) {
	cfg := &ToolsetConfig{GeminiFetch: true}
	ResolveGeminiFetchModel(cfg, "google", "   ")
	if cfg.GeminiModel != "" {
		t.Errorf("GeminiModel should remain empty when agent model is whitespace")
	}
}

func TestPruneUnconfiguredToolFlags_nilConfig(t *testing.T) {
	skipped := PruneUnconfiguredToolFlags(nil)
	if len(skipped) != 0 {
		t.Errorf("nil config should return nil, got %v", skipped)
	}
}

func TestPruneUnconfiguredToolFlags_emptyConfig(t *testing.T) {
	cfg := &ToolsetConfig{}
	skipped := PruneUnconfiguredToolFlags(cfg)
	if len(skipped) != 0 {
		t.Errorf("empty config should skip nothing, got %v", skipped)
	}
}

func TestPruneUnconfiguredToolFlags_webResearchNotReady(t *testing.T) {
	cfg := &ToolsetConfig{WebResearch: true, WebResearchCfg: webresearchpkg.Config{}}
	skipped := PruneUnconfiguredToolFlags(cfg)
	if cfg.WebResearch {
		t.Error("WebResearch should be pruned when config not ready")
	}
	found := false
	for _, s := range skipped {
		if s == "web_research" {
			found = true
		}
	}
	if !found {
		t.Errorf("web_research should be in skipped list, got %v", skipped)
	}
}

func TestPruneUnconfiguredToolFlags_geminiWhitespaceModel(t *testing.T) {
	cfg := &ToolsetConfig{GeminiFetch: true, GeminiModel: "   "}
	skipped := PruneUnconfiguredToolFlags(cfg)
	if cfg.GeminiFetch {
		t.Error("GeminiFetch should be pruned when model is whitespace")
	}
	if len(skipped) != 1 || skipped[0] != "gemini_web_fetch" {
		t.Errorf("skipped = %v, want [gemini_web_fetch]", skipped)
	}
}

func TestPruneUnconfiguredToolFlags_googleMissingCX(t *testing.T) {
	cfg := &ToolsetConfig{GoogleSearch: true, GoogleAPIKey: "key", GoogleCX: ""}
	PruneUnconfiguredToolFlags(cfg)
	if cfg.GoogleSearch {
		t.Error("GoogleSearch should be pruned when CX is missing")
	}
}

func TestPruneUnconfiguredToolFlags_googleMissingAPIKey(t *testing.T) {
	cfg := &ToolsetConfig{GoogleSearch: true, GoogleAPIKey: "", GoogleCX: "cx"}
	PruneUnconfiguredToolFlags(cfg)
	if cfg.GoogleSearch {
		t.Error("GoogleSearch should be pruned when API key is missing")
	}
}

func TestPruneUnconfiguredToolFlags_allPruned(t *testing.T) {
	cfg := &ToolsetConfig{
		GeminiFetch:  true,
		GoogleSearch: true,
		WebResearch:  true,
	}
	skipped := PruneUnconfiguredToolFlags(cfg)
	if cfg.GeminiFetch || cfg.GoogleSearch || cfg.WebResearch {
		t.Error("all unconfigured tools should be pruned")
	}
	if len(skipped) != 3 {
		t.Errorf("expected 3 skipped, got %d: %v", len(skipped), skipped)
	}
}

func TestPatchConfirmationDeclaration_nil(t *testing.T) {
	if got := patchConfirmationDeclaration(nil); got != nil {
		t.Error("nil input should return nil")
	}
}

func TestPatchConfirmationDeclaration_emptyDesc(t *testing.T) {
	d := &trpctool.Declaration{Name: "test", Description: ""}
	result := patchConfirmationDeclaration(d)
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if !strings.Contains(result.Description, "Requires explicit user approval") {
		t.Errorf("Description = %q, should contain approval text", result.Description)
	}
}

func TestPatchConfirmationDeclaration_existingDesc(t *testing.T) {
	d := &trpctool.Declaration{Name: "test", Description: "run shell commands"}
	result := patchConfirmationDeclaration(d)
	if !strings.HasPrefix(result.Description, "run shell commands") {
		t.Errorf("Description should start with original, got %q", result.Description)
	}
	if !strings.Contains(result.Description, "Requires explicit user approval") {
		t.Errorf("Description should contain approval suffix, got %q", result.Description)
	}
}

func TestPatchConfirmationDeclaration_alreadyHasApproval(t *testing.T) {
	desc := "run shell commands\n\n[Requires explicit user approval before execution.]"
	d := &trpctool.Declaration{Name: "test", Description: desc}
	result := patchConfirmationDeclaration(d)
	if result.Description != desc {
		t.Errorf("Description should not be double-appended, got %q", result.Description)
	}
}

func TestPatchConfirmationDeclaration_doesNotMutateOriginal(t *testing.T) {
	d := &trpctool.Declaration{Name: "test", Description: "original"}
	result := patchConfirmationDeclaration(d)
	if d.Description != "original" {
		t.Error("original declaration should not be mutated")
	}
	_ = result
}

func TestToolKeyFromTool_nil(t *testing.T) {
	if got := toolKeyFromTool(nil); got != "" {
		t.Errorf("nil tool should return empty string, got %q", got)
	}
}

func TestToolKeyFromTool_noDeclaration(t *testing.T) {
	tool := stubToolNoDecl{}
	if got := toolKeyFromTool(tool); got != "" {
		t.Errorf("tool with nil declaration should return empty string, got %q", got)
	}
}

func TestToolKeyFromTool_withDeclaration(t *testing.T) {
	tool := stubTool{name: "shell_exec"}
	if got := toolKeyFromTool(tool); got != "shell_exec" {
		t.Errorf("got %q, want %q", got, "shell_exec")
	}
}

func TestToolKeyFromTool_trimsName(t *testing.T) {
	tool := stubTool{name: "  shell_exec  "}
	if got := toolKeyFromTool(tool); got != "shell_exec" {
		t.Errorf("got %q, want %q", got, "shell_exec")
	}
}

func TestPlatformFieldsFromBiz_fieldMapping(t *testing.T) {
	tests := []struct {
		name     string
		platform biz.WebResearchSetting
		want     webresearchpkg.PlatformFields
	}{
		{
			name: "all_fields_mapped",
			platform: biz.WebResearchSetting{
				HasAPIKey:   true,
				APIKey:      "  key  ",
				Provider:    "  tavily  ",
				MaxResults:  10,
				FetchTop:    5,
				SearchDepth: "  advanced  ",
				TimeoutSec:  30,
				HTTPProxy:   "  http://proxy  ",
			},
			want: webresearchpkg.PlatformFields{
				HasAPIKey:   true,
				APIKey:      "key",
				Provider:    "tavily",
				MaxResults:  10,
				FetchTop:    5,
				SearchDepth: "advanced",
				TimeoutSec:  30,
				HTTPProxy:   "http://proxy",
			},
		},
		{
			name:     "empty_fields",
			platform: biz.WebResearchSetting{},
			want:     webresearchpkg.PlatformFields{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlatformFieldsFromBiz(tt.platform)
			if got != tt.want {
				t.Errorf("PlatformFieldsFromBiz() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

type stubToolNoDecl struct{}

func (stubToolNoDecl) Declaration() *trpctool.Declaration { return nil }

func TestApplyRuntimeConfigMaps_webResearchKey(t *testing.T) {
	cfg := &ToolsetConfig{}
	ApplyRuntimeConfigMaps(cfg, map[string]map[string]any{
		biz.ToolKeyWebResearch: {"provider": "tavily", "api_key": "tvly-test"},
	})
	if cfg.WebResearchCfg.Provider != "tavily" {
		t.Errorf("WebResearchCfg.Provider = %q, want %q", cfg.WebResearchCfg.Provider, "tavily")
	}
	if cfg.WebResearchCfg.APIKey != "tvly-test" {
		t.Errorf("WebResearchCfg.APIKey = %q, want %q", cfg.WebResearchCfg.APIKey, "tvly-test")
	}
}

func TestApplyRuntimeConfigMaps_webResearchEmptyConfig(t *testing.T) {
	cfg := &ToolsetConfig{}
	ApplyRuntimeConfigMaps(cfg, map[string]map[string]any{
		biz.ToolKeyWebResearch: {},
	})
	if cfg.WebResearchCfg.Provider != "" {
		t.Errorf("empty config with no env vars should have empty Provider, got %q", cfg.WebResearchCfg.Provider)
	}
}

func TestWrapToolDeclaration_NilTool(t *testing.T) {
	result := wrapToolDeclaration(nil, true)
	if result != nil {
		t.Fatal("nil tool should return nil")
	}
}

func TestWrapToolDeclaration_NotRequired(t *testing.T) {
	original := stubTool{name: "test", desc: "desc"}
	result := wrapToolDeclaration(original, false)
	if result != original {
		t.Fatal("non-required tool should be returned as-is")
	}
}

func TestConfirmingToolSet_NilInnerName(t *testing.T) {
	s := confirmingToolSet{inner: nil}
	if got := s.Name(); got != "" {
		t.Fatalf("Name() = %q, want empty string for nil inner", got)
	}
}

func TestConfirmingToolSet_NilInnerClose(t *testing.T) {
	s := confirmingToolSet{inner: nil}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil for nil inner", err)
	}
}

func TestConfirmingToolSet_NilInnerTools(t *testing.T) {
	s := confirmingToolSet{inner: nil}
	if got := s.Tools(context.Background()); got != nil {
		t.Fatalf("Tools() = %v, want nil for nil inner", got)
	}
}
