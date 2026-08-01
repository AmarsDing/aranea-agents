package biz

import (
	"github.com/bmatcuk/doublestar/v4"
)

// ── RiskClassifier (design D6, 73-self-iteration-v3) ────────────────────────
//
// Pure-code risk classification of a Patcher patch + Critic report into a
// GovernanceDecision (risk level + apply channel). No ctx, no IO — the same
// input always yields the same decision (table-driven rules R1-R5).

// Default rule thresholds (D6).
const (
	siRiskLowMaxLines    = 100 // R1: soft-kind single-file diff line cap
	siRiskMediumMaxLines = 300 // R2/R3: business-code single-file line cap
)

// siRiskCorePathGlobs are the default R3 core paths (D6): any hit escalates
// to high. Used when SIRiskRules.CorePathGlobs is empty.
var siRiskCorePathGlobs = []string{
	"internal/service/chat*",          // 聊天服务核心
	"internal/agent/**",               // Agent 运行时
	"internal/data/ent/schema/**",     // 数据模型
	"**/*.proto",                      // Proto 契约
	"internal/data/sql/migrations/**", // DDL 迁移（含新增）
}

// SIRiskRules is the admin-configurable view of the D6 risk-classification
// rule set (P5 console, UpdateRiskRules). Zero fields mean "inherit the code
// default" so a freshly-migrated row (all zeros) behaves exactly like the
// hardcoded D6 rules.
type SIRiskRules struct {
	LowMaxLines    int      // R1 soft-kind single-file diff line cap; ≤0 → 100
	MediumMaxLines int      // R2/R3 single-file line cap; ≤0 → 300
	CorePathGlobs  []string // R3 core path globs; empty → default D6 set
	DailyAutoQuota int32    // D10 daily auto-apply quota; ≤0 → code default/conf
}

// DefaultSIRiskRules returns the code-default D6/D10 rule set.
func DefaultSIRiskRules() SIRiskRules {
	return SIRiskRules{
		LowMaxLines:    siRiskLowMaxLines,
		MediumMaxLines: siRiskMediumMaxLines,
		CorePathGlobs:  append([]string(nil), siRiskCorePathGlobs...),
		DailyAutoQuota: DefaultSIAutoApplyQuotaPerDay,
	}
}

// NormalizeSIRiskRules fills zero/empty fields with the code defaults so
// consumers never branch on "unset".
func NormalizeSIRiskRules(r SIRiskRules) SIRiskRules {
	d := DefaultSIRiskRules()
	if r.LowMaxLines <= 0 {
		r.LowMaxLines = d.LowMaxLines
	}
	if r.MediumMaxLines <= 0 {
		r.MediumMaxLines = d.MediumMaxLines
	}
	if len(r.CorePathGlobs) == 0 {
		r.CorePathGlobs = d.CorePathGlobs
	}
	if r.DailyAutoQuota <= 0 {
		r.DailyAutoQuota = d.DailyAutoQuota
	}
	return r
}

// SIRiskClassifier classifies patches per the D6 rule matrix.
// Stability:evolving
type SIRiskClassifier struct {
	protectedRules []ProtectedFileRule
	rules          SIRiskRules
}

// NewSIRiskClassifier returns a classifier with the default D6 rule set.
func NewSIRiskClassifier() *SIRiskClassifier {
	return NewSIRiskClassifierWithRules(SIRiskRules{})
}

// NewSIRiskClassifierWithRules returns a classifier on the admin-configured
// rule set (P5); zero fields inherit the D6 defaults.
func NewSIRiskClassifierWithRules(rules SIRiskRules) *SIRiskClassifier {
	return &SIRiskClassifier{
		protectedRules: DefaultProtectedFileRules(),
		rules:          NormalizeSIRiskRules(rules),
	}
}

