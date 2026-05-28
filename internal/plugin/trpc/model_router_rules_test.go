package plugintrpc

import "testing"

func TestResolveModelFromRules_Priority(t *testing.T) {
	rules := []ModelRouterRule{
		{Model: "cheap", Contains: []string{"hello"}, Priority: 1},
		{Model: "strong", Contains: []string{"hello"}, Priority: 10},
	}
	got := resolveModelFromRules("say hello", rules)
	if got != "strong" {
		t.Fatalf("expected strong, got %q", got)
	}
}

func TestResolveModelAPI_RulesBeforeHeuristic(t *testing.T) {
	cfg := ModelRouterConfig{
		Rules: []ModelRouterRule{
			{Model: "rule-model", Regex: `(?i)translate`},
		},
		CodeModel: "code-model",
	}
	compileModelRouterRules(cfg.Rules)
	got := ResolveModelAPI("please translate this ``` code", cfg)
	if got != "rule-model" {
		t.Fatalf("expected rule-model, got %q", got)
	}
}

func TestMatchConfirmationGuard_Patterns(t *testing.T) {
	cfg := ConfirmationGuardConfig{ConfirmPatterns: []string{"delete"}}
	if !MatchConfirmationGuard(cfg, "read_file", []byte(`{"action":"delete_all"}`)) {
		t.Fatal("expected pattern match")
	}
}

func TestConfirmationDefaultAllow(t *testing.T) {
	if !ConfirmationDefaultAllow(ConfirmationGuardConfig{DefaultAction: "allow"}) {
		t.Fatal("expected allow")
	}
}
