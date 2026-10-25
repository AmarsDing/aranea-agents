package biz

import (
	"encoding/json"
	"regexp"
	"strings"
)

var evalListLine = regexp.MustCompile(`^\s*(?:[-*+]\s+|\d{1,3}[.)]\s+)`)
var evalTableSeparator = regexp.MustCompile(`^\s*\|?\s*:?-{2,}`)
var evalProjectScope = regexp.MustCompile(`(?i)\b(?:project|workspace|scenario)\s+([a-z0-9_-]+)\b`)
var evalScopedInstruction = regexp.MustCompile(`(?i)^\s*(?:only\s+)?(?:for|in|within)\s+(?:project|workspace|scenario)\s+([a-z0-9_-]+)\b`)
var evalProjectCN = regexp.MustCompile(`项目[“「『"]?([\p{Han}a-zA-Z0-9_-]{1,24})`)
var evalAllergyAttribute = regexp.MustCompile(`(?i)\ballergic\s+to\s+([a-z][a-z -]{0,60})`)

// IsEvalStructuredEvidence keeps fields, headers, rows and ordered steps in
// their original unit; it does not extract or generate a response object.
func IsEvalStructuredEvidence(text string) bool {
	body := strings.TrimSpace(text)
	if body == "" {
		return false
	}
	if (body[0] == '{' || body[0] == '[') && json.Valid([]byte(body)) {
		return true
	}
	if strings.HasPrefix(body, "```") && strings.Count(body, "```") >= 2 {
		return true
	}
	list, table, tsv := 0, 0, 0
	separator := false
	for _, line := range strings.Split(body, "\n") {
		if evalListLine.MatchString(line) {
			list++
		}
		if strings.Contains(line, "|") {
			table++
			separator = separator || evalTableSeparator.MatchString(line)
		}
		if strings.Contains(line, "\t") {
			tsv++
		}
	}
	return list >= 2 || table >= 2 && separator || tsv >= 2
}

func evalScopeCN(text string) string {
	m := evalProjectCN.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	value := m[1]
	for _, suffix := range []string{"的", "中", "里", "采用", "使用"} {
		if i := strings.Index(value, suffix); i > 0 {
			value = value[:i]
		}
	}
	return strings.ToLower(value)
}

func EvalInstructionScope(statement string) string {
	_, body := evalSpeaker(statement)
	if m := evalScopedInstruction.FindStringSubmatch(body); len(m) > 1 {
		return strings.ToLower(m[1])
	}
	if strings.HasPrefix(body, "针对项目") || strings.HasPrefix(body, "对于项目") || strings.HasPrefix(body, "在项目") || strings.HasPrefix(body, "仅适用于项目") {
		return evalScopeCN(body)
	}
	return ""
}

// EvalEvidenceApplicable only applies explicit instruction scope; ordinary
// facts mentioning two projects remain available for cross-project reasoning.
func EvalEvidenceApplicable(statement, query string) bool {
	if EvalInstructionSlot(statement) == "" {
		kind := ClassifyEvalFactKind(statement)
		if kind != "constraint" && kind != "agent_instruction" {
			return true
		}
	}
	scope := EvalInstructionScope(statement)
	if scope == "" {
		return true
	}
	requested := ""
	if m := evalProjectScope.FindStringSubmatch(query); len(m) > 1 {
		requested = strings.ToLower(m[1])
	} else {
		requested = evalScopeCN(query)
	}
	return requested == "" || requested == scope
}

func EvalInstructionSlot(statement string) string {
	role, body := evalSpeaker(statement)
	if role != "user" && role != "system" && role != "developer" {
		return ""
	}
	low := strings.ToLower(strings.TrimSpace(body))
	if low == "" || strings.ContainsRune("\"'“「『>`", []rune(low)[0]) || IsEvalStructuredEvidence(body) || IsHypotheticalStatement(body) {
		return ""
	}
	if evalQueryContainsAny(low, []string{"quoted", "example says", "the document says", "引文", "文档写着"}) {
		return ""
	}
	request := evalQueryContainsAny(low, []string{"always", "please", "must", "from now on", "keep", "output format", "response format", "回答", "回复", "请用", "格式", "始终"})
	response := evalQueryContainsAny(low, []string{"answer", "respond", "reply", "replies", "return", "output", "format", "回答", "回复", "格式"})
	if !request || !response {
		return ""
	}
	if evalQueryContainsAny(low, []string{"json", "xml", "yaml", "csv", "markdown", "plain text", "bullet points", "表格", "列表"}) {
		return "instruction:format"
	}
	if evalQueryContainsAny(low, []string{"brief", "concise", "words", "sentences", "简短", "字数", "句话", "不超过"}) {
		return "instruction:verbosity"
	}
	return ""
}

