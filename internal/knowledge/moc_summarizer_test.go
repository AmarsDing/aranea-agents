package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── G-34 ADR-C：MOC 摘要生成器（生产实现）契约 ──────────────────────────────
// hub 居首装帧 + 成员保序 + 截断提示注入；摘要纯文本剥包裹 + 300 runes 上限；
// usage 成败均记（CompletionTok=totalTokens）；LLM 失败上抛 error 由 biz 降级。

type stubMOCUsageRecorder struct {
	calls int
	last  MOCUsageInput
}

func (s *stubMOCUsageRecorder) RecordMOCSummaryUsage(_ context.Context, in MOCUsageInput) {
	s.calls++
	s.last = in
}

func mocSummaryTestRequest() bizknowledge.MOCSummaryRequest {
	return bizknowledge.MOCSummaryRequest{
		CollectionID: "c1", DocID: "d-moc", RelPath: "moc/发布体系.md", Title: "发布体系",
		Hub: bizknowledge.MOCSummaryExcerpt{RelPath: "entries/发布体系.md", Body: "发布体系统领灰度/回滚/告警三要素。"},
		Members: []bizknowledge.MOCSummaryExcerpt{
			{RelPath: "entries/回滚.md", Body: "回滚预案保证发布可逆。"},
			{RelPath: "entries/灰度.md", Body: "灰度发布按比例逐步放量。"},
		},
		Truncated: true,
	}
}

// 装帧契约：主题/hub 居首/成员保序/截断提示；产出剥包裹；usage 记账成功路径。
func TestMOCSummarizer_SummarizesAndRecords(t *testing.T) {
	t.Parallel()
	llm := &stubRelationLLM{responses: []string{"「本簇围绕发布全链路，灰度放量、回滚兜底。」  "}, tokens: 42}
	rec := &stubMOCUsageRecorder{}
	s := NewMOCSummarizer(llm, stubRelationSys{}, nil, nil)
	s.SetUsageRecorder(rec)

	got, err := s.SummarizeMOC(context.Background(), mocSummaryTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	// 包裹符号/首尾空白剥除。
	if got.Summary != "本簇围绕发布全链路，灰度放量、回滚兜底。" {
		t.Fatalf("summary = %q", got.Summary)
	}
	user := llm.lastUser
	// hub 居首、成员保序（回滚先于灰度）、截断提示带注入。
	hi := strings.Index(user, "核心词条（hub）《发布体系》")
	ri := strings.Index(user, "关联词条《回滚》")
	gi := strings.Index(user, "关联词条《灰度》")
	if !strings.Contains(user, "MOC 主题：发布体系") || hi < 0 || ri < 0 || gi < 0 ||
		!(hi < ri && ri < gi) {
		t.Fatalf("prompt framing broken:\n%s", user)
	}
	if !strings.Contains(user, "因长度预算被截断") {
		t.Fatalf("truncation note missing:\n%s", user)
	}
	// usage：成功路径记 totalTokens → Tokens、无错误。
	if rec.calls != 1 || rec.last.Provider != "p" || rec.last.Model != "m" ||
		rec.last.DocID != "d-moc" || rec.last.Tokens != 42 || rec.last.Err != nil {
		t.Fatalf("usage = %+v calls=%d", rec.last, rec.calls)
	}
}

// F12 终审钉：provider 漏报 usage（零 token）成功行不记账——幻影成功会虚报
// 调用量；title/memory/embed 三先例同纪律。失败行（零 token）仍照记（见上）。
func TestMOCSummarizer_ZeroTokenSuccessSkipsUsage(t *testing.T) {
	t.Parallel()
	llm := &stubRelationLLM{responses: []string{"本簇摘要。"}, tokens: 0}
	rec := &stubMOCUsageRecorder{}
	s := NewMOCSummarizer(llm, stubRelationSys{}, nil, nil)
	s.SetUsageRecorder(rec)

	if _, err := s.SummarizeMOC(context.Background(), mocSummaryTestRequest()); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("zero-token success must not be recorded: %+v calls=%d", rec.last, rec.calls)
	}
}

// 失控长文截断至 300 runes；空白产出上抛 error（biz 侧占位保留重试）。
func TestMOCSummarizer_CapsAndRejectsBlank(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("摘", mocSummaryMaxRunes+100)
	s := NewMOCSummarizer(&stubRelationLLM{responses: []string{long}}, stubRelationSys{}, nil, nil)
	got, err := s.SummarizeMOC(context.Background(), mocSummaryTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(got.Summary)) > mocSummaryMaxRunes {
		t.Fatalf("summary not capped: %d runes", len([]rune(got.Summary)))
	}

	s2 := NewMOCSummarizer(&stubRelationLLM{responses: []string{"   "}}, stubRelationSys{}, nil, nil)
	if _, err := s2.SummarizeMOC(context.Background(), mocSummaryTestRequest()); err == nil {
		t.Fatal("blank summary must be an error (biz keeps placeholder for retry)")
	}
}

// LLM 失败：error 上抛 + usage 失败行仍记（Tokens=0 Err 埋点）。
func TestMOCSummarizer_LLMFailurePropagatesAndRecords(t *testing.T) {
	t.Parallel()
	boom := errors.New("llm down")
	llm := &stubRelationLLM{err: boom}
	rec := &stubMOCUsageRecorder{}
	s := NewMOCSummarizer(llm, stubRelationSys{}, nil, nil)
	s.SetUsageRecorder(rec)

	if _, err := s.SummarizeMOC(context.Background(), mocSummaryTestRequest()); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if rec.calls != 1 || rec.last.Err == nil || rec.last.Tokens != 0 {
		t.Fatalf("failure usage = %+v calls=%d", rec.last, rec.calls)
	}
}

// 未接线：nil llm 显式报错（biz 侧 Warn 降级不中断整轮）。
func TestMOCSummarizer_NotWired(t *testing.T) {
	t.Parallel()
	s := NewMOCSummarizer(nil, stubRelationSys{}, nil, nil)
	if _, err := s.SummarizeMOC(context.Background(), mocSummaryTestRequest()); err == nil {
		t.Fatal("nil llm must error")
	}
}
