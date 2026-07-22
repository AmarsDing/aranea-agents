package biz

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// stubSynthesisModel is a test double for SynthesisModelPort. C-24 tests.
type stubSynthesisModel struct {
	text     string
	err      error
	called   bool
	lastSys  string
	lastUser string
}

func (s *stubSynthesisModel) SynthesizeWithModel(ctx context.Context, system, user string) (string, error) {
	s.called = true
	s.lastSys = system
	s.lastUser = user
	if s.err != nil {
		return "", s.err
	}
	return s.text, nil
}

func TestSynthesize_PromptStrategy_CallsModel(t *testing.T) {
	stub := &stubSynthesisModel{text: "综合分析结果：所有团队已完成任务。"}
	engine := NewSynthesisEngine(stub, loggateway.NewNoop())

	out, err := engine.Synthesize(context.Background(), SynthesisInput{
		TeamResults: []TeamSynthesisResult{
			{TeamID: "t1", TeamName: "团队A", Status: "completed", Summary: "结果A"},
		},
		Strategy:    SynthesisStrategyPrompt,
		SpiritQuery: "分析销售数据",
	})
	if err != nil {
		t.Fatalf("Synthesize returned error: %v", err)
	}
	if !stub.called {
		t.Fatal("model port was not called for prompt strategy")
	}
	if out.Content != "综合分析结果：所有团队已完成任务。" {
		t.Fatalf("expected model output, got %q", out.Content)
	}
	if !strings.Contains(stub.lastUser, "分析销售数据") {
		t.Fatalf("user prompt should contain spirit query, got %q", stub.lastUser)
	}
}

func TestSynthesize_PromptStrategy_NilModelFallsBackToRawPrompt(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")
	engine := NewSynthesisEngine(nil, loggateway.NewNoop())

	out, err := engine.Synthesize(context.Background(), SynthesisInput{
		TeamResults: []TeamSynthesisResult{
			{TeamID: "t1", TeamName: "团队A", Status: "completed"},
		},
		Strategy:    SynthesisStrategyPrompt,
		SpiritQuery: "测试查询",
	})
	if err != nil {
		t.Fatalf("Synthesize returned error: %v", err)
	}
	if !strings.Contains(out.Content, "测试查询") {
		t.Fatalf("fallback should contain spirit query, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "团队结果") {
		t.Fatalf("fallback should contain team results header, got %q", out.Content)
	}
}

func TestSynthesize_PromptStrategy_NilModelErrorsInProduction(t *testing.T) {
	t.Setenv("ARANEA_ENV", "production")
	engine := NewSynthesisEngine(nil, loggateway.NewNoop())

	_, err := engine.Synthesize(context.Background(), SynthesisInput{
		TeamResults: []TeamSynthesisResult{
			{TeamID: "t1", TeamName: "团队A", Status: "completed"},
		},
		Strategy:    SynthesisStrategyPrompt,
		SpiritQuery: "生产环境",
	})
	if err == nil {
		t.Fatal("expected error when model is nil in production")
	}
	if !errors.Is(err, ErrSynthesisModelRequired) {
		t.Fatalf("want ErrSynthesisModelRequired, got %v", err)
	}
}

func TestSynthesize_PromptStrategy_ModelErrorFallsBackToRawPrompt(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")
	stub := &stubSynthesisModel{err: errors.New("LLM unavailable")}
	engine := NewSynthesisEngine(stub, loggateway.NewNoop())

	out, err := engine.Synthesize(context.Background(), SynthesisInput{
		TeamResults: []TeamSynthesisResult{
			{TeamID: "t1", TeamName: "团队A", Status: "completed"},
		},
		Strategy:    SynthesisStrategyPrompt,
		SpiritQuery: "错误回退测试",
	})
	if err != nil {
		t.Fatalf("Synthesize returned error: %v", err)
	}
	if !stub.called {
		t.Fatal("model port should have been called (and failed)")
	}
	if !strings.Contains(out.Content, "错误回退测试") {
		t.Fatalf("fallback should contain spirit query on model error, got %q", out.Content)
	}
}

func TestSynthesize_PromptStrategy_ModelErrorInProduction(t *testing.T) {
	t.Setenv("ARANEA_ENV", "prod")
	stub := &stubSynthesisModel{err: errors.New("LLM unavailable")}
	engine := NewSynthesisEngine(stub, loggateway.NewNoop())

	_, err := engine.Synthesize(context.Background(), SynthesisInput{
		TeamResults: []TeamSynthesisResult{
			{TeamID: "t1", TeamName: "团队A", Status: "completed"},
		},
		Strategy:    SynthesisStrategyPrompt,
		SpiritQuery: "生产失败",
	})
	if err == nil {
		t.Fatal("expected error when model fails in production")
	}
	if !errors.Is(err, ErrSynthesisModelFailed) {
		t.Fatalf("want ErrSynthesisModelFailed, got %v", err)
	}
}

func TestSynthesize_PromptStrategy_EmptyModelOutputFallsBack(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")
	stub := &stubSynthesisModel{text: "   "}
	engine := NewSynthesisEngine(stub, loggateway.NewNoop())

	out, err := engine.Synthesize(context.Background(), SynthesisInput{
		TeamResults: []TeamSynthesisResult{
			{TeamID: "t1", TeamName: "团队A", Status: "completed"},
		},
		Strategy:    SynthesisStrategyPrompt,
		SpiritQuery: "空输出测试",
	})
	if err != nil {
		t.Fatalf("Synthesize returned error: %v", err)
	}
	if !strings.Contains(out.Content, "空输出测试") {
		t.Fatalf("empty model output should fall back to raw prompt, got %q", out.Content)
	}
}

func TestSynthesize_HybridStrategy_CallsModel(t *testing.T) {
	stub := &stubSynthesisModel{text: "LLM 综合内容"}
	engine := NewSynthesisEngine(stub, loggateway.NewNoop())

	out, err := engine.Synthesize(context.Background(), SynthesisInput{
		TeamResults: []TeamSynthesisResult{
			{TeamID: "t1", TeamName: "团队A", Status: "completed", Summary: "摘要A"},
		},
		Strategy:    SynthesisStrategyHybrid,
		SpiritQuery: "混合策略测试",
	})
	if err != nil {
		t.Fatalf("Synthesize returned error: %v", err)
	}
	if !stub.called {
		t.Fatal("model port should be called for hybrid strategy")
	}
	if !strings.Contains(out.Content, "LLM 综合内容") {
		t.Fatalf("hybrid output should contain model text, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "团队A") {
		t.Fatalf("hybrid output should also contain template part, got %q", out.Content)
	}
}

func TestSynthesize_TemplateStrategy_DoesNotCallModel(t *testing.T) {
	stub := &stubSynthesisModel{text: "不应该被调用"}
	engine := NewSynthesisEngine(stub, loggateway.NewNoop())

	out, err := engine.Synthesize(context.Background(), SynthesisInput{
		TeamResults: []TeamSynthesisResult{
			{TeamID: "t1", TeamName: "团队A", Status: "completed", Summary: "摘要A"},
		},
		Strategy: SynthesisStrategyTemplate,
	})
	if err != nil {
		t.Fatalf("Synthesize returned error: %v", err)
	}
	if stub.called {
		t.Fatal("model port should NOT be called for template strategy")
	}
	if !strings.Contains(out.Content, "团队A") {
		t.Fatalf("template output should contain team name, got %q", out.Content)
	}
}
