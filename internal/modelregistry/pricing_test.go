package modelregistry

import (
	"encoding/json"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestUSDPer1MToMicroPer1K(t *testing.T) {
	if got := USDPer1MToMicroPer1K(3); got != 3000 {
		t.Fatalf("got %d", got)
	}
}

// models.dev 上游价格偶发 float32 加宽噪声（如 0.14000000059604645），
// 写入 config_json cost 块前必须归一化，否则管理端编辑弹窗原样显示长小数。
func TestMicroPricingFromModelCost_RoundsUpstreamFloatNoise(t *testing.T) {
	cost, micro := MicroPricingFromModelCost(&ModelCost{Input: 0.14000000059604645, Output: 2.5})
	if cost.Input != 0.14 {
		t.Fatalf("cost.Input: got %v, want 0.14", cost.Input)
	}
	if cost.Output != 2.5 {
		t.Fatalf("cost.Output: got %v, want 2.5", cost.Output)
	}
	if micro.Input != 140 {
		t.Fatalf("micro.Input: got %d, want 140", micro.Input)
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
	out, changed := mergeCatalogIntoConfig(loggateway.NewNoop(), "{}", prov, model, "metadata_and_pricing", "")
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
	out, changed := mergeCatalogIntoConfig(loggateway.NewNoop(), `{"catalog_source":"custom"}`, Provider{}, Model{}, "full_spec", "")
	if changed {
		t.Fatalf("should skip custom, got %s", out)
	}
}
