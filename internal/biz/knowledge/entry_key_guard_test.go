package knowledge

import "testing"

// 2026-08-18 域 D 污染事故回归：provenance 字段名/kind 词表/UUID/纯数字键
// 不得作为话题键（tags）或实体名进入词条定位、实体抽取与 autolink target。
func TestEntryKeyGuard(t *testing.T) {
	reserved := []string{
		"fact_id", "session-id", "Session_ID", "agent id", "source_id",
		"kind", "tags", "statement", "entry", "confidence",
		"preference", "profile", "goal", "constraint", "decision", "relationship",
		"会话沉淀", "词条页", "同主题新事实",
	}
	for _, k := range reserved {
		if !IsReservedEntryKey(k) {
			t.Errorf("IsReservedEntryKey(%q) = false, want true", k)
		}
	}

	noise := []string{
		"", "  ", "10.20.99.2", "20260818", "v1.2.3",
		"69ba5e53-4429-9860-db10-f4abc12345",
		"entries/foo", "[[wikilink]]", `a\b`,
	}
	for _, k := range noise {
		if !IsNoiseEntryKey(k) {
			t.Errorf("IsNoiseEntryKey(%q) = false, want true", k)
		}
	}

	ok := []string{"评测-核心交换机", "SW-Eval-01", "通信协议", "Aranea"}
	for _, k := range ok {
		if IsReservedEntryKey(k) || IsNoiseEntryKey(k) {
			t.Errorf("guard(%q) = true, want false", k)
		}
	}
}

func TestNormalizedEntryTagsFiltersGuardKeys(t *testing.T) {
	got := normalizedEntryTags([]string{"评测-核心交换机", "fact_id", "session-id", "10.20.99.2", "评测-核心交换机"})
	if len(got) != 1 || got[0] != "评测-核心交换机" {
		t.Fatalf("normalizedEntryTags = %v, want [评测-核心交换机]", got)
	}
}
