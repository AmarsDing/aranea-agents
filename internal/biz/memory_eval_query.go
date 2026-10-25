package biz

import (
	"regexp"
	"strings"
	"time"
)

// memory_eval_query.go — AMC C3-A Q6 deterministic query analysis.
//
// AnalyzeEvalQuery preserves deterministic date parsing and offline query
// modes. The data layer enriches these modes with the optional vector router;
// embedding outages must not disable the existing Add/Search contract.

// EvalQueryAnalysis is the deterministic parse of one eval search query.
// RangeStart/RangeEnd are inclusive day bounds (nil = open side).
// Deterministic modes remain available without an embedding service. Vector
// routing may add modes without clearing explicit calendar or workflow intent.
type EvalQueryAnalysis struct {
	Chronological bool
	Aggregate     bool
	Historical    bool
	Procedural    bool
	Enumerative   bool
	Relational    bool
	Stepwise      bool
	Trend         bool
	RangeStart    *time.Time
	RangeEnd      *time.Time
}

var (
	evalChronoCues = []string{
		"顺序", "先后", "第几", "之前", "之后", "最早", "最近第一次",
		"order", "before", "after", "first time", "sequence", "timeline", "chronological",
	}
	evalAggregateCues = []string{
		"总结", "总体", "一共", "总共", "概括", "介绍一下",
		"overall", "summary", "summarize", "summarise", "total", "how many", "over time", "throughout",
		"profile", "what do you know about", "what do you remember about",
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
		for offset := 0; offset < len(low); {
			i := strings.Index(low[offset:], c)
			if i < 0 {
				break
			}
			i += offset
			end := i + len(c)
			if c[0] >= 128 || ((i == 0 || !evalASCIIWord(low[i-1])) && (end == len(low) || !evalASCIIWord(low[end]))) {
				return true
			}
			offset = end
		}
	}
	return false
}

// AnalyzeEvalQuery parses date ranges and deterministic modes. now anchors
// relative dates; semantic intent enrichment is optional in the data layer.
func AnalyzeEvalQuery(query string, now time.Time) EvalQueryAnalysis {
	low := strings.ToLower(query)
	var qa EvalQueryAnalysis
	qa.Chronological = evalQueryContainsAny(low, evalChronoCues)
	qa.Aggregate = evalQueryContainsAny(low, evalAggregateCues)
	qa.Historical = qa.Chronological || evalQueryContainsAny(low, []string{"以前", "曾经", "原来", "之前", "历次", "变迁", "变化", "previous", "previously", "used to", "formerly", "history", "changed", "over time"})
	qa.Historical = qa.Historical || qa.Aggregate && evalQueryContainsAny(low, []string{"all conversations", "entire history", "life story", "所有对话", "全部对话", "完整经历"})
	qa.Procedural = len(EvalRuleAnchors(query)) > 0 || evalQueryContainsAny(low, []string{"规则", "按照", "根据", "推断", "模拟", "规律", "输出", "输入", "函数", "类推", "编码", "解码", "rule", "rules", "apply", "according to", "predict", "pattern", "simulate", "decode", "encode", "instruction", "instructions", "infer", "compute", "calculate", "output", "mapping", "based on examples"})
	qa.Enumerative = evalQueryContainsAny(low, []string{"list all", "enumerate", "all members", "all owners", "fields", "which items", "集合", "枚举", "字段", "清单", "所有成员", "全部成员", "有哪些"})
	qa.Relational = evalQueryContainsAny(low, []string{"end up", "ultimately", "received", "handoff", "path", "chain", "depends", "cause", "caused", "why", "connected", "related", "relationship between", "转交", "传递", "归属", "依赖", "导致", "原因", "证据链", "路径", "最终", "谁接收", "什么关系"})
	qa.Stepwise = evalQueryContainsAny(low, []string{"steps", "workflow", "procedure", "playbook", "checklist", "步骤", "流程", "程序"})
	trajectory := evalQueryContainsAny(low, []string{"evolve", "evolved", "evolution", "trajectory", "progression", "轨迹", "演变", "历程"})
	qa.Chronological = qa.Chronological || trajectory
	qa.Historical = qa.Historical || trajectory
	qa.Aggregate = qa.Aggregate || qa.Enumerative
	qa.Procedural = qa.Procedural || qa.Stepwise || evalQueryContainsAny(low, []string{"format", "response format", "格式", "输出约束"})
	qa.RangeStart, qa.RangeEnd = evalQueryDateRange(query, low, now)
	qa.Historical = qa.Historical || qa.RangeStart != nil || qa.RangeEnd != nil
	return qa
}

// evalQueryDateRange resolves absolute (2025年5月7日 / 2025-05-07 /
// MM/DD/YYYY) and relative date mentions combined with range connectives
// (之后/以前/从…到…/between X and Y/since X) into inclusive day bounds.
func evalQueryDateRange(query, low string, now time.Time) (*time.Time, *time.Time) {
	return evalQuerySpanRange(query, low, now)
}

func evalASCIIWord(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

// Absolute-date scanning is shared with Add so source mentions and query
// ranges use the same calendar validation and granularity.
func evalQuerySpanRange(query, low string, now time.Time) (*time.Time, *time.Time) {
	spans := ResolveEvalDateSpans(query, now)
	if len(spans) == 0 {
		return nil, nil
	}
	dayPtr := func(day string) *time.Time {
		t, err := time.ParseInLocation("2006-01-02", day, now.Location())
		if err != nil {
			return nil
		}
		return &t
	}
	start, end := spans[0].Start, spans[0].End
	for _, span := range spans[1:] {
		if span.Start < start {
			start = span.Start
		}
		if span.End > end {
			end = span.End
		}
	}
	since := evalQueryContainsAny(low, evalRangeSinceCues)
	until := evalQueryContainsAny(low, evalRangeUntilCues)
	between := strings.Contains(low, "between") || strings.Contains(query, "从") && (strings.Contains(query, "到") || strings.Contains(query, "至"))
	if !between && since && !until {
		return dayPtr(start), nil
	}
	if !between && until && !since {
		return nil, dayPtr(end)
	}
	return dayPtr(start), dayPtr(end)
}
