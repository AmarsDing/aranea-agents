package biz

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
)

const (
	EvalControlForget   = "forget"
	EvalControlSuppress = "suppress"
	EvalControlAllow    = "allow"
)

// EvalMemorySelector contains no plaintext target or person name. It is an
// internal retrieval-suppression selector, not a claim of physical erasure.
type EvalMemorySelector struct {
	All         bool   `json:"all,omitempty"`
	Category    string `json:"category,omitempty"`
	SubjectHash string `json:"subject_hash,omitempty"`
	TargetHash  string `json:"target_hash,omitempty"`
	TargetWords int    `json:"target_words,omitempty"`
	TargetRunes int    `json:"target_runes,omitempty"`
}

type EvalMemoryDirective struct {
	Mode     string
	Selector EvalMemorySelector
}

var evalControlQuotedTarget = regexp.MustCompile(`["“「『]([^"”」』]{1,160})["”」』]`)
var evalControlPossessive = regexp.MustCompile(`(?i)^([a-z][a-z -]{0,40})['’]s\s+`)
var evalControlFavorite = regexp.MustCompile(`(?i)\bfavou?rite\s+([a-z][a-z -]{0,40})$`)

// ParseEvalMemoryDirective accepts a direct user imperative. Callers must only
// enable it for messages whose first statement is a directive; a pasted email
// or document with a later imperative is evidence, not a control channel.
func ParseEvalMemoryDirective(role, text string) (EvalMemoryDirective, bool) {
	if !strings.EqualFold(strings.TrimSpace(role), "user") {
		return EvalMemoryDirective{}, false
	}
	body := strings.TrimSpace(text)
	if body == "" || strings.ContainsAny(body, "\n\r?？") || strings.ContainsRune("\"'“「『>`", []rune(body)[0]) {
		return EvalMemoryDirective{}, false
	}
	low := strings.ToLower(body)
	mode, rest := "", ""
	for _, rule := range []struct {
		mode     string
		prefixes []string
	}{
		{EvalControlSuppress, []string{"please do not remember ", "please do not store ", "do not remember ", "don't remember ", "do not store ", "don't store ", "stop remembering ", "stop storing ", "never remember ", "请不要再记住", "请不要记住", "请不要保存", "不要再记住", "不要记住", "不要再保存", "不要保存", "别记住"}},
		{EvalControlAllow, []string{"you may remember ", "you can remember ", "please remember ", "可以重新记住", "恢复记忆", "重新记住"}},
		{EvalControlForget, []string{"please forget ", "forget ", "please delete ", "delete ", "remove from memory ", "请忘记", "忘记", "请删除", "删除"}},
	} {
		for _, prefix := range rule.prefixes {
			if strings.HasPrefix(low, prefix) {
				mode, rest = rule.mode, strings.TrimSpace(body[len(prefix):])
				break
			}
		}
		if mode != "" {
			break
		}
	}
	if mode == "" {
		return EvalMemoryDirective{}, false
	}
	if mode == EvalControlAllow && !strings.Contains(low, "again") && !strings.Contains(body, "重新") && !strings.Contains(body, "恢复") {
		return EvalMemoryDirective{}, false
	}
	for _, suffix := range []string{" anymore", " again", " from now on", "从现在起", "的记忆", "这件事", "这条信息", "。", ".", "！", "!"} {
		rest = strings.TrimSpace(strings.TrimSuffix(rest, suffix))
	}
	// Strip trailing punctuation before the English adverb as well.
	rest = strings.TrimRight(rest, "。.!！ ")
	for _, suffix := range []string{" anymore", " again", " from now on"} {
		rest = strings.TrimSpace(strings.TrimSuffix(rest, suffix))
	}
	sel, ok := evalControlSelector(rest)
	return EvalMemoryDirective{Mode: mode, Selector: sel}, ok
}

func evalControlSelector(target string) (EvalMemorySelector, bool) {
	low := strings.ToLower(target)
	switch low {
	case "everything you know about me", "all my memories", "all information about me", "all memories", "everything", "全部记忆", "所有记忆", "所有关于我的信息", "关于我的全部信息":
		return EvalMemorySelector{All: true}, true
	}
	category := evalControlAttributeTarget(low)
	subject := EvalMemorySubject("user: " + target)
	if subject == "" {
		if m := evalControlPossessive.FindStringSubmatch(target); len(m) > 1 {
			subject = strings.ToLower(strings.TrimSpace(m[1]))
		}
	}
	if subject == "" && strings.Contains(target, "的") {
		subject = strings.TrimSpace(target[:strings.LastIndex(target, "的")])
		if subject == "我" {
			subject = "self"
		}
	}
	if subject == "" && strings.HasPrefix(low, "my ") && !strings.HasPrefix(low, "my friend") {
		subject = "self"
	}
	sel := EvalMemorySelector{Category: category}
	if category != "" {
		if subject != "" {
			sel.SubjectHash = evalControlHash(subject)
		}
		return sel, true
	}
	if m := evalControlQuotedTarget.FindStringSubmatch(target); len(m) > 1 {
		target = m[1]
	} else {
		for _, prefix := range []string{"information about ", "details about ", "memories about ", "memory of ", "about ", "the ", "my ", "关于", "有关"} {
			target = strings.TrimSpace(strings.TrimPrefix(target, prefix))
		}
	}
	normal := evalControlNormalize(target)
	if normal == "" || len([]rune(normal)) > 160 {
		return EvalMemorySelector{}, false
	}
	sel.TargetHash = evalControlHash(normal)
	sel.TargetWords = len(strings.Fields(normal))
	for _, r := range normal {
		if unicode.Is(unicode.Han, r) {
			sel.TargetRunes = len([]rune(strings.ReplaceAll(normal, " ", "")))
			sel.TargetHash = evalControlHash(strings.ReplaceAll(normal, " ", ""))
			break
		}
	}
	return sel, true
}