// Classify evaluates R5 (protected → reject) → base tier (R1/R3/R2) → R4
// (critic force-upgrade) and maps the final tier to an apply channel.
// A nil critic skips R4 (Critic stage degraded — e.g. quota exhausted).
func (c *SIRiskClassifier) Classify(p PatcherOutput, critic *CriticReport) GovernanceDecision {
	if c == nil {
		c = NewSIRiskClassifier()
	}
	changes := ParseUnifiedDiffFiles(p.Diff)
	stats := ComputeDiffStats(p.Diff)
	lines := stats.Additions + stats.Deletions

	// R5: protected-file hit rejects before any tiering.
	if hits := CheckProtectedFiles(changes, c.protectedRules); len(hits) > 0 {
		return GovernanceDecision{
			RiskLevel: RiskLevelHigh,
			Channel:   "reject",
			RuleHits:  []string{"R5"},
		}
	}

	// Base tier.
	var risk SelfImprovementRiskLevel
	var rule string
	switch {
	case len(changes) == 1 && lines <= c.rules.LowMaxLines && siRiskSoftChange(p.Kind, changes):
		risk, rule = RiskLevelLow, "R1"
	case len(changes) > 1 || lines > c.rules.MediumMaxLines || siRiskHitsCorePath(changes, c.rules.CorePathGlobs):
		risk, rule = RiskLevelHigh, "R3"
	default:
		// R2 为中位默认桶：未命中 R1/R3 的补丁按 medium 处理
		// （D6 未覆盖组合——如单文件 docs 100-300 行——的保守落点）。
		risk, rule = RiskLevelMedium, "R2"
	}
	hits := []string{rule}

	// R4: critic force-upgrade.
	if critic != nil && (!critic.IsSafe || critic.RiskLevel == string(RiskLevelHigh)) {
		if risk != RiskLevelHigh {
			risk = RiskLevelHigh
		}
		hits = append(hits, "R4")
	}

	return GovernanceDecision{
		RiskLevel: risk,
		Channel:   c.channelFor(risk),
		RuleHits:  hits,
	}
}

// channelFor maps a risk tier to its apply channel (D6):
// low→auto、medium→auto+notify、high→approval；未知等级落 approval（保守）。
func (c *SIRiskClassifier) channelFor(risk SelfImprovementRiskLevel) string {
	switch risk {
	case RiskLevelLow:
		return "auto"
	case RiskLevelMedium:
		return "notify"
	default:
		return "approval"
	}
}

// siRiskSoftChange reports whether the change qualifies for R1: the declared
// kind is a soft kind (config/prompt/docs/test；i18n 按路径归入), or every
// touched file lives under an i18n directory.
func siRiskSoftChange(kind SelfImprovementPatchKind, changes []PatchFileChange) bool {
	switch kind {
	case PatchKindConfig, PatchKindPrompt, PatchKindDocs, PatchKindTest:
		return true
	}
	for _, ch := range changes {
		p := ch.Path
		if p == "" {
			p = ch.OldPath
		}
		if ok, err := doublestar.Match("**/i18n/**", p); err != nil || !ok {
			return false
		}
	}
	return len(changes) > 0
}

// siRiskHitsCorePath reports whether any touched file matches an R3 core glob.
// An empty globs set means "no core paths" (callers pass normalized rules).
func siRiskHitsCorePath(changes []PatchFileChange, globs []string) bool {
	for _, ch := range changes {
		for _, p := range []string{ch.Path, ch.OldPath} {
			if p == "" {
				continue
			}
			for _, glob := range globs {
				if ok, err := doublestar.Match(glob, p); err == nil && ok {
					return true
				}
			}
		}
	}
	return false
}

// SICoreAreas returns the set of R3 core-path area identifiers (the matched
// glob patterns) touched by a unified diff. Two runs whose area sets
// intersect must not observe concurrently (design §九: 同核心路径补丁串行,
// D10 观察窗指标隔离). globs follows the same convention as
// siRiskHitsCorePath; pass NormalizeSIRiskRules(...).CorePathGlobs.
func SICoreAreas(diff string, globs []string) map[string]bool {
	areas := map[string]bool{}
	for _, ch := range ParseUnifiedDiffFiles(diff) {
		for _, p := range []string{ch.Path, ch.OldPath} {
			if p == "" {
				continue
			}
			for _, glob := range globs {
				if ok, err := doublestar.Match(glob, p); err == nil && ok {
					areas[glob] = true
				}
			}
		}
	}
	return areas
}

// SICoreAreasIntersect reports whether two core-area sets share an area.
func SICoreAreasIntersect(a, b map[string]bool) bool {
	for area := range a {
		if b[area] {
			return true
		}
	}
	return false
}
