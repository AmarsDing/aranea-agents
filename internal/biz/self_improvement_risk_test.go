package biz

import (
	"fmt"
	"strings"
	"testing"
)

// siRiskDiff builds a minimal unified diff touching the given file with the
// given number of added lines (deletions 0).
func siRiskDiff(file string, addedLines int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -0,0 +1,%d @@\n", file, file, file, file, addedLines)
	for i := 0; i < addedLines; i++ {
		b.WriteString("+x\n")
	}
	return b.String()
}

// siRiskAddedDiff builds a diff that ADDS a new file (--- /dev/null).
func siRiskAddedDiff(file string, addedLines int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", file, file, file, addedLines)
	for i := 0; i < addedLines; i++ {
		b.WriteString("+x\n")
	}
	return b.String()
}

func TestSIRiskClassifier_RuleMatrix(t *testing.T) {
	safeLow := &CriticReport{IsSafe: true, RiskLevel: "low"}
	cases := []struct {
		name        string
		patch       PatcherOutput
		critic      *CriticReport
		wantRisk    SelfImprovementRiskLevel
		wantChannel string
		wantRule    string // first rule hit
	}{
		// R5 — protected files reject outright.
		{
			name:        "R5 protected Makefile",
			patch:       PatcherOutput{Diff: siRiskDiff("Makefile", 5), Kind: PatchKindConfig},
			critic:      safeLow,
			wantRisk:    RiskLevelHigh,
			wantChannel: "reject",
			wantRule:    "R5",
		},
		{
			name:        "R5 protected ent generated",
			patch:       PatcherOutput{Diff: siRiskDiff("internal/data/ent/client.go", 5), Kind: PatchKindCode},
			critic:      safeLow,
			wantRisk:    RiskLevelHigh,
			wantChannel: "reject",
			wantRule:    "R5",
		},
		{
			name:        "R5 modify existing proto rejects",
			patch:       PatcherOutput{Diff: siRiskDiff("api/kratos/admin/v1/x.proto", 5), Kind: PatchKindCode},
			critic:      safeLow,
			wantRisk:    RiskLevelHigh,
			wantChannel: "reject",
			wantRule:    "R5",
		},
		// R4 — critic force-upgrade beats base low.
		{
			name:        "R4 unsafe critic upgrades docs patch",
			patch:       PatcherOutput{Diff: siRiskDiff("docs/development/x.md", 10), Kind: PatchKindDocs},
			critic:      &CriticReport{IsSafe: false, RiskLevel: "high", Concerns: []string{"bad"}},
			wantRisk:    RiskLevelHigh,
			wantChannel: "approval",
			wantRule:    "R1",
		},
		{
			name:        "R4 medium-critic safe keeps base",
			patch:       PatcherOutput{Diff: siRiskDiff("docs/development/x.md", 10), Kind: PatchKindDocs},
			critic:      &CriticReport{IsSafe: true, RiskLevel: "medium"},
			wantRisk:    RiskLevelLow,
			wantChannel: "auto",
			wantRule:    "R1",
		},
		// R1 — soft kinds, single file, ≤100 lines.
		{
			name:        "R1 docs small",
			patch:       PatcherOutput{Diff: siRiskDiff("docs/development/x.md", 60), Kind: PatchKindDocs},
			critic:      safeLow,
			wantRisk:    RiskLevelLow,
			wantChannel: "auto",
			wantRule:    "R1",
		},
		{
			name:        "R1 prompt small",
			patch:       PatcherOutput{Diff: siRiskDiff("internal/scenario/system/prompts/x/x.md", 100), Kind: PatchKindPrompt},
			critic:      safeLow,
			wantRisk:    RiskLevelLow,
			wantChannel: "auto",
			wantRule:    "R1",
		},
		{
			name:        "R1 i18n path treated as soft",
			patch:       PatcherOutput{Diff: siRiskDiff("web/src/i18n/zh-CN/index.ts", 20), Kind: PatchKindCode},
			critic:      safeLow,
			wantRisk:    RiskLevelLow,
			wantChannel: "auto",
			wantRule:    "R1",
		},
		// R3 — multi-file / core path / large diff.
		{
			name:        "R3 multi file",
			patch:       PatcherOutput{Diff: siRiskDiff("docs/a.md", 5) + siRiskDiff("docs/b.md", 5), Kind: PatchKindDocs},
			critic:      safeLow,
			wantRisk:    RiskLevelHigh,
			wantChannel: "approval",
			wantRule:    "R3",
		},
		{
			name:        "R3 core path internal/agent",
			patch:       PatcherOutput{Diff: siRiskDiff("internal/agent/v2/sequencer.go", 10), Kind: PatchKindCode},
			critic:      safeLow,
			wantRisk:    RiskLevelHigh,
			wantChannel: "approval",
			wantRule:    "R3",
		},
		{
			name:        "R3 core path chat service",
			patch:       PatcherOutput{Diff: siRiskDiff("internal/service/chat_message.go", 10), Kind: PatchKindCode},
			critic:      safeLow,
			wantRisk:    RiskLevelHigh,
			wantChannel: "approval",
			wantRule:    "R3",
		},
		{
			name:        "R3 ent schema path",
			patch:       PatcherOutput{Diff: siRiskDiff("internal/data/ent/schema/agent.go", 10), Kind: PatchKindCode},
			critic:      safeLow,
			wantRisk:    RiskLevelHigh,
			wantChannel: "approval",
			wantRule:    "R3",
		},
		{
			name:        "R3 new proto file",
			patch:       PatcherOutput{Diff: siRiskAddedDiff("api/kratos/admin/v1/x.proto", 10), Kind: PatchKindCode},
			critic:      safeLow,
			wantRisk:    RiskLevelHigh,
			wantChannel: "approval",
			wantRule:    "R3",
		},
		{
			name:        "R3 new ddl migration",
			patch:       PatcherOutput{Diff: siRiskAddedDiff("internal/data/sql/migrations/20260730_x.sql", 10), Kind: PatchKindCode},
			critic:      safeLow,
			wantRisk:    RiskLevelHigh,
			wantChannel: "approval",
			wantRule:    "R3",
		},
		{
			name:        "R3 large diff",
			patch:       PatcherOutput{Diff: siRiskDiff("internal/biz/x.go", 301), Kind: PatchKindCode},
			critic:      safeLow,
			wantRisk:    RiskLevelHigh,
			wantChannel: "approval",
			wantRule:    "R3",
		},
		// R2 — single business-code file, non-core, ≤300 lines.
		{
			name:        "R2 code single file",
			patch:       PatcherOutput{Diff: siRiskDiff("internal/biz/x.go", 120), Kind: PatchKindCode},
			critic:      safeLow,
			wantRisk:    RiskLevelMedium,
			wantChannel: "notify",
			wantRule:    "R2",
		},
		{
			name:        "R2 default bucket: docs 200 lines single file",
			patch:       PatcherOutput{Diff: siRiskDiff("docs/development/x.md", 200), Kind: PatchKindDocs},
			critic:      safeLow,
			wantRisk:    RiskLevelMedium,
			wantChannel: "notify",
			wantRule:    "R2",
		},
		// Nil critic degrades to no R4 escalation.
		{
			name:        "nil critic tolerated",
			patch:       PatcherOutput{Diff: siRiskDiff("internal/biz/x.go", 50), Kind: PatchKindCode},
			critic:      nil,
			wantRisk:    RiskLevelMedium,
			wantChannel: "notify",
			wantRule:    "R2",
		},
	}
	clf := NewSIRiskClassifier()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := clf.Classify(c.patch, c.critic)
			if got.RiskLevel != c.wantRisk {
				t.Errorf("RiskLevel = %q, want %q", got.RiskLevel, c.wantRisk)
			}
			if got.Channel != c.wantChannel {
				t.Errorf("Channel = %q, want %q", got.Channel, c.wantChannel)
			}
			if len(got.RuleHits) == 0 || got.RuleHits[0] != c.wantRule {
				t.Errorf("RuleHits = %v, want first %q", got.RuleHits, c.wantRule)
			}
			// R4 escalation must be recorded as an additional hit.
			if strings.HasPrefix(c.name, "R4 unsafe") {
				found := false
				for _, h := range got.RuleHits {
					if h == "R4" {
						found = true
					}
				}
				if !found {
					t.Errorf("R4 escalation missing from RuleHits: %v", got.RuleHits)
				}
			}
		})
	}
}

