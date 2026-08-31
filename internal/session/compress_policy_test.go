package session

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
)

func assertRatioEqual(t *testing.T, got, want float64, msg string) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %.6f, want %.6f", msg, got, want)
	}
}

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
		nil,
		[]biz.ChatMessage{
			{Role: "user", ContentMarkdown: "hello", CreatedAt: "2026-05-24T10:00:00Z"},
			{Role: "assistant", ContentMarkdown: "hi", CreatedAt: "2026-05-24T10:00:01Z"},
		},
		"my-agent",
		nil,
		0,
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
		if got != biz.DefaultCompressionBufferRatio {
			t.Fatalf("default ratio = %f, want %f", got, biz.DefaultCompressionBufferRatio)
		}
	})

	t.Run("nil_settings", func(t *testing.T) {
		got := compressionBufferRatio(biz.Agent{})
		if got != biz.DefaultCompressionBufferRatio {
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
		if got != biz.DefaultCompressionBufferRatio {
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
		bufRatio := biz.DefaultCompressionBufferRatio
		budget := effectiveBudget(window, reserved, bufRatio)
		want := reserved + int(float64(budget)*biz.DefaultSoftTriggerRatio) + int(float64(window)*bufRatio)
		if got != want {
			t.Errorf("softTriggerTokens(default, 100000) = %d, want %d", got, want)
		}
	})

	t.Run("custom_ratio", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{SoftTriggerRatio: 0.50}}
		window := 100000
		got := softTriggerTokens(ag, window)
		reserved := profileBasedDefault("")
		bufRatio := biz.DefaultCompressionBufferRatio
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
		bufRatio := biz.DefaultCompressionBufferRatio
		budget := effectiveBudget(window, reserved, bufRatio)
		want := reserved + int(float64(budget)*biz.DefaultHardTriggerRatio) + int(float64(window)*bufRatio)
		if got != want {
			t.Errorf("hardTriggerTokens(default, 100000) = %d, want %d", got, want)
		}
	})

	t.Run("custom_ratio", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{HardTriggerRatio: 0.80}}
		window := 100000
		got := hardTriggerTokens(ag, window)
		reserved := profileBasedDefault("")
		bufRatio := biz.DefaultCompressionBufferRatio
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

func TestCompressThresholdAndKeep(t *testing.T) {
	t.Run("default_values", func(t *testing.T) {
		threshold, keepTurns := compressThresholdAndKeep(biz.Agent{})
		if threshold != defaultCompressThreshold {
			t.Errorf("threshold = %f, want %f", threshold, defaultCompressThreshold)
		}
		if keepTurns != defaultKeepTurns {
			t.Errorf("keepTurns = %d, want %d", keepTurns, defaultKeepTurns)
		}
	})

	t.Run("nil_settings", func(t *testing.T) {
		threshold, keepTurns := compressThresholdAndKeep(biz.Agent{Settings: nil})
		if threshold != defaultCompressThreshold {
			t.Errorf("threshold = %f, want %f", threshold, defaultCompressThreshold)
		}
		if keepTurns != defaultKeepTurns {
			t.Errorf("keepTurns = %d, want %d", keepTurns, defaultKeepTurns)
		}
	})

	t.Run("custom_threshold_and_keep", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{
			L0SummaryThreshold: 0.8,
			L0SummaryKeepTurns: 10,
		}}
		threshold, keepTurns := compressThresholdAndKeep(ag)
		if threshold != 0.8 {
			t.Errorf("threshold = %f, want 0.8", threshold)
		}
		if keepTurns != 10 {
			t.Errorf("keepTurns = %d, want 10", keepTurns)
		}
	})

	t.Run("recent_window_fallback", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{
			L0SummaryKeepTurns:  0,
			L0RecentWindowTurns: 7,
		}}
		_, keepTurns := compressThresholdAndKeep(ag)
		if keepTurns != 7 {
			t.Errorf("keepTurns = %d, want 7 (fallback to L0RecentWindowTurns)", keepTurns)
		}
	})

	t.Run("keep_turns_takes_precedence_over_recent_window", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{
			L0SummaryKeepTurns:  5,
			L0RecentWindowTurns: 7,
		}}
		_, keepTurns := compressThresholdAndKeep(ag)
		if keepTurns != 5 {
			t.Errorf("keepTurns = %d, want 5 (L0SummaryKeepTurns takes precedence)", keepTurns)
		}
	})
}

