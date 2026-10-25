package skillruntime

import (
	"regexp"
	"strings"

	"aranea-agents/internal/biz"
)

// skillMentionRE matches `$slug` tokens (letters, digits, underscore, hyphen).
var skillMentionRE = regexp.MustCompile(`\$([A-Za-z][A-Za-z0-9_-]{0,63})`)

// ParseSkillMentions returns distinct `$skill` slugs from a user query,
// lowercased, in first-seen order.
func ParseSkillMentions(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	matches := skillMentionRE.FindAllStringSubmatch(query, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		slug := strings.ToLower(strings.TrimSpace(m[1]))
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	return out
}

// mergeMentionedSlugs prepends Layer-A-allowed `$skill` mentions so they
// load without a second search. Mentions consume the cap first; excess
// routed slugs are dropped.
//
// D4-c mention 坐席（就绪不翻）：blockedByTrustGate 非 nil 时，命中候选却
// 低于 L2 信任门槛的提及被死拒（reasons[slug]=blocked_by_trust_gate）——
// 候选集与 L1 概览不受影响（门禁语义：候选集不剔）。
func mergeMentionedSlugs(routed []string, query string, afterA []biz.SkillRuntimeCandidate, reasons map[string]string, max int, blockedByTrustGate func(biz.SkillRuntimeCandidate) bool) []string {
	mentions := ParseSkillMentions(query)
	if len(mentions) == 0 {
		return routed
	}
	allowed := map[string]bool{}
	var candBySlug map[string]biz.SkillRuntimeCandidate
	if blockedByTrustGate != nil {
		candBySlug = make(map[string]biz.SkillRuntimeCandidate, len(afterA))
	}
	for _, c := range afterA {
		slug := strings.ToLower(strings.TrimSpace(c.Slug))
		if slug != "" {
			allowed[slug] = true
			if candBySlug != nil {
				candBySlug[slug] = c
			}
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, slug := range mentions {
		if !allowed[slug] || seen[slug] {
			if !allowed[slug] && reasons != nil && reasons[slug] == "" {
				reasons[slug] = "mention not in enabled catalog"
			}
			continue
		}
		// D4-c mention 坐席：候选在目录但低于信任门槛 → 死拒提及式加载。
		if blockedByTrustGate != nil {
			if cand, ok := candBySlug[slug]; ok && blockedByTrustGate(cand) {
				if reasons != nil && reasons[slug] == "" {
					reasons[slug] = "blocked_by_trust_gate"
				}
				continue
			}
		}
		seen[slug] = true
		if reasons != nil {
			reasons[slug] = "user mention"
		}
		out = append(out, slug)
	}
	for _, slug := range routed {
		slug = strings.ToLower(strings.TrimSpace(slug))
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	if max > 0 && len(out) > max {
		for _, dropped := range out[max:] {
			if reasons != nil && reasons[dropped] != "user mention" {
				reasons[dropped] = "exceeded max_skills_in_toolset cap"
			}
		}
		// Never drop an in-cap mention; if mentions alone exceed max, keep them.
		keep := max
		if mentionCount := countMentionReasons(out, reasons); mentionCount > keep {
			keep = mentionCount
		}
		out = out[:keep]
	}
	return out
}

func countMentionReasons(slugs []string, reasons map[string]string) int {
	n := 0
	for _, s := range slugs {
		if reasons != nil && reasons[s] == "user mention" {
			n++
		}
	}
	return n
}
