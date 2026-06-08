package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestSessionCompressEnabled(t *testing.T) {
	if !sessionCompressEnabled(biz.Agent{Settings: &biz.AgentRuntimeSettings{L0SnapshotMode: "on_warning"}}) {
		t.Fatal("legacy on_warning should enable compress")
	}
	if sessionCompressEnabled(biz.Agent{Settings: &biz.AgentRuntimeSettings{L0SnapshotMode: "off"}}) {
		t.Fatal("legacy off should disable compress")
	}
	if !sessionCompressEnabled(biz.Agent{Settings: &biz.AgentRuntimeSettings{ContextCompactionEnabled: true, L0SnapshotMode: "off"}}) {
		t.Fatal("context_compaction_enabled should enable compress even when snapshot mode is off")
	}
}

func TestFilterMessagesForTruncateStrategy_dropsTool(t *testing.T) {
	msgs := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "hi"},
		{Role: "tool", ContentMarkdown: "result"},
	}
	out := filterMessagesForTruncateStrategy(msgs, "drop_tool_results")
	if len(out) != 1 || out[0].Role != "user" {
		t.Fatalf("got %#v", out)
	}
}

func TestCompressMinGapFromAgent_default(t *testing.T) {
	got := compressMinGapFromAgent(biz.Agent{})
	if got != DefaultCompressMinGap {
		t.Fatalf("default gap = %v want %v", got, DefaultCompressMinGap)
	}
}

func TestCompressMinGapFromAgent_custom(t *testing.T) {
	got := compressMinGapFromAgent(biz.Agent{
		Settings: &biz.AgentRuntimeSettings{L0CompressMinGapSec: 120},
	})
	if got != 2*time.Minute {
		t.Fatalf("custom gap = %v want 2m", got)
	}
}

func TestCompressDebounceActive_withinGap(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	last := now.Add(-5 * time.Minute).Format(time.RFC3339)
	if !compressDebounceActive(last, 10*time.Minute, now) {
		t.Fatal("expected debounce within min gap")
	}
}

func TestCompressProviderModel_agentOverride(t *testing.T) {
	p, m := compressProviderModel(
		biz.Session{DefaultProvider: "openai", DefaultModel: "gpt-4"},
		biz.Agent{
			Settings: &biz.AgentRuntimeSettings{
				L0CompressProvider: "deepseek",
				L0CompressModel:    "deepseek-chat",
			},
		},
	)
	if p != "deepseek" || m != "deepseek-chat" {
		t.Fatalf("got %q/%q", p, m)
	}
}

func TestRewriteSnapshotWithCompression_tailEvents(t *testing.T) {
	raw, err := RewriteSnapshotWithCompression(
		`{"state":{}}`,
		"merged summary",
		[]biz.ChatMessage{
			{Role: "user", ContentMarkdown: "hello", CreatedAt: "2026-05-24T10:00:00Z"},
			{Role: "assistant", ContentMarkdown: "hi", CreatedAt: "2026-05-24T10:00:01Z"},
		},
		"my-agent",
	)
	if err != nil {
		t.Fatal(err)
	}
	var bundle map[string]any
	if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
		t.Fatal(err)
	}
	events, ok := bundle["events"].([]any)
	if !ok || len(events) != 3 {
		t.Fatalf("events = %#v", bundle["events"])
	}
	first, ok := events[0].(map[string]any)
	if !ok || !strings.Contains(first["content"].(string), "merged summary") {
		t.Fatalf("summary event = %#v", events[0])
	}
}

func TestKey_defaultsAppName(t *testing.T) {
	k := Key("user-1", "sess-1")
	if k.AppName != DefaultAppName || k.UserID != "user-1" || k.SessionID != "sess-1" {
		t.Fatalf("unexpected key %+v", k)
	}
}

func TestProfileBasedDefault(t *testing.T) {
	tests := []struct {
		profile string
		want    int
	}{
		{"coding", 15000},
		{"full", 15000},
		{"Coding", 15000},
		{"FULL", 15000},
		{"research", 12000},
		{"RESEARCH", 12000},
		{"chat_only", 4000},
		{"minimal", 4000},
		{"CHAT_ONLY", 4000},
		{"", 8000},
		{"unknown", 8000},
		{"  coding  ", 15000},
	}
	for _, tt := range tests {
		got := profileBasedDefault(tt.profile)
		if got != tt.want {
			t.Errorf("profileBasedDefault(%q) = %d, want %d", tt.profile, got, tt.want)
		}
	}
}

func TestCalculateReservedSystem(t *testing.T) {
	t.Run("nil_settings", func(t *testing.T) {
		got := calculateReservedSystem(biz.Agent{})
		if got != 8000 {
			t.Fatalf("nil settings should use default profile, got %d", got)
		}
	})

	t.Run("with_tools_profile", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsProfile: "coding"}}
		got := calculateReservedSystem(ag)
		if got != 15000 {
			t.Fatalf("coding profile should return 15000, got %d", got)
		}
	})

	t.Run("with_research_profile", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsProfile: "research"}}
		got := calculateReservedSystem(ag)
		if got != 12000 {
			t.Fatalf("research profile should return 12000, got %d", got)
		}
	})

	t.Run("with_minimal_profile", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsProfile: "minimal"}}
		got := calculateReservedSystem(ag)
		if got != 4000 {
			t.Fatalf("minimal profile should return 4000, got %d", got)
		}
	})
}

