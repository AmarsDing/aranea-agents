package intent

import (
	"regexp"
	"strings"
	"unicode"
)

// SafetyAction is the product policy for a pre-LLM input scan (Q6).
// Deny is zero-LLM refuse. HITL continues the turn but tools stay behind
// confirmation. Inform is log-only (shadow). h2-class BGP wording must
// never land in Deny.
type SafetyAction string

const (
	SafetyNone   SafetyAction = ""
	SafetyInform SafetyAction = "inform"
	SafetyHITL   SafetyAction = "hitl"
	SafetyDeny   SafetyAction = "deny"
)

// SafetyDenyUserMessage is the user-visible refuse text for Deny.
const SafetyDenyUserMessage = "该请求包含不可逆的破坏性操作（删库、格式化磁盘、rm -rf 等），系统已拒绝执行。故障注入类运维请使用已授权工具并经确认，不会在输入扫描层直接拒绝。"

// SafetyVerdict is the policy-table result for one user turn.
type SafetyVerdict struct {
	Action SafetyAction
	Flags  []string
	Hits   []string
}

// inputRiskDenyKeywords are irreversible destruction. HITL/fault language
// stays out of this list so S14-h2 cannot be hard-refused.
var inputRiskDenyKeywords = []string{
	"drop table", "truncate table", "delete from ", "drop database",
	"删库", "删除数据库",
	"格式化磁盘", "format disk",
}

// inputRiskHITLKeywords are destructive-adjacent ops that still need a
// human confirm on the tool, not a zero-LLM refuse.
var inputRiskHITLKeywords = []string{
	"fault_inject", "fault inject", "故障注入", "注入故障", "gns3_fault_inject",
	"模拟故障", "故障模拟",
}

// inputRiskSoftKeywords is the Inform (shadow) near-miss table.
var inputRiskSoftKeywords = []string{
	"bgp", "邻居断", "断邻居", "port down", "link down",
}

// inputRiskSep is command/path prefix separators (aligned with L3 20261267).
const inputRiskSep = `(^|[;&|/\s"'($` + "`" + `])`

// inputRiskPatterns are Deny-class regexes (rm / mkfs / dd).
var inputRiskPatterns = []*regexp.Regexp{
	regexp.MustCompile(inputRiskSep + `(?:sudo\s+(?:-\S+\s+)*)?(?:/[\w.-]+)*/?rm(?:\s+(?:-{1,2}[\w=-]+|--))*\s+(?:-[a-z]*r[a-z]*|--recursive)(?:\s+(?:-{1,2}[\w=-]+|--))*\s+[/~.*$\w-]\S*`),
	regexp.MustCompile(`(?i)` + inputRiskSep + `(?:sudo\s+(?:-\S+\s+)*)?(?:/[\w.-]+)*/?mkfs(?:\.[\w]+)?\s`),
	regexp.MustCompile(`(?i)` + inputRiskSep + `(?:sudo\s+(?:-\S+\s+)*)?dd\s+[^;&|]*\bof=/dev/`),
}

var concreteTargetRe = regexp.MustCompile(`(?i)(?:\b(?:sw|pc|r|fw|core-sw)\d+\b|\beth\d+\b|\b(?:\d{1,3}\.){3}\d{1,3}\b|https?://)`)

// ClassifyInputSafety is the Deny / HITL / Inform policy table (Q6).
func ClassifyInputSafety(userText string) SafetyVerdict {
	if strings.TrimSpace(userText) == "" {
		return SafetyVerdict{}
	}
	lower := strings.ToLower(userText)
	for _, kw := range inputRiskDenyKeywords {
		if strings.Contains(lower, kw) {
			return SafetyVerdict{Action: SafetyDeny, Flags: []string{"destructive"}, Hits: []string{kw}}
		}
	}
	for _, re := range inputRiskPatterns {
		if re.MatchString(userText) {
			return SafetyVerdict{Action: SafetyDeny, Flags: []string{"destructive"}, Hits: []string{re.String()}}
		}
	}
	for _, kw := range inputRiskHITLKeywords {
		if strings.Contains(lower, kw) {
			return SafetyVerdict{Action: SafetyHITL, Flags: []string{"destructive"}, Hits: []string{kw}}
		}
	}
	if shadow := scanShadowHits(lower); len(shadow) > 0 {
		return SafetyVerdict{Action: SafetyInform, Hits: shadow}
	}
	return SafetyVerdict{}
}

// ScanInputRisk returns destructive flags for Deny and HITL hits (audit /
// ForceDestructiveFlag). Inform-only shadow hits are not flags.
func ScanInputRisk(userText string) []string {
	v := ClassifyInputSafety(userText)
	if v.Action == SafetyDeny || v.Action == SafetyHITL {
		return v.Flags
	}
	return nil
}

// ScanInputRiskShadowHits returns Inform near-miss tokens when the hard
// scan missed. Callers log-only; do not treat as a flag.
func ScanInputRiskShadowHits(userText string) []string {
	v := ClassifyInputSafety(userText)
	if v.Action != SafetyInform {
		return nil
	}
	return v.Hits
}

func scanShadowHits(lower string) []string {
	var hits []string
	for _, kw := range inputRiskSoftKeywords {
		if strings.Contains(lower, kw) {
			hits = append(hits, kw)
		}
	}
	return hits
}

// LooksLikeExplicitInstruction reports a user turn that already names a
// concrete target and an action (Q2 armB: memory/intent must not force
// clarification). Underspecified "帮我做个…" stays false.
func LooksLikeExplicitInstruction(userText string) bool {
	t := strings.TrimSpace(userText)
	if t == "" || LooksLikeUnderspecifiedTask(t) {
		return false
	}
	if !concreteTargetRe.MatchString(t) {
		return false
	}
	hasLetter := false
	n := 0
	for _, r := range t {
		n++
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	if !hasLetter || n < 8 {
		return false
	}
	return true
}