func EvalHealthSlot(statement string) string {
	_, body := evalSpeaker(statement)
	if m := evalAllergyAttribute.FindStringSubmatch(body); len(m) > 1 {
		object := strings.TrimSpace(strings.ToLower(m[1]))
		for _, suffix := range []string{" anymore", " now"} {
			object = strings.TrimSuffix(object, suffix)
		}
		return "health:allergy:" + object
	}
	return ""
}

func EvalBackgroundFacets(query string) []string {
	q := strings.ToLower(query)
	personal := evalQueryContainsAny(q, []string{"i", "my", "me", "suggest", "recommend", "我", "建议", "推荐"})
	if !personal {
		return nil
	}
	if evalQueryContainsAny(q, []string{"my friend", "my mother", "my daughter", "my son", "我的朋友", "我朋友", "我的妈妈", "我的女儿", "我的儿子"}) {
		return nil
	}
	var facets []string
	if evalQueryContainsAny(q, []string{"eat", "food", "meal", "meals", "snack", "snacks", "dinner", "lunch", "breakfast", "diet", "餐", "饮食", "零食", "吃"}) {
		facets = append(facets, "diet")
	}
	if evalQueryContainsAny(q, []string{"cope", "coping", "comfort", "unwind", "stress", "stressed", "support", "sad", "overwhelmed", "压力", "安慰", "难过", "缓解", "关怀"}) {
		facets = append(facets, "care")
	}
	return facets
}

func evalBackgroundWords(facet string) []string {
	if facet == "diet" {
		return []string{"allergic", "allergy", "allergies", "intolerant", "intolerance", "celiac", "diabetes", "vegetarian", "vegan", "dietary", "过敏", "不耐受", "糖尿病", "素食", "忌口"}
	}
	return []string{"anxiety", "depression", "divorce", "grief", "bereavement", "burnout", "overworked", "stressed", "焦虑", "抑郁", "离婚", "丧亲", "过劳", "失眠", "压力"}
}

func EvalBackgroundHints(query string) []string {
	var hints []string
	for _, facet := range EvalBackgroundFacets(query) {
		hints = append(hints, evalBackgroundWords(facet)...)
	}
	return hints
}

func EvalPersonalBackgroundRelevant(statement, query string) bool {
	if EvalMemorySubject(statement) != "self" {
		return false
	}
	for _, facet := range EvalBackgroundFacets(query) {
		if evalQueryContainsAny(strings.ToLower(statement), evalBackgroundWords(facet)) {
			return true
		}
	}
	return false
}

func EvalPersonalBackground(statement string) bool {
	return EvalMemorySubject(statement) == "self" &&
		(evalQueryContainsAny(strings.ToLower(statement), evalBackgroundWords("diet")) ||
			evalQueryContainsAny(strings.ToLower(statement), evalBackgroundWords("care")))
}

var evalEntityName = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_-]{2,31}\b`)
var evalEntityCN = regexp.MustCompile(`(?:小[\p{Han}]{1,2}|[王李张刘陈杨赵黄周吴][\p{Han}]{1,2})(?:把|将|给|向|从|在|转交|交给|负责|管理)`)
var evalEntityAddressCN = regexp.MustCompile(`(?:老|小|阿)[王李张刘陈杨赵黄周吴徐孙胡朱高林何郭马罗梁宋郑谢韩唐冯于董程曹袁邓许傅沈曾彭吕苏卢蒋蔡贾丁魏薛叶余潘杜戴夏钟汪田任姜范方石姚谭廖邹熊金陆郝孔白崔康毛邱秦江史顾侯邵孟龙万段雷钱汤尹黎易常武乔贺赖龚文]`)

// EvalEvidenceEntities discovers literal link candidates, never inferred
// relations. Search must still enforce scope and return original support rows.
func EvalEvidenceEntities(statement string) []string {
	_, body := evalSpeaker(statement)
	var out []string
	seen := map[string]bool{}
	for _, name := range evalEntityName.FindAllString(body, 32) {
		low := strings.ToLower(name)
		if evalQueryContainsAny(low, []string{"the", "this", "that", "rule", "rules", "project", "procedure", "format", "json", "xml", "yaml", "user", "assistant", "today", "yesterday", "tomorrow", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday", "january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december"}) {
			continue
		}
		if !seen[low] {
			out = append(out, name)
			seen[low] = true
		}
		if len(out) >= 8 {
			return out
		}
	}
	for _, match := range evalEntityCN.FindAllString(body, 8) {
		name := match
		for _, verb := range []string{"转交", "交给", "负责", "管理", "把", "将", "给", "向", "从", "在"} {
			name = strings.TrimSuffix(name, verb)
		}
		if name != "" && !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	for _, name := range evalEntityAddressCN.FindAllString(body, 8) {
		if !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	return out
}
