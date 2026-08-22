package skillruntime

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

type mockSkillResolver struct {
	candidates []biz.SkillRuntimeCandidate
	embScores  map[string]float64
	embErr     error
}

func (m *mockSkillResolver) ListEnabledPublishedSkillCandidates(_ context.Context) ([]biz.SkillRuntimeCandidate, error) {
	return m.candidates, nil
}

func (m *mockSkillResolver) ListEnabledPublishedSkillKeys(_ context.Context) ([]string, error) {
	out := make([]string, len(m.candidates))
	for i, c := range m.candidates {
		out[i] = c.Slug
	}
	return out, nil
}

func (m *mockSkillResolver) ScoreByEmbedding(_ context.Context, _ string, candidates []biz.SkillRuntimeCandidate) (map[string]float64, error) {
	if m.embErr != nil {
		return nil, m.embErr
	}
	return m.embScores, nil
}

type mockRuntime struct{ json string }

func (m *mockRuntime) GetSkillRuntimeJSON() string { return m.json }

func makeCandidate(slug, name, desc string, tags []biz.SkillTag, paths []string) biz.SkillRuntimeCandidate {
	return biz.SkillRuntimeCandidate{
		Slug:          slug,
		Name:          name,
		Description:   desc,
		Tags:          tags,
		TaxonomyPaths: paths,
	}
}

func tag(name string) biz.SkillTag { return biz.SkillTag{Name: name, Source: "manual"} }

func TestApplyLayerAWithReasons_DenyList(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc a", nil, nil),
		makeCandidate("skill-b", "B", "desc b", nil, nil),
		makeCandidate("skill-c", "C", "desc c", nil, nil),
	}
	policy := biz.SkillRuntimePolicy{DeniedSlugs: []string{"skill-b"}}
	reasons := map[string]string{}
	out := applyLayerAWithReasons(candidates, policy, reasons)

	if len(out) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(out))
	}
	if out[0].Slug != "skill-a" || out[1].Slug != "skill-c" {
		t.Errorf("unexpected slugs: %v", slugsOf(out))
	}
	if reasons["skill-b"] != "denied by policy" {
		t.Errorf("reason for skill-b = %q, want %q", reasons["skill-b"], "denied by policy")
	}
}

func TestApplyLayerAWithReasons_AllowList(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc a", nil, nil),
		makeCandidate("skill-b", "B", "desc b", nil, nil),
		makeCandidate("skill-c", "C", "desc c", nil, nil),
	}
	policy := biz.SkillRuntimePolicy{AllowedSlugs: []string{"skill-a", "skill-c"}}
	reasons := map[string]string{}
	out := applyLayerAWithReasons(candidates, policy, reasons)

	if len(out) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(out))
	}
	if reasons["skill-b"] != "not in allowed slugs" {
		t.Errorf("reason for skill-b = %q, want %q", reasons["skill-b"], "not in allowed slugs")
	}
}

func TestApplyLayerAWithReasons_EmptySlug(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("", "Empty", "empty slug", nil, nil),
		makeCandidate("   ", "Spaces", "whitespace slug", nil, nil),
		makeCandidate("skill-a", "A", "desc a", nil, nil),
	}
	policy := biz.SkillRuntimePolicy{}
	reasons := map[string]string{}
	out := applyLayerAWithReasons(candidates, policy, reasons)

	if len(out) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(out))
	}
	if out[0].Slug != "skill-a" {
		t.Errorf("expected skill-a, got %s", out[0].Slug)
	}
	if reasons[""] != "empty slug" {
		t.Errorf("reason for empty slug = %q, want %q", reasons[""], "empty slug")
	}
	if reasons["   "] != "empty slug" {
		t.Errorf("reason for whitespace slug = %q, want %q", reasons["   "], "empty slug")
	}
}

func TestApplyLayerAWithReasons_Mixed(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc a", nil, nil),
		makeCandidate("skill-b", "B", "desc b", nil, nil),
		makeCandidate("", "Empty", "empty", nil, nil),
	}
	policy := biz.SkillRuntimePolicy{
		AllowedSlugs: []string{"skill-a", "skill-b"},
		DeniedSlugs:  []string{"skill-b"},
	}
	reasons := map[string]string{}
	out := applyLayerAWithReasons(candidates, policy, reasons)

	if len(out) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(out))
	}
	if out[0].Slug != "skill-a" {
		t.Errorf("expected skill-a, got %s", out[0].Slug)
	}
	if reasons["skill-b"] != "denied by policy" {
		t.Errorf("reason for skill-b = %q, want %q", reasons["skill-b"], "denied by policy")
	}
	if reasons[""] != "empty slug" {
		t.Errorf("reason for empty slug = %q, want %q", reasons[""], "empty slug")
	}
}