func TestCompressionBufferRatio(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		got := compressionBufferRatio(biz.Agent{})
		if got != defaultCompressionBufferRatio {
			t.Fatalf("default ratio = %f, want %f", got, defaultCompressionBufferRatio)
		}
	})

	t.Run("nil_settings", func(t *testing.T) {
		got := compressionBufferRatio(biz.Agent{})
		if got != defaultCompressionBufferRatio {
			t.Fatalf("nil settings should use default, got %f", got)
		}
	})

	t.Run("custom", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{CompressionBufferRatio: 0.20}}
		got := compressionBufferRatio(ag)
		if got != 0.20 {
			t.Fatalf("custom ratio = %f, want 0.20", got)
		}
	})

	t.Run("zero_uses_default", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{CompressionBufferRatio: 0}}
		got := compressionBufferRatio(ag)
		if got != defaultCompressionBufferRatio {
			t.Fatalf("zero ratio should use default, got %f", got)
		}
	})
}

func TestEffectiveBudget(t *testing.T) {
	tests := []struct {
		name           string
		contextWindow  int
		reservedSystem int
		bufferRatio    float64
		want           int
	}{
		{"normal", 100000, 8000, 0.15, 77000},
		{"zero_window", 0, 8000, 0.15, 0},
		{"large_reserved", 10000, 8000, 0.15, 500},
		{"overflow_clamp", 10000, 12000, 0.15, 0},
		{"no_buffer", 100000, 8000, 0, 92000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveBudget(tt.contextWindow, tt.reservedSystem, tt.bufferRatio)
			if got != tt.want {
				t.Errorf("effectiveBudget(%d, %d, %f) = %d, want %d",
					tt.contextWindow, tt.reservedSystem, tt.bufferRatio, got, tt.want)
			}
		})
	}
}

func TestSoftTriggerTokens(t *testing.T) {
	t.Run("default_agent", func(t *testing.T) {
		ag := biz.Agent{}
		window := 100000
		got := softTriggerTokens(ag, window)
		reserved := profileBasedDefault("")
		bufRatio := defaultCompressionBufferRatio
		budget := effectiveBudget(window, reserved, bufRatio)
		want := reserved + int(float64(budget)*defaultSoftTriggerRatio) + int(float64(window)*bufRatio)
		if got != want {
			t.Errorf("softTriggerTokens(default, 100000) = %d, want %d", got, want)
		}
	})

	t.Run("custom_ratio", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{SoftTriggerRatio: 0.50}}
		window := 100000
		got := softTriggerTokens(ag, window)
		reserved := profileBasedDefault("")
		bufRatio := defaultCompressionBufferRatio
		budget := effectiveBudget(window, reserved, bufRatio)
		want := reserved + int(float64(budget)*0.50) + int(float64(window)*bufRatio)
		if got != want {
			t.Errorf("softTriggerTokens(custom, 100000) = %d, want %d", got, want)
		}
	})

	t.Run("coding_profile", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsProfile: "coding"}}
		window := 200000
		got := softTriggerTokens(ag, window)
		if got <= 0 {
			t.Errorf("softTriggerTokens(coding, 200000) = %d, want > 0", got)
		}
	})
}

func TestHardTriggerTokens(t *testing.T) {
	t.Run("default_agent", func(t *testing.T) {
		ag := biz.Agent{}
		window := 100000
		got := hardTriggerTokens(ag, window)
		reserved := profileBasedDefault("")
		bufRatio := defaultCompressionBufferRatio
		budget := effectiveBudget(window, reserved, bufRatio)
		want := reserved + int(float64(budget)*defaultHardTriggerRatio) + int(float64(window)*bufRatio)
		if got != want {
			t.Errorf("hardTriggerTokens(default, 100000) = %d, want %d", got, want)
		}
	})

	t.Run("custom_ratio", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{HardTriggerRatio: 0.80}}
		window := 100000
		got := hardTriggerTokens(ag, window)
		reserved := profileBasedDefault("")
		bufRatio := defaultCompressionBufferRatio
		budget := effectiveBudget(window, reserved, bufRatio)
		want := reserved + int(float64(budget)*0.80) + int(float64(window)*bufRatio)
		if got != want {
			t.Errorf("hardTriggerTokens(custom, 100000) = %d, want %d", got, want)
		}
	})

	t.Run("hard_greater_than_soft", func(t *testing.T) {
		ag := biz.Agent{}
		window := 100000
		soft := softTriggerTokens(ag, window)
		hard := hardTriggerTokens(ag, window)
		if hard <= soft {
			t.Errorf("hardTriggerTokens(%d) = %d should be > softTriggerTokens(%d) = %d", hard, soft, hard, soft)
		}
	})
}

func TestEffectiveBudgetRatio(t *testing.T) {
	tests := []struct {
		name          string
		usedTokens    int
		contextWindow int
		ag            biz.Agent
		want          float64
	}{
		{"normal", 50000, 100000, biz.Agent{}, 0.5},
		{"full_usage", 100000, 100000, biz.Agent{}, 1.0},
		{"zero_window", 5000, 0, biz.Agent{}, 0},
		{"half_usage", 50000, 200000, biz.Agent{}, 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveBudgetRatio(tt.usedTokens, tt.contextWindow, tt.ag)
			if got != tt.want {
				t.Errorf("effectiveBudgetRatio(%d, %d, _) = %f, want %f",
					tt.usedTokens, tt.contextWindow, got, tt.want)
			}
		})
	}
}
