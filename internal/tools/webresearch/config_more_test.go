package webresearch_test

import (
	"testing"
	"time"

	"aranea-agents/internal/tools/webresearch"
)

func TestConfigFromMap_allFieldTypes(t *testing.T) {
	m := map[string]any{
		"search_depth":        "advanced",
		"max_results":         float64(5),
		"fetch_top":           float64(3),
		"include_answer":      true,
		"include_raw_content": false,
		"timeout_sec":         float64(30),
		"http_proxy":          "http://proxy:8080",
		"api_key":             "test-key",
		"provider":            "tavily",
	}
	cfg := webresearch.ConfigFromMap(m)
	if cfg.SearchDepth != "advanced" {
		t.Fatalf("SearchDepth = %q", cfg.SearchDepth)
	}
	if cfg.MaxResults != 5 {
		t.Fatalf("MaxResults = %d", cfg.MaxResults)
	}
	if cfg.FetchTop != 3 {
		t.Fatalf("FetchTop = %d", cfg.FetchTop)
	}
	if cfg.IncludeAnswer != true {
		t.Fatalf("IncludeAnswer = %v", cfg.IncludeAnswer)
	}
	if cfg.IncludeRawContent != false {
		t.Fatalf("IncludeRawContent = %v", cfg.IncludeRawContent)
	}
	if cfg.Timeout != 30*time.Second {
		t.Fatalf("Timeout = %v", cfg.Timeout)
	}
	if cfg.HTTPProxy != "http://proxy:8080" {
		t.Fatalf("HTTPProxy = %q", cfg.HTTPProxy)
	}
	if cfg.APIKey != "test-key" {
		t.Fatalf("APIKey = %q", cfg.APIKey)
	}
	if cfg.Provider != "tavily" {
		t.Fatalf("Provider = %q", cfg.Provider)
	}
}

func TestConfigFromMap_intValues(t *testing.T) {
	m := map[string]any{
		"max_results": 7,
		"fetch_top":   2,
		"timeout_sec": 20,
	}
	cfg := webresearch.ConfigFromMap(m)
	if cfg.MaxResults != 7 {
		t.Fatalf("MaxResults = %d", cfg.MaxResults)
	}
	if cfg.FetchTop != 2 {
		t.Fatalf("FetchTop = %d", cfg.FetchTop)
	}
	if cfg.Timeout != 20*time.Second {
		t.Fatalf("Timeout = %v", cfg.Timeout)
	}
}

func TestConfigFromMap_emptyMap(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "env-key")
	cfg := webresearch.ConfigFromMap(nil)
	if cfg.APIKey != "env-key" {
		t.Fatalf("APIKey = %q, want env-key", cfg.APIKey)
	}
	if cfg.Provider != "tavily" {
		t.Fatalf("Provider = %q", cfg.Provider)
	}
	if cfg.MaxResults != 8 {
		t.Fatalf("MaxResults = %d, want 8", cfg.MaxResults)
	}
}

func TestConfigFromMap_defaults(t *testing.T) {
	cfg := webresearch.ConfigFromMap(map[string]any{"api_key": "k"})
	if cfg.MaxResults != 8 {
		t.Fatalf("MaxResults = %d, want 8", cfg.MaxResults)
	}
	if cfg.FetchTop != 5 {
		t.Fatalf("FetchTop = %d, want 5", cfg.FetchTop)
	}
	if cfg.SearchDepth != "basic" {
		t.Fatalf("SearchDepth = %q", cfg.SearchDepth)
	}
	if cfg.Timeout != 15*time.Second {
		t.Fatalf("Timeout = %v", cfg.Timeout)
	}
	if !cfg.IncludeAnswer {
		t.Fatal("IncludeAnswer should default true")
	}
	if !cfg.IncludeRawContent {
		t.Fatal("IncludeRawContent should default true")
	}
}

