package biz_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestMustJSON(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  string
	}{
		{"nil slice", nil, "[]"},
		{"empty slice", []string{}, "[]"},
		{"single", []string{"a"}, `["a"]`},
		{"multiple", []string{"a", "b"}, `["a","b"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.MustJSON(tc.input)
			if got != tc.want {
				t.Fatalf("MustJSON(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestJsonList(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty array", "[]", []string{}},
		{"single element", `["x"]`, []string{"x"}},
		{"multiple elements", `["a","b","c"]`, []string{"a", "b", "c"}},
		{"invalid json", "not json", []string{}},
		{"empty string", "", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.JsonList(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("JsonList(%q) = %v, want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("JsonList(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestStrFallback(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		fallback  string
		want      string
	}{
		{"non-empty value", "hello", "default", "hello"},
		{"empty value", "", "default", "default"},
		{"whitespace value", "  ", "default", "default"},
		{"both empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.StrFallback(tc.value, tc.fallback)
			if got != tc.want {
				t.Fatalf("StrFallback(%q, %q) = %q, want %q", tc.value, tc.fallback, got, tc.want)
			}
		})
	}
}

func TestWithSettingDefaults(t *testing.T) {
	t.Run("zero settings get defaults", func(t *testing.T) {
		v := biz.AgentRuntimeSettings{}
		got := biz.WithSettingDefaults(v)
		d := biz.DefaultAgentRuntimeSettings()
		if got.ToolsProfile != d.ToolsProfile {
			t.Fatalf("ToolsProfile = %q, want %q", got.ToolsProfile, d.ToolsProfile)
		}
		if got.SubagentsMaxConcurrency != d.SubagentsMaxConcurrency {
			t.Fatalf("SubagentsMaxConcurrency = %d, want %d", got.SubagentsMaxConcurrency, d.SubagentsMaxConcurrency)
		}
		if got.MemoryMinScore != d.MemoryMinScore {
			t.Fatalf("MemoryMinScore = %f, want %f", got.MemoryMinScore, d.MemoryMinScore)
		}
	})

	t.Run("non-zero values preserved", func(t *testing.T) {
		v := biz.DefaultAgentRuntimeSettings()
		v.SubagentsMaxConcurrency = 99
		v.ToolsProfile = "custom"
		got := biz.WithSettingDefaults(v)
		if got.SubagentsMaxConcurrency != 99 {
			t.Fatalf("SubagentsMaxConcurrency = %d, want 99", got.SubagentsMaxConcurrency)
		}
		if got.ToolsProfile != "custom" {
			t.Fatalf("ToolsProfile = %q, want %q", got.ToolsProfile, "custom")
		}
	})

	t.Run("SelfEvolve propagates to EvolutionSelfEvolve", func(t *testing.T) {
		v := biz.AgentRuntimeSettings{SelfEvolve: true}
		got := biz.WithSettingDefaults(v)
		if !got.EvolutionSelfEvolve {
			t.Fatalf("EvolutionSelfEvolve should be true when SelfEvolve=true")
		}
	})

	t.Run("SelfEvolve propagates to EvolutionSelfEvolve when not set", func(t *testing.T) {
		v := biz.AgentRuntimeSettings{SelfEvolve: true, EvolutionSelfEvolve: false}
		got := biz.WithSettingDefaults(v)
		if !got.EvolutionSelfEvolve {
			t.Fatalf("EvolutionSelfEvolve should be propagated from SelfEvolve when false")
		}
	})
}

func TestSettingsFromLegacyConfig(t *testing.T) {
	t.Run("invalid json returns defaults", func(t *testing.T) {
		got := biz.SettingsFromLegacyConfig("not json")
		d := biz.DefaultAgentRuntimeSettings()
		if got.ToolsProfile != d.ToolsProfile {
			t.Fatalf("expected default ToolsProfile, got %q", got.ToolsProfile)
		}
	})

	t.Run("empty string returns defaults", func(t *testing.T) {
		got := biz.SettingsFromLegacyConfig("")
		d := biz.DefaultAgentRuntimeSettings()
		if got.ToolsProfile != d.ToolsProfile {
			t.Fatalf("expected default ToolsProfile, got %q", got.ToolsProfile)
		}
	})

	t.Run("partial config overrides defaults", func(t *testing.T) {
		raw := `{"self_evolve":false,"subagents":{"enabled":true,"max_concurrency":5},"tools":{"profile":"minimal"},"memory":{"max_results":10},"heartbeat":{"interval_minutes":15}}`
		got := biz.SettingsFromLegacyConfig(raw)
		if got.SelfEvolve {
			t.Fatalf("SelfEvolve should be false")
		}
		if !got.SubagentsEnabled {
			t.Fatalf("SubagentsEnabled should be true")
		}
		if got.SubagentsMaxConcurrency != 5 {
			t.Fatalf("SubagentsMaxConcurrency = %d, want 5", got.SubagentsMaxConcurrency)
		}
		if got.ToolsProfile != "minimal" {
			t.Fatalf("ToolsProfile = %q, want %q", got.ToolsProfile, "minimal")
		}
		if got.MemoryMaxResults != 10 {
			t.Fatalf("MemoryMaxResults = %d, want 10", got.MemoryMaxResults)
		}
		if got.HeartbeatIntervalMinutes != 15 {
			t.Fatalf("HeartbeatIntervalMinutes = %d, want 15", got.HeartbeatIntervalMinutes)
		}
	})

	t.Run("tools allow/deny arrays", func(t *testing.T) {
		raw := `{"tools":{"allow":["tool1","tool2"],"deny":["tool3"]}}`
		got := biz.SettingsFromLegacyConfig(raw)
		allow := biz.JsonList(got.ToolsAllowJSON)
		if len(allow) != 2 || allow[0] != "tool1" || allow[1] != "tool2" {
			t.Fatalf("ToolsAllowJSON parsed = %v, want [tool1 tool2]", allow)
		}
		deny := biz.JsonList(got.ToolsDenyJSON)
		if len(deny) != 1 || deny[0] != "tool3" {
			t.Fatalf("ToolsDenyJSON parsed = %v, want [tool3]", deny)
		}
	})

	t.Run("evolution fields", func(t *testing.T) {
		raw := `{"evolution":{"self_evolve":true,"skill_evolve":false,"evolution_metrics_enabled":true,"evolution_suggestions_enabled":false}}`
		got := biz.SettingsFromLegacyConfig(raw)
		if !got.EvolutionSelfEvolve {
			t.Fatalf("EvolutionSelfEvolve should be true")
		}
		if got.EvolutionSkillEvolve {
			t.Fatalf("EvolutionSkillEvolve should be false")
		}
		if !got.EvolutionMetricsEnabled {
			t.Fatalf("EvolutionMetricsEnabled should be true")
		}
		if got.EvolutionSuggestionsEnabled {
			t.Fatalf("EvolutionSuggestionsEnabled should be false")
		}
	})
}

func TestFilesFromLegacyConfig(t *testing.T) {
	t.Run("invalid json returns nil", func(t *testing.T) {
		got := biz.FilesFromLegacyConfig("not json")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("valid config with files", func(t *testing.T) {
		raw := `{"files":[{"name":"AGENTS_CORE.md","body":"core content"},{"name":"RULE.md","body":"rules"}]}`
		got := biz.FilesFromLegacyConfig(raw)
		if len(got) != 2 {
			t.Fatalf("expected 2 files, got %d", len(got))
		}
		if got[0].Name != "AGENTS_CORE.md" {
			t.Fatalf("first file name = %q, want AGENTS_CORE.md", got[0].Name)
		}
		if got[0].SortOrder != 10 {
			t.Fatalf("first file SortOrder = %d, want 10", got[0].SortOrder)
		}
		if got[1].SortOrder != 20 {
			t.Fatalf("second file SortOrder = %d, want 20", got[1].SortOrder)
		}
	})
}

func TestWithFileDefaults(t *testing.T) {
	cases := []struct {
		name  string
		input []biz.AgentPromptFile
		want  int
	}{
		{"empty name filtered", []biz.AgentPromptFile{{Name: "", Body: "x"}}, 0},
		{"whitespace name filtered", []biz.AgentPromptFile{{Name: "  ", Body: "x"}}, 0},
		{"valid file gets auto sort", []biz.AgentPromptFile{{Name: "test.md", Body: "x"}}, 1},
		{"explicit sort preserved", []biz.AgentPromptFile{{Name: "test.md", Body: "x", SortOrder: 50}}, 1},
		{"mixed", []biz.AgentPromptFile{{Name: "", Body: "x"}, {Name: "a.md", Body: "a"}, {Name: "b.md", Body: "b", SortOrder: 99}}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.WithFileDefaults(tc.input)
			if len(got) != tc.want {
				t.Fatalf("WithFileDefaults returned %d files, want %d", len(got), tc.want)
			}
			for _, f := range got {
				if f.SortOrder == 0 {
					t.Fatalf("file %q has SortOrder=0, should have been set", f.Name)
				}
			}
		})
	}
}

func TestDefaultPromptFilesV2(t *testing.T) {
	files := biz.DefaultPromptFilesV2()
	if len(files) != 5 {
		t.Fatalf("V2 should have 5 files, got %d", len(files))
	}
	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
		if f.SortOrder == 0 {
			t.Fatalf("file %q has SortOrder=0", f.Name)
		}
	}
	for _, required := range []string{"AGENTS_CORE.md", "AGENTS_TASK.md", "IDENTITY.md", "CAPABILITIES.md", "RULE.md"} {
		if !names[required] {
			t.Fatalf("V2 missing required file %q", required)
		}
	}
}

func TestDefaultPromptFilesLegacy(t *testing.T) {
	files := biz.DefaultPromptFilesLegacy()
	if len(files) != 9 {
		t.Fatalf("Legacy should have 9 files, got %d", len(files))
	}
}

func TestConfigJSONFromSettings(t *testing.T) {
	t.Run("round-trip with default settings", func(t *testing.T) {
		s := biz.DefaultAgentRuntimeSettings()
		result, err := biz.ConfigJSONFromSettings(s, nil)
		if err != nil {
			t.Fatalf("ConfigJSONFromSettings error: %v", err)
		}
		var parsed map[string]any
		if json.Unmarshal([]byte(result), &parsed) != nil {
			t.Fatalf("result is not valid JSON")
		}
		if parsed["self_evolve"] == nil {
			t.Fatalf("missing self_evolve in output")
		}
	})

	t.Run("includes files", func(t *testing.T) {
		s := biz.DefaultAgentRuntimeSettings()
		files := []biz.AgentPromptFile{{Name: "test.md", Body: "hello", SortOrder: 10}}
		result, err := biz.ConfigJSONFromSettings(s, files)
		if err != nil {
			t.Fatalf("ConfigJSONFromSettings error: %v", err)
		}
		if !strings.Contains(result, "test.md") {
			t.Fatalf("result should contain file name test.md")
		}
	})
}

func TestFilesForMode(t *testing.T) {
	files := []biz.AgentPromptFile{
		{Name: "AGENTS_CORE.md", Body: "core"},
		{Name: "AGENTS_TASK.md", Body: "task"},
		{Name: "IDENTITY.md", Body: "identity"},
		{Name: "CAPABILITIES.md", Body: "caps"},
		{Name: "RULE.md", Body: "rule"},
		{Name: "HEARTBEAT.md", Body: "heartbeat"},
	}

	cases := []struct {
		mode string
		want int
	}{
		{"complete", 6},
		{"", 6},
		{"COMPLETE", 6},
		{"task", 5},
		{"minimized", 2},
		{"none", 0},
		{"unknown", 2},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			got := biz.FilesForMode(files, tc.mode)
			if len(got) != tc.want {
				t.Fatalf("FilesForMode(%q) = %d files, want %d", tc.mode, len(got), tc.want)
			}
		})
	}

	t.Run("task mode excludes HEARTBEAT", func(t *testing.T) {
		got := biz.FilesForMode(files, "task")
		for _, f := range got {
			if f.Name == "HEARTBEAT.md" {
				t.Fatalf("task mode should not include HEARTBEAT.md")
			}
		}
	})

	t.Run("minimized only AGENTS_CORE and RULE", func(t *testing.T) {
		got := biz.FilesForMode(files, "minimized")
		for _, f := range got {
			if f.Name != "AGENTS_CORE.md" && f.Name != "RULE.md" {
				t.Fatalf("minimized mode should only have AGENTS_CORE.md and RULE.md, got %q", f.Name)
			}
		}
	})
}

func TestComposePromptPreview(t *testing.T) {
	t.Run("none mode returns no files", func(t *testing.T) {
		agent := biz.Agent{
			DisplayName: "TestAgent",
			AgentKey:    "test",
			Provider:    "openrouter",
			Model:       "gpt-4",
			Files: []biz.AgentPromptFile{
				{Name: "AGENTS_CORE.md", Body: "core"},
			},
		}
		got := biz.ComposePromptPreview(agent, "none")
		if strings.Contains(got, "core") {
			t.Fatalf("none mode should not include file content")
		}
		if !strings.Contains(got, "TestAgent") {
			t.Fatalf("should contain agent name")
		}
	})

	t.Run("with description", func(t *testing.T) {
		agent := biz.Agent{
			DisplayName:      "TestAgent",
			AgentKey:         "test",
			Provider:         "openrouter",
			Model:            "gpt-4",
			AgentDescription: "A test agent",
		}
		got := biz.ComposePromptPreview(agent, "none")
		if !strings.Contains(got, "A test agent") {
			t.Fatalf("should contain agent description")
		}
	})

	t.Run("empty description uses fallback", func(t *testing.T) {
		agent := biz.Agent{
			DisplayName: "TestAgent",
			AgentKey:    "test",
			Provider:    "openrouter",
			Model:       "gpt-4",
		}
		got := biz.ComposePromptPreview(agent, "none")
		if !strings.Contains(got, "未配置描述") {
			t.Fatalf("should contain fallback description")
		}
	})

	t.Run("with category responsibility preview", func(t *testing.T) {
		agent := biz.Agent{
			DisplayName:                    "TestAgent",
			AgentKey:                       "test",
			Provider:                       "openrouter",
			Model:                          "gpt-4",
			CategoryResponsibilityPreview:  "You are a senior engineer",
		}
		got := biz.ComposePromptPreview(agent, "none")
		if !strings.Contains(got, "角色职责") {
			t.Fatalf("should contain Role Responsibility section")
		}
		if !strings.Contains(got, "senior engineer") {
			t.Fatalf("should contain category responsibility content")
		}
	})
}

func TestEstimateTokensForFiles(t *testing.T) {
	cases := []struct {
		name       string
		files      []biz.AgentPromptFile
		wantTokens int
	}{
		{"empty", nil, 0},
		{"single short file", []biz.AgentPromptFile{{Name: "a.md", Body: "hi"}}, 1},
		{"4 chars = 1 token", []biz.AgentPromptFile{{Name: "a.md", Body: "abcd"}}, 1},
		{"8 chars = 2 tokens", []biz.AgentPromptFile{{Name: "a.md", Body: "abcdefgh"}}, 2},
		{"multiple files", []biz.AgentPromptFile{
			{Name: "a.md", Body: strings.Repeat("x", 100)},
			{Name: "b.md", Body: strings.Repeat("y", 200)},
		}, 25 + 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.EstimateTokensForFiles(tc.files)
			if got.TotalTokens != tc.wantTokens {
				t.Fatalf("TotalTokens = %d, want %d", got.TotalTokens, tc.wantTokens)
			}
			if len(got.FileEstimates) != len(tc.files) {
				t.Fatalf("FileEstimates count = %d, want %d", len(got.FileEstimates), len(tc.files))
			}
		})
	}
}

func TestSettingsFromAgentInput(t *testing.T) {
	t.Run("with Settings uses withSettingDefaults", func(t *testing.T) {
		s := &biz.AgentRuntimeSettings{ToolsProfile: "custom"}
		agent := biz.Agent{Settings: s}
		got := biz.SettingsFromAgentInput(agent)
		if got.ToolsProfile != "custom" {
			t.Fatalf("ToolsProfile = %q, want custom", got.ToolsProfile)
		}
	})

	t.Run("without Settings uses legacy config", func(t *testing.T) {
		agent := biz.Agent{ConfigJSON: `{"tools":{"profile":"legacy"}}`}
		got := biz.SettingsFromAgentInput(agent)
		if got.ToolsProfile != "legacy" {
			t.Fatalf("ToolsProfile = %q, want legacy", got.ToolsProfile)
		}
	})
}

func TestFilesFromAgentInput(t *testing.T) {
	t.Run("with Files uses withFileDefaults", func(t *testing.T) {
		agent := biz.Agent{
			Files: []biz.AgentPromptFile{{Name: "custom.md", Body: "hello"}},
		}
		got := biz.FilesFromAgentInput(agent)
		if len(got) != 1 || got[0].Name != "custom.md" {
			t.Fatalf("expected 1 file custom.md, got %v", got)
		}
	})

	t.Run("without Files and empty config uses defaults", func(t *testing.T) {
		origVal := os.Getenv("PGO_DEFAULT_FILES_V2")
		os.Setenv("PGO_DEFAULT_FILES_V2", "1")
		defer os.Setenv("PGO_DEFAULT_FILES_V2", origVal)

		agent := biz.Agent{ConfigJSON: "{}"}
		got := biz.FilesFromAgentInput(agent)
		if len(got) != 5 {
			t.Fatalf("expected 5 default V2 files, got %d", len(got))
		}
	})

	t.Run("without Files but with config files", func(t *testing.T) {
		agent := biz.Agent{ConfigJSON: `{"files":[{"name":"AGENTS_CORE.md","body":"core"}]}`}
		got := biz.FilesFromAgentInput(agent)
		if len(got) != 1 {
			t.Fatalf("expected 1 file from config, got %d", len(got))
		}
	})
}

func TestPgoDefaultFilesV2(t *testing.T) {
	cases := []struct {
		envVal string
		want   bool
	}{
		{"1", true},
		{"true", true},
		{"yes", true},
		{"TRUE", true},
		{"YES", true},
		{"0", false},
		{"false", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.envVal, func(t *testing.T) {
			os.Setenv("PGO_DEFAULT_FILES_V2", tc.envVal)
			defer os.Unsetenv("PGO_DEFAULT_FILES_V2")
			got := biz.PgoDefaultFilesV2()
			if got != tc.want {
				t.Fatalf("PgoDefaultFilesV2() with env %q = %v, want %v", tc.envVal, got, tc.want)
			}
		})
	}
}
