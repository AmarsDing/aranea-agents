package agent

import (
	"strings"

	"aranea-agents/internal/biz"
)

// ---------------------------------------------------------------------------
// 领域词表（Domain Lexicon，B.10.21.3）
//
// 内置两级词表，planner / allocator / factory 三方共用。词表是**约束**而非
// 封闭集：LLM 可输出词表外路径，NormalizeDomainPath 将其归并到最近的已知
// 域（词表外二级域归并一级域），完全无法归类时归入 "其他"，防止路径漂移
// 导致匹配域碎片化。
// ---------------------------------------------------------------------------

// domainLexiconOther is the fallback bucket for unclassifiable paths.
const domainLexiconOther = "其他"

// domainLexicon lists the canonical domain paths (top-level/second-level).
var domainLexicon = []string{
	"软件/后端",
	"软件/前端",
	"软件/测试",
	"软件/运维",
	"软件/产品",
	"软件/项目",
	"软件/架构",
	"软件/安全",
	"软件/移动",
	"软件/游戏",
	"软件/空间",
	"软件/合规",
	"数据/分析",
	"数据/空间",
	"创作/文学",
	"创作/文案",
	"设计/视觉",
	"研究/调研",
	"办公/文档",
	"办公/专项",
	"运维/告警",
	"运维/诊断",
	"运维/变更",
	"运维/巡检",
	"运维/复盘",
	"商务/销售",
	"商务/财务",
	"商务/客服",
	"商务/电商",
	"商务/推广",
	"医疗/临床",
	"医疗/创新",
	"医疗/公卫",
	domainLexiconOther,
}

// domainLexiconSet holds every canonical entry for O(1) lookup.
var domainLexiconSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(domainLexicon))
	for _, p := range domainLexicon {
		m[p] = struct{}{}
	}
	return m
}()

// domainTopLevelSet holds the top-level domains implied by the lexicon
// ("创作/文学" → "创作"); they are valid normalization targets themselves.
var domainTopLevelSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(domainLexicon))
	for _, p := range domainLexicon {
		if i := strings.Index(p, "/"); i > 0 {
			m[p[:i]] = struct{}{}
		}
	}
	return m
}()

// DomainLexiconPromptList renders the lexicon for LLM prompts (planner
// decomposition + factory generation), constraining domain_path output.
func DomainLexiconPromptList() string {
	return strings.Join(domainLexicon, "、")
}

// NormalizeDomainPath normalizes an LLM-produced domain path against the
// lexicon: collapses separators/whitespace, returns the canonical entry on
// hit, merges unknown deeper paths up to the longest known prefix
// ("创作/诗歌" → "创作"), and falls back to "其他" when nothing matches.
// Empty input stays "".
func NormalizeDomainPath(raw string) string {
	segs := strings.FieldsFunc(raw, func(r rune) bool { return r == '/' || r == '\\' })
	clean := make([]string, 0, len(segs))
	for _, s := range segs {
		if t := strings.TrimSpace(s); t != "" {
			clean = append(clean, t)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	for n := len(clean); n >= 1; n-- {
		cand := strings.Join(clean[:n], "/")
		if _, ok := domainLexiconSet[cand]; ok {
			return cand
		}
		if _, ok := domainTopLevelSet[cand]; ok {
			return cand
		}
	}
	return domainLexiconOther
}

// TopLevelDomain returns the first segment of the normalized path
// ("创作/文学" → "创作"). "" stays "".
func TopLevelDomain(path string) string {
	norm := NormalizeDomainPath(path)
	if i := strings.Index(norm, "/"); i > 0 {
		return norm[:i]
	}
	return norm
}

// DomainPathRelated reports whether two domain paths belong to the same
// matching domain: equal, path-boundary prefix of each other (either
// direction), or sharing the same top-level domain ("创作/文学" ↔ "创作/文案").
// Empty paths are never related.
func DomainPathRelated(a, b string) bool {
	a, b = NormalizeDomainPath(a), NormalizeDomainPath(b)
	if a == "" || b == "" {
		return false
	}
	if a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
		return true
	}
	return TopLevelDomain(a) == TopLevelDomain(b)
}

// PrimaryDomainPath returns the plan-level dominant domain: the first
// non-empty normalized subtask DomainPath (B.10.21.7). "" when none.
func PrimaryDomainPath(subTasks []biz.SubTask) string {
	for _, st := range subTasks {
		if dp := NormalizeDomainPath(st.DomainPath); dp != "" {
			return dp
		}
	}
	return ""
}
