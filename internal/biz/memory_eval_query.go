package biz

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// memory_eval_query.go — AMC C3-A Q6 deterministic query-mode analysis.
//
// AnalyzeEvalQuery classifies a search query (chronological / aggregate /
// date-range) without any LLM involvement so Search can switch ranking and
// evidence filtering deterministically.

// EvalQueryAnalysis is the deterministic parse of one eval search query.
// RangeStart/RangeEnd are inclusive day bounds (nil = open side).
type EvalQueryAnalysis struct {
	Chronological bool
	Aggregate     bool
	RangeStart    *time.Time
	RangeEnd      *time.Time
}

var (
	evalChronoCues = []string{
		"顺序", "先后", "第几", "之前", "之后", "最早", "最近第一次",
		"order", "before", "after", "first time", "sequence",
	}
	evalAggregateCues = []string{
		"总结", "总体", "一共", "总共", "概括",
		"overall", "summary", "total", "how many times",
	}

	evalAbsDateCNRe  = regexp.MustCompile(`(\d{4})年(\d{1,2})月(\d{1,2})日?`)
	evalAbsDateISORe = regexp.MustCompile(`\b(\d{4})-(\d{1,2})-(\d{1,2})\b`)
	evalAbsDateUSRe  = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})/(\d{4})\b`)

	evalRangeSinceCues = []string{"之后", "以后", "自从", "since", "after"}
	evalRangeUntilCues = []string{"以前", "之前", "before", "until"}
)

// evalQueryContainsAny reports case-insensitive substring presence (the cue
// vocabulary is semantics-insensitive to casing, e.g. "Order" / "BEFORE").
func evalQueryContainsAny(low string, cues []string) bool {
	for _, c := range cues {
		if strings.Contains(low, c) {
			return true
		}
	}
	return false
}

// AnalyzeEvalQuery parses query into chronological / aggregate / range
// modes. now anchors relative-date resolution for range bounds.
func AnalyzeEvalQuery(query string, now time.Time) EvalQueryAnalysis {
	low := strings.ToLower(query)
	var qa EvalQueryAnalysis
	qa.Chronological = evalQueryContainsAny(low, evalChronoCues)
	qa.Aggregate = evalQueryContainsAny(low, evalAggregateCues)
	qa.RangeStart, qa.RangeEnd = evalQueryDateRange(query, low, now)
	return qa
}

// evalQueryDateRange resolves absolute (2025年5月7日 / 2025-05-07 /
// MM/DD/YYYY) and relative date mentions combined with range connectives
// (之后/以前/从…到…/between X and Y/since X) into inclusive day bounds.
func evalQueryDateRange(query, low string, now time.Time) (*time.Time, *time.Time) {
	type absHit struct {
		day        time.Time
		start, end int
	}
	var hits []absHit
	addAbs := func(re *regexp.Regexp, mk func(y, m, d int) time.Time) {
		for _, m := range re.FindAllStringSubmatchIndex(query, -1) {
			y, _ := strconv.Atoi(query[m[2]:m[3]])
			mo, _ := strconv.Atoi(query[m[4]:m[5]])
			d, _ := strconv.Atoi(query[m[6]:m[7]])
			hits = append(hits, absHit{day: mk(y, mo, d), start: m[0], end: m[1]})
		}
	}
	addAbs(evalAbsDateCNRe, func(y, m, d int) time.Time {
		return time.Date(y, time.Month(m), d, 0, 0, 0, 0, now.Location())
	})
	addAbs(evalAbsDateISORe, func(y, m, d int) time.Time {
		return time.Date(y, time.Month(m), d, 0, 0, 0, 0, now.Location())
	})
	// MM/DD/YYYY keeps the US month-first order.
	for _, m := range evalAbsDateUSRe.FindAllStringSubmatchIndex(query, -1) {
		mo, _ := strconv.Atoi(query[m[2]:m[3]])
		d, _ := strconv.Atoi(query[m[4]:m[5]])
		y, _ := strconv.Atoi(query[m[6]:m[7]])
		hits = append(hits, absHit{day: time.Date(y, time.Month(mo), d, 0, 0, 0, 0, now.Location()), start: m[0], end: m[1]})
	}
	for _, h := range resolveEvalDateExprs(query, now) {
		hits = append(hits, absHit{day: h.day, start: h.start, end: h.end})
	}
	if len(hits) == 0 {
		return nil, nil
	}
	// Sort by start, longest first on ties; drop overlapping spans.
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0; j-- {
			a, b := hits[j-1], hits[j]
			if a.start < b.start || (a.start == b.start && a.end-a.start >= b.end-b.start) {
				break
			}
			hits[j-1], hits[j] = hits[j], hits[j-1]
		}
	}
	uniq := hits[:0]
	lastEnd := -1
	for _, h := range hits {
		if h.start < lastEnd {
			continue
		}
		lastEnd = h.end
		uniq = append(uniq, h)
	}
	hits = uniq

	dayPtr := func(t time.Time) *time.Time {
		d := evalDayFloor(t)
		return &d
	}
	minDay := func(a, b time.Time) time.Time {
		if a.Before(b) {
			return a
		}
		return b
	}
	maxDay := func(a, b time.Time) time.Time {
		if a.After(b) {
			return a
		}
		return b
	}
	hasSince := evalQueryContainsAny(low, evalRangeSinceCues)
	hasUntil := evalQueryContainsAny(low, evalRangeUntilCues)
	hasBetween := strings.Contains(low, "between") || (strings.Contains(query, "从") && (strings.Contains(query, "到") || strings.Contains(query, "至")))

	switch {
	case len(hits) >= 2 && hasBetween:
		return dayPtr(minDay(hits[0].day, hits[1].day)), dayPtr(maxDay(hits[0].day, hits[1].day))
	case len(hits) >= 2 && hasSince && hasUntil:
		return dayPtr(minDay(hits[0].day, hits[1].day)), dayPtr(maxDay(hits[0].day, hits[1].day))
	case hasSince:
		return dayPtr(hits[0].day), nil
	case hasUntil:
		return nil, dayPtr(hits[len(hits)-1].day)
	case len(hits) == 1:
		// A bare absolute/relative date pins the whole day.
		return dayPtr(hits[0].day), dayPtr(hits[0].day)
	default:
		start := hits[0].day
		end := hits[0].day
		for _, h := range hits[1:] {
			start = minDay(start, h.day)
			end = maxDay(end, h.day)
		}
		return dayPtr(start), dayPtr(end)
	}
}
