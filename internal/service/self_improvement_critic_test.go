package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// fakeLLMCaller records requests and returns scripted responses.
type fakeLLMCaller struct {
	reqs []biz.LLMCallRequest
	resp string
	err  error
}

func (f *fakeLLMCaller) Call(_ context.Context, req biz.LLMCallRequest) (string, int, error) {
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return "", 0, f.err
	}
	return f.resp, 10, nil
}

func siCriticFixture(resp string, dailyMax int32) (*SICriticAgent, *fakeLLMCaller) {
	caller := &fakeLLMCaller{resp: resp}
	return NewSICriticAgent(caller, "openai", "gpt-x", dailyMax, loggateway.NewNoop()), caller
}

func siCriticArgs() (*biz.SelfImprovementRun, *biz.PatcherOutput) {
	run := &biz.SelfImprovementRun{ID: "run-1", TriggerSource: biz.TriggerSourceErrorCluster}
	patch := &biz.PatcherOutput{Diff: "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -0,0 +1,1 @@\n+fix\n", Kind: biz.PatchKindCode}
	return run, patch
}

func TestSICriticAgent_ReviewSuccess(t *testing.T) {
	agent, caller := siCriticFixture(`{"is_safe":true,"risk_level":"low","concerns":[],"suggestion":"ok"}`, 10)
	run, patch := siCriticArgs()
	report, err := agent.Review(context.Background(), run, patch)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if !report.IsSafe || report.RiskLevel != "low" {
		t.Errorf("unexpected report: %+v", report)
	}
	if len(caller.reqs) != 1 {
		t.Fatalf("caller reqs = %d, want 1", len(caller.reqs))
	}
	req := caller.reqs[0]
	if req.Provider != "openai" || req.Model != "gpt-x" {
		t.Errorf("provider/model not honored: %+v", req)
	}
	if req.System != biz.SICriticSystemPrompt {
		t.Error("system prompt must be biz.SICriticSystemPrompt")
	}
	if !strings.Contains(req.User, patch.Diff) {
		t.Error("user message must contain the diff")
	}
}

func TestSICriticAgent_ReviewInvalidJSON(t *testing.T) {
	agent, _ := siCriticFixture(`not json`, 10)
	run, patch := siCriticArgs()
	if _, err := agent.Review(context.Background(), run, patch); err == nil {
		t.Fatal("want parse error")
	}
}

func TestSICriticAgent_ReviewLLMError(t *testing.T) {
	agent, caller := siCriticFixture("", 10)
	caller.err = errors.New("llm down")
	run, patch := siCriticArgs()
	if _, err := agent.Review(context.Background(), run, patch); err == nil {
		t.Fatal("want LLM error propagated")
	}
}

func TestSICriticAgent_DailyQuotaExhaustedDegrades(t *testing.T) {
	agent, caller := siCriticFixture(`{"is_safe":true,"risk_level":"low"}`, 2)
	run, patch := siCriticArgs()
	for i := 0; i < 2; i++ {
		if _, err := agent.Review(context.Background(), run, patch); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	// 第 3 次：配额耗尽 → 哨兵错误，且不再调用 LLM
	_, err := agent.Review(context.Background(), run, patch)
	if !errors.Is(err, ErrSICriticQuotaExceeded) {
		t.Fatalf("err = %v, want ErrSICriticQuotaExceeded", err)
	}
	if len(caller.reqs) != 2 {
		t.Errorf("caller reqs = %d, want 2（配额耗尽不得再调 LLM）", len(caller.reqs))
	}
	// 窗口推进 24h 后配额恢复
	agent.nowOffsetSec.Store(int64(25 * 3600))
	if _, err := agent.Review(context.Background(), run, patch); err != nil {
		t.Fatalf("after window reset: %v", err)
	}
}

func TestSICriticAgent_NilCaller(t *testing.T) {
	agent := NewSICriticAgent(nil, "openai", "gpt-x", 10, loggateway.NewNoop())
	run, patch := siCriticArgs()
	if _, err := agent.Review(context.Background(), run, patch); err == nil {
		t.Fatal("nil caller must error, not panic")
	}
}

func TestSICriticAgent_DiffTruncated(t *testing.T) {
	agent, caller := siCriticFixture(`{"is_safe":true,"risk_level":"low"}`, 10)
	run, patch := siCriticArgs()
	patch.Diff = strings.Repeat("+x\n", 100000) // ~300KB
	if _, err := agent.Review(context.Background(), run, patch); err != nil {
		t.Fatalf("Review: %v", err)
	}
	if got := len(caller.reqs[0].User); got > 128*1024 {
		t.Errorf("user message too large: %d bytes, want bounded", got)
	}
}
