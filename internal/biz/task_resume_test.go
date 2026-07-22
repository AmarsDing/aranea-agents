package biz

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildTaskResumeTrace_Empty(t *testing.T) {
	if got := BuildTaskResumeTrace(nil); got != "" {
		t.Fatalf("empty steps: got %q, want \"\"", got)
	}
	// No completed action/reply → no trace.
	steps := []Step{
		{Kind: StepKindThinking, Status: StepStatusCompleted, Content: "思考"},
		{Kind: StepKindAction, Status: StepStatusFailed, ToolName: "run_command"},
		{Kind: StepKindReply, Status: StepStatusRunning, Content: "半成品"},
	}
	if got := BuildTaskResumeTrace(steps); got != "" {
		t.Fatalf("no completed action/reply: got %q, want \"\"", got)
	}
}

func TestBuildTaskResumeTrace_OrdersAndFormats(t *testing.T) {
	base := time.Now().UTC()
	steps := []Step{
		{Kind: StepKindReply, Status: StepStatusCompleted, Content: "初步分析完成", StartedAt: base.Add(2 * time.Second), Seq: 2},
		{Kind: StepKindAction, Status: StepStatusCompleted, ToolName: "search_codebase", ToolArgs: json.RawMessage(`{"query":"auth"}`), StartedAt: base.Add(time.Second), Seq: 1},
		{Kind: StepKindNotice, Status: StepStatusCompleted, Content: "噪音不入轨迹", StartedAt: base.Add(3 * time.Second), Seq: 3},
	}
	got := BuildTaskResumeTrace(steps)
	if !strings.Contains(got, "1. [工具] search_codebase(") {
		t.Fatalf("missing action entry: %q", got)
	}
	if !strings.Contains(got, "2. [回复] 初步分析完成") {
		t.Fatalf("missing reply entry: %q", got)
	}
	if strings.Contains(got, "噪音不入轨迹") {
		t.Fatalf("notice step leaked into trace: %q", got)
	}
	// action (t+1s) must come before reply (t+2s).
	if strings.Index(got, "[工具]") > strings.Index(got, "[回复]") {
		t.Fatalf("order wrong: %q", got)
	}
}

func TestBuildTaskResumeTrace_TruncatesLongLists(t *testing.T) {
	base := time.Now().UTC()
	steps := make([]Step, 0, resumeTraceMaxEntries+5)
	for i := 0; i < resumeTraceMaxEntries+5; i++ {
		steps = append(steps, Step{
			Kind: StepKindReply, Status: StepStatusCompleted,
			Content:   "entry",
			StartedAt: base.Add(time.Duration(i) * time.Second), Seq: int64(i + 1),
		})
	}
	got := BuildTaskResumeTrace(steps)
	if !strings.Contains(got, "仅保留最近") {
		t.Fatalf("expected truncation header: %q", got)
	}
	if n := strings.Count(got, "[回复]"); n != resumeTraceMaxEntries {
		t.Fatalf("entries=%d, want %d", n, resumeTraceMaxEntries)
	}
}

func TestInterruptedResumeUserContent(t *testing.T) {
	withTrace := InterruptedResumeUserContent("帮我修复登录 bug", "1. [工具] x → 成功")
	if !strings.Contains(withTrace, "执行轨迹") || !strings.Contains(withTrace, "原始任务：\n帮我修复登录 bug") {
		t.Fatalf("with trace: %q", withTrace)
	}
	noTrace := InterruptedResumeUserContent("帮我修复登录 bug", "")
	if !strings.Contains(noTrace, "重新执行用户的原始任务") || !strings.HasSuffix(noTrace, "帮我修复登录 bug") {
		t.Fatalf("without trace: %q", noTrace)
	}
}