func TestTruncateStrategy(t *testing.T) {
	t.Run("nil_settings", func(t *testing.T) {
		got := truncateStrategy(biz.Agent{})
		if got != "summary" {
			t.Errorf("nil settings = %q, want %q", got, "summary")
		}
	})

	t.Run("empty_string", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{L0TruncateStrategy: ""}}
		got := truncateStrategy(ag)
		if got != "summary" {
			t.Errorf("empty string = %q, want %q", got, "summary")
		}
	})

	t.Run("whitespace_only", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{L0TruncateStrategy: "   "}}
		got := truncateStrategy(ag)
		if got != "summary" {
			t.Errorf("whitespace only = %q, want %q", got, "summary")
		}
	})

	t.Run("custom_strategy", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{L0TruncateStrategy: "drop_tool_results"}}
		got := truncateStrategy(ag)
		if got != "drop_tool_results" {
			t.Errorf("custom strategy = %q, want %q", got, "drop_tool_results")
		}
	})

	t.Run("custom_strategy_case_insensitive", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{L0TruncateStrategy: "Hybrid"}}
		got := truncateStrategy(ag)
		if got != "hybrid" {
			t.Errorf("case insensitive = %q, want %q", got, "hybrid")
		}
	})
}

func TestAtFullContextUsage(t *testing.T) {
	t.Run("ratio_at_one", func(t *testing.T) {
		sess := biz.Session{ContextUsedRatio: 1.0}
		if !atFullContextUsage(sess) {
			t.Error("ratio >= 1.0 should return true")
		}
	})

	t.Run("ratio_above_one", func(t *testing.T) {
		sess := biz.Session{ContextUsedRatio: 1.5}
		if !atFullContextUsage(sess) {
			t.Error("ratio > 1.0 should return true")
		}
	})

	t.Run("tokens_at_window", func(t *testing.T) {
		sess := biz.Session{
			ContextUsedTokens:       llmcontext.DefaultWindowTokens,
			LastContextWindowTokens: 100000,
		}
		if !atFullContextUsage(sess) {
			t.Error("used tokens >= 256K product window should return true")
		}
	})

	t.Run("tokens_above_window", func(t *testing.T) {
		sess := biz.Session{
			ContextUsedTokens:       llmcontext.DefaultWindowTokens + 1000,
			LastContextWindowTokens: 100000,
		}
		if !atFullContextUsage(sess) {
			t.Error("used tokens > 256K product window should return true")
		}
	})

	t.Run("neither_condition", func(t *testing.T) {
		sess := biz.Session{
			ContextUsedRatio:        0.5,
			ContextUsedTokens:       50000,
			LastContextWindowTokens: 100000,
		}
		if atFullContextUsage(sess) {
			t.Error("neither condition met should return false")
		}
	})

	t.Run("below_product_window", func(t *testing.T) {
		sess := biz.Session{
			ContextUsedRatio:        0.9,
			ContextUsedTokens:       90000,
			LastContextWindowTokens: 0,
		}
		if atFullContextUsage(sess) {
			t.Error("used tokens below 256K product window should return false")
		}
	})
}

