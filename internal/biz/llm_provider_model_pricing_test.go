package biz

import (
	"testing"

	"aranea-agents/internal/modelregistry"
)

func TestCostBlockHasValue(t *testing.T) {
	tests := []struct {
		name string
		c    providerCostBlock
		want bool
	}{
		{"zero block", providerCostBlock{}, false},
		{"only input", providerCostBlock{InputUSDPer1M: 0.5}, true},
		{"only output", providerCostBlock{OutputUSDPer1M: 0.3}, true},
		{"only cache_read", providerCostBlock{CacheReadUSDPer1M: 0.1}, true},
		{"only cache_write", providerCostBlock{CacheWriteUSDPer1M: 0.05}, true},
		{"only reasoning", providerCostBlock{ReasoningUSDPer1M: 0.2}, true},
		{"only embedding", providerCostBlock{EmbeddingUSDPer1M: 0.15}, true},
		{"all fields", providerCostBlock{
			InputUSDPer1M: 1, OutputUSDPer1M: 2, CacheReadUSDPer1M: 3,
			CacheWriteUSDPer1M: 4, ReasoningUSDPer1M: 5, EmbeddingUSDPer1M: 6,
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := costBlockHasValue(tt.c); got != tt.want {
				t.Errorf("costBlockHasValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildMicroPricing_FromTopLevelFields(t *testing.T) {
	cfg := providerPricingConfig{
		InputPriceMicroUSDPer1K:       1000,
		OutputPriceMicroUSDPer1K:      2000,
		CachedInputPriceMicroUSDPer1K: 500,
		ReasoningPriceMicroUSDPer1K:   300,
		EmbeddingPriceMicroUSDPer1K:   400,
	}
	micro := buildMicroPricing(cfg)
	if micro.Input != 1000 {
		t.Errorf("Input = %d, want 1000", micro.Input)
	}
	if micro.Output != 2000 {
		t.Errorf("Output = %d, want 2000", micro.Output)
	}
	if micro.CacheRead != 500 {
		t.Errorf("CacheRead = %d, want 500", micro.CacheRead)
	}
	if micro.Reasoning != 300 {
		t.Errorf("Reasoning = %d, want 300", micro.Reasoning)
	}
	if micro.Embedding != 400 {
		t.Errorf("Embedding = %d, want 400", micro.Embedding)
	}
}

func TestBuildMicroPricing_CostBlockOverrides(t *testing.T) {
	cfg := providerPricingConfig{
		InputPriceMicroUSDPer1K:  1000,
		OutputPriceMicroUSDPer1K: 2000,
		Cost: providerCostBlock{
			InputUSDPer1M:  3.0,
			OutputUSDPer1M: 6.0,
		},
	}
	micro := buildMicroPricing(cfg)
	if micro.Input != modelregistry.USDPer1MToMicroPer1K(3.0) {
		t.Errorf("Input = %d, want %d", micro.Input, modelregistry.USDPer1MToMicroPer1K(3.0))
	}
	if micro.Output != modelregistry.USDPer1MToMicroPer1K(6.0) {
		t.Errorf("Output = %d, want %d", micro.Output, modelregistry.USDPer1MToMicroPer1K(6.0))
	}
}

func TestBuildMicroPricing_CostBlockOnlyNonInputOutput(t *testing.T) {
	cfg := providerPricingConfig{
		InputPriceMicroUSDPer1K:  1000,
		OutputPriceMicroUSDPer1K: 2000,
		Cost: providerCostBlock{
			CacheReadUSDPer1M: 0.5,
		},
	}
	micro := buildMicroPricing(cfg)
	if micro.CacheRead != modelregistry.USDPer1MToMicroPer1K(0.5) {
		t.Errorf("CacheRead = %d, want %d", micro.CacheRead, modelregistry.USDPer1MToMicroPer1K(0.5))
	}
	if micro.Input != 0 {
		t.Errorf("Input should be 0 from CostBlock (InputUSDPer1M=0), got %d", micro.Input)
	}
}

func TestIsValidPricing(t *testing.T) {
	if isValidPricing(modelregistry.MicroPricing{}) {
		t.Error("zero pricing should not be valid")
	}
	if !isValidPricing(modelregistry.MicroPricing{Input: 1}) {
		t.Error("non-zero Input should be valid")
	}
	if !isValidPricing(modelregistry.MicroPricing{CacheWrite: 1}) {
		t.Error("non-zero CacheWrite should be valid")
	}
	if !isValidPricing(modelregistry.MicroPricing{Embedding: 1}) {
		t.Error("non-zero Embedding should be valid")
	}
}

func TestBuildCostUSD_FromMicro(t *testing.T) {
	cfg := providerPricingConfig{}
	micro := modelregistry.MicroPricing{Input: 3000, Output: 6000}
	costUSD := buildCostUSD(cfg, micro)
	if costUSD.Input != modelregistry.MicroPer1KToUSDPer1M(3000) {
		t.Errorf("Input = %f, want %f", costUSD.Input, modelregistry.MicroPer1KToUSDPer1M(3000))
	}
	if costUSD.Output != modelregistry.MicroPer1KToUSDPer1M(6000) {
		t.Errorf("Output = %f, want %f", costUSD.Output, modelregistry.MicroPer1KToUSDPer1M(6000))
	}
}

func TestBuildCostUSD_CostBlockOverrides(t *testing.T) {
	cfg := providerPricingConfig{
		Cost: providerCostBlock{
			InputUSDPer1M:  3.0,
			OutputUSDPer1M: 6.0,
		},
	}
	micro := modelregistry.MicroPricing{Input: 1000, Output: 2000}
	costUSD := buildCostUSD(cfg, micro)
	if costUSD.Input != 3.0 {
		t.Errorf("Input = %f, want 3.0 (from CostBlock)", costUSD.Input)
	}
	if costUSD.Output != 6.0 {
		t.Errorf("Output = %f, want 6.0 (from CostBlock)", costUSD.Output)
	}
}

func TestBuildCostUSD_CostBlockOnlyCacheRead(t *testing.T) {
	cfg := providerPricingConfig{
		Cost: providerCostBlock{
			CacheReadUSDPer1M: 0.5,
		},
	}
	micro := modelregistry.MicroPricing{Input: 3000}
	costUSD := buildCostUSD(cfg, micro)
	if costUSD.CacheRead != 0.5 {
		t.Errorf("CacheRead = %f, want 0.5 (from CostBlock)", costUSD.CacheRead)
	}
	if costUSD.Input != 0 {
		t.Errorf("Input = %f, want 0 (CostBlock.InputUSDPer1M=0)", costUSD.Input)
	}
}

func TestBuildCostUSD_CostBlockOnlyCacheWrite(t *testing.T) {
	cfg := providerPricingConfig{
		Cost: providerCostBlock{
			CacheWriteUSDPer1M: 0.05,
		},
	}
	micro := modelregistry.MicroPricing{}
	costUSD := buildCostUSD(cfg, micro)
	if costUSD.CacheWrite != 0.05 {
		t.Errorf("CacheWrite = %f, want 0.05 (from CostBlock)", costUSD.CacheWrite)
	}
}

func TestParsePricingConfig(t *testing.T) {
	cfg, ok := parsePricingConfig(`{"input_price_micro_usd_per_1k":1000}`)
	if !ok {
		t.Fatal("expected valid JSON to parse")
	}
	if cfg.InputPriceMicroUSDPer1K != 1000 {
		t.Errorf("InputPriceMicroUSDPer1K = %d, want 1000", cfg.InputPriceMicroUSDPer1K)
	}

	_, ok = parsePricingConfig(`{invalid}`)
	if ok {
		t.Error("expected invalid JSON to fail")
	}

	_, ok = parsePricingConfig("")
	if ok {
		t.Error("expected empty string to fail")
	}
}