func TestApplyLayerAWithReasons_NoPolicy(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc a", nil, nil),
		makeCandidate("skill-b", "B", "desc b", nil, nil),
	}
	policy := biz.SkillRuntimePolicy{}
	reasons := map[string]string{}
	out := applyLayerAWithReasons(candidates, policy, reasons)

	if len(out) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(out))
	}
	for _, s := range []string{"skill-a", "skill-b"} {
		if reasons[s] != "passed layer A" {
			t.Errorf("reason for %s = %q, want %q", s, reasons[s], "passed layer A")
		}
	}
}

func TestFilterByIntentPathsWithReasons_Matching(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("read-xlsx", "Read XLSX", "read spreadsheets", nil,
			[]string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}),
		makeCandidate("sentiment", "Sentiment", "sentiment analysis", nil,
			[]string{"分析与推理/自然语言理解（情感分析）"}),
	}
	paths := []string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}
	reasons := map[string]string{}
	out := filterByIntentPathsWithReasons(candidates, paths, reasons)

	if len(out) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(out))
	}
	if out[0].Slug != "read-xlsx" {
		t.Errorf("expected read-xlsx, got %s", out[0].Slug)
	}
	if reasons["sentiment"] != "no intent path match" {
		t.Errorf("reason for sentiment = %q, want %q", reasons["sentiment"], "no intent path match")
	}
}

func TestFilterByIntentPathsWithReasons_NonMatching(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc a", nil, []string{"some/other/path"}),
	}
	paths := []string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}
	reasons := map[string]string{}
	out := filterByIntentPathsWithReasons(candidates, paths, reasons)

	if len(out) != 0 {
		t.Fatalf("expected 0 candidates, got %d", len(out))
	}
	if reasons["skill-a"] != "no intent path match" {
		t.Errorf("reason = %q, want %q", reasons["skill-a"], "no intent path match")
	}
}

func TestFilterByIntentPathsWithReasons_EmptyPaths(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc a", nil, []string{"some/path"}),
	}
	reasons := map[string]string{}
	out := filterByIntentPathsWithReasons(candidates, nil, reasons)

	if len(out) != 0 {
		t.Fatalf("expected 0 candidates (nil paths means nothing matches), got %d", len(out))
	}
	if reasons["skill-a"] != "no intent path match" {
		t.Errorf("reason = %q, want %q", reasons["skill-a"], "no intent path match")
	}
}

func TestFilterByAllTagsWithReasons_MatchingAll(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc", []biz.SkillTag{tag("file_type:xlsx"), tag("domain:sales")}, nil),
	}
	required := []string{"file_type:xlsx", "domain:sales"}
	reasons := map[string]string{}
	out := filterByAllTagsWithReasons(candidates, required, reasons)

	if len(out) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(out))
	}
}

func TestFilterByAllTagsWithReasons_MissingTags(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc", []biz.SkillTag{tag("file_type:xlsx")}, nil),
	}
	required := []string{"file_type:xlsx", "domain:sales"}
	reasons := map[string]string{}
	out := filterByAllTagsWithReasons(candidates, required, reasons)

	if len(out) != 0 {
		t.Fatalf("expected 0 candidates, got %d", len(out))
	}
	if reasons["skill-a"] != "missing required tags" {
		t.Errorf("reason = %q, want %q", reasons["skill-a"], "missing required tags")
	}
}

func TestFilterByAllTagsWithReasons_EmptyRequired(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc", []biz.SkillTag{tag("file_type:xlsx")}, nil),
	}
	reasons := map[string]string{}
	out := filterByAllTagsWithReasons(candidates, nil, reasons)

	if len(out) != 1 {
		t.Fatalf("expected 1 candidate (empty required = pass all), got %d", len(out))
	}
}

