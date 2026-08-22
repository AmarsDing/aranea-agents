package deferred

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

// P1-4 + B3（2026-08-22）：deferred 工具语义预激活。
//
// 按当前用户 query 对 catalog 做字段化 BM25（name 权重大于 description），
// Top-N 以「推荐区」提升进 catalog cue。排序器与 tool_search 共享同一打分
// 逻辑，保证「搜索看到的」与「推荐的」一致。
//
// 精度护栏：子串 / BM25 子词要求 token ≥3 runes，防止短虚词噪声命中。
// 精确等值匹配不限长度。CJK query 仍靠 name 子词回扣（save ⊂ "please save"）。

// CatalogRecommendLimit 是推荐区展示的条目数上限。
const CatalogRecommendLimit = 3

const (
	bm25K1            = 1.2
	bm25B             = 0.75
	bm25NameWeight    = 4.0
	bm25BaseWeight    = 2.5
	bm25CatWeight     = 1.5
	bm25DescWeight    = 1.0
	exactNameBonus    = 10.0
	nameContainsBonus = 5.0
	catContainsBonus  = 3.0
	descContainsBonus = 2.0
	subwordBonus      = 4.0
)

type catalogStats struct {
	n      int
	avgLen float64
	df     map[string]int
}

// scoreEntryAgainstQuery 计算单个目录条目与 query 的相关度得分。
// tokens 为 query 小写后按空白分词的结果；queryLower 为完整小写 query。
// stats 为当前 catalog 的 BM25 文档频率；nil 时退回纯加性打分（测试单条）。
func scoreEntryAgainstQuery(entry DeferredToolEntry, queryLower string, tokens []string, stats catalogStats) float64 {
	nameLower := strings.ToLower(entry.Name)
	baseLower := strings.ToLower(entry.BaseName)
	descLower := strings.ToLower(entry.Description)
	catLower := strings.ToLower(entry.Category)
	score := 0.0
	for _, token := range tokens {
		if nameLower == token || baseLower == token {
			score += exactNameBonus
		} else if matchableSubstring(token) && (strings.Contains(nameLower, token) || strings.Contains(baseLower, token)) {
			score += nameContainsBonus
		}
		if matchableSubstring(token) {
			if strings.Contains(catLower, token) {
				score += catContainsBonus
			}
			if strings.Contains(descLower, token) {
				score += descContainsBonus
			}
		}
	}
	for _, word := range nameSubwords(entry.Name) {
		if strings.Contains(queryLower, word) {
			score += subwordBonus
		}
	}
	if stats.n > 0 {
		score += bm25Field(tokenizeToolText(entry.Name), tokens, stats, bm25NameWeight)
		if entry.BaseName != "" && entry.BaseName != entry.Name {
			score += bm25Field(tokenizeToolText(entry.BaseName), tokens, stats, bm25BaseWeight)
		}
		score += bm25Field(tokenizeToolText(entry.Category), tokens, stats, bm25CatWeight)
		score += bm25Field(tokenizeToolText(entry.Description), tokens, stats, bm25DescWeight)
	}
	return score
}

func buildCatalogStats(catalog []DeferredToolEntry) catalogStats {
	st := catalogStats{n: len(catalog), df: map[string]int{}}
	if st.n == 0 {
		return st
	}
	totalLen := 0
	for _, entry := range catalog {
		tokens := fieldTokens(entry)
		totalLen += len(tokens)
		seenDoc := make(map[string]struct{}, len(tokens))
		for _, tok := range tokens {
			if _, ok := seenDoc[tok]; ok {
				continue
			}
			seenDoc[tok] = struct{}{}
			st.df[tok]++
		}
	}
	st.avgLen = float64(totalLen) / float64(st.n)
	if st.avgLen <= 0 {
		st.avgLen = 1
	}
	return st
}

func fieldTokens(entry DeferredToolEntry) []string {
	var out []string
	out = append(out, tokenizeToolText(entry.Name)...)
	if entry.BaseName != "" && entry.BaseName != entry.Name {
		out = append(out, tokenizeToolText(entry.BaseName)...)
	}
	out = append(out, tokenizeToolText(entry.Category)...)
	out = append(out, tokenizeToolText(entry.Description)...)
	return out
}

