package biz

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// memory_eval_time.go — AMC C3-A Q5 deterministic relative-date resolution.
//
// ResolveEvalRelativeTimes scans a statement for Chinese/English relative
// day expressions and returns the absolute dates they point at (RFC3339
// date part, YYYY-MM-DD) relative to the anchor. Everything is table-driven
// and deterministic — no LLM is involved on the Add/Search contract path.

// evalDateHit is one resolved date mention inside a text span.
type evalDateHit struct {
	day   time.Time
	start int
	end   int
}

type evalDatePattern struct {
	re      *regexp.Regexp
	resolve func(sub []string, anchor time.Time) (time.Time, bool)
}

var evalWeekdayCN = map[string]int{
	"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "日": 7, "天": 7,
}

var evalWeekdayEN = map[string]time.Weekday{
	"monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
	"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday,
	"sunday": time.Sunday,
}

// evalDayFloor normalizes t to midnight in its own location.
func evalDayFloor(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// evalWeekMonday returns Monday 00:00 of the natural week containing day.
func evalWeekMonday(day time.Time) time.Time {
	day = evalDayFloor(day)
	return day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
}

// parseEvalCNNum parses a small Chinese numeral (1..99): 一二两三四五六七八九,
// 十, 十X, X十, X十Y.
func parseEvalCNNum(s string) (int, bool) {
	digits := map[rune]int{'一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	rs := []rune(s)
	if len(rs) == 0 || len(rs) > 3 {
		return 0, false
	}
	if len(rs) == 1 {
		if rs[0] == '十' {
			return 10, true
		}
		if v, ok := digits[rs[0]]; ok {
			return v, true
		}
		return 0, false
	}
	if len(rs) == 2 {
		if rs[0] == '十' {
			if v, ok := digits[rs[1]]; ok {
				return 10 + v, true
			}
			return 0, false
		}
		if rs[1] == '十' {
			if v, ok := digits[rs[0]]; ok {
				return v * 10, true
			}
			return 0, false
		}
		return 0, false
	}
	if rs[1] == '十' {
		a, ok1 := digits[rs[0]]
		b, ok2 := digits[rs[2]]
		if ok1 && ok2 {
			return a*10 + b, true
		}
	}
	return 0, false
}

// parseEvalN parses an Arabic or small Chinese numeral.
func parseEvalN(s string) (int, bool) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	return parseEvalCNNum(s)
}

var evalDatePatterns = []evalDatePattern{
	// --- Chinese relative days (longest/most specific first) ---
	{regexp.MustCompile(`大前天`), func(_ []string, a time.Time) (time.Time, bool) {
		return evalDayFloor(a).AddDate(0, 0, -3), true
	}},
	{regexp.MustCompile(`前天|昨天|今天|明天|后天`), func(sub []string, a time.Time) (time.Time, bool) {
		d := evalDayFloor(a)
		switch sub[0] {
		case "前天":
			return d.AddDate(0, 0, -2), true
		case "昨天":
			return d.AddDate(0, 0, -1), true
		case "今天":
			return d, true
		case "明天":
			return d.AddDate(0, 0, 1), true
		case "后天":
			return d.AddDate(0, 0, 2), true
		}
		return time.Time{}, false
	}},
	// 上周X / 本周X / 下周X (natural week, Monday start): 上周X=上一自然周之周X.
	{regexp.MustCompile(`(上|本|下)(?:个)?(?:周|星期)([一二三四五六日天])`), func(sub []string, a time.Time) (time.Time, bool) {
		wd, ok := evalWeekdayCN[sub[2]]
		if !ok {
			return time.Time{}, false
		}
		monday := evalWeekMonday(a)
		switch sub[1] {
		case "上":
			monday = monday.AddDate(0, 0, -7)
		case "下":
			monday = monday.AddDate(0, 0, 7)
		}
		return monday.AddDate(0, 0, wd-1), true
	}},
	// 上个月 / 下个月 → first day of the previous / next month.
	{regexp.MustCompile(`(上|下)个月`), func(sub []string, a time.Time) (time.Time, bool) {
		y, m, _ := evalDayFloor(a).Date()
		if sub[1] == "上" {
			return time.Date(y, m-1, 1, 0, 0, 0, 0, a.Location()), true
		}
		return time.Date(y, m+1, 1, 0, 0, 0, 0, a.Location()), true
	}},
	// 去年 / 明年 → same month-day one year before / after.
	{regexp.MustCompile(`去年|明年`), func(sub []string, a time.Time) (time.Time, bool) {
		d := evalDayFloor(a)
		if sub[0] == "去年" {
			return d.AddDate(-1, 0, 0), true
		}
		return d.AddDate(1, 0, 0), true
	}},
	// N 天前 / N 天后 (日 accepted as a 天 alias).
	{regexp.MustCompile(`([0-9]+|[一二两三四五六七八九十]{1,3})(?:天|日)(前|后)`), func(sub []string, a time.Time) (time.Time, bool) {
		n, ok := parseEvalN(sub[1])
		if !ok || n > 36600 {
			return time.Time{}, false
		}
		if sub[2] == "前" {
			n = -n
		}
		return evalDayFloor(a).AddDate(0, 0, n), true
	}},
	// N 周前 / N 周后.
	{regexp.MustCompile(`([0-9]+|[一二两三四五六七八九十]{1,3})(?:周|星期)(前|后)`), func(sub []string, a time.Time) (time.Time, bool) {
		n, ok := parseEvalN(sub[1])
		if !ok || n > 5200 {
			return time.Time{}, false
		}
		if sub[2] == "前" {
			n = -n
		}
		return evalDayFloor(a).AddDate(0, 0, 7*n), true
	}},
	// N 个月前 / N 个月后.
	{regexp.MustCompile(`([0-9]+|[一二两三四五六七八九十]{1,3})个月(前|后)`), func(sub []string, a time.Time) (time.Time, bool) {
		n, ok := parseEvalN(sub[1])
		if !ok || n > 1200 {
			return time.Time{}, false
		}
		if sub[2] == "前" {
			n = -n
		}
		return evalShiftMonths(a, n), true
	}},
	{regexp.MustCompile(`(?i)\b(in\s+)?([0-9]+)\s+(weeks?|months?|years?)(\s+ago)?\b`), func(sub []string, a time.Time) (time.Time, bool) {
		if sub[1] == "" && sub[4] == "" {
			return time.Time{}, false
		}
		n, err := strconv.Atoi(sub[2])
		if err != nil || n > 1200 {
			return time.Time{}, false
		}
		if sub[4] != "" {
			n = -n
		}
		switch strings.ToLower(sub[3])[:4] {
		case "week":
			return evalDayFloor(a).AddDate(0, 0, 7*n), true
		case "mont":
			return evalShiftMonths(a, n), true
		default:
			return evalShiftMonths(a, 12*n), true
		}
	}},
	{regexp.MustCompile(`(?i)\bnext\s+(monday|tuesday|wednesday|thursday|friday|saturday|sunday)\b`), func(sub []string, a time.Time) (time.Time, bool) {
		target, ok := evalWeekdayEN[strings.ToLower(sub[1])]
		if !ok {
			return time.Time{}, false
		}
		delta := (int(target) - int(a.Weekday()) + 7) % 7
		if delta == 0 {
			delta = 7
		}
		return evalDayFloor(a).AddDate(0, 0, delta), true
	}},
	// --- English relative days ---
	{regexp.MustCompile(`(?i)\b(today|yesterday|tomorrow)\b`), func(sub []string, a time.Time) (time.Time, bool) {
		d := evalDayFloor(a)
		switch strings.ToLower(sub[1]) {
		case "yesterday":
			return d.AddDate(0, 0, -1), true
		case "today":
			return d, true
		case "tomorrow":
			return d.AddDate(0, 0, 1), true
		}
		return time.Time{}, false
	}},
	// last Monday..Sunday → most recent such weekday strictly before today.
	{regexp.MustCompile(`(?i)\blast\s+(monday|tuesday|wednesday|thursday|friday|saturday|sunday)\b`), func(sub []string, a time.Time) (time.Time, bool) {
		target, ok := evalWeekdayEN[strings.ToLower(sub[1])]
		if !ok {
			return time.Time{}, false
		}
		d := evalDayFloor(a)
		delta := (int(d.Weekday()) - int(target) + 7) % 7
		if delta == 0 {
			delta = 7
		}
		return d.AddDate(0, 0, -delta), true
	}},
	// last week / next week → Monday of the previous / next natural week;
	// last month / next month → first day of the previous / next month.
	{regexp.MustCompile(`(?i)\b(last|next)\s+(week|month)\b`), func(sub []string, a time.Time) (time.Time, bool) {
		sign := 1
		if strings.ToLower(sub[1]) == "last" {
			sign = -1
		}
		if strings.ToLower(sub[2]) == "week" {
			return evalWeekMonday(a).AddDate(0, 0, 7*sign), true
		}
		y, m, _ := evalDayFloor(a).Date()
		return time.Date(y, m+time.Month(sign), 1, 0, 0, 0, 0, a.Location()), true
	}},
	// in N days / N days ago.
	{regexp.MustCompile(`(?i)\bin\s+([0-9]+)\s+days?\b`), func(sub []string, a time.Time) (time.Time, bool) {
		n, err := strconv.Atoi(sub[1])
		if err != nil {
			return time.Time{}, false
		}
		return evalDayFloor(a).AddDate(0, 0, n), true
	}},
	{regexp.MustCompile(`(?i)\b([0-9]+)\s+days?\s+ago\b`), func(sub []string, a time.Time) (time.Time, bool) {
		n, err := strconv.Atoi(sub[1])
		if err != nil {
			return time.Time{}, false
		}
		return evalDayFloor(a).AddDate(0, 0, -n), true
	}},
}

func evalShiftMonths(anchor time.Time, months int) time.Time {
	y, m, d := anchor.Date()
	first := time.Date(y, m+time.Month(months), 1, 0, 0, 0, 0, anchor.Location())
	last := first.AddDate(0, 1, -1).Day()
	if d > last {
		d = last
	}
	return first.AddDate(0, 0, d-1)
}

// resolveEvalDateExprs finds every date expression in text and resolves it
// against anchor. Matches are emitted in text order; overlapping spans keep
// the earliest-then-longest match.
func resolveEvalDateExprs(text string, anchor time.Time) []evalDateHit {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var hits []evalDateHit
	for _, p := range evalDatePatterns {
		for _, idx := range p.re.FindAllStringSubmatchIndex(text, -1) {
			sub := make([]string, len(idx)/2)
			for i := range sub {
				if idx[2*i] >= 0 {
					sub[i] = text[idx[2*i]:idx[2*i+1]]
				}
			}
			day, ok := p.resolve(sub, anchor)
			if !ok {
				continue
			}
			hits = append(hits, evalDateHit{day: day, start: idx[0], end: idx[1]})
		}
	}
	// Sort by start asc, then longer span first so "上周一" beats its "周" parts.
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0; j-- {
			a, b := hits[j-1], hits[j]
			if a.start < b.start || (a.start == b.start && a.end-a.start >= b.end-b.start) {
				break
			}
			hits[j-1], hits[j] = hits[j], hits[j-1]
		}
	}
	out := hits[:0]
	lastEnd := -1
	for _, h := range hits {
		if h.start < lastEnd {
			continue
		}
		lastEnd = h.end
		out = append(out, h)
	}
	return out
}

// ResolveEvalRelativeTimes extracts deterministic absolute dates (RFC3339
// date part, YYYY-MM-DD) for the relative day expressions in stmt, relative
// to anchor. Covers 今天/昨天/前天/明天/后天, 上周X/本周X/下周X (natural week,
// Monday start), 上个月/下个月, N 天前/后, N 周前/后, N 个月前/后, 去年/明年,
// and today/yesterday/tomorrow/last week/next week/last month/next month/
// in N days/N days ago/last <weekday>. Returns dates deduplicated in order
// of appearance; empty when nothing resolves.
func ResolveEvalRelativeTimes(stmt string, anchor time.Time) []string {
	hits := resolveEvalDateExprs(stmt, anchor)
	if len(hits) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		day := h.day.Format("2006-01-02")
		if _, dup := seen[day]; dup {
			continue
		}
		seen[day] = struct{}{}
		out = append(out, day)
	}
	return out
}
