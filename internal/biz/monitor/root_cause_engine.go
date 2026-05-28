package monitor

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

type RootCauseRule struct {
	ID          string
	Name        string
	Description string
	Condition   RootCauseCondition
	RootCause   string
	FixSuggest  string
	Severity    string
}

type RootCauseCondition struct {
	StepID           string
	Phase            string
	ErrorCodes       []string
	Pattern          string
	compiledPattern  *regexp.Regexp
	Prerequisites    []Prerequisite
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
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type RootCauseEngine struct {
	rules []RootCauseRule
}

func NewRootCauseEngine() *RootCauseEngine {
	rules := builtinRootCauseRules()
	for i := range rules {
		if p := rules[i].Condition.Pattern; p != "" {
			if re, err := regexp.Compile(p); err == nil {
				rules[i].Condition.compiledPattern = re
			}
		}
	}
	return &RootCauseEngine{rules: rules}
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
				Metadata:   metadata,
			})
		}
	}
	return results
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
