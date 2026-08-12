// matcher.go a11y 元素模糊匹配器：实现 biz 层 ElementMatcher 端口。
// 算法（设计文档 §3.2）：归一化 → 精确 → 包含 → 编辑距离 ≤2；
// 候选打分，top1 与 top2 分差 ≥0.2 才判命中（防歧义误点）。纯函数无 I/O。
package computeruse

import (
	"strings"
	"unicode"
	"unicode/utf8"

	bizcomputeruse "aranea-agents/internal/biz/computeruse"
)

// 打分常量。
const (
	scoreExact    = 1.0 // 精确匹配
	scoreLevDist1 = 0.7 // 编辑距离 1
	scoreLevDist2 = 0.5 // 编辑距离 2
	// 包含匹配：基准 0.8，按目标/名称长度比在 [0.7, 0.9] 微调
	scoreContainsBase  = 0.7
	scoreContainsRange = 0.2
	// minScoreGap top1 与 top2 的最小分差，小于则视为歧义返回 nil
	minScoreGap = 0.2
	// maxLevDistance 容错的最大编辑距离
	maxLevDistance = 2
)

// Matcher a11y 模糊匹配器（实现 bizcomputeruse.ElementMatcher）。
type Matcher struct{}

// NewMatcher 构造匹配器。
func NewMatcher() *Matcher { return &Matcher{} }

var _ bizcomputeruse.ElementMatcher = (*Matcher)(nil)

// Match 在 elements 中为 target 选出最佳匹配；歧义或无候选返回 nil。
// 只匹配 Interactivity && Enabled 的元素。返回命中元素的副本。
func (m *Matcher) Match(elements []bizcomputeruse.UIElement, target string) *bizcomputeruse.UIElement {
	normTarget := normalize(target)
	if normTarget == "" {
		return nil
	}

	best := -1
	var bestScore, secondScore float64
	for i, el := range elements {
		if !el.Interactivity || !el.Enabled {
			continue
		}
		s := matchScore(normalize(el.Name), normTarget)
		if s <= 0 {
			continue
		}
		if s > bestScore {
			best, bestScore, secondScore = i, s, bestScore
		} else if s > secondScore {
			secondScore = s
		}
	}

	if best < 0 {
		return nil
	}
	// top1 与 top2 分差不足 → 歧义，拒绝误点
	if bestScore-secondScore < minScoreGap {
		return nil
	}
	hit := elements[best]
	return &hit
}

// matchScore 计算归一化后名称与目标的匹配得分（0 表示不匹配）。
func matchScore(name, target string) float64 {
	if name == "" {
		return 0
	}
	if name == target {
		return scoreExact
	}
	if strings.Contains(name, target) {
		ratio := float64(utf8.RuneCountInString(target)) / float64(utf8.RuneCountInString(name))
		return scoreContainsBase + scoreContainsRange*ratio
	}
	switch d := levenshtein(name, target); d {
	case 1:
		return scoreLevDist1
	case maxLevDistance:
		return scoreLevDist2
	}
	return 0
}

// normalize 归一化：小写、去空白、去标点、全角转半角。
func normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		r = fullwidthToHalfwidth(r)
		r = unicode.ToLower(r)
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// fullwidthToHalfwidth 全角字符转半角（ASCII 区间 + 全角空格）。
func fullwidthToHalfwidth(r rune) rune {
	if r == '　' { // U+3000 全角空格
		return ' '
	}
	if r >= 0xFF01 && r <= 0xFF5E {
		return r - 0xFEE0
	}
	return r
}

// levenshtein rune 级编辑距离（DP，滚动数组）。
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 0
			if ra[i-1] != rb[j-1] {
				cost = 1
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
