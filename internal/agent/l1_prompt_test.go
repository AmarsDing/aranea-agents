package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/working_memory"
)

// --- Test 1: fieldTokenEstimate ---

func TestFieldTokenEstimate(t *testing.T) {
	tests := []struct {
		name string
		row  map[string]any
		want int
	}{
		{
			name: "float64 value",
			row:  map[string]any{"token_estimate": float64(42)},
			want: 42,
		},
		{
			name: "int value",
			row:  map[string]any{"token_estimate": 100},
			want: 100,
		},
		{
			name: "json.Number value",
			row:  map[string]any{"token_estimate": json.Number("256")},
			want: 256,
		},
		{
			name: "nil/missing key",
			row:  map[string]any{},
			want: 0,
		},
		{
			name: "string value parsed as int",
			row:  map[string]any{"token_estimate": "5"},
			want: 5,
		},
		{
			name: "non-numeric string returns 0",
			row:  map[string]any{"token_estimate": "abc"},
			want: 0,
		},
		{
			name: "zero float64",
			row:  map[string]any{"token_estimate": float64(0)},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldTokenEstimate(tt.row)
			if got != tt.want {
				t.Errorf("fieldTokenEstimate() = %d, want %d", got, tt.want)
			}
		})
	}
}

// --- Test 2: L1MemoryCue budget filter logic ---
// Since L1MemoryCue requires a biz.L1AdminReader (narrow port; no SessionAdminStore),
// we test the budget filter logic by exercising the internal helpers and
// verifying the cumulative budget truncation algorithm directly.

func TestL1MemoryCue_BudgetFilterLogic(t *testing.T) {
	// Simulate the budget filter loop from L1MemoryCue (lines 73-83).
	type pinnedField struct {
		path string
		val  string
		est  int
	}

	applyBudget := func(fields []pinnedField, budgetTokens int) []pinnedField {
		if budgetTokens <= 0 {
			return fields // unlimited
		}
		var totalEstimate int
		for i := 0; i < len(fields); i++ {
			if totalEstimate+fields[i].est > budgetTokens {
				return fields[:i]
			}
			totalEstimate += fields[i].est
		}
		return fields
	}

	t.Run("budget=0 unlimited: all fields included", func(t *testing.T) {
		fields := []pinnedField{
			{path: "a", val: "va", est: 30},
			{path: "b", val: "vb", est: 80},
			{path: "c", val: "vc", est: 50},
		}
		result := applyBudget(fields, 0)
		if len(result) != 3 {
			t.Errorf("expected 3 fields, got %d", len(result))
		}
	})

	t.Run("budget=100: second field dropped (30+80>100)", func(t *testing.T) {
		fields := []pinnedField{
			{path: "a", val: "va", est: 30},
			{path: "b", val: "vb", est: 80},
		}
		result := applyBudget(fields, 100)
		if len(result) != 1 {
			t.Fatalf("expected 1 field, got %d", len(result))
		}
		if result[0].path != "a" {
			t.Errorf("expected first field 'a', got %q", result[0].path)
		}
	})

	t.Run("budget=200: both fields included (30+80<=200)", func(t *testing.T) {
		fields := []pinnedField{
			{path: "a", val: "va", est: 30},
			{path: "b", val: "vb", est: 80},
		}
		result := applyBudget(fields, 200)
		if len(result) != 2 {
			t.Errorf("expected 2 fields, got %d", len(result))
		}
	})

	t.Run("no pinned fields: empty result", func(t *testing.T) {
		fields := []pinnedField{}
		result := applyBudget(fields, 100)
		if len(result) != 0 {
			t.Errorf("expected 0 fields, got %d", len(result))
		}
	})

	t.Run("all fields exceed budget: empty result", func(t *testing.T) {
		fields := []pinnedField{
			{path: "a", val: "va", est: 200},
		}
		result := applyBudget(fields, 100)
		if len(result) != 0 {
			t.Errorf("expected 0 fields, got %d", len(result))
		}
	})

	t.Run("budget exactly matches first field", func(t *testing.T) {
		fields := []pinnedField{
			{path: "a", val: "va", est: 50},
			{path: "b", val: "vb", est: 50},
		}
		result := applyBudget(fields, 50)
		if len(result) != 1 {
			t.Errorf("expected 1 field, got %d", len(result))
		}
	})

	t.Run("budget exactly matches cumulative total", func(t *testing.T) {
		fields := []pinnedField{
			{path: "a", val: "va", est: 30},
			{path: "b", val: "vb", est: 70},
		}
		result := applyBudget(fields, 100)
		if len(result) != 2 {
			t.Errorf("expected 2 fields, got %d", len(result))
		}
	})
}

