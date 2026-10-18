package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ── 审计 W5② 测试桩：报告分析队列重试/死信链路的轻量 mock ─────────────────────

// mockUnanalyzedReaderW5 记录 MarkAnalyzed / RecordAnalysisFailure 的调用面。
type mockUnanalyzedReaderW5 struct {
	invs         []SkillInvocationWrite
	marked       []string
	failCalls    map[string]int
	failMessages map[string]string
}

func (m *mockUnanalyzedReaderW5) ListUnanalyzed(_ context.Context, _ int) ([]SkillInvocationWrite, error) {
	return m.invs, nil
}

func (m *mockUnanalyzedReaderW5) MarkAnalyzed(_ context.Context, id string) error {
	m.marked = append(m.marked, id)
	return nil
}

func (m *mockUnanalyzedReaderW5) RecordAnalysisFailure(_ context.Context, activationID, errMsg string) error {
	if m.failCalls == nil {
		m.failCalls = map[string]int{}
		m.failMessages = map[string]string{}
	}
	m.failCalls[activationID]++
	m.failMessages[activationID] = errMsg
	return nil
}

// selectiveFailWriter 对指定 invocationID 返回写入失败，其余成功——用于构造
// 「同行批内毒行 + 正常行」混合场景。
type selectiveFailWriter struct {
	failFor map[string]bool
	created int
}

func (w *selectiveFailWriter) Create(_ context.Context, report ExperienceReport) error {
	if w.failFor[report.InvocationID] {
		return errors.New("simulated write failure")
	}
	w.created++
	return nil
}

func (w *selectiveFailWriter) BatchCreate(_ context.Context, _ []ExperienceReport) error {
	return nil
}

// 审计 W5② 验收：GenerateReport 失败的毒行必须落失败计数（RecordAnalysisFailure），
// 不得 MarkAnalyzed；毒行不得阻断同批其他正常行的标记。
func TestScanAndGenerateReports_PoisonRowRecordsFailure(t *testing.T) {
	reader := &mockUnanalyzedReaderW5{
		invs: []SkillInvocationWrite{
			{ActivationID: "act-poison-1", SkillID: "skill-a", Outcome: "failure"},
			{ActivationID: "act-poison-2", SkillID: "skill-b", Outcome: "failure"},
		},
	}
	writer := &selectiveFailWriter{failFor: map[string]bool{"act-poison-1": true, "act-poison-2": true}}

	uc := NewSkillReportUsecase(nil, writer, nil, nil, nil, loggateway.NewNoop())
	uc.SetUnanalyzedReader(reader)

	if err := uc.ScanAndGenerateReports(context.Background()); err != nil {
		t.Fatalf("ScanAndGenerateReports: %v", err)
	}

	if len(reader.marked) != 0 {
		t.Fatalf("poison rows must not be MarkAnalyzed, marked=%v", reader.marked)
	}
	for _, id := range []string{"act-poison-1", "act-poison-2"} {
		if reader.failCalls[id] != 1 {
			t.Fatalf("expected 1 RecordAnalysisFailure for %s, got %d", id, reader.failCalls[id])
		}
		if reader.failMessages[id] == "" {
			t.Fatalf("expected non-empty failure message for %s", id)
		}
	}
}

// 审计 W5② 附：毒行不队头阻塞——同批正常行照常 GenerateReport + MarkAnalyzed。
func TestScanAndGenerateReports_PoisonRowDoesNotBlockBatch(t *testing.T) {
	reader := &mockUnanalyzedReaderW5{
		invs: []SkillInvocationWrite{
			{ActivationID: "act-poison", SkillID: "skill-a", Outcome: "failure"},
			{ActivationID: "act-ok", SkillID: "skill-b", Outcome: "success"},
		},
	}
	writer := &selectiveFailWriter{failFor: map[string]bool{"act-poison": true}}

	uc := NewSkillReportUsecase(nil, writer, nil, nil, nil, loggateway.NewNoop())
	uc.SetUnanalyzedReader(reader)

	if err := uc.ScanAndGenerateReports(context.Background()); err != nil {
		t.Fatalf("ScanAndGenerateReports: %v", err)
	}

	if len(reader.marked) != 1 || reader.marked[0] != "act-ok" {
		t.Fatalf("expected act-ok marked, marked=%v", reader.marked)
	}
	if writer.created != 1 {
		t.Fatalf("expected 1 report persisted, got %d", writer.created)
	}
	if reader.failCalls["act-poison"] != 1 {
		t.Fatalf("expected 1 failure record for act-poison, got %d", reader.failCalls["act-poison"])
	}
}
