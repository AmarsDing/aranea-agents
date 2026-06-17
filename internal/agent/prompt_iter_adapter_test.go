package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	astructure "trpc.group/trpc-go/trpc-agent-go/agent/structure"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter/engine"
)

// stubEngine implements engine.Engine for testing.
type stubEngine struct {
	result *engine.RunResult
	err    error
}

func (e *stubEngine) Run(_ context.Context, _ *engine.RunRequest, _ ...engine.Option) (*engine.RunResult, error) {
	return e.result, e.err
}

func (e *stubEngine) Describe(_ context.Context) (*astructure.Snapshot, error) {
	return nil, nil
}

// stubRefiner implements biz.Refiner for testing.
type stubRefiner struct {
	result *biz.RefineResult
	err    error
}

func (r *stubRefiner) Refine(_ context.Context, _ biz.RefineRequest) (*biz.RefineResult, error) {
	return r.result, r.err
}

func newTestAdapter(eng engine.Engine) *PromptIterAdapter {
	fallback := &stubRefiner{
		result: &biz.RefineResult{
			Refined:     "fallback refined",
			ModelSource: biz.ModelSourceSystemDefault,
		},
	}
	return NewPromptIterAdapter(fallback, eng, loggateway.NewNoop())
}

func baseRefineRequest() biz.RefineRequest {
	return biz.RefineRequest{
		Scope:        biz.ScopeAgentDescription,
		ResourceID:   "test-agent",
		OriginalText: "original text",
	}
}