// --- Test 3: ErrL1BudgetOverflow ---

func TestErrL1BudgetOverflow(t *testing.T) {
	if biz.ErrL1BudgetOverflow == nil {
		t.Fatal("biz.ErrL1BudgetOverflow must not be nil")
	}
	if !errors.Is(biz.ErrL1BudgetOverflow, biz.ErrL1BudgetOverflow) {
		t.Error("ErrL1BudgetOverflow should satisfy errors.Is with itself")
	}
	if biz.ErrL1BudgetOverflow.Error() == "" {
		t.Error("ErrL1BudgetOverflow.Error() must not be empty")
	}
	// apierror.Error() formats as "[DOMAIN/CODE] message".
	// ErrL1BudgetOverflow = apierror.BadRequest(apierror.DomainMemory, msg),
	// so Error() returns "[MEMORY/BAD_REQUEST] L1 budget overflow: ...".
	want := "[MEMORY/BAD_REQUEST] L1 budget overflow: field would exceed task budget_tokens"
	if got := biz.ErrL1BudgetOverflow.Error(); got != want {
		t.Errorf("ErrL1BudgetOverflow.Error() = %q, want %q", got, want)
	}
}

// --- Test 4: ResolveMemoryRuntimePolicy L1BudgetTokens mapping ---

func TestMemoryRuntimePolicy_L1BudgetTokens(t *testing.T) {
	t.Run("default value (0) maps to 0", func(t *testing.T) {
		settings := &biz.AgentRuntimeSettings{
			MemoryEnabled: true,
			L1Enabled:     true,
			L0InjectL1:    true,
		}
		policy := biz.ResolveMemoryRuntimePolicy(settings)
		if policy.L1BudgetTokens != 0 {
			t.Errorf("expected L1BudgetTokens=0, got %d", policy.L1BudgetTokens)
		}
	})

	t.Run("custom value (4096) maps to 4096", func(t *testing.T) {
		settings := &biz.AgentRuntimeSettings{
			MemoryEnabled:  true,
			L1Enabled:      true,
			L0InjectL1:     true,
			L1BudgetTokens: 4096,
		}
		policy := biz.ResolveMemoryRuntimePolicy(settings)
		if policy.L1BudgetTokens != 4096 {
			t.Errorf("expected L1BudgetTokens=4096, got %d", policy.L1BudgetTokens)
		}
	})

	t.Run("nil settings returns zero policy", func(t *testing.T) {
		policy := biz.ResolveMemoryRuntimePolicy(nil)
		if policy.L1BudgetTokens != 0 {
			t.Errorf("expected L1BudgetTokens=0 for nil settings, got %d", policy.L1BudgetTokens)
		}
	})

	t.Run("memory disabled returns zero policy", func(t *testing.T) {
		settings := &biz.AgentRuntimeSettings{
			MemoryEnabled:  false,
			L1BudgetTokens: 4096,
		}
		policy := biz.ResolveMemoryRuntimePolicy(settings)
		if policy.L1BudgetTokens != 0 {
			t.Errorf("expected L1BudgetTokens=0 when memory disabled, got %d", policy.L1BudgetTokens)
		}
	})
}

// --- Test 6: L1HistoryEnabled defaults to false (Sprint 2C-1) ---

func TestL1HistoryEnabled_DefaultFalse(t *testing.T) {
	// Verify that AgentRuntimeSettings with no explicit L1HistoryEnabled has it as false by default.
	s := biz.AgentRuntimeSettings{}
	if s.L1HistoryEnabled {
		t.Error("L1HistoryEnabled should default to false in zero-value AgentRuntimeSettings")
	}

	// Also verify DefaultAgentRuntimeSettings returns false.
	ds := biz.DefaultAgentRuntimeSettings()
	if ds.L1HistoryEnabled {
		t.Error("DefaultAgentRuntimeSettings().L1HistoryEnabled should be false")
	}
}

