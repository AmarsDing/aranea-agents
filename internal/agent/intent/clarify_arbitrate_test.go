package intent

import "testing"

func TestLooksLikeExplicitInstruction(t *testing.T) {
	if !LooksLikeExplicitInstruction("对 core-sw1 的 eth1 执行 ip link show") {
		t.Fatal("named device + command must be explicit")
	}
	if LooksLikeExplicitInstruction("帮我做个应用") {
		t.Fatal("underspecified task must not look explicit")
	}
	if LooksLikeExplicitInstruction("帮我写一份方案") {
		t.Fatal("underspecified write-a-thing must not look explicit")
	}
}

func TestArbitrateClarification_ExplicitStrips(t *testing.T) {
	art := &Artifact{
		RiskFlags:      []string{RiskFlagNeedsClarification},
		Clarifications: []ClarificationQuestion{{Question: "目标？", Mode: "single", Options: []string{"A", "B"}, Recommended: []string{"A"}}},
	}
	got := ArbitrateClarification("对 core-sw1 执行 ip link show", art)
	if got.NeedsClarification() {
		t.Fatal("explicit instruction must not force clarification")
	}
}

func TestArbitrateClarification_UnderspecifiedKeeps(t *testing.T) {
	art := &Artifact{
		RiskFlags:      []string{RiskFlagNeedsClarification},
		Clarifications: []ClarificationQuestion{{Question: "目标平台？", Mode: "single", Options: []string{"Web", "iOS"}, Recommended: []string{"Web"}}},
	}
	got := ArbitrateClarification("帮我做个应用", art)
	if !got.NeedsClarification() {
		t.Fatal("underspecified task must keep clarification")
	}
	if !ShouldSkipAutoResolveClarification("帮我做个应用", got) {
		t.Fatal("underspecified + recommended must hang, not auto-draft")
	}
}

func TestUnderspecifiedClarifyArtifact_S02(t *testing.T) {
	art := UnderspecifiedClarifyArtifact("帮我弄个报告。")
	if !art.NeedsClarification() {
		t.Fatal("synthetic artifact must trigger the clarification gate")
	}
	if len(art.Clarifications) == 0 || len(art.Clarifications[0].Recommended) != 0 {
		t.Fatal("synthetic questions must not carry recommended defaults")
	}
}

func TestArbitrateClarification_HighRiskKept(t *testing.T) {
	art := &Artifact{
		RiskFlags:      []string{RiskFlagNeedsClarification, "destructive"},
		Clarifications: []ClarificationQuestion{{Question: "确认删库？", Mode: "single", Options: []string{"是", "否"}}},
	}
	got := ArbitrateClarification("对 core-sw1 执行 drop table", art)
	if !got.NeedsClarification() {
		t.Fatal("high-risk must keep clarification")
	}
}
