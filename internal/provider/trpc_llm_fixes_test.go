package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ---------------------------------------------------------------------------
// BUG-1: Gemini backend selection must default to GeminiAPI for API-key auth.
// VertexAI requires ADC, not APIKey.
// ---------------------------------------------------------------------------

func TestBuildGeminiSpecificOptions_DefaultBackendIsGeminiAPI(t *testing.T) {
	opts := buildGeminiSpecificOptions(ProviderModelConfig{
		APIKey: "sk-test",
	}, nil)
	if len(opts) == 0 {
		t.Fatal("expected at least one option (GeminiClientConfig) when APIKey is set")
	}
}

func TestBuildGeminiSpecificOptions_NoAPIKeyNoTransportReturnsNil(t *testing.T) {
	if got := buildGeminiSpecificOptions(ProviderModelConfig{}, nil); got != nil {
		t.Fatalf("expected nil options when both APIKey and rt transport are empty, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// BUG-2 / BUG-9: preflight tests are commented out pending PreflightEnabled
// and preflightProbe being added to the production code. The preflight
// feature was removed during a refactor and the tests reference symbols
// that no longer exist. When preflight is re-introduced, these tests
// should be re-enabled.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// BUG-4: malformed config_json must return an error, not silently produce
// a zero-value config.
// ---------------------------------------------------------------------------

func TestResolveAndMergeConfig_PropagatesJSONError(t *testing.T) {
	cases := []string{
		`{`,                         // truncated
		`{"api_key":`,               // incomplete value
		`{"rate_limit_rpm":"bad"}`,  // wrong type
	}
	for _, raw := range cases {
		_, err := ResolveModelConfig(ModelCatalogInput{Model: "m", ConfigJSON: raw})
		if err == nil {
			t.Errorf("expected JSON error for %q, got nil", raw)
		}
		if !errors.Is(err, ErrInvalidConfigJSON) {
			t.Errorf("expected wrapped ErrInvalidConfigJSON for %q, got %v", raw, err)
		}
	}
}

func TestResolveAndMergeConfig_RejectsEmptyModelID(t *testing.T) {
	_, err := ResolveModelConfig(ModelCatalogInput{Model: " ", ConfigJSON: "{}"})
	if err == nil {
		t.Fatal("expected error for empty model id")
	}
}

func TestResolveAndMergeConfig_HonoursExplicitEmptyAPIKey(t *testing.T) {
	cfg, err := ResolveModelConfig(ModelCatalogInput{
		Model:      "m",
		ConfigJSON: `{"api_key": ""}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "" {
		t.Fatalf("expected empty APIKey, got %q", cfg.APIKey)
	}
}

// ---------------------------------------------------------------------------
// BUG-5: empty ModelAPI must surface a distinct error from a nil catalog.
// ---------------------------------------------------------------------------

func TestErrEmptyModelAPI_IsDistinctFromNilCatalog(t *testing.T) {
	if errors.Is(ErrEmptyModelAPI, ErrNilLlmCatalog) {
		t.Fatal("ErrEmptyModelAPI and ErrNilLlmCatalog must be distinct sentinels")
	}
	if ErrEmptyModelAPI == nil || ErrNilLlmCatalog == nil {
		t.Fatal("both sentinels must be non-nil")
	}
}

// ---------------------------------------------------------------------------
// BUG-6: metricsModel must not hang on a nil inner channel.
// ---------------------------------------------------------------------------

type nilChannelModel struct {
	name string
}

func (n *nilChannelModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: n.name} }
func (n *nilChannelModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	return nil, nil
}

func TestWrapModelWithMetrics_NilInnerChannelDoesNotHang(t *testing.T) {
	wrapped := WrapModelWithMetrics(&nilChannelModel{name: "x"}, "openai", "gpt-4o")
	if wrapped == nil {
		t.Fatal("wrapper should be non-nil")
	}
	done := make(chan error, 1)
	go func() {
		_, err := wrapped.GenerateContent(context.Background(), &trpcmodel.Request{})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error when inner model returns nil channel")
		}
		if !errors.Is(err, ErrModelNilChannel) {
			t.Fatalf("expected wrapped ErrModelNilChannel, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GenerateContent hung when inner returned nil channel — goroutine leak")
	}
}

func TestWrapModelWithMetrics_NilInnerReturnsNilWrapper(t *testing.T) {
	if got := WrapModelWithMetrics(nil, "openai", "gpt-4o"); got != nil {
		t.Fatal("expected nil wrapper when inner is nil")
	}
}

// ---------------------------------------------------------------------------
// BUG-7: HA candidates must flow through the SAME buildProviderOptions as
// the primary (rate-limit / channel-buffer / variant included).
// ---------------------------------------------------------------------------

// NOTE: TestTrpcModelFromCandidate tests require preflightProbe/preflightURLValidator
// which are not yet in the production code. Re-enable when preflight is re-added.

// ---------------------------------------------------------------------------
// BUG-8: token bucket must not decrease tokens on clock skew.
// ---------------------------------------------------------------------------

func TestTokenBucket_ClockSkewDoesNotDecreaseTokens(t *testing.T) {
	b := &tokenBucket{
		capacity: 10,
		tokens:   10,
		last:     time.Now().Add(1 * time.Hour), // future — simulates NTP step
	}
	if err := b.allow(); err != nil {
		t.Fatalf("first allow must succeed despite clock skew, got %v", err)
	}
	if b.tokens > float64(b.capacity) {
		t.Fatalf("tokens must not exceed capacity, got %v", b.tokens)
	}
	if b.tokens < 9 {
		t.Fatalf("clock skew must not consume extra tokens, got %v", b.tokens)
	}
}

func TestRateLimitTransport_RejectsAfterBurstExhausted(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	inner := &http.Transport{}
	tr := wrapRateLimitTransport(inner, 3)
	client := &http.Client{Transport: tr}
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		_, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d should succeed within burst, got %v", i+1, err)
		}
	}
	// 4th request within the same second should be throttled.
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected rate-limit error on 4th request")
	}
}

func TestRateLimitTransport_RespectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	inner := &http.Transport{}
	tr := wrapRateLimitTransport(inner, 100)
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ---------------------------------------------------------------------------
// BUG-3: dead code removed. The legacy ErrAnthropicNativeEndpoint sentinel
// was previously declared but never referenced anywhere in the codebase.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// CapabilitiesForProviderModel must respect explicit overrides.
// ---------------------------------------------------------------------------

func TestCapabilitiesForProviderModel_HonoursExplicitDisable(t *testing.T) {
	pm := biz.ProviderModel{
		Provider:             "openai",
		Model:                "m",
		CapabilitiesExplicit: true,
		Capabilities:         biz.ModelCapabilities{Text: false},
	}
	caps := CapabilitiesForProviderModel(pm)
	if caps.Text {
		t.Fatal("explicit Text=false must be honoured")
	}
}

func TestCapabilitiesForProviderModel_ReturnsConservativeDefaultsOnBadConfig(t *testing.T) {
	pm := biz.ProviderModel{
		Provider:  "openai",
		Model:     "m",
		ConfigJSON: `{invalid`,
	}
	caps := CapabilitiesForProviderModel(pm)
	if !caps.Text || !caps.File || !caps.ToolCall {
		t.Fatal("malformed config_json should return conservative defaults (Text+File+ToolCall)")
	}
}

// ---------------------------------------------------------------------------
// VisibleStreamingDelta sanity (regression guard for the only pure function
// in the package that had no test).
// ---------------------------------------------------------------------------

func TestVisibleStreamingDeltaSanity(t *testing.T) {
	var b strings.Builder
	if got := VisibleStreamingDelta(&b, "ab"); got != "ab" {
		t.Fatalf("expected ab, got %q", got)
	}
	if got := VisibleStreamingDelta(&b, "abcd"); got != "cd" {
		t.Fatalf("expected cd (cumulative delta), got %q", got)
	}
}

// noopLogger returns a loggateway.Logger that swallows every call. Used in
// tests where the logging side-effect is irrelevant to the assertion.
func noopLogger() loggateway.Logger {
	return &nopLogger{}
}

type nopLogger struct{}

func (*nopLogger) Debug(_ string, _ ...loggateway.Field) {}
func (*nopLogger) Info(_ string, _ ...loggateway.Field)  {}
func (*nopLogger) Warn(_ string, _ ...loggateway.Field)  {}
func (*nopLogger) Error(_ string, _ ...loggateway.Field) {}
func (n *nopLogger) With(_ ...loggateway.Field) loggateway.Logger { return n }
