package biz

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	evalYearCN        = regexp.MustCompile(`([12]\d{3})年`)
	evalYearEN        = regexp.MustCompile(`(?i)\b(?:in|during|year)\s+([12]\d{3})\b`)
	evalBareYear      = regexp.MustCompile(`^\s*([12]\d{3})\s*$`)
	evalYearlessCN    = regexp.MustCompile(`(\d{1,2})月(\d{1,2})日`)
	evalYearlessEN    = regexp.MustCompile(`(?i)\b(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\s+(\d{1,2})(?:st|nd|rd|th)?\b`)
	evalFollowingYear = regexp.MustCompile(`^\s*,?\s*\d{4}\b`)
	evalDateHeading   = regexp.MustCompile(`(?i)^(?:\d{4}-\d{1,2}-\d{1,2}|\d{4}年\d{1,2}月\d{1,2}日?|\d{1,2}月\d{1,2}日|today|yesterday|tomorrow|今天|昨天|明天)$`)
)

func evalAdditionalCalendarHits(text string, anchor time.Time) []evalSpanHit {
	var out []evalSpanHit
	appendDay := func(start, end, month, day int) {
		if d, ok := evalCheckedDay(anchor.Year(), month, day, anchor.Location()); ok {
			out = append(out, evalSpanHit{EvalDateSpan{d.Format("2006-01-02"), d.Format("2006-01-02")}, start, end})
		}
	}
	for _, re := range []*regexp.Regexp{evalYearCN, evalYearEN, evalBareYear} {
		for _, idx := range re.FindAllStringSubmatchIndex(text, -1) {
			if re == evalYearEN && idx[1] < len(text) && (text[idx[1]] == '-' || text[idx[1]] == '/') {
				continue
			}
			if re == evalYearCN && idx[1] < len(text) && text[idx[1]] >= '0' && text[idx[1]] <= '9' {
				continue
			}
			year, _ := strconv.Atoi(text[idx[2]:idx[3]])
			a := time.Date(year, 1, 1, 0, 0, 0, 0, anchor.Location())
			out = append(out, evalSpanHit{EvalDateSpan{a.Format("2006-01-02"), a.AddDate(1, 0, -1).Format("2006-01-02")}, idx[0], idx[1]})
		}
	}
	for _, idx := range evalYearlessCN.FindAllStringSubmatchIndex(text, -1) {
		if idx[0] > 0 {
			prev, _ := utf8.DecodeLastRuneInString(text[:idx[0]])
			if prev == '年' || prev >= '0' && prev <= '9' {
				continue
			}
		}
		month, _ := strconv.Atoi(text[idx[2]:idx[3]])
		day, _ := strconv.Atoi(text[idx[4]:idx[5]])
		appendDay(idx[0], idx[1], month, day)
	}
	for _, idx := range evalYearlessEN.FindAllStringSubmatchIndex(text, -1) {
		if evalFollowingYear.MatchString(text[idx[1]:]) {
			continue
		}
		month := int(evalMonthNames[strings.ToLower(text[idx[2]:idx[2]+3])])
		day, _ := strconv.Atoi(text[idx[4]:idx[5]])
		appendDay(idx[0], idx[1], month, day)
	}
	return out
}

// EvalDateHeadingSpans accepts an isolated date heading, not an arbitrary
// sentence mentioning a date. Only headings can date following undated atoms.
func EvalDateHeadingSpans(text string, anchor time.Time) []EvalDateSpan {
	body := strings.TrimSpace(text)
	for _, prefix := range []string{"Date:", "date:", "日期：", "日志：", "日记："} {
		body = strings.TrimSpace(strings.TrimPrefix(body, prefix))
	}
	body = strings.Trim(body, "[]【】# :：\t\r\n")
	if !evalDateHeading.MatchString(body) {
		return nil
	}
	return ResolveEvalDateSpans(body, anchor)
}
