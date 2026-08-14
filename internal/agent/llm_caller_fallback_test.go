package agent

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
)

// ─── fakes ──────────────────────────────────────────────────────────────────

type fallbackFakeCatalog struct {
	rows []biz.ProviderModel
}

func (c *fallbackFakeCatalog) List(context.Context) ([]biz.ProviderModel, error) {
	return c.rows, nil
}

func (c *fallbackFakeCatalog) GetByProviderAndModel(_ context.Context, prov, mod string) (biz.ProviderModel, error) {
	for _, pm := range c.rows {
		if pm.Provider == prov && pm.Model == mod {
			return pm, nil
		}
	}
	return biz.ProviderModel{}, biz.ErrProviderModelNotFound
}

type fallbackFakeSys struct{}

func (fallbackFakeSys) GetRefineLLM(context.Context) (biz.RefineLLMSetting, error) {
	return biz.RefineLLMSetting{}, errors.New("no refine llm")
}

func fallbackCatalogRow(prov, mod string, enabled bool) biz.ProviderModel {
	return biz.ProviderModel{
		Provider: prov,
		Model:    mod,
		Enabled:  enabled,
		ConfigJSON: `{"provider_type":"openai","api_base_url":"https://api.openai.com/v1",` +
			`"api_key":"sk-test"}`,
	}
}

// fallbackCallRecorder records the per-call model sequence and fails models
// listed in failModels.
type fallbackCallRecorder struct {
	mu         sync.Mutex
	seq        []string
	cfgs       []ProviderAPIConfig
	failModels map[string]error
}

func (r *fallbackCallRecorder) fn() func(context.Context, *http.Client, ProviderAPIConfig, string, []OpenAICompatMessage) (string, string, int, int, error) {
	return func(_ context.Context, _ *http.Client, cfg ProviderAPIConfig, modelName string, _ []OpenAICompatMessage) (string, string, int, int, error) {
		r.mu.Lock()
		r.seq = append(r.seq, modelName)
		r.cfgs = append(r.cfgs, cfg)
		r.mu.Unlock()
		if err, ok := r.failModels[modelName]; ok {
			return "", "", 0, 0, err
		}
		return "ok-" + modelName, "", 3, 5, nil
	}
}

func (r *fallbackCallRecorder) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seq...)
}

// callCfgs 返回每次底层调用收到的 cfg（与 calls() 同序）。
func (r *fallbackCallRecorder) callCfgs() []ProviderAPIConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ProviderAPIConfig(nil), r.cfgs...)
}

func newFallbackCaller(t *testing.T, rows []biz.ProviderModel, rec *fallbackCallRecorder) *DynamicLLMCaller {
	t.Helper()
	rt := &provider.RoundTrip{HTTP: &http.Client{Timeout: 5 * time.Second}}
	c := NewDynamicLLMCaller(fallbackFakeSys{}, &fallbackFakeCatalog{rows: rows}, rt)
	c.callFn = rec.fn()
	return c
}

func fallbackRequest(model string) biz.LLMCallRequest {
	return biz.LLMCallRequest{Provider: "openai", Model: model, System: "s", User: "u"}
}

// ─── tests ──────────────────────────────────────────────────────────────────

func TestDynamicLLMCaller_FallbackOnPrimaryError(t *testing.T) {
	rec := &fallbackCallRecorder{failModels: map[string]error{
		"gpt-primary": errors.New("500 Internal Server Error"),
	}}
	c := newFallbackCaller(t, []biz.ProviderModel{
		fallbackCatalogRow("openai", "gpt-primary", true),
		fallbackCatalogRow("openai", "gpt-backup", true),
	}, rec)

	text, tok, err := c.Call(context.Background(), fallbackRequest("gpt-primary"))
	if err != nil {
		t.Fatalf("fallback must succeed via backup model, got err=%v", err)
	}
	if text != "ok-gpt-backup" {
		t.Fatalf("text = %q, want ok-gpt-backup", text)
	}
	if tok != 8 {
		t.Fatalf("tokens = %d, want 8", tok)
	}
	if got := strings.Join(rec.calls(), ","); got != "gpt-primary,gpt-backup" {
		t.Fatalf("call sequence = %s, want gpt-primary,gpt-backup", got)
	}
}

func TestDynamicLLMCaller_FallbackOnlyOnce(t *testing.T) {
	rec := &fallbackCallRecorder{failModels: map[string]error{
		"gpt-primary": errors.New("500 primary"),
		"gpt-backup":  errors.New("500 backup"),
	}}
	c := newFallbackCaller(t, []biz.ProviderModel{
		fallbackCatalogRow("openai", "gpt-primary", true),
		fallbackCatalogRow("openai", "gpt-backup", true),
	}, rec)

	_, _, err := c.Call(context.Background(), fallbackRequest("gpt-primary"))
	if err == nil {
		t.Fatal("both models failing must return error")
	}
	if got := len(rec.calls()); got != 2 {
		t.Fatalf("calls = %d, want exactly 2 (primary + one fallback, no second degradation)", got)
	}
}

