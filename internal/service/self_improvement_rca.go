package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/pkg/loggateway"
)

// siRepoFilePattern picks a repo-relative source path out of a stack or
// sample message so Analyst.affected_files can be traced to evidence.
var siRepoFilePattern = regexp.MustCompile(`(?i)((?:internal|cmd|pkg|web|api|configs)/[A-Za-z0-9_./\\-]+\.(?:go|ts|tsx|vue|js|yaml|yml|md|proto))`)

// WithSIAnalystRCA injects the monitor RootCauseAnalyzer as a structured prior.
func WithSIAnalystRCA(rca heal.RootCauseAnalyzer) SIAnalystOption {
	return func(a *SIAnalystAgent) {
		a.rca = rca
	}
}

func (a *SIAnalystAgent) rcaPrior(ctx context.Context, run *biz.SelfImprovementRun, sug *biz.UnifiedEvolutionSuggestion) (report *heal.FailureReport, hint string) {
	report = siFailureReportFromSuggestion(run, sug)
	if report == nil {
		return nil, ""
	}
	var b strings.Builder
	b.WriteString("\n[FailureReport 先验]\n")
	fmt.Fprintf(&b, "type=%s source=%s job=%s error_code=%s file=%s\n",
		report.Type, report.Source, report.Job, report.ErrorCode, report.File)
	if msg := strings.TrimSpace(report.Message); msg != "" {
		if len(msg) > 1024 {
			msg = msg[:1024] + "…[truncated]"
		}
		fmt.Fprintf(&b, "message: %s\n", msg)
	}
	if a == nil || a.rca == nil {
		return report, b.String()
	}
	result, err := a.rca.AnalyzeFromReport(ctx, report)
	if err != nil {
		a.lg.Warn("si analyst: RCA failed, continuing without rule match",
			loggateway.StepID("si_analyst.rca"), loggateway.Err(err))
		return report, b.String()
	}
	if result == nil {
		return report, b.String()
	}
	b.WriteString("[RootCauseAnalyzer]\n")
	fmt.Fprintf(&b, "rule_id=%s confidence=%.2f\nroot_cause=%s\nfix_suggest=%s\n",
		result.RuleID, result.Confidence, result.RootCause, result.FixSuggest)
	b.WriteString("请核对此先验；affected_files 必须能回溯到 FailureReport.file 或你用工具读到的路径。\n")
	return report, b.String()
}

func siEnrichDiagnosis(d *biz.Diagnosis, report *heal.FailureReport) *biz.Diagnosis {
	if d == nil || report == nil {
		return d
	}
	file := strings.TrimSpace(report.File)
	if file == "" {
		return d
	}
	for _, existing := range d.AffectedFiles {
		if filepathToSlash(existing) == filepathToSlash(file) {
			return d
		}
	}
	if len(d.AffectedFiles) == 0 {
		d.AffectedFiles = []string{filepathToSlash(file)}
	}
	return d
}

func siFailureReportFromSuggestion(run *biz.SelfImprovementRun, sug *biz.UnifiedEvolutionSuggestion) *heal.FailureReport {
	if sug == nil && run == nil {
		return nil
	}
	meta := siSuggestionMeta(sug)
	source := ""
	if run != nil {
		source = run.TriggerSource
	}
	if source == "" && sug != nil {
		source = sug.TriggerSource
	}
	report := heal.NewFailureReport()
	report.Source = "runtime"
	switch source {
	case biz.TriggerSourceErrorCluster:
		report.Type = heal.FailureTypeRuntime
		report.Job = metaString(meta, "component")
		report.ErrorCode = metaString(meta, "error_code")
		report.Message = firstNonEmpty(metaString(meta, "sample_message"), sugReason(sug))
	case biz.TriggerSourceTestFailure:
		report.Type = heal.FailureTypeTest
		report.Source = "ci"
		report.Job = firstNonEmpty(metaString(meta, "package"), "test")
		report.ErrorCode = metaString(meta, "test_name")
		report.Message = firstNonEmpty(metaString(meta, "last_error"), sugReason(sug))
		if pkg := metaString(meta, "package"); pkg != "" {
			report.Metadata["package"] = pkg
		}
	case biz.TriggerSourceEvalRegression:
		report.Type = heal.FailureTypeRuntime
		report.Job = firstNonEmpty(metaString(meta, "agent_id"), "eval")
		report.ErrorCode = "EVAL_REGRESSION"
		report.Message = sugReason(sug)
	case biz.TriggerSourcePerfBottleneck:
		report.Type = heal.FailureTypeRuntime
		report.Job = firstNonEmpty(metaString(meta, "step_id"), metaString(meta, "scope"), "perf")
		report.ErrorCode = firstNonEmpty(metaString(meta, "signal"), "PERF")
		report.Message = sugReason(sug)
	default:
		if sug == nil && len(meta) == 0 {
			return nil
		}
		report.Type = heal.FailureTypeRuntime
		report.Job = source
		report.Message = sugReason(sug)
	}
	if file := firstNonEmpty(metaString(meta, "file"), siGuessRepoFile(report.Message), siGuessRepoFile(sugReason(sug))); file != "" {
		report.File = filepathToSlash(file)
	}
	if report.ErrorCode == "" && report.Message == "" && report.File == "" {
		return nil
	}
	return report
}

func siSuggestionMeta(sug *biz.UnifiedEvolutionSuggestion) map[string]any {
	if sug == nil || len(sug.Metadata) == 0 {
		return nil
	}
	var meta map[string]any
	if err := json.Unmarshal(sug.Metadata, &meta); err != nil {
		return nil
	}
	return meta
}

func metaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func sugReason(sug *biz.UnifiedEvolutionSuggestion) string {
	if sug == nil {
		return ""
	}
	return strings.TrimSpace(sug.TriggerReason)
}

func siGuessRepoFile(text string) string {
	m := siRepoFilePattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return filepathToSlash(m[1])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
}
