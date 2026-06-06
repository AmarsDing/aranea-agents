package skillruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/skillrecommend"
	"aranea-agents/internal/tools/skillrouter"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"
)

// SkillToolsetOptions narrows which published Skills are mounted for one turn (layer A + B).
type SkillToolsetOptions struct {
	Runtime   RuntimeSettings
	UserQuery string
	// HealthProvider provides historical performance data for ranking. If nil, ranking is skipped.
	HealthProvider skillrecommend.HealthMetricsProvider
}

// ResolveResult holds the output of skill resolution with per-slug reasons.
type ResolveResult struct {
	Slugs   []string
	Reasons map[string]string
}

// ResolveSkillSlugs applies Layer A (allow/deny) and Layer B (intent + tags + score cap).
func ResolveSkillSlugs(ctx context.Context, skillUC SkillResolver, opts *SkillToolsetOptions) ([]string, error) {
	result, err := ResolveSkillSlugsDetailed(ctx, skillUC, opts, nil)
	if err != nil {
		return nil, err
	}
	return result.Slugs, nil
}

func ResolveSkillSlugsDetailed(ctx context.Context, skillUC SkillResolver, opts *SkillToolsetOptions, lg loggateway.Logger) (*ResolveResult, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	candidates, err := skillUC.ListEnabledPublishedSkillCandidates(ctx)
	if err != nil {
		return nil, err
	}
	rawPolicy := "{}"
	if opts != nil && opts.Runtime != nil && strings.TrimSpace(opts.Runtime.GetSkillRuntimeJSON()) != "" {
		rawPolicy = opts.Runtime.GetSkillRuntimeJSON()
	}
	policy := biz.ParseSkillRuntimePolicy(rawPolicy)
	query := ""
	if opts != nil {
		query = opts.UserQuery
	}

	reasons := make(map[string]string, len(candidates))

	afterA := applyLayerAWithReasons(candidates, policy, reasons)

	paths := []string(nil)
	if policy.IntentRoutingEnabled && strings.TrimSpace(query) != "" {
		paths = skillrouter.DetectIntentPaths(query, policy.IntentMaxPaths)
	}

	afterB := afterA
	if len(paths) > 0 {
		narrowed := filterByIntentPathsWithReasons(afterA, paths, reasons)
		if len(narrowed) > 0 {
			afterB = narrowed
		}
	}

	requiredTags := mergeTagRequirements(policy.AllowedTags, skillrouter.ExtractTagHints(query))
	final := afterB
	if len(requiredTags) > 0 {
		tagged := filterByAllTagsWithReasons(afterB, requiredTags, reasons)
		if len(tagged) > 0 {
			final = tagged
		}
	}

	scored := scoreCandidatesWithReasons(final, paths, reasons)

	if policy.EmbeddingScoringEnabled && strings.TrimSpace(query) != "" {
		embScores, embErr := skillUC.ScoreByEmbedding(ctx, query, final)
		if embErr != nil {
			lg.Warn("embedding scoring failed; falling back to keyword scores",
				loggateway.StepID("tool.skillruntime.embedding_score_fail"),
				loggateway.Err(embErr))
		} else if len(embScores) > 0 {
			weight := policy.EmbeddingScoreWeight
			for i := range scored {
				if sim, ok := embScores[scored[i].slug]; ok {
					scored[i].score += int(sim * 1000 * weight)
					if scored[i].reason == "enabled and published" || scored[i].reason == "no intent match; included by default" {
						scored[i].reason = "embedding similarity: " + formatSimilarity(sim)
					} else {
						scored[i].reason += " + embedding: " + formatSimilarity(sim)
					}
					reasons[scored[i].slug] = scored[i].reason
				}
			}
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].slug < scored[j].slug
	})

	// Apply historical performance ranking if provider is available.
	if opts != nil && opts.HealthProvider != nil {
		candidates := buildRankCandidates(ctx, scored, opts.HealthProvider, lg)
		if len(candidates) > 0 {
			factors := skillrecommend.DynamicRankFactors(opts.HealthProvider, candidates)
			ranked := skillrecommend.Rank(candidates, factors)
			applyRankResults(scored, ranked, reasons)
		}
	}

	out := make([]string, 0, len(scored))
	for _, s := range scored {
		out = append(out, s.slug)
		if len(out) >= policy.MaxSkillsInToolset {
			for _, remaining := range scored[len(out):] {
				reasons[remaining.slug] = "exceeded max_skills_in_toolset cap"
			}
			break
		}
	}
	return &ResolveResult{Slugs: out, Reasons: reasons}, nil
}

type slugScore struct {
	slug   string
	score  int
	reason string
}