func TestCompressDebounceActive(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

	t.Run("zero_gap", func(t *testing.T) {
		if compressDebounceActive("2026-05-24T11:55:00Z", 0, now) {
			t.Error("minGap <= 0 should return false")
		}
	})

	t.Run("negative_gap", func(t *testing.T) {
		if compressDebounceActive("2026-05-24T11:55:00Z", -1*time.Minute, now) {
			t.Error("negative minGap should return false")
		}
	})

	t.Run("empty_string", func(t *testing.T) {
		if compressDebounceActive("", 10*time.Minute, now) {
			t.Error("empty string should return false")
		}
	})

	t.Run("whitespace_only", func(t *testing.T) {
		if compressDebounceActive("   ", 10*time.Minute, now) {
			t.Error("whitespace only should return false")
		}
	})

	t.Run("invalid_rfc3339", func(t *testing.T) {
		if compressDebounceActive("not-a-timestamp", 10*time.Minute, now) {
			t.Error("invalid RFC3339 should return false")
		}
	})

	t.Run("within_gap", func(t *testing.T) {
		last := now.Add(-5 * time.Minute).Format(time.RFC3339)
		if !compressDebounceActive(last, 10*time.Minute, now) {
			t.Error("within gap should return true")
		}
	})

	t.Run("past_gap", func(t *testing.T) {
		last := now.Add(-15 * time.Minute).Format(time.RFC3339)
		if compressDebounceActive(last, 10*time.Minute, now) {
			t.Error("past gap should return false")
		}
	})

	t.Run("exactly_at_gap", func(t *testing.T) {
		last := now.Add(-10 * time.Minute).Format(time.RFC3339)
		if compressDebounceActive(last, 10*time.Minute, now) {
			t.Error("exactly at gap boundary should return false (not strictly less)")
		}
	})
}

func TestDetectConversationMode(t *testing.T) {
	t.Run("coding_mode_high_density", func(t *testing.T) {
		got := DetectConversationMode(30, 10)
		if got != ConversationModeCoding {
			t.Errorf("toolCallCount=30, turnCount=10 → %d, want Coding", got)
		}
	})

	t.Run("coding_mode_just_above_threshold", func(t *testing.T) {
		got := DetectConversationMode(21, 10)
		if got != ConversationModeCoding {
			t.Errorf("toolCallCount=21, turnCount=10 → %d, want Coding", got)
		}
	})

	t.Run("chat_mode_low_density", func(t *testing.T) {
		got := DetectConversationMode(2, 10)
		if got != ConversationModeChat {
			t.Errorf("toolCallCount=2, turnCount=10 → %d, want Chat", got)
		}
	})

	t.Run("chat_mode_just_below_threshold", func(t *testing.T) {
		got := DetectConversationMode(4, 10)
		if got != ConversationModeChat {
			t.Errorf("toolCallCount=4, turnCount=10 → %d, want Chat", got)
		}
	})

	t.Run("mixed_mode_middle", func(t *testing.T) {
		got := DetectConversationMode(10, 10)
		if got != ConversationModeMixed {
			t.Errorf("toolCallCount=10, turnCount=10 → %d, want Mixed", got)
		}
	})

	t.Run("mixed_mode_boundary_upper", func(t *testing.T) {
		got := DetectConversationMode(20, 10)
		if got != ConversationModeMixed {
			t.Errorf("toolCallCount=20, turnCount=10 → %d, want Mixed", got)
		}
	})

	t.Run("mixed_mode_boundary_lower", func(t *testing.T) {
		got := DetectConversationMode(5, 10)
		if got != ConversationModeMixed {
			t.Errorf("toolCallCount=5, turnCount=10 → %d, want Mixed", got)
		}
	})

	t.Run("zero_turns", func(t *testing.T) {
		got := DetectConversationMode(0, 0)
		if got != ConversationModeMixed {
			t.Errorf("toolCallCount=0, turnCount=0 → %d, want Mixed", got)
		}
	})

	t.Run("zero_tool_calls", func(t *testing.T) {
		got := DetectConversationMode(0, 10)
		if got != ConversationModeChat {
			t.Errorf("toolCallCount=0, turnCount=10 → %d, want Chat", got)
		}
	})
}