func TestScoreCandidatesWithReasons_ExactMatch(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("read-xlsx", "Read", "desc", nil,
			[]string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}),
	}
	paths := []string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}
	reasons := map[string]string{}
	scored := scoreCandidatesWithReasons(candidates, paths, reasons)

	if len(scored) != 1 {
		t.Fatalf("expected 1 scored, got %d", len(scored))
	}
	if scored[0].score < 1000 {
		t.Errorf("expected score >= 1000 for exact match, got %d", scored[0].score)
	}
	if scored[0].reason != "exact taxonomy path match" {
		t.Errorf("reason = %q, want %q", scored[0].reason, "exact taxonomy path match")
	}
}

func TestScoreCandidatesWithReasons_PartialMatch(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("read-xlsx", "Read", "desc", nil,
			[]string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}),
	}
	paths := []string{"数据获取与集成"}
	reasons := map[string]string{}
	scored := scoreCandidatesWithReasons(candidates, paths, reasons)

	if len(scored) != 1 {
		t.Fatalf("expected 1 scored, got %d", len(scored))
	}
	if scored[0].score < 400 {
		t.Errorf("expected score >= 400 for partial match, got %d", scored[0].score)
	}
	if scored[0].reason != "partial taxonomy path match" {
		t.Errorf("reason = %q, want %q", scored[0].reason, "partial taxonomy path match")
	}
}

func TestScoreCandidatesWithReasons_NoMatch(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc a", nil, []string{"some/other/path"}),
	}
	paths := []string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}
	reasons := map[string]string{}
	scored := scoreCandidatesWithReasons(candidates, paths, reasons)

	if len(scored) != 1 {
		t.Fatalf("expected 1 scored, got %d", len(scored))
	}
	if scored[0].score != 0 {
		t.Errorf("expected score 0 for no match, got %d", scored[0].score)
	}
	if scored[0].reason != "no intent match; included by default" {
		t.Errorf("reason = %q, want %q", scored[0].reason, "no intent match; included by default")
	}
}

func TestScoreCandidatesWithReasons_NoPaths(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("skill-a", "A", "desc a", nil, nil),
	}
	reasons := map[string]string{}
	scored := scoreCandidatesWithReasons(candidates, nil, reasons)

	if len(scored) != 1 {
		t.Fatalf("expected 1 scored, got %d", len(scored))
	}
	if scored[0].score != 0 {
		t.Errorf("expected score 0 with no paths, got %d", scored[0].score)
	}
	if scored[0].reason != "enabled and published" {
		t.Errorf("reason = %q, want %q", scored[0].reason, "enabled and published")
	}
}

func TestScoreCandidatesWithReasons_KeywordMatch(t *testing.T) {
	candidates := []biz.SkillRuntimeCandidate{
		makeCandidate("xlsx-reader", "XLSX Reader", "reads xlsx spreadsheets", nil, nil),
	}
	paths := []string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}
	reasons := map[string]string{}
	scored := scoreCandidatesWithReasons(candidates, paths, reasons)

	if len(scored) != 1 {
		t.Fatalf("expected 1 scored, got %d", len(scored))
	}
	if scored[0].score < 100 {
		t.Errorf("expected score >= 100 for keyword match, got %d", scored[0].score)
	}
	if scored[0].reason != "keyword match" {
		t.Errorf("reason = %q, want %q", scored[0].reason, "keyword match")
	}
}

func TestResolveSkillSlugsDetailed_EmptyCandidates(t *testing.T) {
	resolver := &mockSkillResolver{candidates: nil}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: "{}"},
		UserQuery: "test",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 0 {
		t.Errorf("expected 0 slugs, got %v", result.Slugs)
	}
}

func TestResolveSkillSlugsDetailed_LayerADeny(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc a", nil, nil),
			makeCandidate("skill-b", "B", "desc b", nil, nil),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"denied_slugs":["skill-b"]}`},
		UserQuery: "",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 1 || result.Slugs[0] != "skill-a" {
		t.Errorf("expected [skill-a], got %v", result.Slugs)
	}
	if result.Reasons["skill-b"] != "denied by policy" {
		t.Errorf("reason for skill-b = %q", result.Reasons["skill-b"])
	}
}

func TestResolveSkillSlugsDetailed_LayerAAllow(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc a", nil, nil),
			makeCandidate("skill-b", "B", "desc b", nil, nil),
			makeCandidate("skill-c", "C", "desc c", nil, nil),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"allowed_slugs":["skill-a","skill-c"]}`},
		UserQuery: "",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 2 {
		t.Errorf("expected 2 slugs, got %v", result.Slugs)
	}
	if result.Reasons["skill-b"] != "not in allowed slugs" {
		t.Errorf("reason for skill-b = %q", result.Reasons["skill-b"])
	}
}

