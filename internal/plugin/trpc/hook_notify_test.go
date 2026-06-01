package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestEnqueueNotify_rejectsPrivateURL(t *testing.T) {
	n := NewHookNotifier(nil, loggateway.NewNoop())
	rh := biz.ResolvedHook{
		Hook: biz.Hook{Key: "t"},
		Rule: biz.HookConfig{
			Action: biz.HookAction{Type: "notify", WebhookURL: "http://127.0.0.1/x"},
		},
	}
	if err := n.EnqueueNotify(context.Background(), rh, map[string]any{"k": "v"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnqueueNotify_acceptsValidURL(t *testing.T) {
	n := NewHookNotifier(nil, loggateway.NewNoop())
	rh := biz.ResolvedHook{
		Hook: biz.Hook{Key: "t"},
		Rule: biz.HookConfig{
			Action: biz.HookAction{Type: "notify", WebhookURL: "https://example.com/hook"},
		},
	}
	if err := n.EnqueueNotify(context.Background(), rh, map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}
}
