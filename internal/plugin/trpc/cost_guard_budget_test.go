package plugintrpc

import (
	"testing"
)

func TestResolveCostGuardTarget_BlockedModel(t *testing.T) {
	cfg := CostGuardConfig{
		BlockedModels: []string{"gpt-4"},
		FallbackModel: "gpt-3.5-turbo",
	}
	target := ResolveCostGuardTarget("gpt-4", cfg, 100, nil)
	if target != "gpt-3.5-turbo" {
		t.Fatalf("expected fallback, got %q", target)
	}
}

func TestResolveCostGuardTarget_PromptBudget(t *testing.T) {
	cfg := CostGuardConfig{
		MaxPromptTokens: 100,
		FallbackModel:   "cheap-model",
	}
	target := ResolveCostGuardTarget("base", cfg, 500, nil)
	if target != "cheap-model" {
		t.Fatalf("expected prompt fallback, got %q", target)
	}
}

func TestResolveCostGuardTarget_DailyBudget(t *testing.T) {
	tracker := NewCostGuardBudgetTracker()
	cfg := CostGuardConfig{
		DailyTokenBudget: 1000,
		FallbackModel:    "cheap-model",
	}
	tracker.TryConsume(cfg.DailyTokenBudget, 900)
	target := ResolveCostGuardTarget("base", cfg, 200, tracker)
	if target != "cheap-model" {
		t.Fatalf("expected daily budget fallback, got %q", target)
	}
}

func TestCostGuardShouldBlock_NoFallback(t *testing.T) {
	cfg := CostGuardConfig{MaxPromptTokens: 10}
	block, reason := costGuardShouldBlock("base", cfg, 100, nil)
	if !block || reason != "max_prompt_tokens" {
		t.Fatalf("expected block without fallback, got block=%v reason=%q", block, reason)
	}
}

func TestIsSkillTool(t *testing.T) {
	if !isSkillTool("skill_run") || !isSkillTool("use_skill") {
		t.Fatal("expected skill tools")
	}
	if isSkillTool("read_file") {
		t.Fatal("read_file is not a skill tool")
	}
}

func TestNormalizeRunStatus(t *testing.T) {
	if normalizeRunStatus("ok") != "success" {
		t.Fatal("ok should map to success")
	}
}

func TestShouldPersistPluginRun(t *testing.T) {
	if shouldPersistPluginRun("success") || shouldPersistPluginRun("ok") {
		t.Fatal("success should not persist run row")
	}
	if !shouldPersistPluginRun("blocked") || !shouldPersistPluginRun("error") {
		t.Fatal("blocked/error should persist")
	}
}