func TestConfig_Ready_withAPIKey(t *testing.T) {
	cfg := webresearch.Config{APIKey: "my-key"}
	if !cfg.Ready() {
		t.Fatal("expected ready with APIKey")
	}
}

func TestConfig_Ready_withoutAPIKey(t *testing.T) {
	cfg := webresearch.Config{}
	if cfg.Ready() {
		t.Fatal("expected not ready without APIKey")
	}
}

func TestConfig_Ready_whitespaceAPIKey(t *testing.T) {
	cfg := webresearch.Config{APIKey: "   "}
	if cfg.Ready() {
		t.Fatal("expected not ready with whitespace APIKey")
	}
}

func TestConfigInt_intValue(t *testing.T) {
	m := map[string]any{"x": 42}
	got := webresearch.ConfigInt(m, "x")
	if got != 42 {
		t.Fatalf("ConfigInt = %d, want 42", got)
	}
}

func TestConfigInt_float64Value(t *testing.T) {
	m := map[string]any{"x": float64(10)}
	got := webresearch.ConfigInt(m, "x")
	if got != 10 {
		t.Fatalf("ConfigInt = %d, want 10", got)
	}
}

func TestConfigInt_missingKey(t *testing.T) {
	m := map[string]any{}
	got := webresearch.ConfigInt(m, "x")
	if got != 0 {
		t.Fatalf("ConfigInt = %d, want 0", got)
	}
}

func TestConfigInt_zero(t *testing.T) {
	m := map[string]any{"x": 0}
	got := webresearch.ConfigInt(m, "x")
	if got != 0 {
		t.Fatalf("ConfigInt = %d, want 0 for zero value", got)
	}
}

func TestConfigInt_negative(t *testing.T) {
	m := map[string]any{"x": -5}
	got := webresearch.ConfigInt(m, "x")
	if got != 0 {
		t.Fatalf("ConfigInt = %d, want 0 for negative value", got)
	}
}

func TestConfigInt_negativeFloat64(t *testing.T) {
	m := map[string]any{"x": float64(-3)}
	got := webresearch.ConfigInt(m, "x")
	if got != 0 {
		t.Fatalf("ConfigInt = %d, want 0 for negative float64", got)
	}
}

func TestConfigInt_altKeys(t *testing.T) {
	m := map[string]any{"alt": 7}
	got := webresearch.ConfigInt(m, "primary", "alt")
	if got != 7 {
		t.Fatalf("ConfigInt = %d, want 7 from alt key", got)
	}
}

func TestConfigBool_true(t *testing.T) {
	m := map[string]any{"x": true}
	got := webresearch.ConfigBool(m, "x")
	if !got {
		t.Fatal("ConfigBool = false, want true")
	}
}

func TestConfigBool_false(t *testing.T) {
	m := map[string]any{"x": false}
	got := webresearch.ConfigBool(m, "x")
	if got {
		t.Fatal("ConfigBool = true, want false")
	}
}

func TestConfigBool_stringTrue(t *testing.T) {
	m := map[string]any{"x": "true"}
	got := webresearch.ConfigBool(m, "x")
	if !got {
		t.Fatal("ConfigBool should return true for string 'true'")
	}
}

func TestConfigBool_float64NonZero(t *testing.T) {
	m := map[string]any{"x": float64(1)}
	got := webresearch.ConfigBool(m, "x")
	if !got {
		t.Fatal("ConfigBool = false, want true for float64(1)")
	}
}

func TestConfigBool_float64Zero(t *testing.T) {
	m := map[string]any{"x": float64(0)}
	got := webresearch.ConfigBool(m, "x")
	if got {
		t.Fatal("ConfigBool = true, want false for float64(0)")
	}
}

func TestConfigBool_missingKey(t *testing.T) {
	m := map[string]any{}
	got := webresearch.ConfigBool(m, "x")
	if got {
		t.Fatal("ConfigBool = true, want false for missing key")
	}
}
