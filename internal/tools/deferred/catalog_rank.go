package deferred

import (
	"fmt"
	"sort"
	"strings"
)

// P1-4：deferred 工具语义预激活。
//
// 按当前用户 query 对 catalog 做轻量相关度排序，Top-N 以「推荐区」提升进
// catalog cue，提高模型发现率（命中率），减少「搜索→加载」的 round-trip。
// 排序器与 tool_search 共享同一打分逻辑（scoreEntryAgainstQuery），保证
// 「搜索看到的」与「推荐的」一致。
//
// 已知限制：纯关键词/子串匹配，纯中文 query 对英文工具名/描述无效；embedding
// 语义召回为后续演进方向（见 29-token.development.md §18）。

// CatalogRecommendLimit 是推荐区展示的条目数上限。
const CatalogRecommendLimit = 3

// scoreEntryAgainstQuery 计算单个目录条目与 query 的相关度得分。
// tokens 为 query 小写后按空白分词的结果；queryLower 为完整小写 query。
//
// 打分权重（与 tool_search 历史口径一致 + 子词扩展）：
//   - token == 工具名：+10
//   - 工具名包含 token：+5
//   - 工具名子词（按下划线/连字符拆分）作为子串出现在 query：+4
//   - 分类包含 token：+3
//   - 描述包含 token：+2
//
// 精度护栏：子串匹配要求 token/子词 ≥3 runes，防止短虚词噪声命中
// （如 query token "me" 子串命中 category "runtime"）。精确等值匹配不限长度。
func scoreEntryAgainstQuery(entry DeferredToolEntry, queryLower string, tokens []string) int {
	nameLower := strings.ToLower(entry.Name)
	descLower := strings.ToLower(entry.Description)
	catLower := strings.ToLower(entry.Category)
	score := 0
	for _, token := range tokens {
		if nameLower == token {
			score += 10
		} else if matchableSubstring(token) && strings.Contains(nameLower, token) {
			score += 5
		}
		if matchableSubstring(token) {
			if strings.Contains(catLower, token) {
				score += 3
			}
			if strings.Contains(descLower, token) {
				score += 2
			}
		}
	}
	// 子词匹配：CJK/自然语言 query 不会按工具名下划线分词，子词作为子串
	// 命中 query 也计分（如 "please save this" 命中 file_save_file 的 save）。
	for _, word := range nameSubwords(entry.Name) {
		if strings.Contains(queryLower, word) {
			score += 4
		}
	}
	return score
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
	type scored struct {
		entry DeferredToolEntry
		score int
	}
	var matches []scored
	for _, entry := range catalog {
		if s := scoreEntryAgainstQuery(entry, queryLower, tokens); s > 0 {
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
