package biz

import (
	"regexp"
	"strings"
)

var evalSubjectVerb = regexp.MustCompile(`(?i)\b(?:favou?rite|likes?|loves?|prefers?|hates?|dislikes?|enjoys?|lives?|lived|moved|works?|joined|phone|name|employee|email|e-mail|timezone|time zone|preferred language)\b`)
var evalSelfEN = regexp.MustCompile(`(?i)^(?:i\b|my\s+(?:favou?rite|name|employee|phone|mobile|address|job|email|e-mail|timezone|time zone|preferred language)\b|the user\b)`)
var evalLikeObject = regexp.MustCompile(`(?i)\b(?:like|love|prefer|hate|enjoy|dislike)s?\s+([^,.;!?]+)`)
var evalCNLikeObject = regexp.MustCompile(`(?:不喜欢|喜欢|偏好|讨厌)([^，。；!?]+)`)
var evalFavoriteAttribute = regexp.MustCompile(`(?i)\bfavou?rite\s+([a-z][a-z -]{0,60}?)\s+(?:is|was|are|changed to|has changed to|became)\b`)

func evalSpeaker(statement string) (string, string) {
	s := strings.TrimSpace(statement)
	if i := strings.Index(s, ": "); i >= 0 {
		role := strings.ToLower(s[:i])
		switch role {
		case "user", "assistant", "system", "developer", "tool":
			return role, strings.TrimSpace(s[i+2:])
		}
	}
	return "user", s
}

func evalSubject(statement string) string {
	role, body := evalSpeaker(statement)
	low := strings.ToLower(body)
	for _, prefix := range []string{"actually, ", "actually ", "in fact, ", "in fact ", "correction: ", "now ", "其实", "不对，", "更正："} {
		low = strings.TrimSpace(strings.TrimPrefix(low, prefix))
	}
	thirdPersonCN := false
	for _, prefix := range []string{"我朋友", "我的朋友", "我的同事", "我同事", "我的妈妈", "我的父亲", "我妈妈", "我父亲", "我的女儿", "我的儿子", "我的妻子", "我的丈夫", "我的哥哥", "我的姐姐", "我的客户", "我的邻居"} {
		thirdPersonCN = thirdPersonCN || strings.HasPrefix(low, prefix)
	}
	if evalSelfEN.MatchString(low) || strings.HasPrefix(low, "我") && !thirdPersonCN || strings.HasPrefix(low, "用户") {
		return role + ":self"
	}
	if idx := evalSubjectVerb.FindStringIndex(low); idx != nil && idx[0] > 0 {
		name := strings.TrimSpace(low[:idx[0]])
		name = strings.TrimSuffix(strings.TrimSuffix(name, "'s"), "’s")
		return role + ":" + name
	}
	for _, marker := range []string{"最喜欢", "喜欢", "住在", "搬到", "搬去", "的名字", "的工号", "的手机号", "的邮箱", "的时区", "的首选语言"} {
		if i := strings.Index(low, marker); i > 0 {
			return role + ":" + strings.TrimSpace(low[:i])
		}
	}
	return ""
}

// EvalGovernanceKey scopes single-value updates to one speaker, subject and
// attribute. The like slot stays object-specific so independent interests coexist.
func EvalGovernanceKey(statement string) string {
	subject, slot := evalGovernanceSubject(statement), EvalFactSlot(statement)
	if subject == "" || slot == "" {
		return ""
	}
	if slot == "like" {
		_, body := evalSpeaker(statement)
		slot += ":" + evalPreferenceObject(body)
	}
	return subject + "|" + slot
}

// IsEvalHistoricalStatement recognizes an explicit retrospective state, rather
// than treating its later reporting time as a new present-tense assertion.
// A statement that also explicitly establishes the present remains current.
func IsEvalHistoricalStatement(statement string) bool {
	_, body := evalSpeaker(statement)
	low := strings.ToLower(body)
	past := evalQueryContainsAny(low, []string{"used to", "previously", "formerly", "in the past", "以前", "曾经", "过去住", "原来住", "过去的"})
	current := evalQueryContainsAny(low, []string{"but now", "currently", "from now on", "但现在", "现在改", "目前"})
	return past && !current
}