func TestAdaptiveBufferState_UpdateAdaptiveBuffer(t *testing.T) {
	const window = 100000

	t.Run("high_increment_increases_ratio", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.15)
		s.LastUsedTokens = 50000
		// buffer = 100000 * 0.15 = 15000; 0.70 * 15000 = 10500
		// increment = 65000 - 50000 = 15000 > 10500 → ratio += 0.02
		ratio := s.UpdateAdaptiveBuffer(65000, window, ConversationModeMixed)
		assertRatioEqual(t, ratio, 0.17, "high increment ratio")
		if s.ConsecutiveLowCount != 0 {
			t.Errorf("consecutiveLowCount = %d, want 0", s.ConsecutiveLowCount)
		}
	})

	t.Run("low_increment_accumulates", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.15)
		s.LastUsedTokens = 50000
		// buffer = 15000; 0.30 * 15000 = 4500
		// increment = 54000 - 50000 = 4000 < 4500 → consecutiveLowCount++
		ratio := s.UpdateAdaptiveBuffer(54000, window, ConversationModeMixed)
		assertRatioEqual(t, ratio, 0.15, "first low increment ratio")
		if s.ConsecutiveLowCount != 1 {
			t.Errorf("consecutiveLowCount = %d, want 1", s.ConsecutiveLowCount)
		}
	})

	t.Run("five_consecutive_low_decreases_ratio", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.15)
		s.LastUsedTokens = 50000
		// buffer = 15000; 0.30 * 15000 = 4500
		// Each increment = 4000 < 4500
		for i := 0; i < 5; i++ {
			s.UpdateAdaptiveBuffer(54000+i*4000, window, ConversationModeMixed)
		}
		assertRatioEqual(t, s.CurrentRatio, 0.14, "after 5 low increments ratio")
		if s.ConsecutiveLowCount != 0 {
			t.Errorf("consecutiveLowCount should reset to 0, got %d", s.ConsecutiveLowCount)
		}
	})

	t.Run("ratio_capped_at_max", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.25)
		s.LastUsedTokens = 50000
		// buffer = 25000; 0.70 * 25000 = 17500
		// increment = 70000 - 50000 = 20000 > 17500 → ratio += 0.02 → 0.27 → clamped to 0.25
		ratio := s.UpdateAdaptiveBuffer(70000, window, ConversationModeMixed)
		assertRatioEqual(t, ratio, 0.25, "capped ratio")
	})

	t.Run("ratio_floored_at_min", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.10)
		s.LastUsedTokens = 50000
		// buffer = 10000; 0.30 * 10000 = 3000
		// Simulate 5 consecutive low increments to trigger decrease
		for i := 0; i < 5; i++ {
			s.UpdateAdaptiveBuffer(52000+i*2000, window, ConversationModeMixed)
		}
		// ratio would go to 0.09 but clamped to 0.10
		if s.CurrentRatio < adaptiveBufferMinRatio {
			t.Errorf("ratio = %f, should not be below %f", s.CurrentRatio, adaptiveBufferMinRatio)
		}
	})

	t.Run("coding_mode_bias", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.12)
		s.LastUsedTokens = 50000
		// Coding mode: if ratio < 0.18, bump to 0.18
		ratio := s.UpdateAdaptiveBuffer(54000, window, ConversationModeCoding)
		assertRatioEqual(t, ratio, 0.18, "coding mode bias ratio")
	})

	t.Run("coding_mode_no_bias_when_above_threshold", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.20)
		s.LastUsedTokens = 50000
		// Coding mode: ratio >= 0.18, no bump
		ratio := s.UpdateAdaptiveBuffer(54000, window, ConversationModeCoding)
		assertRatioEqual(t, ratio, 0.20, "coding mode no bias ratio")
	})

	t.Run("chat_mode_bias", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.15)
		s.LastUsedTokens = 50000
		// Chat mode: if ratio > 0.12, cap to 0.12 (one-time adjustment)
		ratio := s.UpdateAdaptiveBuffer(54000, window, ConversationModeChat)
		assertRatioEqual(t, ratio, 0.12, "chat mode bias ratio")
	})

	t.Run("chat_mode_no_bias_when_at_floor", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.12)
		s.LastUsedTokens = 50000
		// Chat mode: ratio == 0.12, not > 0.12, no reduction
		ratio := s.UpdateAdaptiveBuffer(54000, window, ConversationModeChat)
		assertRatioEqual(t, ratio, 0.12, "chat mode no bias ratio")
	})

	t.Run("normal_increment_resets_consecutive_low", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.15)
		s.LastUsedTokens = 50000
		// First: low increment
		s.UpdateAdaptiveBuffer(54000, window, ConversationModeMixed)
		if s.ConsecutiveLowCount != 1 {
			t.Fatalf("after 1 low increment, count = %d, want 1", s.ConsecutiveLowCount)
		}
		// Then: normal increment (between 0.30*buffer and 0.70*buffer)
		// buffer = 15000; 0.30*15000 = 4500; 0.70*15000 = 10500
		// increment = 50000 + 8000 - 54000 = 4000... wait, LastUsedTokens is now 54000
		// We need increment between 4500 and 10500, so usedTokens = 54000 + 7000 = 61000
		s.UpdateAdaptiveBuffer(61000, window, ConversationModeMixed)
		if s.ConsecutiveLowCount != 0 {
			t.Errorf("normal increment should reset consecutiveLowCount, got %d", s.ConsecutiveLowCount)
		}
	})
}

