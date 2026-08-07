package trpc_test

import (
	"context"
	"testing"

	"aranea-agents/internal/tools/clientbridge"
	"aranea-agents/internal/tools/trpc"
)

func hasToolSet(out *trpc.AssembledToolsets, name string) bool {
	for _, ts := range out.ToolSets {
		if ts != nil && ts.Name() == name {
			return true
		}
	}
	return false
}

func TestBuildToolsets_ClientBridgeAssembled(t *testing.T) {
	bridge := clientbridge.NewBridge(clientbridge.Deps{})
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		ClientBridge:    true,
		ClientBridgeSvc: bridge,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasToolSet(out, clientbridge.ToolSetName) {
		names := []string{}
		for _, ts := range out.ToolSets {
			names = append(names, ts.Name())
		}
		t.Fatalf("client toolset missing, got toolsets %v", names)
	}
}

func TestBuildToolsets_ClientBridgeSkippedWithoutService(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		ClientBridge: true, // flag on but service not wired
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasToolSet(out, clientbridge.ToolSetName) {
		t.Fatal("client toolset must be skipped when the bridge service is nil")
	}
}

func TestToolsetConfigFromEffectiveKeys_clientBridge(t *testing.T) {
	for _, key := range []string{"client_open_app", "client_open_url"} {
		cfg := trpc.ToolsetConfigFromEffectiveKeys(map[string]bool{key: true})
		if !cfg.ClientBridge {
			t.Errorf("key %q should enable ClientBridge", key)
		}
	}
	if trpc.ToolsetConfigFromEffectiveKeys(map[string]bool{"read_file": true}).ClientBridge {
		t.Error("unrelated keys must not enable ClientBridge")
	}
}

func TestToolsetConfigHasAny_clientBridge(t *testing.T) {
	if !trpc.ToolsetConfigHasAny(trpc.ToolsetConfig{ClientBridge: true}) {
		t.Error("ClientBridge should make HasAny true")
	}
}