func TestSIRiskClassifier_CustomRules(t *testing.T) {
	safe := &CriticReport{IsSafe: true, RiskLevel: "low"}

	t.Run("custom low threshold", func(t *testing.T) {
		clf := NewSIRiskClassifierWithRules(SIRiskRules{LowMaxLines: 30})
		got := clf.Classify(PatcherOutput{Diff: siRiskDiff("docs/development/x.md", 60), Kind: PatchKindDocs}, safe)
		if got.RiskLevel != RiskLevelMedium || got.RuleHits[0] != "R2" {
			t.Errorf("60-line docs patch with LowMaxLines=30: got %+v, want medium/R2", got)
		}
	})

	t.Run("custom core globs replace defaults", func(t *testing.T) {
		clf := NewSIRiskClassifierWithRules(SIRiskRules{CorePathGlobs: []string{"custom/core/**"}})
		// Default core path no longer escalates.
		got := clf.Classify(PatcherOutput{Diff: siRiskDiff("internal/agent/v2/sequencer.go", 10), Kind: PatchKindCode}, safe)
		if got.RiskLevel != RiskLevelMedium {
			t.Errorf("default core path with custom globs: got %+v, want medium", got)
		}
		// Custom core path escalates.
		got = clf.Classify(PatcherOutput{Diff: siRiskDiff("custom/core/x.go", 10), Kind: PatchKindCode}, safe)
		if got.RiskLevel != RiskLevelHigh || got.RuleHits[0] != "R3" {
			t.Errorf("custom core path hit: got %+v, want high/R3", got)
		}
	})

	t.Run("zero rules inherit defaults", func(t *testing.T) {
		clf := NewSIRiskClassifierWithRules(SIRiskRules{})
		got := clf.Classify(PatcherOutput{Diff: siRiskDiff("docs/development/x.md", 60), Kind: PatchKindDocs}, safe)
		if got.RiskLevel != RiskLevelLow || got.RuleHits[0] != "R1" {
			t.Errorf("zero rules should behave like defaults: got %+v, want low/R1", got)
		}
	})
}