// Attribute deletion requires an attribute noun phrase. A sentence such as
// "the phone call yesterday" is a literal target, never permission to delete
// every stored telephone number. Quoted targets always retain literal scope.
func evalControlAttributeTarget(target string) string {
	if evalControlQuotedTarget.MatchString(target) {
		return ""
	}
	attribute := target
	if m := evalControlPossessive.FindStringIndex(attribute); m != nil {
		attribute = attribute[m[1]:]
	}
	for _, prefix := range []string{"all ", "my ", "the ", "我的", "我"} {
		attribute = strings.TrimSpace(strings.TrimPrefix(attribute, prefix))
	}
	if i := strings.LastIndex(attribute, "的"); i > 0 {
		attribute = attribute[i+len("的"):]
	}
	if strings.HasPrefix(attribute, "favorite ") || strings.HasPrefix(attribute, "favourite ") || strings.HasPrefix(attribute, "最喜欢的") {
		return evalControlCategory(attribute)
	}
	for _, group := range []struct {
		category string
		phrases  []string
	}{
		{"phone", []string{"phone", "phone number", "phone numbers", "telephone", "telephone number", "mobile number", "电话", "电话号码", "手机号", "手机号码"}},
		{"email", []string{"email", "e-mail", "email address", "email addresses", "邮箱", "电子邮件"}},
		{"live", []string{"address", "residence", "home address", "住址", "地址"}},
		{"name", []string{"name", "名字", "姓名"}},
		{"employee_id", []string{"employee id", "employee number", "工号"}},
		{"job", []string{"job", "employer", "工作单位"}},
		{"timezone", []string{"timezone", "time zone", "时区"}},
		{"language", []string{"preferred language", "首选语言"}},
		{"health", []string{"health", "health information", "medical history", "medical records", "健康", "健康信息", "病史"}},
		{"preference", []string{"preferences", "preference", "偏好"}},
	} {
		for _, phrase := range group.phrases {
			if attribute == phrase {
				return group.category
			}
		}
	}
	return ""
}

func evalControlCategory(text string) string {
	if m := evalControlFavorite.FindStringSubmatch(text); len(m) > 1 {
		return "favorite:" + strings.TrimSpace(m[1])
	}
	if m := chineseFavoriteRe.FindStringSubmatch(text + "是"); len(m) > 1 {
		return "favorite:" + strings.TrimSpace(m[1])
	}
	for _, category := range []struct {
		name  string
		words []string
	}{
		{"email", []string{"email", "e-mail", "邮箱", "电子邮件"}},
		{"phone", []string{"phone", "telephone", "mobile", "电话", "手机号"}},
		{"live", []string{"address", "residence", "住址", "地址"}},
		{"employee_id", []string{"employee id", "employee number", "工号"}},
		{"name", []string{"name", "名字", "姓名"}},
		{"job", []string{"job", "employer", "工作单位"}},
		{"timezone", []string{"timezone", "time zone", "时区"}},
		{"language", []string{"preferred language", "首选语言"}},
		{"health", []string{"health", "medical", "健康", "病史"}},
		{"preference", []string{"preferences", "preference", "偏好"}},
	} {
		if evalQueryContainsAny(text, category.words) {
			return category.name
		}
	}
	return ""
}

// EvalMemorySubject preserves the owner of an assertion across speaker roles.
// An assistant's "I" is not the user's self; "your" explicitly refers back.
func EvalMemorySubject(statement string) string {
	role, body := evalSpeaker(statement)
	low := strings.ToLower(body)
	if role == "assistant" && (strings.HasPrefix(low, "your ") || strings.HasPrefix(body, "你的")) {
		return "self"
	}
	subject := evalSubject(statement)
	if subject == role+":self" && role != "user" {
		return role + ":self"
	}
	return strings.TrimPrefix(subject, role+":")
}

func (s EvalMemorySelector) Matches(statement string) bool {
	if s.All {
		return true
	}
	if s.Category != "" {
		if s.SubjectHash != "" && evalControlHash(EvalMemorySubject(statement)) != s.SubjectHash {
			return false
		}
		slot := EvalFactSlot(statement)
		if slot == s.Category {
			return true
		}
		if s.Category == "preference" {
			return ClassifyEvalFactKind(statement) == "preference" || strings.HasPrefix(slot, "favorite:") || slot == "like"
		}
		if s.Category == "health" {
			return evalQueryContainsAny(strings.ToLower(statement), []string{"allergic", "allergy", "diabetes", "asthma", "medication", "diagnosed", "过敏", "糖尿病", "哮喘", "病史", "服药"})
		}
		return evalControlCategory(strings.ToLower(statement)) == s.Category
	}
	_, body := evalSpeaker(statement)
	normal := evalControlNormalize(body)
	if s.TargetRunes > 0 {
		runes := []rune(strings.ReplaceAll(normal, " ", ""))
		for i := 0; i+s.TargetRunes <= len(runes); i++ {
			if evalControlHash(string(runes[i:i+s.TargetRunes])) == s.TargetHash {
				return true
			}
		}
		return false
	}
	words := strings.Fields(normal)
	if s.TargetWords <= 0 {
		return false
	}
	for i := 0; i+s.TargetWords <= len(words); i++ {
		if evalControlHash(strings.Join(words[i:i+s.TargetWords], " ")) == s.TargetHash {
			return true
		}
	}
	return false
}

func (s EvalMemorySelector) Key() string {
	b, _ := json.Marshal(s) // Only JSON primitives; encoding cannot fail.
	return evalControlHash(string(b))
}

func evalControlNormalize(text string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }), " ")
}

func evalControlHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}