func TestResolveSkillSlugsDetailed_IntentRouting(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("read-xlsx", "Read XLSX", "reads xlsx spreadsheets", nil,
				[]string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}),
			makeCandidate("email-sender", "Send Email", "sends emails", nil,
				[]string{"交互与执行/消息发送（发邮件）"}),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"intent_routing_enabled":true,"intent_max_paths":3}`},
		UserQuery: "读取表格文件",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) == 0 {
		t.Fatal("expected at least 1 slug")
	}
	if result.Slugs[0] != "read-xlsx" {
		t.Errorf("expected read-xlsx first, got %v", result.Slugs)
	}
}

func TestResolveSkillSlugsDetailed_TagFiltering(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc", []biz.SkillTag{tag("file_type:xlsx")}, nil),
			makeCandidate("skill-b", "B", "desc", []biz.SkillTag{tag("domain:sales")}, nil),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"allowed_tags":["file_type:xlsx"],"intent_routing_enabled":false}`},
		UserQuery: "",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 1 || result.Slugs[0] != "skill-a" {
		t.Errorf("expected [skill-a], got %v", result.Slugs)
	}
	if result.Reasons["skill-b"] != "missing required tags" {
		t.Errorf("reason for skill-b = %q", result.Reasons["skill-b"])
	}
}

func TestResolveSkillSlugsDetailed_MaxSkillsCap(t *testing.T) {
	candidates := make([]biz.SkillRuntimeCandidate, 5)
	for i := range candidates {
		candidates[i] = makeCandidate(
			"skill-"+string(rune('a'+i)),
			string(rune('A'+i)),
			"desc",
			nil, nil,
		)
	}
	resolver := &mockSkillResolver{candidates: candidates}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"max_skills_in_toolset":2,"intent_routing_enabled":false}`},
		UserQuery: "",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 2 {
		t.Errorf("expected 2 slugs (capped), got %d: %v", len(result.Slugs), result.Slugs)
	}
	capped := false
	for _, r := range result.Reasons {
		if r == "exceeded max_skills_in_toolset cap" {
			capped = true
			break
		}
	}
	if !capped {
		t.Error("expected at least one 'exceeded max_skills_in_toolset cap' reason")
	}
}

func TestResolveSkillSlugsDetailed_UserMention(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("xlsx-review", "XLSX", "review spreadsheets", nil, nil),
			makeCandidate("other", "Other", "unrelated", nil, nil),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"intent_routing_enabled":false}`},
		UserQuery: "please run $xlsx-review on this file",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) == 0 || result.Slugs[0] != "xlsx-review" {
		t.Fatalf("mention must load first, got %v", result.Slugs)
	}
	if result.Reasons["xlsx-review"] != "user mention" {
		t.Fatalf("reason = %q", result.Reasons["xlsx-review"])
	}
}

func TestResolveSkillSlugsDetailed_EmbeddingScoring(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc a", nil, nil),
			makeCandidate("skill-b", "B", "desc b", nil, nil),
		},
		embScores: map[string]float64{
			"skill-a": 0.95,
			"skill-b": 0.10,
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"embedding_scoring_enabled":true,"embedding_score_weight":0.5,"intent_routing_enabled":false}`},
		UserQuery: "test query",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 2 {
		t.Fatalf("expected 2 slugs, got %d", len(result.Slugs))
	}
	if result.Slugs[0] != "skill-a" {
		t.Errorf("expected skill-a first (higher embedding), got %v", result.Slugs)
	}
	if result.Reasons["skill-a"] == "" {
		t.Error("expected non-empty reason for skill-a")
	}
}

func TestResolveSkillSlugsDetailed_EmbeddingError(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc a", nil, nil),
		},
		embScores: nil,
		embErr:    context.DeadlineExceeded,
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"embedding_scoring_enabled":true,"embedding_score_weight":0.5,"intent_routing_enabled":false}`},
		UserQuery: "test query",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 1 {
		t.Errorf("expected 1 slug (fallback on embedding error), got %d", len(result.Slugs))
	}
}

