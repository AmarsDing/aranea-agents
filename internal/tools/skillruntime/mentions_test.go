package skillruntime

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestParseSkillMentions(t *testing.T) {
	t.Parallel()
	got := ParseSkillMentions("please use $xlsx-review and $XLSX-review then $code_review")
	if len(got) != 2 || got[0] != "xlsx-review" || got[1] != "code_review" {
		t.Fatalf("got %v", got)
	}
	if ParseSkillMentions("no mentions here") != nil {
		t.Fatal("expected nil")
	}
}

func TestMergeMentionedSlugs_PrependsAndRespectsLayerA(t *testing.T) {
	t.Parallel()
	afterA := []biz.SkillRuntimeCandidate{{Slug: "xlsx-review"}, {Slug: "other"}}
	reasons := map[string]string{"other": "routed"}
	got := mergeMentionedSlugs([]string{"other"}, "run $xlsx-review please", afterA, reasons, 8)
	if len(got) != 2 || got[0] != "xlsx-review" || got[1] != "other" {
		t.Fatalf("got %v", got)
	}
	if reasons["xlsx-review"] != "user mention" {
		t.Fatalf("reason = %q", reasons["xlsx-review"])
	}
	denied := mergeMentionedSlugs(nil, "use $secret", []biz.SkillRuntimeCandidate{{Slug: "other"}}, map[string]string{}, 8)
	if len(denied) != 0 {
		t.Fatalf("denied mention must not load, got %v", denied)
	}
}
