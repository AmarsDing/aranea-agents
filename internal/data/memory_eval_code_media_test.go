package data

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestEvalAttachMedia_OmitsFilesystemPath(t *testing.T) {
	base := evalSourceMetadata("s-1", 0, 0, nil, nil, nil, "2026-10-24T00:00:00Z", "", "user: login nil-pointer on empty token")
	got := evalAttachMedia(base, []biz.EvalContentPart{{
		Type: "image_url", Index: 1, Mime: "image/png", Bytes: 12, Ref: `C:\secret\shot.png`,
	}})
	if strings.Contains(got, "secret") || strings.Contains(got, "shot.png") {
		t.Fatalf("filesystem path leaked into metadata: %s", got)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatal(err)
	}
	media, _ := m["media"].([]any)
	if len(media) != 1 {
		t.Fatalf("media = %#v", m["media"])
	}
	if evalAttachMedia(base, nil) != base {
		t.Fatal("text-only metadata changed")
	}
}

func TestRankEvalEvidence_CodeScopeOutranksProse(t *testing.T) {
	query := "What was the fix in module auth?"
	qa := biz.AnalyzeEvalQuery(query, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	items := []evalCollectedItem{
		{item: biz.EvalMemoryItem{ID: "prose", Content: "user: My favorite topic is auth refresh tokens", Score: 0.2}, kind: "preference"},
		{item: biz.EvalMemoryItem{ID: "fix", Content: "user: fix(auth): reject expired refresh tokens", Score: 0.2}, kind: biz.EvalFactKindCaseFix, caseScope: "auth"},
	}
	out := rankEvalEvidence(items, []string{query}, qa)
	if len(out) == 0 || out[0].item.ID != "fix" {
		t.Fatalf("code evidence did not rank first: %+v", out)
	}
}

func TestRankEvalEvidence_CodeQueryDropsOtherScopeAndLookalikes(t *testing.T) {
	query := "What was the fix in module auth?"
	qa := biz.AnalyzeEvalQuery(query, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	episode := "user: fix(auth): nil-pointer panic in login.go\n" +
		"user: fix(api/v2)!: drop legacy paging\n" +
		"user: 修复: 这不是提交\n" +
		"user: hotfix: different word\n" +
		"user: My favorite color is red\n" +
		"system: fix: bump the memory window\n" +
		"user: diff --git a/auth/login.go b/auth/login.go\n+token check"
	items := []evalCollectedItem{
		{item: biz.EvalMemoryItem{ID: "l2:sess", Content: episode, Score: 0.9}},
		{item: biz.EvalMemoryItem{ID: "other", Content: "user: fix(api/v2)!: drop legacy paging", Score: 0.4}, kind: biz.EvalFactKindCaseFix, caseScope: "api/v2"},
		{item: biz.EvalMemoryItem{ID: "look", Content: "user: hotfix: different word", Score: 0.4}, kind: "event"},
		{item: biz.EvalMemoryItem{ID: "fix", Content: "user: fix(auth): nil-pointer panic in login.go", Score: 0.2}, kind: biz.EvalFactKindCaseFix, caseScope: "auth"},
	}
	out := rankEvalEvidence(items, []string{query}, qa)
	text := ""
	for _, env := range out {
		text += env.item.Content + "\n"
	}
	if !strings.Contains(text, "nil-pointer panic") || !strings.Contains(text, "+token check") {
		t.Fatalf("auth evidence missing: %s", text)
	}
	for _, absent := range []string{"这不是提交", "different word", "drop legacy paging", "favorite color", "memory window"} {
		if strings.Contains(text, absent) {
			t.Fatalf("%s leaked into auth evidence: %s", absent, text)
		}
	}
}

func TestRankEvalEvidence_VisualQueryReturnsCaptionOnly(t *testing.T) {
	query := "截图里有什么"
	qa := biz.AnalyzeEvalQuery(query, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	items := []evalCollectedItem{
		{item: biz.EvalMemoryItem{ID: "tea", Content: "user: I like tea", Score: 0.9}, kind: "preference"},
		{item: biz.EvalMemoryItem{ID: "cap", Content: "user: login nil-pointer on empty token", Score: 0.2}, kind: "event", hasMedia: true},
	}
	out := rankEvalEvidence(items, []string{query}, qa)
	if len(out) != 1 || out[0].item.ID != "cap" {
		t.Fatalf("caption = %+v", out)
	}
}

func TestEvalStore_CodeScopeBeatsProse(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "code-board", "s", []biz.EvalMessage{
		{Role: "user", Content: "My favorite topic is auth refresh tokens", MessageID: "p"},
		{Role: "user", Content: "fix(auth): reject expired tokens. Also rotate the signing key.", MessageID: "c"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "code-board", "What was the fix in module auth?", 5)
	text := evalContents(items)
	if err != nil || len(items) == 0 || !strings.Contains(items[0].Content, "reject expired") || !strings.Contains(text, "signing key") {
		t.Fatalf("code recall: first=%v all=%s err=%v", items, text, err)
	}
}

func TestEvalStore_VisualQueryReturnsCaption(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "visual-board", "s", []biz.EvalMessage{
		{Role: "user", Content: "I like tea", MessageID: "t"},
		{Role: "user", Content: "login nil-pointer on empty token", MessageID: "c", Parts: []biz.EvalContentPart{{
			Type: "image_url", Index: 1, Mime: "image/png", Bytes: 12, Ref: `C:\secret\shot.png`,
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "visual-board", "截图里有什么", 5)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "nil-pointer") || strings.Contains(text, "tea") || strings.Contains(text, "secret") || strings.Contains(text, "shot.png") {
		t.Fatalf("visual recall: %s %v", text, err)
	}
}

// Competition coding board: a later fix in the same scope is another event.
// It must not invalidate the earlier fix, and both stay retrievable.
func TestEvalStore_EarlierFixStaysAfterLaterFix(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	msgs := []biz.EvalMessage{
		{Role: "user", Content: "fix(auth): reject expired refresh tokens", MessageID: "old", Timestamp: "2024-03-01T09:00:00Z"},
		{Role: "user", Content: "fix(auth): reject empty access tokens", MessageID: "new", Timestamp: "2024-04-01T09:00:00Z"},
	}
	if _, err := s.AddMessages(ctx, "code-keep", "s", msgs); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "code-keep", "What was the fix in module auth?", 10)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "expired refresh") || !strings.Contains(text, "empty access") {
		t.Fatalf("later fix dropped the earlier one: %s %v", text, err)
	}
	before := strings.Count(text, "expired refresh")
	if _, err := s.AddMessages(ctx, "code-keep", "s", msgs); err != nil {
		t.Fatal(err)
	}
	items, err = s.SearchMemories(ctx, "code-keep", "What was the fix in module auth?", 10)
	text = evalContents(items)
	if err != nil || strings.Count(text, "expired refresh") != before {
		t.Fatalf("replay changed evidence count %d -> %d: %s %v", before, strings.Count(text, "expired refresh"), text, err)
	}
}

// Two repositories share wording. The asked scope has to lead.
func TestEvalStore_ScopeLeadsTheOtherRepo(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "code-repos", "s", []biz.EvalMessage{
		{Role: "user", Content: "fix(billing): reject expired invoice numbers", MessageID: "bill"},
		{Role: "user", Content: "fixture: auth token sample, not a commit", MessageID: "lookalike"},
		{Role: "user", Content: "fix(auth): reject expired refresh tokens", MessageID: "auth"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "code-repos", "What was the fix in module auth?", 5)
	if err != nil || len(items) == 0 || !strings.Contains(items[0].Content, "refresh tokens") {
		t.Fatalf("auth fix did not lead: %+v %v", items, err)
	}
	other, err := s.SearchMemories(ctx, "code-repos-other", "What was the fix in module auth?", 5)
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-user code evidence leaked: %+v %v", other, err)
	}
}

// Competition multimodal contract: captions around an image stay text evidence,
// a visual question finds them, and neither the file path nor image bytes appear.
func TestEvalStore_CaptionAroundImageStaysTextEvidence(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "visual-parts", "s", []biz.EvalMessage{{
		Role:      "user",
		MessageID: "cap",
		Content:   "第一段说明空指针。第二段说明令牌过期。",
		Parts: []biz.EvalContentPart{{
			Type: "image_url", Index: 1, Mime: "image/png", Bytes: 8,
			Ref: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB",
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "visual-parts", "这张图里写了什么", 10)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "空指针") || !strings.Contains(text, "令牌过期") || strings.Contains(text, "iVBORw0KGgo") || strings.Contains(text, "data:image") {
		t.Fatalf("caption evidence: %s %v", text, err)
	}
	other, err := s.SearchMemories(ctx, "visual-parts-other", "这张图里写了什么", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-user caption leaked: %+v %v", other, err)
	}
}

func TestEvalStore_CompetitionCodeMatrix(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	const user = "cam-bench"
	if _, err := s.AddMessages(ctx, user, "repo-auth", []biz.EvalMessage{
		{Role: "user", Content: "fix(auth): nil-pointer panic in login.go", MessageID: "panic", Timestamp: "2024-01-01T00:00:00Z"},
		{Role: "user", Content: "feat(api/v2): batch export endpoint. Writes login.go rows.", MessageID: "feat", Timestamp: "2024-02-01T00:00:00Z"},
		{Role: "user", Content: "fix(api/v2)!: drop legacy paging", MessageID: "break", Timestamp: "2024-03-01T00:00:00Z"},
		{Role: "user", Content: "修复: 这不是提交", MessageID: "cn", Timestamp: "2024-03-02T00:00:00Z"},
		{Role: "user", Content: "hotfix: different word", MessageID: "hot", Timestamp: "2024-03-02T00:00:01Z"},
		{Role: "user", Content: "My favorite color is blue", MessageID: "c1", Timestamp: "2024-03-03T00:00:00Z"},
		{Role: "user", Content: "My favorite color is red", MessageID: "c2", Timestamp: "2024-03-04T00:00:00Z"},
		{Role: "system", Content: "fix: bump the memory window", MessageID: "sys", Timestamp: "2024-03-05T00:00:00Z"},
		{Role: "user", Content: "diff --git a/auth/login.go b/auth/login.go\n--- a/auth/login.go\n+++ b/auth/login.go\n@@ -1 +1 @@\n-old\n+token check", MessageID: "diff", Timestamp: "2024-03-06T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	search := func(query string) string {
		t.Helper()
		items, err := s.SearchMemories(ctx, user, query, 10)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return evalContents(items)
	}
	panicText := search("nil pointer panic in login.go")
	if !strings.Contains(panicText, "nil-pointer panic") || !strings.Contains(panicText, "login.go") {
		t.Fatalf("file and panic evidence missing: %s", panicText)
	}
	featText := search("What was added in module api/v2?")
	if !strings.Contains(featText, "batch export") || !strings.Contains(featText, "Writes login.go rows") || !strings.Contains(featText, "drop legacy paging") {
		t.Fatalf("feature and breaking change were split or dropped: %s", featText)
	}
	if color := search("What is my favorite color now?"); !strings.Contains(color, "favorite color is red") || strings.Contains(color, "favorite color is blue") {
		t.Fatalf("preference update damaged or lost: %s", color)
	}
	if again := search("nil pointer panic in login.go"); !strings.Contains(again, "nil-pointer panic") {
		t.Fatalf("preference update invalidated the fix: %s", again)
	}
	if diff := search("token check in login.go"); !strings.Contains(diff, "diff --git") || !strings.Contains(diff, "+token check") {
		t.Fatalf("diff hunk was split: %s", diff)
	}
	if auth := search("What was the fix in module auth?"); strings.Contains(auth, "这不是提交") || strings.Contains(auth, "different word") || strings.Contains(auth, "drop legacy paging") || strings.Contains(auth, "favorite color") || strings.Contains(auth, "memory window") {
		t.Fatalf("non-commit lookalikes entered the auth fix: %s", auth)
	}
	other, err := s.SearchMemories(ctx, user+"-other", "nil pointer panic in login.go", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-user code evidence leaked: %+v %v", other, err)
	}
}

func TestEvalStore_CompetitionMediaMatrix(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	const user = "mm-bench"
	if _, err := s.AddMessages(ctx, user, "s", []biz.EvalMessage{
		{Role: "user", MessageID: "jpeg", Content: "login.go nil-pointer on the diagram", Parts: []biz.EvalContentPart{{Type: "image_url", Index: 0, Mime: "image/jpeg", Bytes: 4, Ref: `C:\secret\a.jpg`}}},
		{Role: "user", MessageID: "webp", Content: "webp caption of the export dialog", Parts: []biz.EvalContentPart{{Type: "image_url", Index: 0, Mime: "image/webp", Bytes: 4, Ref: "data:image/webp;base64,AAAA"}}},
		{Role: "user", MessageID: "plain", Content: "plain note about tea"},
		{Role: "user", MessageID: "pair", Content: "第一段说明空指针。第二段说明令牌过期。", Parts: []biz.EvalContentPart{
			{Type: "image_url", Index: 1, Mime: "image/png", Bytes: 4, Ref: "data:image/png;base64,BBBB"},
			{Type: "image_url", Index: 2, Mime: "image/png", Bytes: 4, Ref: "data:image/png;base64,CCCC"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	shots, err := s.SearchMemories(ctx, user, "截图里有什么", 10)
	text := evalContents(shots)
	if err != nil || !strings.Contains(text, "diagram") || !strings.Contains(text, "export dialog") || !strings.Contains(text, "空指针") || !strings.Contains(text, "令牌过期") || strings.Contains(text, "tea") || strings.Contains(text, "secret") || strings.Contains(text, "base64") {
		t.Fatalf("caption matrix: %s %v", text, err)
	}
	file, err := s.SearchMemories(ctx, user, "What failed in login.go?", 5)
	if err != nil || len(file) == 0 || !strings.Contains(file[0].Content, "diagram") {
		t.Fatalf("image caption for login.go did not lead: %+v %v", file, err)
	}
	other, err := s.SearchMemories(ctx, user+"-other", "截图里有什么", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-user captions leaked: %+v %v", other, err)
	}
}
