package biz

import (
	"regexp"
	"strings"
	"time"
	"unicode"
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
	// Code asks for commit/diff evidence (case_fix / case_feature).
	// CodeSolve is the narrower single-trajectory repair prompt used by
	// CAMBench; ordinary code queries retain cross-session and L2 behavior.
	// Visual asks for an image caption. These are lexical hints only.
	Code      bool
	CodeSolve bool
	// CodeError is an exception/test-failure follow-up. It needs precise
	// source/API evidence and must not be flooded by unrelated solved patches.
	CodeError  bool
	Visual     bool
	RangeStart *time.Time
	RangeEnd   *time.Time
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

	evalCodePathRe = regexp.MustCompile(`(?i)\b[\w.-]+/[\w./-]+\.(?:py|go|js|jsx|ts|tsx|java|rs|cpp|cc|c|h|hpp|rb|php|cs)\b`)
	// CamelCase class names such as TimeSeries and QTable distinguish code
	// implementation follow-ups from ordinary prose questions.
	evalCodeIdentifierRe = regexp.MustCompile(`\b[A-Z][a-z0-9]+(?:[A-Z][A-Za-z0-9]*)+\b`)
	evalCodeSymbolRe     = regexp.MustCompile(`\b_?[a-z][a-z0-9]*(?:_[a-z0-9]+)+\b`)
	// Strong code signals route a query to the coding evidence pack even
	// without error/fix keywords. CAMBench coding agents mostly issue bare
	// keyword-soup follow-ups ("count query strip unused annotations
	// Count('chapters')"); under the keyword-only rule 65% of real queries
	// missed Code classification and fell back to generic MMR fragments.
	//
	// evalCodeCallRe matches function-call shapes such as count() or
	// Book.objects.annotate(Count('chapters')). Natural prose essentially
	// never attaches a parenthesised argument list directly to a word.
	evalCodeCallRe = regexp.MustCompile(`\b[A-Za-z_][\w.]*\([^)]{0,60}\)`)
	// evalCodeModAttrRe matches module/Class attribute access
	// (Card.fromstring, requests.get). The attribute side must start
	// lowercase/underscore so prose abbreviations ("U.S.") do not qualify.
	evalCodeModAttrRe = regexp.MustCompile(`\b(?:[A-Z]\w*|[a-z_][\w]*_[\w]*)\.[a-z_]\w*`)
	// evalCodeHeaderRe matches HTTP-header-style tokens (Content-Length,
	// WWW-Authenticate): capitalised segments joined by hyphens. A match
	// must contain a lowercase letter (checked in evalCodeStrongSignal) so
	// all-caps prose acronyms such as COVID-19 stay non-code.
	evalCodeHeaderRe = regexp.MustCompile(`\b[A-Z][A-Za-z0-9]*(?:-[A-Za-z0-9]+)+\b`)
)

// evalCodeStrongSignal reports whether the query carries an unambiguous code
// token — a source path, snake_case symbol, function call, module attribute
// or header-style token. Unlike the CamelCase identifier heuristic these
// shapes essentially never occur in natural-language prose, so they alone
// suffice for Code classification.
func evalCodeStrongSignal(query, low string) bool {
	if evalCodePathRe.MatchString(query) || evalCodeSymbolRe.MatchString(low) ||
		evalCodeCallRe.MatchString(query) || evalCodeModAttrRe.MatchString(query) {
		return true
	}
	for _, m := range evalCodeHeaderRe.FindAllString(query, -1) {
		if strings.IndexFunc(m, unicode.IsLower) >= 0 {
			return true
		}
	}
	return false
}

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

// EvalQueryAsksCurrentState reports a present-tense current-value question
// ("now", "currently", "目前", "现在"). Such asks must not open Historical
// mode from VIR chronological false-positives, or past residence leaks into
// the current-city answer (AMC live-quality: Rome vs Berlin).
func EvalQueryAsksCurrentState(query string) bool {
	low := strings.ToLower(strings.TrimSpace(query))
	if low == "" {
		return false
	}
	// Cues must be bare tokens: trailing spaces break evalQueryContainsAny's
	// ASCII word-boundary check ("current " fails on "current address").
	return evalQueryContainsAny(low, []string{
		"now", "currently", "current", "at the moment", "right now",
		"现在", "目前", "当前", "此刻",
	})
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
	errorCue := evalQueryContainsAny(low, []string{
		"error", "invalid", "missing", "required", "expected", "exception", "failed", "failure", "traceback",
	}) || strings.Contains(low, "required_columns")
	codeEntity := evalCodePathRe.MatchString(query) || evalCodeIdentifierRe.MatchString(query) ||
		evalCodeSymbolRe.MatchString(low) || evalQueryContainsAny(low, []string{"column", "columns"})
	qa.CodeSolve = evalQueryContainsAny(low, []string{
		"pr introduces", "source changes", "repository is checked out",
		"investigate the issue", "implement the fix",
	}) || evalCodeSolveFollowupQuery(query, low)
	qa.Code = qa.CodeSolve || EvalCaseFactKind(query) != "" || len(EvalQueryCaseScopes(query)) > 0 || evalQueryContainsAny(low, []string{
		"bug", "panic", "stack trace", "nil pointer", "nil-pointer", "commit", "patch", "regression",
		"traceback", "exception", "diff", "fix", "sub-package", "codebase", "pull request",
		"报错", "空指针", "堆栈", "补丁", "编译失败", "怎么修", "如何修", "修复",
	}) || codeEntity && (errorCue || evalQueryContainsAny(low, []string{"test", "tests"})) ||
		evalCodeStrongSignal(query, low)
	qa.CodeError = qa.Code && !qa.CodeSolve && errorCue && codeEntity
	qa.Visual = evalQueryContainsAny(low, []string{
		"screenshot", "image", "picture", "diagram", "photo", "figure",
		"截图", "图片", "照片", "示意图", "看图", "图里", "图中", "这张图", "附图",
	})
	return qa
}

// evalCodeSolveFollowupQuery keeps subsequent implementation questions on the
// same complete trajectory as the initial repair prompt. Without this, a
// follow-up about a class, source file or expected test behavior falls back to
// generic fragment retrieval and loses the very patch found by the first query.
func evalCodeSolveFollowupQuery(query, low string) bool {
	if evalQueryContainsAny(low, []string{
		"exception", "error", "invalid", "missing", "failed", "failure", "traceback", "fix",
	}) {
		// Error-driven searches need the exact function/source evidence, not a
		// whole historical solve trajectory merely because they also ask about
		// expected behavior.
		return false
	}
	codeEntity := evalCodePathRe.MatchString(query) || evalCodeIdentifierRe.MatchString(query)
	if codeEntity && evalQueryContainsAny(low, []string{
		"implementation details", "implemented using", "implementation of",
		"source implementation", "expected behavior", "test behavior",
	}) {
		return true
	}
	return evalQueryContainsAny(low, []string{"expected behavior", "test behavior"}) &&
		evalQueryContainsAny(low, []string{"test", "tests"})
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
