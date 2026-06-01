package plugintrpc

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestBuiltin_AllKeysConstruct(t *testing.T) {
	keys := []string{
		"audit_log", "skill_usage_tracker", "retry_and_reflect", "sensitive_data_mask",
		"confirmation_guard", "cost_guard", "model_router", "permission_guard", "output_policy",
	}
	for _, key := range keys {
		p := biz.Plugin{Key: key, Enabled: true, ConfigJSON: "{}"}
		if builtin(p, nil, nil, NewRuntime(nil, loggateway.NewNoop()), loggateway.NewNoop()) == nil {
			t.Fatalf("expected plugin for key %q", key)
		}
	}
}

func TestConfirmationGuard_MatchPolicy(t *testing.T) {
	var cfg ConfirmationGuardConfig
	parsePluginConfig(`{"confirm_tools":["delete_file"],"default_action":"reject"}`, "{}", &cfg)
	if !MatchConfirmationGuard(cfg, "delete_file", []byte(`{}`)) {
		t.Fatal("expected confirm for delete_file")
	}
	if MatchConfirmationGuard(cfg, "read_file", []byte(`{}`)) {
		t.Fatal("did not expect confirm for read_file")
	}
}

func TestRedactText(t *testing.T) {
	in := "contact user@example.com sk-abcdefghijklmnop"
	out := redactText(in, true, false, true)
	if out == in {
		t.Fatalf("expected redaction, got %q", out)
	}
}