func TestPromptIterAdapter_FallbackWhenEngineNil(t *testing.T) {
	adapter := newTestAdapter(nil)
	result, err := adapter.Refine(context.Background(), baseRefineRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ModelSource != biz.ModelSourceSystemDefault {
		t.Errorf("expected fallback model source, got %q", result.ModelSource)
	}
}

func TestPromptIterAdapter_FallbackOnEngineError(t *testing.T) {
	adapter := newTestAdapter(&stubEngine{err: context.DeadlineExceeded})
	result, err := adapter.Refine(context.Background(), baseRefineRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ModelSource != biz.ModelSourceSystemDefault {
		t.Errorf("expected fallback model source, got %q", result.ModelSource)
	}
}

func TestPromptIterAdapter_FallbackOnRunFailed(t *testing.T) {
	adapter := newTestAdapter(&stubEngine{
		result: &engine.RunResult{Status: engine.RunStatusFailed},
	})
	result, err := adapter.Refine(context.Background(), baseRefineRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ModelSource != biz.ModelSourceSystemDefault {
		t.Errorf("expected fallback model source, got %q", result.ModelSource)
	}
}

func TestPromptIterAdapter_FallbackOnNilAcceptedProfile(t *testing.T) {
	adapter := newTestAdapter(&stubEngine{
		result: &engine.RunResult{
			Status:          engine.RunStatusSucceeded,
			AcceptedProfile: nil,
		},
	})
	result, err := adapter.Refine(context.Background(), baseRefineRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ModelSource != biz.ModelSourceSystemDefault {
		t.Errorf("expected fallback model source, got %q", result.ModelSource)
	}
}

func TestPromptIterAdapter_FallbackOnNoMatchingOverride(t *testing.T) {
	text := "other text"
	adapter := newTestAdapter(&stubEngine{
		result: &engine.RunResult{
			Status: engine.RunStatusSucceeded,
			AcceptedProfile: &promptiter.Profile{
				Overrides: []promptiter.SurfaceOverride{
					{SurfaceID: "wrong-scope", Value: astructure.SurfaceValue{Text: &text}},
				},
			},
		},
	})
	result, err := adapter.Refine(context.Background(), baseRefineRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ModelSource != biz.ModelSourceSystemDefault {
		t.Errorf("expected fallback model source, got %q", result.ModelSource)
	}
}

func TestPromptIterAdapter_EngineSuccess(t *testing.T) {
	refined := "engine-refined text"
	adapter := newTestAdapter(&stubEngine{
		result: &engine.RunResult{
			Status:       engine.RunStatusSucceeded,
			CurrentRound: 1,
			AcceptedProfile: &promptiter.Profile{
				Overrides: []promptiter.SurfaceOverride{
					{SurfaceID: string(biz.ScopeAgentDescription), Value: astructure.SurfaceValue{Text: &refined}},
				},
			},
		},
	})
	result, err := adapter.Refine(context.Background(), baseRefineRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Refined != refined {
		t.Errorf("expected %q, got %q", refined, result.Refined)
	}
	if result.ModelSource != biz.ModelSourcePromptIter {
		t.Errorf("expected ModelSourcePromptIter, got %q", result.ModelSource)
	}
}

func TestPromptIterAdapter_EngineSuccessWithFileName(t *testing.T) {
	refined := "engine-refined file"
	req := baseRefineRequest()
	req.Scope = biz.ScopeAgentFile
	req.FileName = "test.md"
	adapter := newTestAdapter(&stubEngine{
		result: &engine.RunResult{
			Status:       engine.RunStatusSucceeded,
			CurrentRound: 1,
			AcceptedProfile: &promptiter.Profile{
				Overrides: []promptiter.SurfaceOverride{
					{SurfaceID: "agent.file/test.md", Value: astructure.SurfaceValue{Text: &refined}},
				},
			},
		},
	})
	result, err := adapter.Refine(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Refined != refined {
		t.Errorf("expected %q, got %q", refined, result.Refined)
	}
}

func TestPromptIterAdapter_BuildRunRequest_SurfaceID(t *testing.T) {
	adapter := newTestAdapter(&stubEngine{})
	req := baseRefineRequest()
	runReq := adapter.buildRunRequest(req)
	if len(runReq.TargetSurfaceIDs) != 1 || runReq.TargetSurfaceIDs[0] != string(biz.ScopeAgentDescription) {
		t.Errorf("expected surface ID %q, got %v", biz.ScopeAgentDescription, runReq.TargetSurfaceIDs)
	}
}

func TestPromptIterAdapter_ExtractRefinedText(t *testing.T) {
	text := "extracted text"
	result := &engine.RunResult{
		AcceptedProfile: &promptiter.Profile{
			Overrides: []promptiter.SurfaceOverride{
				{SurfaceID: string(biz.ScopeAgentDescription), Value: astructure.SurfaceValue{Text: &text}},
			},
		},
	}
	adapter := newTestAdapter(nil)
	got, ok := adapter.extractRefinedText(result, baseRefineRequest())
	if !ok {
		t.Error("expected ok=true")
	}
	if got != text {
		t.Errorf("expected %q, got %q", text, got)
	}
}

func TestPromptIterAdapter_ExtractRefinedText_WithNilText(t *testing.T) {
	result := &engine.RunResult{
		AcceptedProfile: &promptiter.Profile{
			Overrides: []promptiter.SurfaceOverride{
				{SurfaceID: string(biz.ScopeAgentDescription), Value: astructure.SurfaceValue{Text: nil}},
			},
		},
	}
	adapter := newTestAdapter(nil)
	_, ok := adapter.extractRefinedText(result, baseRefineRequest())
	if ok {
		t.Error("expected ok=false for nil text")
	}
}

func TestPromptIterAdapter_SatisfiesRefinerInterface(t *testing.T) {
	// Compile-time check is at package level via `var _ biz.Refiner = (*PromptIterAdapter)(nil)`.
	var _ biz.Refiner = NewPromptIterAdapter(nil, nil, loggateway.NewNoop())
}

func TestModelSourcePromptIter(t *testing.T) {
	if biz.ModelSourcePromptIter != "prompt_iter" {
		t.Errorf("expected ModelSourcePromptIter=prompt_iter, got %q", biz.ModelSourcePromptIter)
	}
}
