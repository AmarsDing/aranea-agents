package skillruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

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
	triggerHits := computeTriggerHits(afterA, query)

	paths := []string(nil)
	if policy.IntentRoutingEnabled && strings.TrimSpace(query) != "" {
		paths = skillrouter.DetectIntentPaths(query, policy.IntentMaxPaths)
	}

	afterB := afterA
	if len(paths) > 0 {
		narrowed := filterByIntentPathsWithReasons(afterA, paths, reasons)
		narrowed = reincludeTriggered(narrowed, afterA, triggerHits)
		if len(narrowed) > 0 {
			afterB = narrowed
		}
	}

	requiredTags := mergeTagRequirements(policy.AllowedTags, skillrouter.ExtractTagHints(query))
	final := afterB
	if len(requiredTags) > 0 {
		tagged := filterByAllTagsWithReasons(afterB, requiredTags, reasons)
		tagged = reincludeTriggered(tagged, afterB, triggerHits)
		if len(tagged) > 0 {
			final = tagged
		}
	}

	scored := scoreCandidatesWithReasons(final, paths, reasons)
	applyEmbeddingScores(ctx, skillUC, policy, query, final, scored, reasons, lg)
	applyTriggerHits(scored, triggerHits, reasons)

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
			factors := skillrecommend.DynamicRankFactors(ctx, opts.HealthProvider, candidates)
			ranked := skillrecommend.Rank(candidates, factors)
			applyRankResults(scored, ranked, reasons, triggerHits)
		}
	}

	out := capScoredSlugs(scored, policy.MaxSkillsInToolset, reasons)
	return &ResolveResult{Slugs: out, Reasons: reasons}, nil
}

// computeTriggerHits 返回 trigger 命中的 slug → 命中词映射（P1-3）。
// 在 Layer A 之后计算：deny 优先于 trigger，被拒候选不参与命中。
func computeTriggerHits(afterA []biz.SkillRuntimeCandidate, query string) map[string]string {
	hits := map[string]string{}
	if strings.TrimSpace(query) == "" {
		return hits
	}
	for _, c := range afterA {
		if hit := matchTrigger(query, c.Triggers); hit != "" {
			hits[c.Slug] = hit
		}
	}
	return hits
}

