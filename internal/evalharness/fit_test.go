package evalharness

import (
	"testing"
)

func TestToolfailArmVerdict_S10(t *testing.T) {
	if g := ToolfailArmVerdict(false, false); g != VerdictInconclusive {
		t.Fatalf("uninjected = %s, want inconclusive", g)
	}
	if g := ToolfailArmVerdict(false, true); g != VerdictInconclusive {
		t.Fatalf("uninjected even with a later failure is still inconclusive, got %s", g)
	}
	if g := ToolfailArmVerdict(true, true); g != VerdictPass {
		t.Fatalf("injected+failure = %s, want pass", g)
	}
	if g := ToolfailArmVerdict(true, false); g != VerdictFail {
		t.Fatalf("injected but tool did not fail = %s, want fail", g)
	}
}

func TestEmptyEvidenceFiles(t *testing.T) {
	empty := EmptyEvidenceFiles(map[string]int64{
		"S07/t2-msg.json":        0,
		"S14/h1-spirit-msg.json": 0,
		"S06/ok.json":            12,
	})
	if len(empty) != 2 {
		t.Fatalf("empty files = %v, want 2", empty)
	}
}

func TestCountSentences_S01(t *testing.T) {
	three := "第一句。第二句。第三句。"
	if err := AssertSentenceCount(three, 3); err != nil {
		t.Fatal(err)
	}
	oneLong := "这是一个把三件事全部塞进同一句里的超长回复，没有句号分割"
	if err := AssertSentenceCount(oneLong, 3); err == nil {
		t.Fatal("single long sentence must fail the three-sentence assertion")
	}
	if got := CountSentences(oneLong); got != 1 {
		t.Fatalf("single clause count = %d, want 1", got)
	}
}

func TestUnattendedEvalAutoApproveConfigured(t *testing.T) {
	if UnattendedEvalAutoApproveConfigured(func(string) string { return "" }) {
		t.Fatal("unset env must be false")
	}
	if !UnattendedEvalAutoApproveConfigured(func(k string) string {
		if k == "ARANEA_TOOL_AUTO_APPROVE" {
			return "1"
		}
		return ""
	}) {
		t.Fatal("ARANEA_TOOL_AUTO_APPROVE=1 must satisfy the freeze")
	}
}

func TestRemapForkedRecordID_SingleLayer(t *testing.T) {
	dst := "12345678-aaaa-bbbb-cccc-ddddeeeeffff"
	if got := RemapForkedRecordID(dst, "inv-A"); got != "fk12345678-inv-A" {
		t.Fatalf("root id remap = %q", got)
	}
	// Inherited prefix is stripped, not stacked.
	if got := RemapForkedRecordID(dst, "fk11111111-inv-A"); got != "fk12345678-inv-A" {
		t.Fatalf("one inherited layer = %q, want fk12345678-inv-A", got)
	}
	if got := RemapForkedRecordID(dst, "fk22222222-fk11111111-inv-A"); got != "fk12345678-inv-A" {
		t.Fatalf("stacked source = %q, want single new layer", got)
	}
	if err := CheckForkedRecordIDContract(dst, "fk11111111-turn-1", "fk12345678-turn-1"); err != nil {
		t.Fatal(err)
	}
	// Eval must not treat remap as "renumber from fork point".
	if err := CheckForkedRecordIDContract(dst, "turn-1", "turn-2"); err == nil {
		t.Fatal("sequential renumber must fail FIT-FORK-1")
	}
	if err := CheckForkedRecordIDContract(dst, "fk11111111-inv-A", "fk12345678-fk11111111-inv-A"); err == nil {
		t.Fatal("stacked remap must fail FIT-FORK-1")
	}
	if ForkIDPrefixLayerCount("fk12345678-fk11111111-inv-A") != 2 {
		t.Fatal("layer count must see stacked prefixes")
	}
	if ForkIDPrefixLayerCount("fk12345678-inv-A") != 1 {
		t.Fatal("single layer count")
	}
}
