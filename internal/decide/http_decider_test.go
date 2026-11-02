package decide

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPDecider_LetteredFallbackWhenSystemOneMissing(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/systemone", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req chatReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.ChatTemplateKwargs["enable_thinking"] != false {
			t.Fatalf("thinking must be disabled: %#v", req.ChatTemplateKwargs)
		}
		_ = json.NewEncoder(w).Encode(chatResp{Choices: []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{{Message: struct {
			Content string `json:"content"`
		}{Content: "A"}}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := &HTTPDecider{
		base:    srv.URL,
		model:   "openjev-q4",
		backend: "openjev-http",
		client:  srv.Client(),
		lg:      nil,
	}
	q := Question{
		ID:           "correction_gate",
		Type:         "choice",
		Instructions: "update or keep?",
		Criteria:     map[string]string{"update_fact": "update", "keep_as_is": "keep"},
		Slots:        []string{"user_message"},
	}
	// sortedKeys → keep_as_is, update_fact → A = keep_as_is
	ans, err := d.Decide(context.Background(), map[string]any{"user_message": "I was charged twice"}, []Question{q})
	if err != nil {
		t.Fatal(err)
	}
	got := ans["correction_gate"]
	if got.Probabilities["keep_as_is"] < 0.9 {
		t.Fatalf("want keep_as_is win, got %#v", got.Probabilities)
	}
	if got.Backend != "openjev-http" {
		t.Fatalf("backend = %q", got.Backend)
	}
}

func TestHTTPDecider_SystemOnePreferred(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/systemone", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(systemOneResp{Answers: map[string]struct {
			Probabilities map[string]float64 `json:"probabilities"`
			Confidence    float64            `json:"confidence"`
		}{
			"route_mode": {Probabilities: map[string]float64{"direct_lookup": 0.91, "no_recall": 0.09}, Confidence: 0.91},
		}})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("chat must not be called when systemone works")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	d := &HTTPDecider{base: srv.URL, model: "m", backend: "b", client: srv.Client()}
	q, _ := QuestionDef("route_mode")
	ans, err := d.Decide(context.Background(), map[string]any{"user_message": "what is my name"}, []Question{q})
	if err != nil {
		t.Fatal(err)
	}
	if ans["route_mode"].Probabilities["direct_lookup"] < 0.9 {
		t.Fatalf("%#v", ans["route_mode"])
	}
}

func TestHTTPDecider_SystemOneHangFallsThroughAndIsRemembered(t *testing.T) {
	t.Parallel()
	var sysCalls, chatCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/systemone", func(w http.ResponseWriter, r *http.Request) {
		sysCalls.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(800 * time.Millisecond):
		}
		http.Error(w, "slow", http.StatusGatewayTimeout)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
		chatCalls.Add(1)
		_ = json.NewEncoder(w).Encode(chatResp{Choices: []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{{Message: struct {
			Content string `json:"content"`
		}{Content: "B"}}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	d := &HTTPDecider{
		base:           srv.URL,
		model:          "openjev-q4",
		backend:        "openjev-http",
		client:         srv.Client(),
		systemOneProbe: 200 * time.Millisecond,
	}
	q := Question{
		ID:           "correction_gate",
		Type:         "choice",
		Instructions: "update or keep?",
		Criteria:     map[string]string{"keep_as_is": "keep", "update_fact": "update"},
		Slots:        []string{"user_message"},
	}
	state := map[string]any{"user_message": "my name is now Tim, not Tom"}
	start := time.Now()
	ans, err := d.Decide(context.Background(), state, []Question{q})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("hang consumed the whole budget: %s", time.Since(start))
	}
	// sortedKeys → keep_as_is, update_fact so B is update_fact
	if ans["correction_gate"].Probabilities["update_fact"] < 0.9 {
		t.Fatalf("%#v", ans["correction_gate"])
	}
	if _, err := d.Decide(context.Background(), state, []Question{q}); err != nil {
		t.Fatal(err)
	}
	if got := sysCalls.Load(); got != 1 {
		t.Fatalf("systemone calls = %d, want 1", got)
	}
	if got := chatCalls.Load(); got != 2 {
		t.Fatalf("chat calls = %d, want 2", got)
	}
}

func TestProbeTimedOutIgnoresParentCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if probeTimedOut(ctx, context.DeadlineExceeded) {
		t.Fatal("parent cancel must not mark systemone unavailable")
	}
	if !probeTimedOut(context.Background(), context.DeadlineExceeded) {
		t.Fatal("probe deadline must mark systemone unavailable")
	}
}

func TestExtractLetter(t *testing.T) {
	t.Parallel()
	if got := extractLetter("  B\n"); got != "B" {
		t.Fatalf("got %q", got)
	}
	if got := extractLetter("Answer: [A]"); got != "A" {
		t.Fatalf("got %q", got)
	}
}

func TestRecorder_FillJEVFromDecider(t *testing.T) {
	t.Parallel()
	c := &captureCollector{}
	stub := DeciderFunc(func(_ context.Context, _ map[string]any, qs []Question) (map[string]Answer, error) {
		return map[string]Answer{
			qs[0].ID: {Probabilities: map[string]float64{"needed": 0.8, "not_needed": 0.2}, Confidence: 0.8, Backend: "stub"},
		}, nil
	})
	r := NewRecorder(c, nil).WithDecider(stub)
	r.Emit(context.Background(), ShadowEvent{
		CorrelationID: "c1",
		QuestionID:    "tool_relevance",
		State:         map[string]any{"task": "list skills", "tool_candidates": []string{"memory_load"}},
		Incumbent:     Incumbent{Key: "needed", Method: "bm25", Matched: true},
	})
	if len(c.recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(c.recs))
	}
	shadow := c.recs[0].Metadata["shadow"].(map[string]any)
	jev, ok := shadow["jev"].(Answer)
	if !ok {
		t.Fatalf("jev missing: %#v", shadow)
	}
	if jev.Backend != "stub" || jev.Probabilities["needed"] < 0.7 {
		t.Fatalf("%#v", jev)
	}
	if c.recs[0].Reasoning != "incumbent=bm25; jev=stub" {
		t.Fatalf("reasoning = %q", c.recs[0].Reasoning)
	}
}

// DeciderFunc adapts a function to Decider for tests.
type DeciderFunc func(ctx context.Context, state map[string]any, questions []Question) (map[string]Answer, error)

func (f DeciderFunc) Decide(ctx context.Context, state map[string]any, questions []Question) (map[string]Answer, error) {
	return f(ctx, state, questions)
}
