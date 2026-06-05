package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"aranea-agents/internal/biz/types"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type RootCauseRule struct {
	ID          string
	Name        string
	Description string
	Condition   RootCauseCondition
	RootCause   string
	FixSuggest  string
	Severity    string
	FixAction   FixAction
}

// FixAction describes an automated corrective action for a root cause match.
type FixAction struct {
	Type        string         `json:"type"`         // "retry", "reconnect", "fallback", "log_only"
	MaxAttempts int            `json:"max_attempts"` // max retry/reconnect attempts (0 = no auto-fix)
	Params      map[string]any `json:"params"`       // action-specific parameters
}

type RootCauseCondition struct {
	StepID          string
	Phase           string
	ErrorCodes      []string
	Pattern         string
	compiledPattern *regexp.Regexp
	Prerequisites   []Prerequisite
	SelfCheckStatus *types.SelfCheckStatusCondition
	AutoHealed      *bool // nil=don't care, true=only match auto-healed, false=only match not auto-healed
	HealAttempts    int   // runtime heal attempts threshold (0=don't care)
}

type Prerequisite struct {
	StepID string
	Phase  string
}

type RootCauseResult struct {
	RuleID              string         `json:"rule_id"`
	Name                string         `json:"name"`
	RootCause           string         `json:"root_cause"`
	FixSuggest          string         `json:"fix_suggest"`
	Severity            string         `json:"severity"`
	Confidence          float64        `json:"confidence"`
	FixAction           FixAction      `json:"fix_action"`
	Metadata            map[string]any `json:"metadata,omitempty"`
	RuntimeAutoHealed   bool           `json:"runtime_auto_healed"`
	RuntimeHealAttempts int            `json:"runtime_heal_attempts"`
}

type RootCauseEngine struct {
	rules []RootCauseRule
	lg    loggateway.Logger
}

func NewRootCauseEngine(lg loggateway.Logger) *RootCauseEngine {
	rules := builtinRootCauseRules()
	for i := range rules {
		if p := rules[i].Condition.Pattern; p != "" {
			if re, err := regexp.Compile(p); err == nil {
				rules[i].Condition.compiledPattern = re
			} else {
				lg.Error("NewRootCauseEngine: regexp.Compile failed",
					loggateway.StepID("monitor.root_cause_regex_fail"),
					loggateway.Str("rule_id", rules[i].ID), loggateway.Str("pattern", p), loggateway.Err(err))
			}
		}
	}
	return &RootCauseEngine{rules: rules, lg: lg}
}