func TestResolveSkillSlugsDetailed_NilOpts(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc a", nil, nil),
		},
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 1 || result.Slugs[0] != "skill-a" {
		t.Errorf("expected [skill-a], got %v", result.Slugs)
	}
}

func TestMergeTagRequirements_Dedup(t *testing.T) {
	out := mergeTagRequirements(
		[]string{"file_type:xlsx", "FILE_TYPE:XLSX", "domain:sales"},
		[]string{"domain:sales", "domain:marketing"},
	)
	if len(out) != 3 {
		t.Fatalf("expected 3 unique tags, got %d: %v", len(out), out)
	}
	set := map[string]bool{}
	for _, t := range out {
		set[t] = true
	}
	if !set["file_type:xlsx"] || !set["domain:sales"] || !set["domain:marketing"] {
		t.Errorf("missing expected tags in %v", out)
	}
}

func TestMergeTagRequirements_Lowercase(t *testing.T) {
	out := mergeTagRequirements([]string{"TAG"}, []string{"Tag"})
	if len(out) != 1 {
		t.Fatalf("expected 1 tag after lowercasing, got %d: %v", len(out), out)
	}
	if out[0] != "tag" {
		t.Errorf("expected lowercase 'tag', got %q", out[0])
	}
}

func TestMergeTagRequirements_Empty(t *testing.T) {
	out := mergeTagRequirements(nil, nil)
	if len(out) != 0 {
		t.Errorf("expected 0 tags, got %d", len(out))
	}

	out = mergeTagRequirements([]string{"", "  "}, []string{})
	if len(out) != 0 {
		t.Errorf("expected 0 tags after trimming, got %d: %v", len(out), out)
	}
}

func TestSkillHasAllTags_AllPresent(t *testing.T) {
	c := makeCandidate("s", "S", "d", []biz.SkillTag{tag("file_type:xlsx"), tag("domain:sales")}, nil)
	if !skillHasAllTags(c, []string{"file_type:xlsx", "domain:sales"}) {
		t.Error("expected true when all tags present")
	}
}

func TestSkillHasAllTags_MissingOne(t *testing.T) {
	c := makeCandidate("s", "S", "d", []biz.SkillTag{tag("file_type:xlsx")}, nil)
	if skillHasAllTags(c, []string{"file_type:xlsx", "domain:sales"}) {
		t.Error("expected false when a tag is missing")
	}
}

func TestSkillHasAllTags_EmptyRequired(t *testing.T) {
	c := makeCandidate("s", "S", "d", []biz.SkillTag{tag("file_type:xlsx")}, nil)
	if !skillHasAllTags(c, nil) {
		t.Error("expected true when no tags required")
	}
	if !skillHasAllTags(c, []string{}) {
		t.Error("expected true when empty tags required")
	}
}

func TestSkillHasAllTags_CaseInsensitive(t *testing.T) {
	c := makeCandidate("s", "S", "d", []biz.SkillTag{tag("File_Type:XLSX")}, nil)
	if !skillHasAllTags(c, []string{"file_type:xlsx"}) {
		t.Error("expected true for case-insensitive tag match")
	}
}

func TestMatchesAnyIntentPath_ExactMatch(t *testing.T) {
	c := makeCandidate("s", "S", "d", nil,
		[]string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"})
	if !matchesAnyIntentPath(c, []string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}) {
		t.Error("expected true for exact taxonomy path match")
	}
}

func TestMatchesAnyIntentPath_PartialMatch(t *testing.T) {
	c := makeCandidate("s", "S", "d", nil,
		[]string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"})
	if !matchesAnyIntentPath(c, []string{"数据获取与集成"}) {
		t.Error("expected true for partial (contains) taxonomy path match")
	}
}

func TestMatchesAnyIntentPath_NoMatch(t *testing.T) {
	c := makeCandidate("s", "S", "d", nil,
		[]string{"some/completely/different/path"})
	if matchesAnyIntentPath(c, []string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}) {
		t.Error("expected false for non-matching path")
	}
}