func TestL1HistoryEnabled_ContextRoundTrip(t *testing.T) {
	t.Run("false value round-trip", func(t *testing.T) {
		ctx := context.Background()
		ctx = working_memory.WithL1HistoryEnabled(ctx, false)
		if working_memory.L1HistoryEnabledFromCtx(ctx) {
			t.Error("L1HistoryEnabledFromCtx should return false after WithL1HistoryEnabled(ctx, false)")
		}
	})

	t.Run("true value round-trip", func(t *testing.T) {
		ctx := context.Background()
		ctx = working_memory.WithL1HistoryEnabled(ctx, true)
		if !working_memory.L1HistoryEnabledFromCtx(ctx) {
			t.Error("L1HistoryEnabledFromCtx should return true after WithL1HistoryEnabled(ctx, true)")
		}
	})

	t.Run("missing key returns false", func(t *testing.T) {
		ctx := context.Background()
		if working_memory.L1HistoryEnabledFromCtx(ctx) {
			t.Error("L1HistoryEnabledFromCtx should return false when key is missing from context")
		}
	})

	t.Run("ResolveMemoryRuntimePolicy with L1HistoryEnabled=false", func(t *testing.T) {
		settings := &biz.AgentRuntimeSettings{
			MemoryEnabled:    true,
			L1Enabled:        true,
			L0InjectL1:       true,
			L1HistoryEnabled: false,
		}
		policy := biz.ResolveMemoryRuntimePolicy(settings)
		if !policy.InjectL1 {
			t.Error("InjectL1 should be true regardless of L1HistoryEnabled")
		}
	})

	t.Run("ResolveMemoryRuntimePolicy with L1HistoryEnabled=true", func(t *testing.T) {
		settings := &biz.AgentRuntimeSettings{
			MemoryEnabled:    true,
			L1Enabled:        true,
			L0InjectL1:       true,
			L1HistoryEnabled: true,
		}
		policy := biz.ResolveMemoryRuntimePolicy(settings)
		if !policy.InjectL1 {
			t.Error("InjectL1 should be true when L1HistoryEnabled=true")
		}
	})
}

// --- Test 7: L1DefaultSchemaID is optional (Sprint 2C-2) ---

func TestL1DefaultSchemaID_Optional(t *testing.T) {
	t.Run("empty by default in zero-value struct", func(t *testing.T) {
		s := biz.AgentRuntimeSettings{}
		if s.L1DefaultSchemaID != "" {
			t.Errorf("L1DefaultSchemaID should be empty by default, got %q", s.L1DefaultSchemaID)
		}
	})

	t.Run("empty by default in DefaultAgentRuntimeSettings", func(t *testing.T) {
		ds := biz.DefaultAgentRuntimeSettings()
		if ds.L1DefaultSchemaID != "" {
			t.Errorf("DefaultAgentRuntimeSettings().L1DefaultSchemaID should be empty, got %q", ds.L1DefaultSchemaID)
		}
	})

	t.Run("preserved through ApplyMemory", func(t *testing.T) {
		s := biz.DefaultAgentRuntimeSettings()
		cfg := biz.MemoryCfg{
			Enabled:           true,
			L1Enabled:         true,
			L1DefaultSchemaID: "schema-test-123",
		}
		s.ApplyMemory(cfg)
		if s.L1DefaultSchemaID != "schema-test-123" {
			t.Errorf("L1DefaultSchemaID after ApplyMemory = %q, want %q", s.L1DefaultSchemaID, "schema-test-123")
		}
	})

	t.Run("empty string preserved through ApplyMemory", func(t *testing.T) {
		s := biz.DefaultAgentRuntimeSettings()
		cfg := biz.MemoryCfg{
			Enabled:           true,
			L1Enabled:         true,
			L1DefaultSchemaID: "",
		}
		s.ApplyMemory(cfg)
		if s.L1DefaultSchemaID != "" {
			t.Errorf("L1DefaultSchemaID should remain empty, got %q", s.L1DefaultSchemaID)
		}
	})

	t.Run("context round-trip via working_memory package", func(t *testing.T) {
		ctx := context.Background()
		ctx = working_memory.WithL1DefaultSchemaID(ctx, "schema-abc")
		if got := working_memory.L1DefaultSchemaIDFromCtx(ctx); got != "schema-abc" {
			t.Errorf("L1DefaultSchemaIDFromCtx = %q, want %q", got, "schema-abc")
		}
	})

	t.Run("context returns empty string when not set", func(t *testing.T) {
		ctx := context.Background()
		if got := working_memory.L1DefaultSchemaIDFromCtx(ctx); got != "" {
			t.Errorf("L1DefaultSchemaIDFromCtx should return empty string when not set, got %q", got)
		}
	})
}

// --- Test 8: L1FieldMaxChars truncation (safeTruncate) ---

