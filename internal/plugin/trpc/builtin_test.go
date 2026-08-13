package plugintrpc

import (
	"slices"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// GAP-02 回归：DB 种子声明的 CallbackPoints 必须与 chain_adapter 的
// 实现级声明一致，否则 UI 展示/校验与实际注册回调点漂移。
func TestBuiltin_SeedCallbackPointsMatchImplementation(t *testing.T) {
	for _, def := range BuiltinPluginDefs() {
		impl := BuiltinCallbackPoints(def.Key)
		if !slices.Equal(def.CallbackPoints, impl) {
			t.Errorf("plugin %q: seed CallbackPoints %v != implemented %v", def.Key, def.CallbackPoints, impl)
		}
	}
}

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
	parsePluginConfig(`{"confirm_tools":["delete_file"],"default_action":"reject"}`, "{}", &cfg, loggateway.NewNoop())
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
