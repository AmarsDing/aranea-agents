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

func TestDetectIntentPaths_EmptyQuery(t *testing.T) {
	paths := DetectIntentPaths("", 3)
	if paths != nil {
		t.Fatalf("expected nil for empty query, got %v", paths)
	}
}

func TestDetectIntentPaths_SingleKeyword(t *testing.T) {
	paths := DetectIntentPaths("帮我发邮件", 3)
	if len(paths) == 0 {
		t.Fatal("expected at least one path for keyword '邮件'")
	}
	found := false
	for _, p := range paths {
		if p == `交互与执行/消息发送（发邮件）` {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected path '交互与执行/消息发送（发邮件）', got %v", paths)
	}
}

func TestDetectIntentPaths_NoMatch(t *testing.T) {
	paths := DetectIntentPaths("zzzzz_no_match_at_all", 3)
	if len(paths) != 0 {
		t.Fatalf("expected empty for no-match query, got %v", paths)
	}
}

func TestExtractTagHints_EmptyInput(t *testing.T) {
	h := ExtractTagHints("")
	if h != nil {
		t.Fatalf("expected nil for empty input, got %v", h)
	}
}

func TestExtractTagHints_DuplicateTokens(t *testing.T) {
	q := "domain:sales domain:sales file_type:xlsx file_type:xlsx"
	h := ExtractTagHints(q)
	got := map[string]int{}
	for _, tok := range h {
		got[tok]++
	}
	for tok, count := range got {
		if count > 1 {
			t.Errorf("duplicate token %q appears %d times", tok, count)
		}
	}
	if len(h) != 2 {
		t.Fatalf("expected 2 unique tokens, got %d: %v", len(h), h)
	}
}

func TestTaxonomyLeaves_NonEmpty(t *testing.T) {
	leaves := TaxonomyLeaves()
	if len(leaves) == 0 {
		t.Fatal("TaxonomyLeaves() returned empty slice")
	}
	for i, leaf := range leaves {
		if leaf.Path == "" {
			t.Errorf("leaf[%d] has empty Path", i)
		}
		if len(leaf.Keywords) == 0 {
			t.Errorf("leaf[%d] has no Keywords", i)
		}
	}
}