func TestDynamicLLMCaller_NoFallbackOnCtxCancel(t *testing.T) {
	rec := &fallbackCallRecorder{failModels: map[string]error{
		"gpt-primary": errors.New("context deadline exceeded"),
	}}
	c := newFallbackCaller(t, []biz.ProviderModel{
		fallbackCatalogRow("openai", "gpt-primary", true),
		fallbackCatalogRow("openai", "gpt-backup", true),
	}, rec)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // caller gave up — fallback must not fire
	_, _, err := c.Call(ctx, fallbackRequest("gpt-primary"))
	if err == nil {
		t.Fatal("expected error")
	}
	if got := len(rec.calls()); got != 1 {
		t.Fatalf("calls = %d, want 1 (no fallback after ctx cancel)", got)
	}
}

func TestDynamicLLMCaller_NoFallbackCandidate(t *testing.T) {
	rec := &fallbackCallRecorder{failModels: map[string]error{
		"gpt-primary": errors.New("500 primary"),
	}}
	c := newFallbackCaller(t, []biz.ProviderModel{
		fallbackCatalogRow("openai", "gpt-primary", true), // only row — no backup
	}, rec)

	_, _, err := c.Call(context.Background(), fallbackRequest("gpt-primary"))
	if err == nil || !strings.Contains(err.Error(), "500 primary") {
		t.Fatalf("err = %v, want original primary error", err)
	}
	if got := len(rec.calls()); got != 1 {
		t.Fatalf("calls = %d, want 1 (no candidate → no fallback)", got)
	}
}

func TestDynamicLLMCaller_PrimarySuccess_NoFallback(t *testing.T) {
	rec := &fallbackCallRecorder{}
	c := newFallbackCaller(t, []biz.ProviderModel{
		fallbackCatalogRow("openai", "gpt-primary", true),
		fallbackCatalogRow("openai", "gpt-backup", true),
	}, rec)

	text, _, err := c.Call(context.Background(), fallbackRequest("gpt-primary"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "ok-gpt-primary" {
		t.Fatalf("text = %q, want ok-gpt-primary", text)
	}
	if got := len(rec.calls()); got != 1 {
		t.Fatalf("calls = %d, want 1 (primary success → no fallback)", got)
	}
}

func TestDynamicLLMCaller_FallbackSkipsDisabledAndOtherProvider(t *testing.T) {
	rec := &fallbackCallRecorder{failModels: map[string]error{
		"gpt-primary": errors.New("500 primary"),
	}}
	c := newFallbackCaller(t, []biz.ProviderModel{
		fallbackCatalogRow("openai", "gpt-primary", true),
		fallbackCatalogRow("openai", "gpt-disabled", false),  // disabled → skip
		fallbackCatalogRow("anthropic", "claude-haiku", true), // other provider → skip
		fallbackCatalogRow("openai", "gpt-backup", true),
	}, rec)

	text, _, err := c.Call(context.Background(), fallbackRequest("gpt-primary"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "ok-gpt-backup" {
		t.Fatalf("text = %q, want ok-gpt-backup (disabled/other-provider skipped)", text)
	}
}

// ─── P2-5 effort 透传 ───────────────────────────────────────────────────────

func TestDynamicLLMCaller_EffortPassedThrough(t *testing.T) {
	rec := &fallbackCallRecorder{}
	c := newFallbackCaller(t, []biz.ProviderModel{
		fallbackCatalogRow("openai", "gpt-primary", true),
	}, rec)

	req := fallbackRequest("gpt-primary")
	req.ThinkingEffort = "high"
	_, _, err := c.Call(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfgs := rec.callCfgs()
	if len(cfgs) != 1 {
		t.Fatalf("calls = %d, want 1", len(cfgs))
	}
	if cfgs[0].ThinkingEffort != "high" {
		t.Fatalf("ThinkingEffort = %q, want high", cfgs[0].ThinkingEffort)
	}
}

func TestDynamicLLMCaller_EffortPassedThroughOnFallback(t *testing.T) {
	rec := &fallbackCallRecorder{failModels: map[string]error{
		"gpt-primary": errors.New("500 primary"),
	}}
	c := newFallbackCaller(t, []biz.ProviderModel{
		fallbackCatalogRow("openai", "gpt-primary", true),
		fallbackCatalogRow("openai", "gpt-backup", true),
	}, rec)

	req := fallbackRequest("gpt-primary")
	req.ThinkingEffort = "max"
	_, _, err := c.Call(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfgs := rec.callCfgs()
	if len(cfgs) != 2 {
		t.Fatalf("calls = %d, want 2", len(cfgs))
	}
	for i, want := range []string{"max", "max"} {
		if cfgs[i].ThinkingEffort != want {
			t.Fatalf("call[%d].ThinkingEffort = %q, want %q", i, cfgs[i].ThinkingEffort, want)
		}
	}
}

func TestDynamicLLMCaller_EffortOffPassedThrough(t *testing.T) {
	rec := &fallbackCallRecorder{}
	c := newFallbackCaller(t, []biz.ProviderModel{
		fallbackCatalogRow("openai", "gpt-primary", true),
	}, rec)

	req := fallbackRequest("gpt-primary")
	req.ThinkingEffort = "off"
	_, _, err := c.Call(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfgs := rec.callCfgs()
	if len(cfgs) != 1 {
		t.Fatalf("calls = %d, want 1", len(cfgs))
	}
	if cfgs[0].ThinkingEffort != "off" {
		t.Fatalf("ThinkingEffort = %q, want off", cfgs[0].ThinkingEffort)
	}
}