// applyEmbeddingScores 叠加 embedding 语义相似分（启用且非空 query 时）。
// 失败降级为 keyword 分，不阻断解析。
func applyEmbeddingScores(ctx context.Context, skillUC SkillResolver, policy biz.SkillRuntimePolicy, query string, final []biz.SkillRuntimeCandidate, scored []slugScore, reasons map[string]string, lg loggateway.Logger) {
	if !policy.EmbeddingScoringEnabled || strings.TrimSpace(query) == "" {
		return
	}
	embScores, embErr := skillUC.ScoreByEmbedding(ctx, query, final)
	if embErr != nil {
		lg.Warn("embedding scoring failed; falling back to keyword scores",
			loggateway.StepID("tool.skillruntime.embedding_score_fail"),
			loggateway.Err(embErr))
		return
	}
	if len(embScores) == 0 {
		return
	}
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

// applyTriggerHits 强制 trigger 命中候选置顶（P1-3）：排序分高于 taxonomy
// 精确匹配（1000），覆盖 intent/tag 过滤阶段写入的 reason，保证确定性 preload。
func applyTriggerHits(scored []slugScore, triggerHits map[string]string, reasons map[string]string) {
	for i := range scored {
		if hit, ok := triggerHits[scored[i].slug]; ok {
			scored[i].score = triggerScore
			scored[i].reason = "trigger match: " + hit
			reasons[scored[i].slug] = scored[i].reason
		}
	}
}

// capScoredSlugs 按 maxSkillsInToolset 截断并记录被截候选的 reason。
func capScoredSlugs(scored []slugScore, max int, reasons map[string]string) []string {
	out := make([]string, 0, len(scored))
	for _, s := range scored {
		out = append(out, s.slug)
		if len(out) >= max {
			for _, remaining := range scored[len(out):] {
				reasons[remaining.slug] = "exceeded max_skills_in_toolset cap"
			}
			break
		}
	}
	return out
}

type slugScore struct {
	slug string
	// skillID 是平台 ID（skill_<unixnano>），从 RuntimeCandidate 透传。
	// 历史健康指标查询（skill_invocation.skill_id 列）必须按平台 ID 匹配；
	// 为空（存量调用方未带 ID）时回退 slug 查询，保持兼容。
	skillID string
	score   int
	reason  string
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
		if tpl == pathLower {
			return true
		}
		// Path segment matching: split by "/" and check if any segment matches
		tplSegments := strings.Split(tpl, "/")
		pathSegments := strings.Split(pathLower, "/")
		for _, ts := range tplSegments {
			ts = strings.TrimSpace(ts)
			if ts == "" {
				continue
			}
			for _, ps := range pathSegments {
				ps = strings.TrimSpace(ps)
				if ps == "" {
					continue
				}
				if ts == ps {
					return true
				}
			}
		}
		// Keyword fallback: check if any segment contains the other
		for _, ts := range tplSegments {
			ts = strings.TrimSpace(ts)
			if ts == "" || len(ts) <= 2 {
				continue
			}
			for _, ps := range pathSegments {
				ps = strings.TrimSpace(ps)
				if ps == "" || len(ps) <= 2 {
					continue
				}
				if strings.Contains(ts, ps) || strings.Contains(ps, ts) {
					return true
				}
			}
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

// splitTaxonomySegments splits a taxonomy path into individual segments
// using common separators (/, -, _). This enables segment-level matching
// instead of substring matching, preventing "code" from matching "encode".
func splitTaxonomySegments(path string) []string {
	// First split by / to get path components, then split each by - and _
	var segments []string
	for _, part := range strings.Split(path, "/") {
		for _, sub := range strings.Split(part, "-") {
			for _, seg := range strings.Split(sub, "_") {
				seg = strings.TrimSpace(seg)
				if seg != "" {
					segments = append(segments, seg)
				}
			}
		}
	}
	return segments
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
		bestScore := 0
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
					if 1000 > bestScore {
						bestScore = 1000
						matchDetail = "exact taxonomy path match"
					}
				} else {
					for _, segment := range splitTaxonomySegments(tpl) {
						if segment == pl {
							if 400 > bestScore {
								bestScore = 400
								matchDetail = "partial taxonomy path match"
							}
							break
						}
					}
				}
			}
		}
		if bestScore == 0 && len(paths) > 0 {
			for _, p := range paths {
				if skillMatchesPath(c, strings.TrimSpace(p)) {
					if 100 > bestScore {
						bestScore = 100
						matchDetail = "keyword match"
					}
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
		out = append(out, slugScore{slug: c.Slug, skillID: c.SkillID, score: bestScore, reason: matchDetail})
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
		// skill_invocation.skill_id 列存平台 ID（skill_<unixnano>），健康指标
		// 必须按 ID 查询；候选未带 ID（存量调用方）时回退 slug 保持兼容。
		healthKey := s.skillID
		if healthKey == "" {
			healthKey = s.slug
		}
		successRate, err := healthProvider.GetRecentSuccessRate(ctx, healthKey, 30)
		if err != nil {
			lg.Warn("health metrics unavailable for ranking; using defaults",
				loggateway.StepID("tool.skillruntime.health_metrics_fail"),
				loggateway.Err(err))
		} else {
			c.HistoricalSuccess = successRate
		}
		avgDuration, err := healthProvider.GetRecentAvgDuration(ctx, healthKey, 30)
		if err == nil && avgDuration > 0 {
			c.LatencyInverse = 1.0 / (1.0 + avgDuration/1000.0)
		}
		candidates = append(candidates, c)
	}
	return candidates
}

// applyRankResults blends ranking scores with existing keyword+embedding scores
// using a weighted fusion rather than replacing them entirely.
// The rank score contributes 60% and the pre-existing score 40%, preserving
// semantic and intent signals while still respecting historical performance.
// protected（trigger 命中的 slug）跳过融合——确定性 preload 不被历史表现稀释。
func applyRankResults(scored []slugScore, ranked []skillrecommend.RankResult, reasons map[string]string, protected map[string]string) {
	rankMap := make(map[string]skillrecommend.RankResult, len(ranked))
	for _, r := range ranked {
		rankMap[r.Slug] = r
	}
	for i := range scored {
		if _, ok := protected[scored[i].slug]; ok {
			continue
		}
		if r, ok := rankMap[scored[i].slug]; ok {
			// Both scores are normalised to 0–1000 before fusion so that the
			// 60/40 weighting is dimensionally consistent. Previously rank
			// was scaled to 0–10000 while the existing score was 0–~2000,
			// causing rank to dominate at ~88% instead of the intended 60%.
			rankScore := int(r.Score * 1000)
			existingScore := scored[i].score
			if existingScore > 1000 {
				existingScore = 1000
			}
			blended := int(float64(existingScore)*0.4 + float64(rankScore)*0.6)
			scored[i].score = blended
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

// triggerScore 是 trigger 命中候选的排序分。必须高于 taxonomy 精确匹配
// （1000），保证确定性 preload 置顶；同时占用 max_skills_in_toolset 配额。
const triggerScore = 2000

// reincludeTriggered 把被过滤阶段（intent 收窄 / tag 过滤）剔除、但 trigger
// 命中的候选重新并入结果集。确定性触发优先于启发式过滤。
func reincludeTriggered(filtered, all []biz.SkillRuntimeCandidate, triggerHits map[string]string) []biz.SkillRuntimeCandidate {
	if len(triggerHits) == 0 {
		return filtered
	}
	present := make(map[string]bool, len(filtered))
	for _, c := range filtered {
		present[c.Slug] = true
	}
	out := filtered
	for _, c := range all {
		if _, ok := triggerHits[c.Slug]; !ok || present[c.Slug] {
			continue
		}
		out = append(out, c)
	}
	return out
}

// MatchTrigger 导出 matchTrigger 的确定性触发判定（P2 F4 触发率黄金集回归
// 复用运行时同一语义）。返回第一个命中用户输入的 trigger；未命中返回空串。
func MatchTrigger(query string, triggers []string) string {
	return matchTrigger(query, triggers)
}

// matchTrigger 返回第一个命中用户输入的 trigger；未命中返回空串。
// CJK trigger 使用子串语义（中文无词边界）；ASCII trigger 要求词边界匹配，
// 避免 "pdf" 误中 "pdftk"。大小写不敏感。
func matchTrigger(query string, triggers []string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return ""
	}
	for _, t := range triggers {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		tl := strings.ToLower(t)
		if containsCJK(t) {
			if strings.Contains(q, tl) {
				return t
			}
			continue
		}
		if asciiWordContains(q, tl) {
			return t
		}
	}
	return ""
}

// containsCJK 判断字符串是否包含 CJK 表意文字（中/日/韩）。
func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// asciiWordContains 在已小写的 query 中查找已小写的 ASCII trigger，
// 要求命中位置前后不是 ASCII 词字符（字母/数字/下划线）。
func asciiWordContains(queryLower, triggerLower string) bool {
	idx := 0
	for idx <= len(queryLower) {
		i := strings.Index(queryLower[idx:], triggerLower)
		if i < 0 {
			return false
		}
		start := idx + i
		end := start + len(triggerLower)
		beforeOK := start == 0 || !isASCIIWordChar(queryLower[start-1])
		afterOK := end == len(queryLower) || !isASCIIWordChar(queryLower[end])
		if beforeOK && afterOK {
			return true
		}
		idx = start + 1
	}
	return false
}

func isASCIIWordChar(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z')
}
