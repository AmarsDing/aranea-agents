package agent

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type fakeSpiritAuxRecorder struct {
	calls []biz.AuxLLMUsageInput
	err   error
}

func (f *fakeSpiritAuxRecorder) RecordAuxLLMUsage(_ context.Context, in biz.AuxLLMUsageInput) error {
	f.calls = append(f.calls, in)
	return f.err
}

func TestRecordSpiritAuxUsage_NilRecorderSkips(t *testing.T) {
	// 旧构造路径：recorder 为 nil 时整体跳过，不 panic。
	recordSpiritAuxUsage(context.Background(), nil, loggateway.NewNoop(), "step", biz.AuxLLMUsageInput{
		Kind: biz.UsageKindAuxPlannerDecompose, PromptTok: 10, CompletionTok: 5,
	})
}

func TestRecordSpiritAuxUsage_ZeroTokensSkips(t *testing.T) {
	rec := &fakeSpiritAuxRecorder{}
	recordSpiritAuxUsage(context.Background(), rec, loggateway.NewNoop(), "step", biz.AuxLLMUsageInput{
		Kind: biz.UsageKindAuxAllocatorMatch,
	})
	if len(rec.calls) != 0 {
		t.Fatalf("zero-token call must be skipped, got %d calls", len(rec.calls))
	}
}

func TestRecordSpiritAuxUsage_RecordsWithEffort(t *testing.T) {
	rec := &fakeSpiritAuxRecorder{}
	in := biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxPlannerDecompose,
		SessionID:     "sess-1",
		RunID:         "plan-1",
		Provider:      "deepseek",
		Model:         "deepseek-chat",
		Status:        biz.TokenUsageStatusSuccess,
		PromptTok:     120,
		CompletionTok: 30,
		UsageSource:   UsageSourceStreaming,
		Effort:        biz.ThinkingEffortMax,
	}
	recordSpiritAuxUsage(context.Background(), rec, loggateway.NewNoop(), "step", in)
	if len(rec.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(rec.calls))
	}
	got := rec.calls[0]
	if got.Kind != biz.UsageKindAuxPlannerDecompose || got.Effort != biz.ThinkingEffortMax {
		t.Fatalf("kind/effort = %q/%q", got.Kind, got.Effort)
	}
	if got.PromptTok != 120 || got.CompletionTok != 30 {
		t.Fatalf("tokens = %d/%d", got.PromptTok, got.CompletionTok)
	}
}

func TestRecordSpiritAuxUsage_RecorderErrorSwallowed(t *testing.T) {
	rec := &fakeSpiritAuxRecorder{err: errors.New("db down")}
	// best-effort：落账失败不得 panic / 不得向上传播。
	recordSpiritAuxUsage(context.Background(), rec, loggateway.NewNoop(), "step", biz.AuxLLMUsageInput{
		Kind: biz.UsageKindAuxAllocatorMatch, PromptTok: 1,
	})
	if len(rec.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(rec.calls))
	}
}

func TestSpiritAuxCallStatus(t *testing.T) {
	if got := spiritAuxCallStatus(nil); got != biz.TokenUsageStatusSuccess {
		t.Fatalf("nil err status = %q", got)
	}
	if got := spiritAuxCallStatus(errors.New("boom")); got != "failed" {
		t.Fatalf("err status = %q", got)
	}
}

func TestAttachAuxUsageRecorder(t *testing.T) {
	rec := &fakeSpiritAuxRecorder{}

	var p biz.TaskPlannerPort = &taskPlannerImpl{}
	AttachPlannerAuxUsageRecorder(p, rec)
	if p.(*taskPlannerImpl).auxUsage != rec {
		t.Fatal("planner auxUsage not attached")
	}
	// 非 taskPlannerImpl 实现（或 nil 接收者语义外的类型）静默跳过。
	AttachPlannerAuxUsageRecorder(nil, rec)

	var a biz.AgentAllocatorPort = &agentAllocatorImpl{}
	AttachAllocatorAuxUsageRecorder(a, rec)
	if a.(*agentAllocatorImpl).auxUsage != rec {
		t.Fatal("allocator auxUsage not attached")
	}
	AttachAllocatorAuxUsageRecorder(nil, rec)
}