func TestNormalizeSIRiskRules(t *testing.T) {
	got := NormalizeSIRiskRules(SIRiskRules{})
	def := DefaultSIRiskRules()
	if got.LowMaxLines != def.LowMaxLines || got.MediumMaxLines != def.MediumMaxLines ||
		got.DailyAutoQuota != def.DailyAutoQuota || len(got.CorePathGlobs) != len(def.CorePathGlobs) {
		t.Errorf("zero rules should normalize to defaults: got %+v want %+v", got, def)
	}
	custom := NormalizeSIRiskRules(SIRiskRules{LowMaxLines: 10, DailyAutoQuota: 2})
	if custom.LowMaxLines != 10 || custom.DailyAutoQuota != 2 {
		t.Errorf("set fields must survive normalization: got %+v", custom)
	}
	if custom.MediumMaxLines != def.MediumMaxLines || len(custom.CorePathGlobs) != len(def.CorePathGlobs) {
		t.Errorf("unset fields should inherit defaults: got %+v", custom)
	}
}

func TestSIRiskClassifier_ChannelMapping(t *testing.T) {
	clf := NewSIRiskClassifier()
	safe := &CriticReport{IsSafe: true, RiskLevel: "low"}
	for risk, channel := range map[SelfImprovementRiskLevel]string{
		RiskLevelLow:    "auto",
		RiskLevelMedium: "notify",
		RiskLevelHigh:   "approval",
	} {
		if got := clf.channelFor(risk); got != channel {
			t.Errorf("channelFor(%q) = %q, want %q", risk, got, channel)
		}
	}
	// Unknown risk degrades to approval (most conservative non-reject channel).
	if got := clf.channelFor(SelfImprovementRiskLevel("bogus")); got != "approval" {
		t.Errorf("channelFor(bogus) = %q, want approval", got)
	}
	_ = safe
}
