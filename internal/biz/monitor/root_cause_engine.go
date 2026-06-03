package monitor

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"aranea-agents/pkg/loggateway"
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
}

type Prerequisite struct {
	StepID string
	Phase  string
}

type RootCauseResult struct {
	RuleID     string         `json:"rule_id"`
	Name       string         `json:"name"`
	RootCause  string         `json:"root_cause"`
	FixSuggest string         `json:"fix_suggest"`
	Severity   string         `json:"severity"`
	Confidence float64        `json:"confidence"`
	FixAction  FixAction      `json:"fix_action"`
	Metadata   map[string]any `json:"metadata,omitempty"`
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

func (e *RootCauseEngine) Evaluate(ctx context.Context, stepID, phase string, metadata map[string]any) []RootCauseResult {
	if e == nil {
		return nil
	}
	var results []RootCauseResult
	for _, rule := range e.rules {
		if match, confidence := e.matchRule(rule, stepID, phase, metadata); match {
			results = append(results, RootCauseResult{
				RuleID:     rule.ID,
				Name:       rule.Name,
				RootCause:  rule.RootCause,
				FixSuggest: rule.FixSuggest,
				Severity:   rule.Severity,
				Confidence: confidence,
				FixAction:  rule.FixAction,
				Metadata:   metadata,
			})
		}
	}
	return results
}

// AddRules appends custom rules to the engine. Rules with duplicate IDs are skipped.
func (e *RootCauseEngine) AddRules(rules []RootCauseRule) {
	if e == nil {
		return
	}
	existing := make(map[string]struct{}, len(e.rules))
	for _, r := range e.rules {
		existing[r.ID] = struct{}{}
	}
	for i := range rules {
		if _, dup := existing[rules[i].ID]; dup {
			continue
		}
		if p := rules[i].Condition.Pattern; p != "" {
			if re, err := regexp.Compile(p); err == nil {
				rules[i].Condition.compiledPattern = re
			} else {
				e.lg.Error("AddRules: regexp.Compile failed",
					loggateway.StepID("monitor.root_cause_regex_fail"),
					loggateway.Str("rule_id", rules[i].ID), loggateway.Str("pattern", p), loggateway.Err(err))
				continue
			}
		}
		e.rules = append(e.rules, rules[i])
		existing[rules[i].ID] = struct{}{}
	}
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
	}
}

func RootCauseResultsToJSON(results []RootCauseResult) string {
	if len(results) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(results)
	return string(b)
}
