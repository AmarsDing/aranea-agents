package biz

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EvalDateSpan represents calendar precision without inventing an exact day
// for a month or a week. Bounds are inclusive ISO calendar dates.
type EvalDateSpan struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type evalSpanHit struct {
	span       EvalDateSpan
	start, end int
}

var evalMonthNames = map[string]time.Month{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
var evalEnglishDate = regexp.MustCompile(`(?i)\b(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\s+(\d{1,2})(?:st|nd|rd|th)?[,]?\s+(\d{4})\b`)
var evalEnglishDateDMY = regexp.MustCompile(`(?i)\b(\d{1,2})(?:st|nd|rd|th)?\s+(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)[,]?\s+(\d{4})\b`)
var evalCalendarPeriod = regexp.MustCompile(`(?i)上(?:个)?周|本周|这周|下(?:个)?周|本月|这个月|这月|去年|今年|明年|\bthis\s+(?:week|month)\b|\b(?:last|this|next)\s+year\b`)
var evalPastDays = regexp.MustCompile(`(?i)\b(?:past|last)\s+(\d{1,4})\s+days?\b|过去([0-9一二两三四五六七八九十]+)天`)
var evalCNMonth = regexp.MustCompile(`(\d{4})年(\d{1,2})月`)
var evalISOMonth = regexp.MustCompile(`\b(\d{4})-(\d{1,2})\b`)
var evalENMonth = regexp.MustCompile(`(?i)\b(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\s+(\d{4})\b`)

func evalCheckedDay(y, m, d int, loc *time.Location) (time.Time, bool) {
	if y < 1 || y > 9999 || m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, loc)
	return t, t.Year() == y && int(t.Month()) == m && t.Day() == d
}

// ResolveEvalDateSpans resolves only source text, never a query answer. Absolute
// and relative mentions share one ordered scan; larger overlapping phrases win.
func ResolveEvalDateSpans(text string, anchor time.Time) []EvalDateSpan {
	hits := evalAdditionalCalendarHits(text, anchor)
	add := func(start, end int, a, b time.Time) {
		hits = append(hits, evalSpanHit{EvalDateSpan{a.Format("2006-01-02"), b.Format("2006-01-02")}, start, end})
	}
	abs := func(re *regexp.Regexp, us bool) {
		for _, idx := range re.FindAllStringSubmatchIndex(text, -1) {
			var nums [3]int
			for j := range nums {
				nums[j], _ = strconv.Atoi(text[idx[2+j*2]:idx[3+j*2]])
			}
			y, m, d := nums[0], nums[1], nums[2]
			if us {
				y, m, d = nums[2], nums[0], nums[1]
			}
			if day, ok := evalCheckedDay(y, m, d, anchor.Location()); ok {
				add(idx[0], idx[1], day, day)
			}
		}
	}
	abs(evalAbsDateCNRe, false)
	abs(evalAbsDateISORe, false)
	abs(evalAbsDateUSRe, true)
	for _, re := range []*regexp.Regexp{evalCNMonth, evalISOMonth, evalENMonth} {
		for _, idx := range re.FindAllStringSubmatchIndex(text, -1) {
			// A malformed full date must not silently become a valid month.
			if idx[1] < len(text) && (text[idx[1]] == '-' || text[idx[1]] >= '0' && text[idx[1]] <= '9') {
				continue
			}
			y, _ := strconv.Atoi(text[idx[2]:idx[3]])
			m, _ := strconv.Atoi(text[idx[4]:idx[5]])
			if re == evalENMonth {
				y, _ = strconv.Atoi(text[idx[4]:idx[5]])
				m = int(evalMonthNames[strings.ToLower(text[idx[2]:idx[2]+3])])
			}
			if day, ok := evalCheckedDay(y, m, 1, anchor.Location()); ok {
				add(idx[0], idx[1], day, day.AddDate(0, 1, -1))
			}
		}
	}
	for _, re := range []*regexp.Regexp{evalEnglishDate, evalEnglishDateDMY} {
		for _, idx := range re.FindAllStringSubmatchIndex(text, -1) {
			a, b := text[idx[2]:idx[3]], text[idx[4]:idx[5]]
			if re == evalEnglishDateDMY {
				a, b = b, a
			}
			y, _ := strconv.Atoi(text[idx[6]:idx[7]])
			d, _ := strconv.Atoi(b)
			if day, ok := evalCheckedDay(y, int(evalMonthNames[strings.ToLower(a[:3])]), d, anchor.Location()); ok {
				add(idx[0], idx[1], day, day)
			}
		}
	}
	for _, h := range resolveEvalDateExprs(text, anchor) {
		a, b := h.day, h.day
		phrase := strings.ToLower(text[h.start:h.end])
		if phrase == "last month" || phrase == "next month" || phrase == "上个月" || phrase == "下个月" {
			b = a.AddDate(0, 1, -1)
		}
		if phrase == "last week" || phrase == "next week" {
			b = a.AddDate(0, 0, 6)
		}
		if phrase == "去年" || phrase == "明年" {
			a = time.Date(a.Year(), 1, 1, 0, 0, 0, 0, a.Location())
			b = a.AddDate(1, 0, -1)
		}
		add(h.start, h.end, a, b)
	}
	for _, idx := range evalCalendarPeriod.FindAllStringIndex(text, -1) {
		p := strings.ToLower(text[idx[0]:idx[1]])
		if idx[1] < len(text) && strings.ContainsRune("一二三四五六日天", []rune(text[idx[1]:])[0]) {
			continue
		}
		a := evalWeekMonday(anchor)
		b := a.AddDate(0, 0, 6)
		if strings.Contains(p, "月") || strings.Contains(p, "month") {
			a = time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, anchor.Location())
			b = a.AddDate(0, 1, -1)
		} else if strings.Contains(p, "年") || strings.Contains(p, "year") {
			y := anchor.Year()
			if p == "去年" || p == "last year" {
				y--
			}
			if p == "明年" || p == "next year" {
				y++
			}
			a = time.Date(y, 1, 1, 0, 0, 0, 0, anchor.Location())
			b = a.AddDate(1, 0, -1)
		} else {
			if strings.HasPrefix(p, "上") {
				a = a.AddDate(0, 0, -7)
			}
			if strings.HasPrefix(p, "下") {
				a = a.AddDate(0, 0, 7)
			}
			b = a.AddDate(0, 0, 6)
		}
		add(idx[0], idx[1], a, b)
	}
	for _, idx := range evalPastDays.FindAllStringSubmatchIndex(text, -1) {
		n := 0
		for j := 2; j+1 < len(idx); j += 2 {
			if idx[j] >= 0 {
				n, _ = parseEvalN(text[idx[j]:idx[j+1]])
			}
		}
		if n > 0 && n <= 3660 {
			b := evalDayFloor(anchor)
			add(idx[0], idx[1], b.AddDate(0, 0, 1-n), b)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].start != hits[j].start {
			return hits[i].start < hits[j].start
		}
		return hits[i].end > hits[j].end
	})
	var spans []EvalDateSpan
	lastEnd := -1
	for _, h := range hits {
		if h.start >= lastEnd {
			spans = append(spans, h.span)
			lastEnd = h.end
		}
	}
	return spans
}
