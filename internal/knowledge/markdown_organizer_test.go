package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type mockOrganizeLLM struct {
	resp  string
	err   error
	calls int
	users []string
	sys   string
}

func (m *mockOrganizeLLM) Call(_ context.Context, req biz.LLMCallRequest) (string, int, error) {
	m.calls++
	m.sys = req.System
	m.users = append(m.users, req.User)
	if m.err != nil {
		return "", 0, m.err
	}
	return m.resp, 0, nil
}

type stubSysGetter struct {
	s   biz.SystemSetting
	err error
}

func (s stubSysGetter) Get(context.Context) (biz.SystemSetting, error) { return s.s, s.err }

type stubCatalogLister struct {
	models []biz.ProviderModel
	err    error
}

func (c stubCatalogLister) List(context.Context) ([]biz.ProviderModel, error) {
	return c.models, c.err
}

func newOrganizerWithModel(llm biz.LLMCaller) *MarkdownOrganizer {
	return &MarkdownOrganizer{
		llm: llm,
		sys: stubSysGetter{s: biz.SystemSetting{
			DefaultRefineLLM: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o-mini"},
		}},
		lg: loggateway.NewNoop(),
	}
}

func TestMarkdownOrganizerNilLLM(t *testing.T) {
	o := NewMarkdownOrganizer(nil, nil, nil, loggateway.NewNoop())
	md, organized, err := o.Organize(context.Background(), "raw text", "a.txt", "text/plain")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if organized {
		t.Error("organized = true, want false for nil LLM")
	}
	if md != "raw text" {
		t.Errorf("md = %q, want passthrough", md)
	}
}

func TestMarkdownOrganizerNilReceiver(t *testing.T) {
	var o *MarkdownOrganizer
	md, organized, err := o.Organize(context.Background(), "raw text", "a.txt", "text/plain")
	if err != nil || organized || md != "raw text" {
		t.Errorf("err=%v organized=%v md=%q, want nil/false/passthrough", err, organized, md)
	}
}

func TestMarkdownOrganizerEmptyText(t *testing.T) {
	o := newOrganizerWithModel(&mockOrganizeLLM{resp: "# x"})
	md, organized, err := o.Organize(context.Background(), "   ", "a.txt", "text/plain")
	if err != nil || organized {
		t.Fatalf("err=%v organized=%v, want nil/false", err, organized)
	}
	if md != "   " {
		t.Errorf("md = %q, want passthrough", md)
	}
}

func TestMarkdownOrganizerNoModelDegrades(t *testing.T) {
	// sys/catalog 均无法提供模型 → ResolveLLM 失败 → 降级透传不调 LLM。
	llm := &mockOrganizeLLM{resp: "# 整理"}
	o := NewMarkdownOrganizer(llm, nil, nil, loggateway.NewNoop())
	md, organized, err := o.Organize(context.Background(), "raw", "a.txt", "text/plain")
	if err != nil {
		t.Fatalf("err = %v, want nil (degrade, not fail)", err)
	}
	if organized {
		t.Error("organized = true, want false when no LLM model resolvable")
	}
	if md != "raw" {
		t.Errorf("md = %q, want passthrough", md)
	}
	if llm.calls != 0 {
		t.Errorf("llm.calls = %d, want 0 (no model resolved)", llm.calls)
	}
}

func TestMarkdownOrganizerSuccess(t *testing.T) {
	llm := &mockOrganizeLLM{resp: "# 标题\n\n正文内容"}
	o := newOrganizerWithModel(llm)
	md, organized, err := o.Organize(context.Background(), "标题\n正文内容", "a.txt", "text/plain")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !organized {
		t.Error("organized = false, want true")
	}
	if md != "# 标题\n\n正文内容" {
		t.Errorf("md = %q", md)
	}
	if llm.calls != 1 {
		t.Errorf("llm.calls = %d, want 1", llm.calls)
	}
	if !strings.Contains(llm.sys, "Markdown") {
		t.Errorf("system prompt should mention Markdown, got %q", llm.sys)
	}
}