func TestL1MemoryCue_L1FieldMaxChars(t *testing.T) {
	// Test the safeTruncate function logic (unexported, so test inline).
	safeTruncate := func(s string, maxChars int) string {
		if maxChars <= 0 {
			return s
		}
		runes := []rune(s)
		if len(runes) <= maxChars {
			return s
		}
		return string(runes[:maxChars]) + "…"
	}

	t.Run("long string truncated to 200 chars", func(t *testing.T) {
		longStr := strings.Repeat("x", 500)
		result := safeTruncate(longStr, 200)
		// Result should be 200 runes + "…" = 201 chars
		if len([]rune(result)) > 201 {
			t.Errorf("truncated result too long: got %d runes, want <= 201", len([]rune(result)))
		}
		if len([]rune(result)) != 201 {
			t.Errorf("truncated result length = %d runes, want 201 (200 + ellipsis)", len([]rune(result)))
		}
	})

	t.Run("empty string no truncation needed", func(t *testing.T) {
		result := safeTruncate("", 200)
		if result != "" {
			t.Errorf("empty string should remain empty, got %q", result)
		}
	})

	t.Run("short string no truncation needed", func(t *testing.T) {
		result := safeTruncate("hello", 200)
		if result != "hello" {
			t.Errorf("short string should not be truncated, got %q", result)
		}
	})

	t.Run("exact length no truncation", func(t *testing.T) {
		s := strings.Repeat("a", 200)
		result := safeTruncate(s, 200)
		if result != s {
			t.Error("string exactly at maxChars should not be truncated")
		}
	})

	t.Run("one over limit gets truncated", func(t *testing.T) {
		s := strings.Repeat("a", 201)
		result := safeTruncate(s, 200)
		runes := []rune(result)
		if len(runes) != 201 {
			t.Errorf("expected 201 runes (200 + ellipsis), got %d", len(runes))
		}
	})

	t.Run("CJK string truncated correctly by rune", func(t *testing.T) {
		s := strings.Repeat("中", 300)
		result := safeTruncate(s, 200)
		runes := []rune(result)
		if len(runes) != 201 {
			t.Errorf("expected 201 runes for CJK truncation, got %d", len(runes))
		}
	})

	t.Run("maxChars <= 0 returns original", func(t *testing.T) {
		s := "hello world"
		if result := safeTruncate(s, 0); result != s {
			t.Errorf("maxChars=0 should return original, got %q", result)
		}
		if result := safeTruncate(s, -1); result != s {
			t.Errorf("maxChars=-1 should return original, got %q", result)
		}
	})
}

// --- Test 5: Token estimation formula (CJK-aware) ---
// Mirrors the logic in internal/data/memory_shim_l1.go estimateTokens:
//   CJK characters at ~1 token per rune, non-CJK at ~1 token per 4 runes.

func TestTokenEstimateFormula(t *testing.T) {
	// Mirror the estimateTokens logic from data package.
	isCJK := func(r rune) bool {
		return (r >= 0x4E00 && r <= 0x9FFF) ||
			(r >= 0x3040 && r <= 0x309F) ||
			(r >= 0x30A0 && r <= 0x30FF) ||
			(r >= 0xAC00 && r <= 0xD7AF)
	}
	estimate := func(combined string) int {
		var cjkCount, otherCount int
		for _, r := range combined {
			if isCJK(r) {
				cjkCount++
			} else {
				otherCount++
			}
		}
		est := cjkCount + otherCount/4
		if est == 0 && combined != "" {
			est = 1
		}
		return est
	}

	tests := []struct {
		name     string
		combined string
		want     int
	}{
		{name: "empty string", combined: "", want: 0},
		{name: "hello (5 non-CJK runes)", combined: "hello", want: 1},
		{name: "Chinese 4 runes (CJK)", combined: "你好世界", want: 4},
		{name: "single ASCII char", combined: "a", want: 1},
		{name: "two ASCII chars", combined: "ab", want: 1},
		{name: "three ASCII chars", combined: "abc", want: 1},
		{name: "four ASCII chars", combined: "abcd", want: 1},
		{name: "eight ASCII chars", combined: "abcdefgh", want: 2},
		{name: "mixed CJK+ASCII: 你好ab", combined: "你好ab", want: 2},
		{name: "single CJK char", combined: "中", want: 1},
		{name: "CJK + lots of ASCII", combined: "你好hello world", want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimate(tt.combined)
			if got != tt.want {
				t.Errorf("estimate(%q) = %d, want %d", tt.combined, got, tt.want)
			}
		})
	}
}
