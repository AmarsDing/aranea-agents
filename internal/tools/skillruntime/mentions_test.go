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
	got := mergeMentionedSlugs([]string{"other"}, "run $xlsx-review please", afterA, reasons, 8, nil)
	if len(got) != 2 || got[0] != "xlsx-review" || got[1] != "other" {
		t.Fatalf("got %v", got)
	}
	if reasons["xlsx-review"] != "user mention" {
		t.Fatalf("reason = %q", reasons["xlsx-review"])
	}
	denied := mergeMentionedSlugs(nil, "use $secret", []biz.SkillRuntimeCandidate{{Slug: "other"}}, map[string]string{}, 8, nil)
	if len(denied) != 0 {
		t.Fatalf("denied mention must not load, got %v", denied)
	}
}

// TestMergeMentionedSlugs_TrustGateBlocked 钉 D4-c mention 坐席(就绪不翻)：
// 候选在目录却低于信任门槛 → 死拒提及式加载、reasons 落
// blocked_by_trust_gate；未阻塞提照常 prepend；blockedByTrustGate=nil
// （门禁 off/shadow 未安装）时行为与历史完全一致。
func TestMergeMentionedSlugs_TrustGateBlocked(t *testing.T) {
	t.Parallel()
	afterA := []biz.SkillRuntimeCandidate{{Slug: "low-trust"}, {Slug: "ok-trust"}}
	blocked := func(c biz.SkillRuntimeCandidate) bool { return c.Slug == "low-trust" }

	reasons := map[string]string{}
	got := mergeMentionedSlugs(nil, "use $low-trust and $ok-trust", afterA, reasons, 8, blocked)
	if len(got) != 1 || got[0] != "ok-trust" {
		t.Fatalf("blocked mention must be refused, got %v", got)
	}
	if reasons["low-trust"] != "blocked_by_trust_gate" {
		t.Fatalf("blocked reason must be recorded, got %q", reasons["low-trust"])
	}
	if reasons["ok-trust"] != "user mention" {
		t.Fatalf("passing mention must keep user-mention reason, got %q", reasons["ok-trust"])
	}

	// nil 谓词（门禁砰未安装）全放行——off/shadow 零行为变化。
	gotNil := mergeMentionedSlugs(nil, "use $low-trust", afterA, map[string]string{}, 8, nil)
	if len(gotNil) != 1 || gotNil[0] != "low-trust" {
		t.Fatalf("nil predicate must pass all mentions, got %v", gotNil)
	}
}