func formatSimilarity(sim float64) string {
	return fmt.Sprintf("%.2f", sim)
}

func applyLayerAWithReasons(in []biz.SkillRuntimeCandidate, policy biz.SkillRuntimePolicy, reasons map[string]string) []biz.SkillRuntimeCandidate {
	deny := strutil.SliceToSet(policy.DeniedSlugs)
	allow := strutil.SliceToSet(policy.AllowedSlugs)
	out := make([]biz.SkillRuntimeCandidate, 0, len(in))
	for _, c := range in {
		slug := strings.TrimSpace(strings.ToLower(c.Slug))
		if slug == "" {
			reasons[c.Slug] = "empty slug"
			continue
		}
		if deny[slug] {
			reasons[slug] = "denied by policy"
			continue
		}
		if len(allow) > 0 && !allow[slug] {
			reasons[slug] = "not in allowed slugs"
			continue
		}
		reasons[slug] = "passed layer A"
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

func filterByAllTagsWithReasons(in []biz.SkillRuntimeCandidate, required []string, reasons map[string]string) []biz.SkillRuntimeCandidate {
	out := make([]biz.SkillRuntimeCandidate, 0, len(in))
	for _, c := range in {
		if skillHasAllTags(c, required) {
			out = append(out, c)
		} else {
			reasons[c.Slug] = "missing required tags"
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

func filterByIntentPathsWithReasons(in []biz.SkillRuntimeCandidate, paths []string, reasons map[string]string) []biz.SkillRuntimeCandidate {
	out := make([]biz.SkillRuntimeCandidate, 0, len(in))
	for _, c := range in {
		if matchesAnyIntentPath(c, paths) {
			out = append(out, c)
		} else {
			reasons[c.Slug] = "no intent path match"
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

func scoreCandidatesWithReasons(in []biz.SkillRuntimeCandidate, paths []string, reasons map[string]string) []slugScore {
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
		matchDetail := ""
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
					matchDetail = "exact taxonomy path match"
				} else if strings.Contains(tpl, pl) || strings.Contains(pl, tpl) {
					sc += 400
					matchDetail = "partial taxonomy path match"
				}
			}
		}
		if sc == 0 && len(paths) > 0 {
			for _, p := range paths {
				if skillMatchesPath(c, strings.TrimSpace(p)) {
					sc += 100
					matchDetail = "keyword match"
					break
				}
			}
		}
		if matchDetail == "" {
			if len(paths) > 0 {
				matchDetail = "no intent match; included by default"
			} else {
				matchDetail = "enabled and published"
			}
		}
		reasons[c.Slug] = matchDetail
		out = append(out, slugScore{slug: c.Slug, score: sc, reason: matchDetail})
	}
	return out
}

// buildRankCandidates converts scored slugs into skillrecommend.Candidate structs
// by fetching historical health metrics from the aggregator.
func buildRankCandidates(ctx context.Context, scored []slugScore, healthProvider skillrecommend.HealthMetricsProvider, lg loggateway.Logger) []skillrecommend.Candidate {
	candidates := make([]skillrecommend.Candidate, 0, len(scored))
	for _, s := range scored {
		c := skillrecommend.Candidate{
			Slug:               s.slug,
			SemanticSimilarity: float64(s.score) / 1000.0,
		}
		if c.SemanticSimilarity > 1.0 {
			c.SemanticSimilarity = 1.0
		}
		successRate, err := healthProvider.GetRecentSuccessRate(ctx, s.slug, 30)
		if err != nil {
			lg.Warn("health metrics unavailable for ranking; using defaults",
				loggateway.StepID("tool.skillruntime.health_metrics_fail"),
				loggateway.Err(err))
		} else {
			c.HistoricalSuccess = successRate
		}
		avgDuration, err := healthProvider.GetRecentAvgDuration(ctx, s.slug, 30)
		if err == nil && avgDuration > 0 {
			c.LatencyInverse = 1.0 / (1.0 + avgDuration/1000.0)
		}
		candidates = append(candidates, c)
	}
	return candidates
}

// applyRankResults reorders scored candidates based on Rank results and updates reasons.
func applyRankResults(scored []slugScore, ranked []skillrecommend.RankResult, reasons map[string]string) {
	rankMap := make(map[string]skillrecommend.RankResult, len(ranked))
	for _, r := range ranked {
		rankMap[r.Slug] = r
	}
	for i := range scored {
		if r, ok := rankMap[scored[i].slug]; ok {
			scored[i].score = int(r.Score * 10000)
			scored[i].reason += " | " + skillrecommend.FormatSelectionReason(r)
			reasons[scored[i].slug] = scored[i].reason
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].slug < scored[j].slug
	})
}