func tokenizeToolText(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func bm25Field(docTokens, queryTokens []string, stats catalogStats, weight float64) float64 {
	if len(docTokens) == 0 || weight == 0 {
		return 0
	}
	tf := map[string]int{}
	for _, t := range docTokens {
		tf[t]++
	}
	dl := float64(len(docTokens))
	var score float64
	seen := map[string]bool{}
	for _, q := range queryTokens {
		if !matchableSubstring(q) || seen[q] {
			continue
		}
		seen[q] = true
		freq := tf[q]
		if freq == 0 {
			continue
		}
		df := stats.df[q]
		idf := math.Log(1 + (float64(stats.n)-float64(df)+0.5)/(float64(df)+0.5))
		denom := float64(freq) + bm25K1*(1-bm25B+bm25B*dl/stats.avgLen)
		score += idf * (float64(freq) * (bm25K1 + 1) / denom)
	}
	return score * weight
}

// matchableSubstring 报告 token 是否适合子串匹配（≥3 runes）。
func matchableSubstring(token string) bool {
	return len([]rune(token)) >= 3
}

// nameSubwords 将工具名按非字母数字拆分为子词，过滤短词（防误命中）。
func nameSubwords(name string) []string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/'
	})
	out := parts[:0]
	seen := make(map[string]bool, len(parts))
	for _, p := range parts {
		if !matchableSubstring(p) || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// RankCatalogEntries 按 query 相关度对 catalog 排序，返回 Top-limit 条目。
// query 为空或无匹配时返回 nil；同分按工具名字典序（确定性，cue 字节稳定）。
func RankCatalogEntries(catalog []DeferredToolEntry, query string, limit int) []DeferredToolEntry {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" || limit <= 0 {
		return nil
	}
	tokens := strings.Fields(queryLower)
	stats := buildCatalogStats(catalog)
	type scored struct {
		entry DeferredToolEntry
		score float64
	}
	var matches []scored
	for _, entry := range catalog {
		if s := scoreEntryAgainstQuery(entry, queryLower, tokens, stats); s > 0 {
			matches = append(matches, scored{entry: entry, score: s})
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].entry.Name < matches[j].entry.Name
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	out := make([]DeferredToolEntry, len(matches))
	for i, m := range matches {
		out[i] = m.entry
	}
	return out
}

// ResolveToolHints maps LLM / intent slugs onto catalog names (runtime or
// base), then fills remaining slots from BM25 rank of the goal text.
func ResolveToolHints(catalog []DeferredToolEntry, goal string, llmHints []string, limit int) []string {
	if limit <= 0 {
		limit = 8
	}
	seen := map[string]bool{}
	var out []string
	index := map[string]string{}
	for _, e := range catalog {
		index[strings.ToLower(e.Name)] = e.Name
		if e.BaseName != "" {
			if _, ok := index[strings.ToLower(e.BaseName)]; !ok {
				index[strings.ToLower(e.BaseName)] = e.Name
			}
		}
	}
	for _, h := range llmHints {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		name, ok := index[h]
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
		if len(out) >= limit {
			return out
		}
	}
	for _, e := range RankCatalogEntries(catalog, goal, limit) {
		if seen[e.Name] {
			continue
		}
		seen[e.Name] = true
		out = append(out, e.Name)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// RenderCatalogCueWithRecommendations 渲染带推荐区的目录 cue。
//
// recommended 非空时在目录分组前插入「Recommended for your current request」
// 区（按相关度降序）；recommended 为空时输出与 RenderCatalogCue 字节一致
// （无 query 场景保持确定性）。推荐是高亮而非裁剪——目录区始终包含全量工具。
//
// 缓存说明：cue 注入在消息流尾部（最后一条用户消息之后），本就处于可缓存
// 前缀之外，每轮按 query 动态渲染对前缀缓存零影响。
func RenderCatalogCueWithRecommendations(catalog []DeferredToolEntry, recommended []DeferredToolEntry) string {
	if len(catalog) == 0 {
		return ""
	}
	static := RenderCatalogCue(catalog)
	if len(recommended) == 0 {
		return static
	}

	var b strings.Builder
	b.WriteString("### Recommended for your current request\n")
	for _, entry := range recommended {
		desc := strings.TrimSpace(entry.Description)
		if desc == "" {
			desc = "(no description)"
		}
		b.WriteString(fmt.Sprintf("- %s: %s\n", entry.Name, desc))
	}
	b.WriteString("\n")

	// 插入位置：第一个类别分组（### <category>）之前，保证推荐区紧随 intro。
	marker := "### "
	idx := strings.Index(static, marker)
	if idx < 0 {
		return static + "\n\n" + strings.TrimSpace(b.String())
	}
	return static[:idx] + b.String() + static[idx:]
}
