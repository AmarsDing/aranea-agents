package skillrouter

import "testing"

func TestDetectIntentPaths_salesFeedbackEmail(t *testing.T) {
	q := `读 sales.xlsx 里客户反馈，做情感分析，把负面反馈发邮件给我`
	paths := DetectIntentPaths(q, 3)
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %v", paths)
	}
	want := map[string]bool{
		`数据获取与集成/内部数据源/文件系统读取（读取表格）`: true,
		`分析与推理/自然语言理解（情感分析）`:         true,
		`交互与执行/消息发送（发邮件）`:            true,
	}
	for _, p := range paths {
		if !want[p] {
			t.Errorf("unexpected path %q", p)
		}
	}
}

func TestExtractTagHints(t *testing.T) {
	q := `domain:sales file_type:xlsx 附件是报表`
	h := ExtractTagHints(q)
	if len(h) != 2 {
		t.Fatalf("got %v", h)
	}
	got := map[string]bool{}
	for _, x := range h {
		got[x] = true
	}
	if !got["domain:sales"] || !got["file_type:xlsx"] {
		t.Fatalf("missing tokens: %v", h)
	}
}