func TestMatchesAnyIntentPath_EmptyPaths(t *testing.T) {
	c := makeCandidate("s", "S", "d", nil,
		[]string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"})
	if matchesAnyIntentPath(c, nil) {
		t.Error("expected false for empty paths slice")
	}
	if matchesAnyIntentPath(c, []string{""}) {
		t.Error("expected false for empty path string")
	}
}

func TestMatchesAnyIntentPath_KeywordFallback(t *testing.T) {
	c := makeCandidate("xlsx-reader", "XLSX Reader", "reads xlsx spreadsheets", nil, nil)
	if !matchesAnyIntentPath(c, []string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}) {
		t.Error("expected true for keyword match via taxonomy leaves")
	}
}

func TestResolveSkillSlugsDetailed_IntentRoutingNoNarrowingWhenNoMatch(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("unrelated", "Unrelated", "completely unrelated skill", nil, nil),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"intent_routing_enabled":true,"intent_max_paths":3}`},
		UserQuery: "读取表格文件",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 1 {
		t.Errorf("expected 1 slug (intent narrowing should not remove all), got %d", len(result.Slugs))
	}
}

func TestResolveSkillSlugsDetailed_TagFilterNoNarrowingWhenNoMatch(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc", []biz.SkillTag{tag("other")}, nil),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"allowed_tags":["file_type:xlsx"],"intent_routing_enabled":false}`},
		UserQuery: "",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 1 {
		t.Errorf("expected 1 slug (tag filter should not remove all), got %d", len(result.Slugs))
	}
}

func TestResolveSkillSlugsDetailed_EmbeddingBoostsLowerScored(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("low-score", "Low", "low score", nil,
				[]string{"分析与推理/自然语言理解（情感分析）"}),
			makeCandidate("high-emb", "HighEmb", "high embedding", nil,
				[]string{"交互与执行/消息发送（发邮件）"}),
		},
		embScores: map[string]float64{
			"low-score": 0.1,
			"high-emb":  0.99,
		},
	}
	opts := &SkillToolsetOptions{
		Runtime: &mockRuntime{json: `{
			"embedding_scoring_enabled":true,
			"embedding_score_weight":1.0,
			"intent_routing_enabled":true,
			"intent_max_paths":3
		}`},
		UserQuery: "情感分析邮件",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 2 {
		t.Fatalf("expected 2 slugs, got %d", len(result.Slugs))
	}
	if result.Slugs[0] != "high-emb" {
		t.Errorf("expected high-emb first (embedding boost), got %v", result.Slugs)
	}
}

func slugsOf(cs []biz.SkillRuntimeCandidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Slug
	}
	return out
}

// fakeHealthProvider implements skillrecommend.HealthMetricsProvider with
// per-key success rates and a call counter proving the fusion branch ran.
// keys records the exact lookup keys so tests can assert whether the provider
// was queried by platform skill ID (skill_invocation.skill_id 口径) or slug.
type fakeHealthProvider struct {
	rates map[string]float64
	calls int
	keys  []string
}

func (f *fakeHealthProvider) GetRecentSuccessRate(_ context.Context, key string, _ int) (float64, error) {
	f.calls++
	f.keys = append(f.keys, key)
	return f.rates[key], nil
}

func (f *fakeHealthProvider) GetRecentAvgDuration(_ context.Context, _ string, _ int) (float64, error) {
	return 0, nil
}

// TestResolveSkillSlugsDetailed_HealthProviderFusion covers R1 (2026-08-13):
// with a non-nil HealthProvider, historical performance must be fused into
// the final order — a low-success candidate that would otherwise win on
// lexicographic tie-break must drop below the high-success one.
func TestResolveSkillSlugsDetailed_HealthProviderFusion(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			// No intent/tag/embedding signals → both score 0 and the
			// pre-fusion order is purely lexicographic: aaa-skill first.
			makeCandidate("aaa-skill", "AAA", "desc", nil, nil),
			makeCandidate("zzz-skill", "ZZZ", "desc", nil, nil),
		},
	}
	provider := &fakeHealthProvider{rates: map[string]float64{
		"aaa-skill": 0.1,  // historically failing
		"zzz-skill": 0.95, // historically reliable
	}}
	opts := &SkillToolsetOptions{Runtime: &mockRuntime{json: `{}`}, HealthProvider: provider}

	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 2 {
		t.Fatalf("expected 2 slugs, got %d", len(result.Slugs))
	}
	if result.Slugs[0] != "zzz-skill" {
		t.Errorf("expected zzz-skill first after health fusion, got %v", result.Slugs)
	}
	if provider.calls == 0 {
		t.Error("health provider was never queried — fusion branch not wired")
	}
	// Fusion appends the rank snapshot to the reason for observability.
	if got := result.Reasons["zzz-skill"]; got == "enabled and published" {
		t.Errorf("reason for zzz-skill lacks rank snapshot: %q", got)
	}
}

