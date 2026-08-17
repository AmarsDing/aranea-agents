package knowledge

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxLexicalVariants = 8
	minCJKNeedleRunes  = 3
	maxCJKNeedleRunes  = 8
)

// lexicalFillers are interrogative / politeness wrappers stripped before BM25.
// Longest first so "是什么" wins over "什么".
var lexicalFillers = []string{
	"需要满足什么条件",
	"请引用原文回答",
	"需要记录并上报",
	"提前几个工作日",
	"根据现有文档",
	"关于这个问题",
	"周几几点开始",
	"知识库检索",
	"运维规范中",
	"帮我查一下",
	"几个工作日",
	"存放在哪里",
	"固定在每周",
	"每周几几点",
	"周几几点",
	"多长时间",
	"什么时段",
	"什么时候",
	"什么条件",
	"什么协议",
	"什么渠道",
	"什么业务",
	"哪四部分",
	"低于多少",
	"超过多少",
	"观察多久",
	"多久一次",
	"通过什么",
	"哪几天",
	"多久内",
	"多少钱",
	"多少分",
	"怎么打",
	"怎么算",
	"是什么",
	"是多少",
	"是多久",
	"分别是",
	"请确认",
	"判定为",
	"哪些",
	"哪个",
	"哪里",
	"如何",
	"怎样",
	"怎么",
	"多少",
	"多久",
	"分别",
	"请问",
	"检索",
	"必须",
	"需要",
}

var lexicalSplitters = map[rune]struct{}{
	'的': {}, '地': {}, '得': {}, '和': {}, '与': {}, '或': {}, '及': {},
	'并': {}, '后': {}, '前': {}, '中': {}, '为': {}, '以': {},
	'对': {}, '从': {}, '把': {}, '被': {}, '让': {}, '给': {}, '到': {},
	'在': {}, '再': {}, '是': {}, '吗': {}, '呢': {}, '吧': {}, '了': {}, '啊': {},
	'要': {},
}

var lexicalStopNeedles = map[string]struct{}{
	"为什么": {}, "怎么样": {}, "是不是": {}, "有没有": {}, "能不能": {},
	"这个": {}, "那个": {}, "进行": {}, "可以": {}, "以及": {}, "或者": {},
	"关于": {}, "帮我": {}, "一下": {}, "现有": {}, "文档": {},
}

// LexicalSearchQueries expands a user query into BM25-friendly variants.
// The original string is always first. Extra entries are distinctive content
// needles (CJK phrases and identifiers) extracted after dropping question fillers.
func LexicalSearchQueries(raw string) []string {
	q := strings.TrimSpace(raw)
	if q == "" {
		return nil
	}
	if !LooksLikeNaturalLanguageQuery(q) {
		return []string{q}
	}
	seen := map[string]struct{}{q: {}}
	out := []string{q}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		if _, stop := lexicalStopNeedles[s]; stop {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	for _, needle := range extractLexicalNeedles(CompactLexicalQuery(q)) {
		add(needle)
		if len(out) >= maxLexicalVariants {
			break
		}
	}
	return out
}

// LooksLikeNaturalLanguageQuery reports whether a query is a question or a
// long Chinese utterance that should run lexical needle expansion / hybrid RRF
// instead of exact-term or dense-only retrieval.
func LooksLikeNaturalLanguageQuery(q string) bool {
	if strings.ContainsAny(q, "？?") {
		return true
	}
	for _, filler := range lexicalFillers {
		if strings.Contains(q, filler) {
			return true
		}
	}
	han := 0
	for _, r := range q {
		if unicode.Is(unicode.Han, r) {
			han++
		}
	}
	return han >= 8
}

// CompactLexicalQuery drops question wrappers and punctuation, keeping content words.
func CompactLexicalQuery(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	for _, filler := range lexicalFillers {
		s = strings.ReplaceAll(s, filler, " ")
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if r == '+' || r == '-' || r == '_' || r == '.' || r == '/' || r == '%' {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func extractLexicalNeedles(compact string) []string {
	if compact == "" {
		return nil
	}
	var needles []string
	seen := map[string]struct{}{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		if _, stop := lexicalStopNeedles[s]; stop {
			return
		}
		seen[s] = struct{}{}
		needles = append(needles, s)
	}

	var cjk []rune
	var ascii []rune
	flushCJK := func() {
		if len(cjk) == 0 {
			return
		}
		addCJKNeedles(string(cjk), add)
		cjk = cjk[:0]
	}
	flushASCII := func() {
		if len(ascii) < 2 {
			ascii = ascii[:0]
			return
		}
		add(string(ascii))
		ascii = ascii[:0]
	}

	for _, r := range compact {
		switch {
		case unicode.Is(unicode.Han, r):
			flushASCII()
			if _, split := lexicalSplitters[r]; split {
				flushCJK()
				continue
			}
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+' || r == '-' || r == '_' || r == '.' || r == '/' || r == '%':
			flushCJK()
			ascii = append(ascii, r)
		default:
			flushCJK()
			flushASCII()
		}
	}
	flushCJK()
	flushASCII()

	sortNeedlesLongestFirst(needles)
	if len(needles) > maxLexicalVariants-1 {
		needles = needles[:maxLexicalVariants-1]
	}
	return needles
}

func addCJKNeedles(run string, add func(string)) {
	rs := []rune(run)
	n := len(rs)
	if n < minCJKNeedleRunes {
		return
	}
	if n <= maxCJKNeedleRunes {
		add(run)
		if n >= 5 {
			add(string(rs[:4]))
			add(string(rs[n-4:]))
			add(string(rs[:3]))
		}
		return
	}
	add(string(rs[:maxCJKNeedleRunes]))
	add(string(rs[n-maxCJKNeedleRunes:]))
	for i := 0; i+4 <= n && i <= 8; i += 2 {
		add(string(rs[i : i+4]))
	}
}

func sortNeedlesLongestFirst(needles []string) {
	sort.SliceStable(needles, func(i, j int) bool {
		li := utf8.RuneCountInString(needles[i])
		lj := utf8.RuneCountInString(needles[j])
		if li != lj {
			return li > lj
		}
		return needles[i] < needles[j]
	})
}
