package skillruntime

import (
	"context"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/skillrouter"
	"aranea-agents/pkg/strutil"
)

// SkillToolsetOptions narrows which published Skills are mounted for one turn (layer A + B).
type SkillToolsetOptions struct {
	Runtime   *biz.AgentRuntimeSettings
	UserQuery string
}

// ResolveSkillSlugs applies Layer A (allow/deny) and Layer B (intent + tags + score cap).
func ResolveSkillSlugs(ctx context.Context, skillUC *biz.SkillUsecase, opts *SkillToolsetOptions) ([]string, error) {
	candidates, err := skillUC.ListEnabledPublishedSkillCandidates(ctx)
	if err != nil {
		return nil, err
	}
	rawPolicy := "{}"
	if opts != nil && opts.Runtime != nil && strings.TrimSpace(opts.Runtime.SkillRuntimeJSON) != "" {
		rawPolicy = opts.Runtime.SkillRuntimeJSON
	}
	policy := biz.ParseSkillRuntimePolicy(rawPolicy)
	query := ""
	if opts != nil {
		query = opts.UserQuery
	}

	afterA := applyLayerA(candidates, policy)

	paths := []string(nil)
	if policy.IntentRoutingEnabled && strings.TrimSpace(query) != "" {
		paths = skillrouter.DetectIntentPaths(query, policy.IntentMaxPaths)
	}

	afterB := afterA
	if len(paths) > 0 {
		narrowed := filterByIntentPaths(afterA, paths)
		if len(narrowed) > 0 {
			afterB = narrowed
		}
	}

	requiredTags := mergeTagRequirements(policy.AllowedTags, skillrouter.ExtractTagHints(query))
	final := afterB
	if len(requiredTags) > 0 {
		tagged := filterByAllTags(afterB, requiredTags)
		if len(tagged) > 0 {
			final = tagged
		}
	}

	scored := scoreCandidates(final, paths)
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].slug < scored[j].slug
	})

	out := make([]string, 0, len(scored))
	for _, s := range scored {
		out = append(out, s.slug)
		if len(out) >= policy.MaxSkillsInToolset {
			break
		}
	}
	return out, nil
}

type slugScore struct {
	slug  string
	score int
}

func applyLayerA(in []biz.SkillRuntimeCandidate, policy biz.SkillRuntimePolicy) []biz.SkillRuntimeCandidate {
	deny := strutil.SliceToSet(policy.DeniedSlugs)
	allow := strutil.SliceToSet(policy.AllowedSlugs)
	out := make([]biz.SkillRuntimeCandidate, 0, len(in))
	for _, c := range in {
		slug := strings.TrimSpace(strings.ToLower(c.Slug))
		if slug == "" || deny[slug] {
			continue
		}
		if len(allow) > 0 && !allow[slug] {
			continue
		}
		out = append(out, c)
	}
	return out
}

func mergeTagRequirements(policyTags, hints []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(policyTags)+len(hints))
	for _, t := range policyTags {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	for _, t := range hints {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func filterByAllTags(in []biz.SkillRuntimeCandidate, required []string) []biz.SkillRuntimeCandidate {
	out := make([]biz.SkillRuntimeCandidate, 0, len(in))
	for _, c := range in {
		if skillHasAllTags(c, required) {
			out = append(out, c)
		}
	}
	return out
}

func skillHasAllTags(c biz.SkillRuntimeCandidate, required []string) bool {
	if len(required) == 0 {
		return true
	}
	tagNames := map[string]bool{}
	for _, t := range c.Tags {
		n := strings.TrimSpace(strings.ToLower(t.Name))
		if n != "" {
			tagNames[n] = true
		}
	}
	for _, req := range required {
		if !tagNames[req] {
			return false
		}
	}
	return true
}

func filterByIntentPaths(in []biz.SkillRuntimeCandidate, paths []string) []biz.SkillRuntimeCandidate {
	out := make([]biz.SkillRuntimeCandidate, 0, len(in))
	for _, c := range in {
		if matchesAnyIntentPath(c, paths) {
			out = append(out, c)
		}
	}
	return out
}

func matchesAnyIntentPath(c biz.SkillRuntimeCandidate, paths []string) bool {
	for _, p := range paths {
		if skillMatchesPath(c, strings.TrimSpace(p)) {
			return true
		}
	}
	return false
}

func skillMatchesPath(c biz.SkillRuntimeCandidate, path string) bool {
	if path == "" {
		return false
	}
	pathLower := strings.ToLower(path)
	for _, tp := range c.TaxonomyPaths {
		tp = strings.TrimSpace(tp)
		if tp == "" {
			continue
		}
		tpl := strings.ToLower(tp)
		if tpl == pathLower || strings.Contains(tpl, pathLower) || strings.Contains(pathLower, tpl) {
			return true
		}
	}
	corpus := strings.ToLower(strings.TrimSpace(c.Slug + " " + c.Name + " " + c.Description))
	for _, leaf := range skillrouter.TaxonomyLeaves() {
		if strings.TrimSpace(leaf.Path) != strings.TrimSpace(path) {
			continue
		}
		for _, kw := range leaf.Keywords {
			kw = strings.TrimSpace(strings.ToLower(kw))
			if kw != "" && strings.Contains(corpus, kw) {
				return true
			}
		}
		return false
	}
	return false
}

func scoreCandidates(in []biz.SkillRuntimeCandidate, paths []string) []slugScore {
	pathSet := map[string]bool{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p != "" {
			pathSet[p] = true
		}
	}
	out := make([]slugScore, 0, len(in))
	for _, c := range in {
		sc := 0
		for _, tp := range c.TaxonomyPaths {
			tp = strings.TrimSpace(tp)
			for p := range pathSet {
				if tp == "" || p == "" {
					continue
				}
				tpl := strings.ToLower(tp)
				pl := strings.ToLower(p)
				if tpl == pl {
					sc += 1000
				} else if strings.Contains(tpl, pl) || strings.Contains(pl, tpl) {
					sc += 400
				}
			}
		}
		if sc == 0 && len(paths) > 0 {
			for _, p := range paths {
				if skillMatchesPath(c, strings.TrimSpace(p)) {
					sc += 100
					break
				}
			}
		}
		out = append(out, slugScore{slug: c.Slug, score: sc})
	}
	return out
}