func evalSameSubject(oldStmt, newStmt string) bool {
	a, b := evalGovernanceSubject(oldStmt), evalGovernanceSubject(newStmt)
	return a != "" && a == b
}

func evalGovernanceSubject(statement string) string {
	if EvalInstructionSlot(statement) != "" {
		role, _ := evalSpeaker(statement)
		return role + ":instruction:" + EvalInstructionScope(statement)
	}
	return evalSubject(statement)
}

// IsEvalPlannedStatement distinguishes an intention from a completed update.
func IsEvalPlannedStatement(statement string) bool {
	low := strings.ToLower(statement)
	for _, cue := range []string{"i plan to", "i am planning", "i will ", "i'm going to", "i intend to", "我打算", "我准备", "我计划", "我将", "即将"} {
		if strings.Contains(low, cue) {
			return true
		}
	}
	return false
}

func evalExplicitPreferenceCorrection(oldStmt, newStmt string) bool {
	_, oldBody := evalSpeaker(oldStmt)
	_, nextBody := evalSpeaker(newStmt)
	object := evalPreferenceObject(oldBody)
	next := strings.ToLower(nextBody)
	return object != "" && strings.Contains(next, object) &&
		(evalPreferenceNegative(oldBody) != evalPreferenceNegative(nextBody) || strings.Contains(next, "instead of") || strings.Contains(next, "而不"))
}

func evalPreferenceObject(body string) string {
	m := evalLikeObject.FindStringSubmatch(body)
	if len(m) < 2 {
		m = evalCNLikeObject.FindStringSubmatch(body)
	}
	if len(m) < 2 {
		return ""
	}
	obj := strings.TrimSpace(strings.ToLower(m[1]))
	for _, suffix := range []string{" now", " anymore", "了"} {
		obj = strings.TrimSuffix(obj, suffix)
	}
	return strings.TrimSpace(obj)
}

func evalPreferenceNegative(body string) bool {
	low := strings.ToLower(body)
	return evalQueryContainsAny(low, []string{"no longer", "not anymore", "don't like", "do not like", "dislike", "dislikes", "hate", "hates", "不喜欢", "不再喜欢", "讨厌"})
}

// EvalCorrectionTargetsFact checks semantic compatibility after vector discovery.
// Similarity alone never establishes a contradiction or authorizes invalidation.
func EvalCorrectionTargetsFact(oldStmt, newStmt string) bool {
	_, body := evalSpeaker(newStmt)
	if !evalSameSubject(oldStmt, newStmt) || IsHypotheticalStatement(body) || IsEvalPlannedStatement(body) || IsEvalHistoricalStatement(newStmt) {
		return false
	}
	oldSlot, nextSlot := EvalFactSlot(oldStmt), EvalFactSlot(newStmt)
	if oldSlot != "" && oldSlot == nextSlot {
		return oldSlot != "like" || evalExplicitPreferenceCorrection(oldStmt, newStmt)
	}
	// Explicit rejection of a named previous value, e.g. favorite blue -> hate blue.
	if strings.HasPrefix(oldSlot, "favorite:") && nextSlot == "like" && evalPreferenceNegative(body) {
		_, old := evalSpeaker(oldStmt)
		value := ""
		if idx := evalFavoriteAttribute.FindStringIndex(old); idx != nil {
			value = strings.TrimSpace(strings.TrimRight(strings.ToLower(old[idx[1]:]), ".!? "))
		}
		if idx := chineseFavoriteRe.FindStringIndex(old); idx != nil {
			value = strings.TrimSpace(strings.TrimRight(old[idx[1]:], "。！了 "))
		}
		obj := evalPreferenceObject(body)
		return value != "" && obj != "" && value == obj
	}
	return false
}