func TestNewAdaptiveBufferState(t *testing.T) {
	t.Run("normal_initial", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.15)
		assertRatioEqual(t, s.CurrentRatio, 0.15, "ratio")
	})

	t.Run("clamp_below_min", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.05)
		assertRatioEqual(t, s.CurrentRatio, adaptiveBufferMinRatio, "ratio")
	})

	t.Run("clamp_above_max", func(t *testing.T) {
		s := NewAdaptiveBufferState(0.30)
		assertRatioEqual(t, s.CurrentRatio, adaptiveBufferMaxRatio, "ratio")
	})
}

func TestSoftTriggerTokensWithRatio(t *testing.T) {
	ag := biz.Agent{}
	window := 100000
	ratio := 0.20

	got := softTriggerTokensWithRatio(ag, window, ratio)
	reserved := profileBasedDefault("")
	budget := effectiveBudget(window, reserved, ratio)
	want := reserved + int(float64(budget)*biz.DefaultSoftTriggerRatio) + int(float64(window)*ratio)
	if got != want {
		t.Errorf("softTriggerTokensWithRatio(_, 100000, 0.20) = %d, want %d", got, want)
	}
}

func TestHardTriggerTokensWithRatio(t *testing.T) {
	ag := biz.Agent{}
	window := 100000
	ratio := 0.20

	got := hardTriggerTokensWithRatio(ag, window, ratio)
	reserved := profileBasedDefault("")
	budget := effectiveBudget(window, reserved, ratio)
	want := reserved + int(float64(budget)*biz.DefaultHardTriggerRatio) + int(float64(window)*ratio)
	if got != want {
		t.Errorf("hardTriggerTokensWithRatio(_, 100000, 0.20) = %d, want %d", got, want)
	}
}

func TestAdaptiveBufferEnabled(t *testing.T) {
	t.Run("nil_settings_defaults_true", func(t *testing.T) {
		if !adaptiveBufferEnabled(biz.Agent{}) {
			t.Error("nil settings should default to true")
		}
	})

	t.Run("explicitly_enabled", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{CompressionBufferAdaptive: true}}
		if !adaptiveBufferEnabled(ag) {
			t.Error("explicitly enabled should return true")
		}
	})

	t.Run("explicitly_disabled", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{CompressionBufferAdaptive: false}}
		if adaptiveBufferEnabled(ag) {
			t.Error("explicitly disabled should return false")
		}
	})
}
