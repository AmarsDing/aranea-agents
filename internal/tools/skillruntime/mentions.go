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
func mergeMentionedSlugs(routed []string, query string, afterA []biz.SkillRuntimeCandidate, reasons map[string]string, max int) []string {
	mentions := ParseSkillMentions(query)
	if len(mentions) == 0 {
		return routed
	}
	allowed := map[string]bool{}
	for _, c := range afterA {
		slug := strings.ToLower(strings.TrimSpace(c.Slug))
		if slug != "" {
			allowed[slug] = true
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