func TestMarkdownOrganizerLLMErrorDegrades(t *testing.T) {
	llm := &mockOrganizeLLM{err: errors.New("llm boom")}
	o := newOrganizerWithModel(llm)
	md, organized, err := o.Organize(context.Background(), "raw text", "a.txt", "text/plain")
	if err != nil {
		t.Fatalf("err = %v, want nil (degrade)", err)
	}
	if organized || md != "raw text" {
		t.Errorf("organized=%v md=%q, want false/passthrough", organized, md)
	}
}

func TestMarkdownOrganizerEmptyResponseDegrades(t *testing.T) {
	llm := &mockOrganizeLLM{resp: "   "}
	o := newOrganizerWithModel(llm)
	md, organized, err := o.Organize(context.Background(), "raw text", "a.txt", "text/plain")
	if err != nil || organized || md != "raw text" {
		t.Errorf("err=%v organized=%v md=%q, want nil/false/passthrough", err, organized, md)
	}
}

func TestMarkdownOrganizerStripsCodeFence(t *testing.T) {
	llm := &mockOrganizeLLM{resp: "```markdown\n# 标题\n正文\n```"}
	o := newOrganizerWithModel(llm)
	md, organized, err := o.Organize(context.Background(), "raw", "a.txt", "text/plain")
	if err != nil || !organized {
		t.Fatalf("err=%v organized=%v", err, organized)
	}
	if strings.Contains(md, "```") {
		t.Errorf("md should have code fence stripped, got %q", md)
	}
	if !strings.Contains(md, "# 标题") {
		t.Errorf("md = %q, want organized content", md)
	}
}

func TestMarkdownOrganizerLongInputWindowed(t *testing.T) {
	// 构造超过单窗口的输入。
	var sb strings.Builder
	for sb.Len() < organizeWindowChars+100 {
		sb.WriteString("这是需要整理的一段较长文本内容，重复填充以超过窗口阈值。\n")
	}
	long := sb.String()

	llm := &mockOrganizeLLM{resp: "整理段"}
	o := newOrganizerWithModel(llm)
	md, organized, err := o.Organize(context.Background(), long, "big.txt", "text/plain")
	if err != nil || !organized {
		t.Fatalf("err=%v organized=%v", err, organized)
	}
	if llm.calls < 2 {
		t.Errorf("llm.calls = %d, want >= 2 (windowed)", llm.calls)
	}
	// 拼接结果应包含各窗口整理输出。
	if strings.Count(md, "整理段") != llm.calls {
		t.Errorf("md windows count = %d, llm.calls = %d", strings.Count(md, "整理段"), llm.calls)
	}
	// 每个窗口输入都不应超过阈值。
	for i, u := range llm.users {
		if len(u) > organizeWindowChars {
			t.Errorf("window %d input len = %d, exceeds %d", i, len(u), organizeWindowChars)
		}
	}
}

func TestSplitOrganizeWindows(t *testing.T) {
	short := "short text"
	if got := splitOrganizeWindows(short, 100); len(got) != 1 || got[0] != short {
		t.Errorf("short input windows = %v", got)
	}

	// 行边界切分：每行 11 字节，窗口 25 → 每窗 2 行。
	text := "aaaaaaaaaa\nbbbbbbbbbb\ncccccccccc\ndddddddddd\n"
	wins := splitOrganizeWindows(text, 25)
	if len(wins) != 2 {
		t.Fatalf("windows = %d, want 2: %q", len(wins), wins)
	}
	if wins[0] != "aaaaaaaaaa\nbbbbbbbbbb\n" {
		t.Errorf("win[0] = %q", wins[0])
	}
	if wins[1] != "cccccccccc\ndddddddddd\n" {
		t.Errorf("win[1] = %q", wins[1])
	}

	// 单行超窗：单独成窗不丢弃。
	longLine := strings.Repeat("x", 50) + "\nshort\n"
	wins = splitOrganizeWindows(longLine, 25)
	if len(wins) != 2 {
		t.Fatalf("windows = %d, want 2: %q", len(wins), wins)
	}
}
