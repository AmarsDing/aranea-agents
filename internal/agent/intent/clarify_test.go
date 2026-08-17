package intent

import (
	"strings"
	"testing"
)

func TestForceDestructiveFlag(t *testing.T) {
	tests := []struct {
		name     string
		userText string
		art      *Artifact
		wantFlag bool
	}{
		{"nil artifact", "inject fault", nil, false},
		{"fault_inject keyword", "请对 sw1 执行 fault_inject", &Artifact{}, true},
		{"gns3_fault_inject keyword", "调用 gns3_fault_inject 注入端口 down", &Artifact{}, true},
		{"故障注入 keyword", "注入故障到交换机 eth1", &Artifact{}, true},
		{"注入故障 keyword", "对 sw1 注入故障", &Artifact{}, true},
		{"drop table keyword", "drop table users", &Artifact{}, true},
		{"rm -rf keyword", "rm -rf /tmp/data", &Artifact{}, true},
		{"normal request no flag", "帮我写一个 landing page", &Artifact{}, false},
		{"delete variable no flag", "delete the unused variable", &Artifact{}, false},
		{"already flagged no dup", "fault_inject sw1", &Artifact{RiskFlags: []string{"destructive"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := 0
			if tt.art != nil {
				before = len(tt.art.RiskFlags)
			}
			ForceDestructiveFlag(tt.userText, tt.art)
			if tt.art == nil {
				return
			}
			got := tt.art.HasRiskFlag("destructive")
			if got != tt.wantFlag {
				t.Errorf("HasRiskFlag(destructive) = %v, want %v", got, tt.wantFlag)
			}
			if tt.name == "already flagged no dup" && len(tt.art.RiskFlags) != before {
				t.Errorf("duplicate flag added: before=%d after=%d", before, len(tt.art.RiskFlags))
			}
		})
	}
}

func TestParseArtifactJSON_WithClarifications(t *testing.T) {
	text := `{"refined_goal":"build a landing page","intent_kind":"task","risk_flags":["needs_clarification"],"clarifications":[{"question":"目标平台？","mode":"single","options":["Web","iOS"],"recommended":["Web"]}]}`
	art, _ := parseArtifactJSON(text)
	if art == nil {
		t.Fatal("expected non-nil Artifact")
	}
	if len(art.Clarifications) != 1 {
		t.Fatalf("Clarifications len = %d, want 1", len(art.Clarifications))
	}
	q := art.Clarifications[0]
	if q.Question != "目标平台？" || q.Mode != "single" || len(q.Options) != 2 || len(q.Recommended) != 1 {
		t.Errorf("question = %+v", q)
	}
}

func TestParseArtifactJSON_WithoutClarifications(t *testing.T) {
	art, _ := parseArtifactJSON(`{"refined_goal":"fix bug","intent_kind":"debug"}`)
	if art == nil {
		t.Fatal("expected non-nil Artifact")
	}
	if art.NeedsClarification() {
		t.Error("NeedsClarification should be false without flag")
	}
}

func TestArtifact_NeedsClarification(t *testing.T) {
	mkQ := func() []ClarificationQuestion {
		return []ClarificationQuestion{{Question: "Q", Mode: "single", Options: []string{"a"}, Recommended: []string{"a"}}}
	}
	tests := []struct {
		name string
		art  *Artifact
		want bool
	}{
		{"nil artifact", nil, false},
		{"flag + questions", &Artifact{RiskFlags: []string{RiskFlagNeedsClarification}, Clarifications: mkQ()}, true},
		{"flag only, no questions", &Artifact{RiskFlags: []string{RiskFlagNeedsClarification}}, false},
		{"questions only, no flag", &Artifact{Clarifications: mkQ()}, false},
		{"other flags + questions", &Artifact{RiskFlags: []string{"touches_auth"}, Clarifications: mkQ()}, false},
	}
	for _, tt := range tests {
		if got := tt.art.NeedsClarification(); got != tt.want {
			t.Errorf("%s: NeedsClarification() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestArtifact_ClarificationQuestions_Truncates(t *testing.T) {
	qs := make([]ClarificationQuestion, 0, MaxClarificationQuestions+3)
	for i := 0; i < MaxClarificationQuestions+3; i++ {
		qs = append(qs, ClarificationQuestion{Question: "Q", Mode: "single"})
	}
	art := &Artifact{Clarifications: qs}
	got := art.ClarificationQuestions()
	if len(got) != MaxClarificationQuestions {
		t.Errorf("ClarificationQuestions len = %d, want %d", len(got), MaxClarificationQuestions)
	}
	if MaxClarificationQuestions != 5 {
		t.Errorf("MaxClarificationQuestions = %d, want 5", MaxClarificationQuestions)
	}
}

func TestIntentSystemPrompts_ContainClarificationContract(t *testing.T) {
	for name, prompt := range map[string]string{"coding": intentSystemCoding, "general": intentSystemGeneral} {
		if !strings.Contains(prompt, "clarifications") {
			t.Errorf("%s prompt should document clarifications key", name)
		}
		if !strings.Contains(prompt, "needs_clarification") {
			t.Errorf("%s prompt should document needs_clarification risk flag", name)
		}
		if !strings.Contains(prompt, "recommended") {
			t.Errorf("%s prompt should document recommended field", name)
		}
	}
}

// TestIntentSystemPrompts_ClarificationDiscipline 锁定防过度澄清的 prompt 规则：
// 历史消歧优先 + 推荐默认强制（支撑门的 auto_default 假设式前进）。
func TestIntentSystemPrompts_ClarificationDiscipline(t *testing.T) {
	for name, prompt := range map[string]string{"coding": intentSystemCoding, "general": intentSystemGeneral} {
		if !strings.Contains(prompt, "Recent conversation") {
			t.Errorf("%s prompt should document resolving references from the Recent conversation section first", name)
		}
		if !strings.Contains(prompt, "never ask about facts already established") {
			t.Errorf("%s prompt should forbid re-asking facts established in history", name)
		}
		if !strings.Contains(prompt, "autonomously") {
			t.Errorf("%s prompt should state recommended defaults may be acted on autonomously", name)
		}
	}
}

func TestArtifact_HasHighRiskFlag(t *testing.T) {
	tests := []struct {
		name string
		art  *Artifact
		want bool
	}{
		{"nil artifact", nil, false},
		{"no flags", &Artifact{}, false},
		{"needs_clarification only is not high-risk", &Artifact{RiskFlags: []string{RiskFlagNeedsClarification}}, false},
		{"touches_auth", &Artifact{RiskFlags: []string{"touches_auth"}}, true},
		{"migrations", &Artifact{RiskFlags: []string{"migrations"}}, true},
		{"sensitive_data", &Artifact{RiskFlags: []string{"sensitive_data"}}, true},
		{"compliance", &Artifact{RiskFlags: []string{"compliance"}}, true},
		{"destructive", &Artifact{RiskFlags: []string{"destructive"}}, true},
		{"irreversible", &Artifact{RiskFlags: []string{"irreversible"}}, true},
		{"high-risk mixed with needs_clarification", &Artifact{RiskFlags: []string{RiskFlagNeedsClarification, "sensitive_data"}}, true},
		{"unknown flag is not high-risk", &Artifact{RiskFlags: []string{"some_other_flag"}}, false},
	}
	for _, tt := range tests {
		if got := tt.art.HasHighRiskFlag(); got != tt.want {
			t.Errorf("%s: HasHighRiskFlag() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
