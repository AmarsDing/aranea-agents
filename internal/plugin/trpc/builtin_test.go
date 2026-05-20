package plugintrpc

import (
	"testing"

	"aranea-agents/internal/biz"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestBuiltin_AllKeysConstruct(t *testing.T) {
	keys := []string{
		"audit_log", "skill_usage_tracker", "retry_and_reflect", "sensitive_data_mask",
		"confirmation_guard", "cost_guard", "model_router", "permission_guard", "output_policy",
	}
	for _, key := range keys {
		p := biz.Plugin{Key: key, Enabled: true, ConfigJSON: "{}"}
		if builtin(p, nil, nil) == nil {
			t.Fatalf("expected plugin for key %q", key)
		}
	}
}

func TestConfirmationGuard_NeedsConfirm(t *testing.T) {
	p := biz.Plugin{
		Key:        "confirmation_guard",
		ConfigJSON: `{"confirm_tools":["delete_file"],"default_action":"reject"}`,
	}
	plug := NewConfirmationGuardPlugin(p, nil, nil)
	if !plug.needsConfirm(&trpctool.BeforeToolArgs{ToolName: "delete_file", Arguments: []byte(`{}`)}) {
		t.Fatal("expected confirm for delete_file")
	}
	if plug.needsConfirm(&trpctool.BeforeToolArgs{ToolName: "read_file", Arguments: []byte(`{}`)}) {
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
