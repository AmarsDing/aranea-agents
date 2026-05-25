package modelcatalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeOverlayMatchesWebCopy(t *testing.T) {
	goPath := filepath.Join("runtime_overlay.json")
	webPath := filepath.Join("..", "..", "web", "src", "config", "provider_runtime_overlay.json")
	goBytes, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatal(err)
	}
	webBytes, err := os.ReadFile(webPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(goBytes) != string(webBytes) {
		t.Fatalf("runtime overlay out of sync: %s vs %s", goPath, webPath)
	}
}

func TestMergeInterleavedHints(t *testing.T) {
	cfg := map[string]any{}
	set := func(k string, v any) { cfg[k] = v }
	applyInterleavedHints(cfg, Model{Interleaved: []byte(`{"field":"reasoning_content"}`)}, set)
	if cfg["interleaved_field"] != "reasoning_content" {
		t.Fatalf("interleaved_field: %#v", cfg)
	}
	if cfg["reasoning_content_backfill"] != true {
		t.Fatal("expected reasoning_content_backfill")
	}
}

func TestCostMicroUSDFromUSDPer1M(t *testing.T) {
	if got := CostMicroUSDFromUSDPer1M(2_000_000, 3); got != 6_000_000 {
		t.Fatalf("got %d want 6000000", got)
	}
}
