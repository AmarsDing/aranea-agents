package deferred

import (
	"strings"
	"testing"
)

// P1-4：deferred 工具语义预激活——按当前用户 query 对 catalog 做轻量相关度
// 排序，Top-N 以「推荐区」提升进 catalog cue，提高模型发现率（命中率）。
// 排序器与 tool_search 共享同一打分逻辑，保证「搜索看到的」与「推荐的」一致。

func TestRankCatalogEntries_EmptyQuery(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
	}
	if got := RankCatalogEntries(catalog, "", 3); got != nil {
		t.Errorf("empty query must yield nil, got %v", got)
	}
	if got := RankCatalogEntries(catalog, "   ", 3); got != nil {
		t.Errorf("blank query must yield nil, got %v", got)
	}
}

func TestRankCatalogEntries_NoMatch(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
	}
	if got := RankCatalogEntries(catalog, "zzz qqq nothing", 3); got != nil {
		t.Errorf("no-match query must yield nil, got %v", got)
	}
}

func TestRankCatalogEntries_RanksNameAboveDescription(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "alpha_tool", BaseName: "alpha_tool", Description: "does browser things", Category: "x"},
		{Name: "browser_navigate", BaseName: "browser_navigate", Description: "Navigate browser", Category: "browser"},
	}
	got := RankCatalogEntries(catalog, "browser", 3)
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
	if got[0].Name != "browser_navigate" {
		t.Errorf("name match must outrank description match, got first=%q", got[0].Name)
	}
}

func TestRankCatalogEntries_SubwordMatch(t *testing.T) {
	// CJK/自然语言 query 不会按工具名下划线分词：name 子词作为子串命中
	// query 也应计分（如 "please save this file" 命中 file_save_file）。
	catalog := []DeferredToolEntry{
		{Name: "file_save_file", BaseName: "save_file", Description: "Save content to a file", Category: "file"},
		{Name: "shell_exec", BaseName: "shell_exec", Description: "Execute shell commands", Category: "runtime"},
	}
	got := RankCatalogEntries(catalog, "please save this for me", 3)
	if len(got) != 1 || got[0].Name != "file_save_file" {
		t.Fatalf("subword match must rank file_save_file, got %v", got)
	}
}

func TestRankCatalogEntries_ShortTokenNoiseGuard(t *testing.T) {
	// 短虚词（<3 runes）不参与子串匹配："me" 不得子串命中 category
	// "runtime"，"go" 不得子串命中 name "golang"。精确等值不受限。
	catalog := []DeferredToolEntry{
		{Name: "shell_exec", BaseName: "shell_exec", Description: "Execute shell commands", Category: "runtime"},
		{Name: "golang_fmt", BaseName: "golang_fmt", Description: "Format Go code", Category: "coding"},
	}
	if got := RankCatalogEntries(catalog, "tell me about it", 3); got != nil {
		t.Errorf("short token 'me' must not substring-match, got %v", got)
	}
	if got := RankCatalogEntries(catalog, "go", 3); got != nil {
		t.Errorf("short token 'go' must not substring-match golang_fmt, got %v", got)
	}
}

func TestRankCatalogEntries_NameOutranksDescriptionSpam(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "alpha_misc", BaseName: "alpha_misc", Description: "browser browser browser browser", Category: "x"},
		{Name: "browser_navigate", BaseName: "browser_navigate", Description: "open a page", Category: "browser"},
	}
	got := RankCatalogEntries(catalog, "browser navigate", 2)
	if len(got) == 0 || got[0].Name != "browser_navigate" {
		t.Fatalf("BM25 name field must beat description spam, got %v", namesOf(got))
	}
}

func TestResolveToolHints_PrefersLLMThenRank(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "diff_edit", BaseName: "diff_edit", Description: "patch files", Category: "file"},
		{Name: "web_fetch", BaseName: "web_fetch", Description: "http get", Category: "web"},
	}
	got := ResolveToolHints(catalog, "download a page", []string{"web_fetch", "nope"}, 2)
	if len(got) < 1 || got[0] != "web_fetch" {
		t.Fatalf("llm hint must come first, got %v", got)
	}
}

func namesOf(entries []DeferredToolEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func TestRankCatalogEntries_Limit(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_a", BaseName: "web_a", Description: "web a", Category: "web"},
		{Name: "web_b", BaseName: "web_b", Description: "web b", Category: "web"},
		{Name: "web_c", BaseName: "web_c", Description: "web c", Category: "web"},
		{Name: "web_d", BaseName: "web_d", Description: "web d", Category: "web"},
	}
	got := RankCatalogEntries(catalog, "web", 2)
	if len(got) != 2 {
		t.Fatalf("limit=2 must cap results, got %d", len(got))
	}
}

func TestRankCatalogEntries_Deterministic(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_b", BaseName: "web_b", Description: "web b", Category: "web"},
		{Name: "web_a", BaseName: "web_a", Description: "web a", Category: "web"},
	}
	first := RankCatalogEntries(catalog, "web", 3)
	second := RankCatalogEntries(catalog, "web", 3)
	if len(first) != len(second) {
		t.Fatalf("non-deterministic length: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Name != second[i].Name {
			t.Fatalf("non-deterministic order at %d: %q vs %q", i, first[i].Name, second[i].Name)
		}
	}
	// 同分时按名字典序，保证 cue 字节稳定。
	if len(first) == 2 && first[0].Name != "web_a" {
		t.Errorf("tie must break by name asc, got first=%q", first[0].Name)
	}
}

func TestRenderCatalogCueWithRecommendations_NoRecommendedMatchesStatic(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
		{Name: "shell_exec", BaseName: "shell_exec", Description: "Execute shell commands", Category: "runtime"},
	}
	dynamic := RenderCatalogCueWithRecommendations(catalog, nil)
	static := RenderCatalogCue(catalog)
	if dynamic != static {
		t.Errorf("no recommendations must render byte-identical to static cue.\nstatic:\n%s\ndynamic:\n%s", static, dynamic)
	}
}

func TestRenderCatalogCueWithRecommendations_WithRecommended(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
		{Name: "shell_exec", BaseName: "shell_exec", Description: "Execute shell commands", Category: "runtime"},
	}
	rec := []DeferredToolEntry{catalog[0]}
	cue := RenderCatalogCueWithRecommendations(catalog, rec)
	if !strings.Contains(cue, "Recommended") {
		t.Error("cue must contain a Recommended section")
	}
	// 推荐区应出现在目录分组之前（模型视线先落推荐区）。
	recIdx := strings.Index(cue, "Recommended")
	catIdx := strings.Index(cue, "### web")
	if recIdx < 0 || catIdx < 0 || recIdx > catIdx {
		t.Errorf("Recommended section must precede category listing (rec=%d cat=%d)", recIdx, catIdx)
	}
	// 目录区仍保持全量（推荐是高亮，不是裁剪）。
	for _, entry := range catalog {
		if !strings.Contains(cue, entry.Name) {
			t.Errorf("full listing must still contain %q", entry.Name)
		}
	}
}

func TestRenderCatalogCueWithRecommendations_EmptyCatalog(t *testing.T) {
	if cue := RenderCatalogCueWithRecommendations(nil, nil); cue != "" {
		t.Errorf("empty catalog must yield empty cue, got %q", cue)
	}
}
