package modelcatalog

import (
	"encoding/json"
	"testing"
)

func TestUSDPer1MToMicroPer1K(t *testing.T) {
	if got := USDPer1MToMicroPer1K(3); got != 3000 {
		t.Fatalf("got %d", got)
	}
}

func TestMergeCatalogIntoConfig(t *testing.T) {
	prov := Provider{ID: "openai", Name: "OpenAI"}
	model := Model{
		ID:       "gpt-4o",
		Name:     "GPT-4o",
		ToolCall: true,
		Cost:     &ModelCost{Input: 2.5, Output: 10},
		Limit:    ModelLimit{Context: 128000, Output: 16384},
	}
	out, changed := mergeCatalogIntoConfig("{}", prov, model, "metadata_and_pricing", "")
	if !changed {
		t.Fatal("expected change")
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatal(err)
	}
	cost, ok := cfg["cost"].(map[string]any)
	if !ok || cost["input_usd_per_1m"].(float64) != 2.5 {
		t.Fatalf("cost block missing: %#v", cfg["cost"])
	}
	if cfg["catalog_managed"] != true {
		t.Fatalf("catalog_managed not set")
	}
}

func TestShouldSkipCustom(t *testing.T) {
	out, changed := mergeCatalogIntoConfig(`{"catalog_source":"custom"}`, Provider{}, Model{}, "full_spec", "")
	if changed {
		t.Fatalf("should skip custom, got %s", out)
	}
}