// TestResolveSkillSlugsDetailed_HealthProviderQueriedBySkillID covers B1
// (2026-08-14): skill_invocation.skill_id 列存平台 ID（skill_<unixnano>），
// 候选带 SkillID 时健康指标必须按 ID 查询，否则永远匹配 0 行，历史排序静默失效。
// 注意：skillrecommend.DynamicRankFactors（不在 B1 修复范围内）仍会按 slug
// 查询一次，因此这里断言「ID 查询发生且融合生效」，而非「无 slug 查询」。
func TestResolveSkillSlugsDetailed_HealthProviderQueriedBySkillID(t *testing.T) {
	aaa := makeCandidate("aaa-skill", "AAA", "desc", nil, nil)
	aaa.SkillID = "skill_111"
	zzz := makeCandidate("zzz-skill", "ZZZ", "desc", nil, nil)
	zzz.SkillID = "skill_999"
	resolver := &mockSkillResolver{candidates: []biz.SkillRuntimeCandidate{aaa, zzz}}
	// 仅按平台 ID 键控：若实现回退用 slug 查询，rates 全部 miss（0 值），
	// zzz-skill 无法凭借 0.95 成功率反超 aaa-skill。
	provider := &fakeHealthProvider{rates: map[string]float64{
		"skill_111": 0.1,
		"skill_999": 0.95,
	}}
	opts := &SkillToolsetOptions{Runtime: &mockRuntime{json: `{}`}, HealthProvider: provider}

	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 2 || result.Slugs[0] != "zzz-skill" {
		t.Errorf("expected [zzz-skill aaa-skill] after ID-keyed fusion, got %v", result.Slugs)
	}
	// buildRankCandidates 必须按平台 ID 查询（DynamicRankFactors 的 slug 查询是
	// 已知残留，见测试 doc 注释）。
	seen := map[string]bool{}
	for _, key := range provider.keys {
		seen[key] = true
	}
	if !seen["skill_111"] || !seen["skill_999"] {
		t.Errorf("provider was not queried by platform skill IDs, keys = %v", provider.keys)
	}
}

// TestBuildRankCandidates_UsesSkillIDWithSlugFallback 直接单测 B1 修复点：
// candidate 带 SkillID 时按 ID 查询健康指标；不带（存量调用方）时回退 slug。
func TestBuildRankCandidates_UsesSkillIDWithSlugFallback(t *testing.T) {
	scored := []slugScore{
		{slug: "with-id", skillID: "skill_111"},
		{slug: "legacy-slug"},
	}
	provider := &fakeHealthProvider{rates: map[string]float64{
		"skill_111":   0.9,
		"legacy-slug": 0.5,
	}}
	out := buildRankCandidates(context.Background(), scored, provider, nil)
	if len(out) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(out))
	}
	if out[0].HistoricalSuccess != 0.9 {
		t.Errorf("with-id HistoricalSuccess = %v, want 0.9 (queried by skill ID)", out[0].HistoricalSuccess)
	}
	if out[1].HistoricalSuccess != 0.5 {
		t.Errorf("legacy-slug HistoricalSuccess = %v, want 0.5 (slug fallback)", out[1].HistoricalSuccess)
	}
	if len(provider.keys) != 2 || provider.keys[0] != "skill_111" || provider.keys[1] != "legacy-slug" {
		t.Errorf("query keys = %v, want [skill_111 legacy-slug]", provider.keys)
	}
}

// TestResolveSkillSlugsDetailed_NilHealthProviderSkipsFusion pins the
// pre-R1 behavior: without a provider the lexicographic tie-break decides.
func TestResolveSkillSlugsDetailed_NilHealthProviderSkipsFusion(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("aaa-skill", "AAA", "desc", nil, nil),
			makeCandidate("zzz-skill", "ZZZ", "desc", nil, nil),
		},
	}
	opts := &SkillToolsetOptions{Runtime: &mockRuntime{json: `{}`}}

	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 2 || result.Slugs[0] != "aaa-skill" {
		t.Errorf("expected lexicographic order [aaa-skill zzz-skill], got %v", result.Slugs)
	}
}