// Analyze implements RootCauseAnalyzer by delegating to Evaluate and returning
// the first matching result, or nil if no rule matches.
func (e *RootCauseEngine) Analyze(ctx context.Context, stepID, phase string, err error, metadata map[string]any) (*RootCauseResult, error) {
	results := e.Evaluate(ctx, stepID, phase, metadata)
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

// AnalyzeFromReport implements RootCauseAnalyzer.AnalyzeFromReport by converting
// a FailureReport into the internal metadata format and delegating to Analyze.
// Returns nil if report is nil or no rule matches.
func (e *RootCauseEngine) AnalyzeFromReport(ctx context.Context, report *FailureReport) (*RootCauseResult, error) {
	if report == nil {
		return nil, nil
	}

	stepID := reportToStepID(report)
	phase := "error"
	metadata := reportToMetadata(report)

	return e.Analyze(ctx, stepID, phase, nil, metadata)
}

// reportToStepID maps a FailureReport to a step ID for rule matching.
func reportToStepID(report *FailureReport) string {
	switch report.Type {
	case FailureTypeBuild, FailureTypeLint, FailureTypeProtoSync:
		return "build*"
	case FailureTypeTest:
		return "test*"
	case FailureTypeRuntime:
		// Use the Job field to infer step ID for runtime errors
		job := strings.ToLower(report.Job)
		if strings.Contains(job, "mcp") {
			return "mcp*"
		}
		if strings.Contains(job, "llm") {
			return "llm*"
		}
		if strings.Contains(job, "tool") {
			return "tool*"
		}
		if strings.Contains(job, "memory") {
			return "memory*"
		}
		if strings.Contains(job, "session") {
			return "session*"
		}
		if strings.Contains(job, "skill") {
			return "skill*"
		}
		if strings.Contains(job, "graph") {
			return "graph*"
		}
		return "runtime*"
	default:
		return "unknown*"
	}
}

// reportToMetadata converts a FailureReport into the metadata map format
// used by the rule matching engine.
func reportToMetadata(report *FailureReport) map[string]any {
	metadata := make(map[string]any)
	if report.ErrorCode != "" {
		metadata["error_code"] = report.ErrorCode
	}
	if report.Message != "" {
		metadata["error_message"] = report.Message
	}
	if report.File != "" {
		metadata["file"] = report.File
	}
	if report.Line > 0 {
		metadata["line"] = report.Line
	}
	if report.Source != "" {
		metadata["source"] = report.Source
	}
	if report.Job != "" {
		metadata["job"] = report.Job
	}
	if report.StackTrace != "" {
		metadata["stack_trace"] = report.StackTrace
	}
	// Copy string metadata into map[string]any
	for k, v := range report.Metadata {
		metadata[k] = v
	}
	return metadata
}

func (e *RootCauseEngine) Evaluate(ctx context.Context, stepID, phase string, metadata map[string]any) []RootCauseResult {
	if e == nil {
		return nil
	}
	var results []RootCauseResult
	for _, rule := range e.rules {
		if match, confidence := e.matchRule(rule, stepID, phase, metadata); match {
			runtimeAutoHealed := false
			runtimeHealAttempts := 0
			if metadata != nil {
				if v, ok := metadata["auto_healed"].(bool); ok {
					runtimeAutoHealed = v
				}
				if v, ok := metadata["heal_attempts"].(int); ok {
					runtimeHealAttempts = v
				}
			}
			results = append(results, RootCauseResult{
				RuleID:              rule.ID,
				Name:                rule.Name,
				RootCause:           rule.RootCause,
				FixSuggest:          rule.FixSuggest,
				Severity:            rule.Severity,
				Confidence:          confidence,
				FixAction:           rule.FixAction,
				Metadata:            metadata,
				RuntimeAutoHealed:   runtimeAutoHealed,
				RuntimeHealAttempts: runtimeHealAttempts,
			})
		}
	}
	return results
}

// AddRules appends custom rules to the engine. Rules with duplicate IDs or
// invalid configuration (empty ID/Name, bad regex) return an error.
func (e *RootCauseEngine) AddRules(rules []RootCauseRule) error {
	if e == nil {
		return nil
	}
	existing := make(map[string]struct{}, len(e.rules))
	for _, r := range e.rules {
		existing[r.ID] = struct{}{}
	}
	for i := range rules {
		if strings.TrimSpace(rules[i].ID) == "" {
			return kerrors.BadRequest("RCA_RULE_INVALID", fmt.Sprintf("rule at index %d has empty ID", i))
		}
		if strings.TrimSpace(rules[i].Name) == "" {
			return kerrors.BadRequest("RCA_RULE_INVALID", fmt.Sprintf("rule %q has empty Name", rules[i].ID))
		}
		if _, dup := existing[rules[i].ID]; dup {
			return kerrors.BadRequest("RCA_RULE_DUPLICATE", fmt.Sprintf("rule %q has duplicate ID", rules[i].ID))
		}
		if p := rules[i].Condition.Pattern; p != "" {
			if re, err := regexp.Compile(p); err == nil {
				rules[i].Condition.compiledPattern = re
			} else {
				return kerrors.BadRequest("RCA_RULE_REGEX", fmt.Sprintf("rule %q has invalid regex %q: %s", rules[i].ID, p, err.Error()))
			}
		}
		e.rules = append(e.rules, rules[i])
		existing[rules[i].ID] = struct{}{}
	}
	return nil
}

// Rules returns a copy of all registered rules.
func (e *RootCauseEngine) Rules() []RootCauseRule {
	if e == nil {
		return nil
	}
	out := make([]RootCauseRule, len(e.rules))
	copy(out, e.rules)
	return out
}

func (e *RootCauseEngine) matchRule(rule RootCauseRule, stepID, phase string, metadata map[string]any) (bool, float64) {
	cond := rule.Condition
	if cond.StepID != "" && !matchStepID(cond.StepID, stepID) {
		return false, 0
	}
	if cond.Phase != "" && !strings.EqualFold(cond.Phase, phase) {
		return false, 0
	}
	if len(cond.ErrorCodes) > 0 {
		code := metaStr(metadata, "error_code")
		if code == "" {
			code = metaStr(metadata, "code")
		}
		found := false
		for _, ec := range cond.ErrorCodes {
			if strings.EqualFold(ec, code) {
				found = true
				break
			}
		}
		if !found {
			return false, 0
		}
	}
	if cond.compiledPattern != nil {
		msg := metaStr(metadata, "error_message")
		if msg == "" {
			msg = metaStr(metadata, "message")
		}
		if !cond.compiledPattern.MatchString(msg) {
			return false, 0
		}
	}
	if cond.SelfCheckStatus != nil {
		sc := cond.SelfCheckStatus
		checkerName := metaStr(metadata, "checker")
		checkStatus := metaStr(metadata, "self_check_status")
		if sc.Checker != "" && !strings.EqualFold(sc.Checker, checkerName) {
			return false, 0
		}
		if sc.Status != "" && !strings.EqualFold(sc.Status, checkStatus) {
			return false, 0
		}
	}
	// AutoHealed condition: nil=don't care, true/false=must match
	if cond.AutoHealed != nil {
		autoHealed := false
		if metadata != nil {
			if v, ok := metadata["auto_healed"].(bool); ok {
				autoHealed = v
			}
		}
		if *cond.AutoHealed != autoHealed {
			return false, 0
		}
	}
	// HealAttempts condition: threshold check (metadata attempts >= condition threshold)
	if cond.HealAttempts > 0 {
		healAttempts := 0
		if metadata != nil {
			if v, ok := metadata["heal_attempts"].(int); ok {
				healAttempts = v
			}
		}
		if healAttempts < cond.HealAttempts {
			return false, 0
		}
	}
	confidence := 0.6
	if len(cond.Prerequisites) > 0 && metadata != nil {
		met := 0
		for _, pre := range cond.Prerequisites {
			if matchPrerequisite(pre, metadata) {
				met++
			}
		}
		if met > 0 {
			confidence += 0.1 * float64(met) / float64(len(cond.Prerequisites))
		}
	}
	if cond.StepID != "" && cond.Phase != "" {
		confidence += 0.1
	}
	if confidence > 1.0 {
		confidence = 1.0
	}
	return true, confidence
}

func matchPrerequisite(pre Prerequisite, metadata map[string]any) bool {
	if pre.StepID != "" {
		ms := metaStr(metadata, "step_id")
		if !matchStepID(pre.StepID, ms) {
			return false
		}
	}
	if pre.Phase != "" {
		mp := metaStr(metadata, "flow_phase")
		if !strings.EqualFold(pre.Phase, mp) {
			return false
		}
	}
	return true
}

func matchStepID(pattern, stepID string) bool {
	if pattern == stepID {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(stepID, prefix)
	}
	return false
}

func builtinRootCauseRules() []RootCauseRule {
	return []RootCauseRule{
		{
			ID: "rc-provider-timeout", Name: "Provider Timeout",
			Description: "LLM provider call exceeded timeout",
			Condition: RootCauseCondition{
				StepID: "llm*", Phase: "error",
				Pattern: `(?i)(timeout|timed out|deadline exceeded)`,
			},
			RootCause:  "LLM provider response time exceeded configured timeout",
			FixSuggest: "Increase provider timeout, switch to a faster model, or check provider service status",
			Severity:   "high",
			FixAction: FixAction{
				Type:        "retry",
				MaxAttempts: 2,
				Params:      map[string]any{"backoff_ms": 2000, "backoff_factor": 2.0},
			},
		},
		{
			ID: "rc-provider-rate-limit", Name: "Provider Rate Limited",
			Description: "LLM provider returned rate limit error",
			Condition: RootCauseCondition{
				StepID: "llm*", Phase: "error",
				ErrorCodes: []string{"429", "rate_limit", "rate_limit_exceeded"},
			},
			RootCause:  "LLM provider rate limit reached",
			FixSuggest: "Reduce request frequency, upgrade provider plan, or add retry with exponential backoff",
			Severity:   "medium",
			FixAction: FixAction{
				Type:        "retry",
				MaxAttempts: 3,
				Params:      map[string]any{"backoff_ms": 5000, "backoff_factor": 2.0},
			},
		},
		{
			ID: "rc-provider-auth-failure", Name: "Provider Auth Failure",
			Description: "LLM provider authentication failed",
			Condition: RootCauseCondition{
				StepID: "llm*", Phase: "error",
				ErrorCodes: []string{"401", "403", "authentication_failed", "invalid_api_key"},
			},
			RootCause:  "LLM provider authentication credentials are invalid or expired",
			FixSuggest: "Check API key configuration, rotate credentials, or verify provider account status",
			Severity:   "critical",
			FixAction:  FixAction{Type: "log_only"},
		},
		{
			ID: "rc-provider-context-exceeded", Name: "Context Window Exceeded",
			Description: "LLM provider context window limit exceeded",
			Condition: RootCauseCondition{
				StepID: "llm*", Phase: "error",
				Pattern: `(?i)(context.length|maximum.context|token.limit|context.window|too.many.tokens)`,
			},
			RootCause:  "Input exceeds the model's maximum context window size",
			FixSuggest: "Reduce input size, enable context compression, or switch to a model with a larger context window",
			Severity:   "medium",
			FixAction: FixAction{
				Type:        "fallback",
				MaxAttempts: 1,
				Params:      map[string]any{"strategy": "compress_and_retry"},
			},
		},
		{
			ID: "rc-tool-execution-failure", Name: "Tool Execution Failure",
			Description: "Tool call failed during execution",
			Condition: RootCauseCondition{
				StepID: "tool*", Phase: "error",
			},
			RootCause:  "Tool execution returned an error",
			FixSuggest: "Check tool configuration, input parameters, and external service availability",
			Severity:   "medium",
			FixAction: FixAction{
				Type:        "retry",
				MaxAttempts: 1,
				Params:      map[string]any{"backoff_ms": 1000},
			},
		},
		{
			ID: "rc-memory-read-error", Name: "Memory Read Error",
			Description: "Memory retrieval operation failed",
			Condition: RootCauseCondition{
				StepID: "memory*", Phase: "error",
			},
			RootCause:  "Memory store read operation failed",
			FixSuggest: "Check memory store connectivity and data integrity",
			Severity:   "low",
			FixAction: FixAction{
				Type:        "retry",
				MaxAttempts: 2,
				Params:      map[string]any{"backoff_ms": 500},
			},
		},
		{
			ID: "rc-mcp-connection-failure", Name: "MCP Connection Failure",
			Description: "MCP server connection failed",
			Condition: RootCauseCondition{
				StepID: "mcp*", Phase: "error",
				Pattern: `(?i)(connection refused|dial tcp|no such host)`,
			},
			RootCause:  "MCP server is unreachable",
			FixSuggest: "Verify MCP server is running, check network connectivity, and validate server URL configuration",
			Severity:   "high",
			FixAction: FixAction{
				Type:        "reconnect",
				MaxAttempts: 3,
				Params:      map[string]any{"backoff_ms": 3000, "backoff_factor": 2.0},
			},
		},
		{
			ID: "rc-session-expired", Name: "Session Expired",
			Description: "Session has expired or been invalidated",
			Condition: RootCauseCondition{
				StepID: "session*", Phase: "error",
				Pattern: `(?i)(session.expired|session.not.found|invalid.session)`,
			},
			RootCause:  "Session has expired or been invalidated",
			FixSuggest: "Create a new session or refresh the existing session token",
			Severity:   "medium",
			FixAction:  FixAction{Type: "log_only"},
		},
		{
			ID: "rc-skill-load-failure", Name: "Skill Load Failure",
			Description: "Skill loading or execution failed",
			Condition: RootCauseCondition{
				StepID: "skill*", Phase: "error",
				Pattern: `(?i)(skill.not.found|skill.load.failed|skill.execution.error)`,
			},
			RootCause:  "Skill file is missing, corrupted, or incompatible",
			FixSuggest: "Check skill file existence and format, verify skill compatibility with current version",
			Severity:   "low",
			FixAction: FixAction{
				Type:        "fallback",
				MaxAttempts: 1,
				Params:      map[string]any{"strategy": "skip_skill"},
			},
		},
		{
			ID: "rc-graph-execution-failure", Name: "Graph Execution Failure",
			Description: "Graph workflow execution failed",
			Condition: RootCauseCondition{
				StepID: "graph*", Phase: "error",
			},
			RootCause:  "Graph workflow encountered an execution error",
			FixSuggest: "Check graph definition, node configuration, and edge conditions",
			Severity:   "high",
			FixAction:  FixAction{Type: "log_only"},
		},
		{
			ID: "rc-self-check-failure", Name: "Self-Check Failure",
			Description: "A subsystem self-check reported failure status",
			Condition: RootCauseCondition{
				SelfCheckStatus: &types.SelfCheckStatusCondition{
					Status: "failed",
				},
			},
			RootCause:  "Subsystem self-check detected a failure condition",
			FixSuggest: "Review self-check report details, check subsystem health, and run repair actions",
			Severity:   "high",
			FixAction: FixAction{
				Type:        "reconnect",
				MaxAttempts: 1,
				Params:      map[string]any{"strategy": "self_repair"},
			},
		},
		{
			ID: "rc-repeated-auto-heal-failure", Name: "Repeated Auto-Heal Failure",
			Description: "Runtime auto-heal has failed repeatedly after multiple attempts",
			Condition: RootCauseCondition{
				AutoHealed:   boolPtr(true),
				HealAttempts: 3,
			},
			RootCause:  "Runtime auto-heal has failed repeatedly",
			FixSuggest: "Investigate the underlying error pattern, check provider/service health, and consider manual intervention",
			Severity:   "critical",
			FixAction:  FixAction{Type: "log_only"},
		},
	}
}

func RootCauseResultsToJSON(results []RootCauseResult) string {
	if len(results) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(results)
	return string(b)
}

func boolPtr(b bool) *bool { return &b }
