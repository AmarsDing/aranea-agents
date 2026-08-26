package biz

import (
	"regexp"
	"strings"
)

var (
	likeStatementRe     = regexp.MustCompile(`(?i)\b(?:i\s+)?(?:like|love|prefer|hate|enjoy|dislike)s?\s+`)
	favoriteStatementRe = regexp.MustCompile(`(?i)\b(?:my\s+)?favou?rite\s+\w+\s+(?:is|was|are)\s+`)
	liveStatementRe     = regexp.MustCompile(`(?i)\b(?:i\s+)?(?:live|lives|lived)\s+(?:in|at)\s+`)
	nameStatementRe     = regexp.MustCompile(`(?i)\b(?:my name is|i am called)\s+`)
	chineseFavoriteRe   = regexp.MustCompile(`最喜欢的([^是为，。,\s]{1,12})(?:是|为)`)
)

// CanonicalizeFactKind maps extractor / tool kinds onto the remember-tool
// vocabulary so identity facts are not split across preference / profile /
// user_identity / domain_knowledge. Statement heuristics override a wrong
// LLM label (e.g. 工号 tagged as preference) and fill vague kinds
// (general/fact/event) from the statement itself.
func CanonicalizeFactKind(kind, statement string) string {
	if LooksLikeUserIdentityStatement(statement) {
		return "user_identity"
	}
	mapped := mapFactKindToken(kind)
	if vagueFactKind(mapped) {
		if LooksLikePreferenceStatement(statement) {
			return "preference"
		}
		if LooksLikeConstraintStatement(statement) {
			return "constraint"
		}
	}
	return mapped
}

func mapFactKindToken(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "identity", "user_identity":
		return "user_identity"
	case "preference", "user_preference", "profile":
		return "preference"
	case "constraint":
		return "constraint"
	case "instruction", "agent_instruction":
		return "agent_instruction"
	case "domain_knowledge":
		return "domain_knowledge"
	default:
		k := strings.ToLower(strings.TrimSpace(kind))
		if k == "" {
			return "general"
		}
		return k
	}
}

func vagueFactKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "general", "fact", "event", "knowledge":
		return true
	default:
		return false
	}
}

// LooksLikeUserIdentityStatement reports stable user-identity statements
// (employee id, name, role) that must not be stored as preference.
func LooksLikeUserIdentityStatement(statement string) bool {
	s := strings.ToLower(strings.TrimSpace(statement))
	if s == "" {
		return false
	}
	for _, p := range []string{"工号", "我叫", "负责", "employee id", "employee number", "my name is", "i am called"} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return nameStatementRe.MatchString(s)
}

// LooksLikeAbsenceMetaStatement reports statements whose content is "the user
// asked X but the system has no record of it" — a conversation meta-observation,
// not a durable fact. Persisting them poisons recall: the absence statement
// outranks the true fact on recency, the model parrots "not found", and the
// reply is saved as yet another absence statement (2026-08-26 domain-B
// regression pollution loop). Genuine negative facts ("原号码已作废") do not
// carry the inquiry marker and are unaffected.
func LooksLikeAbsenceMetaStatement(statement string) bool {
	s := strings.ToLower(strings.TrimSpace(statement))
	if s == "" {
		return false
	}
	inquiry := false
	for _, p := range []string{"用户询问", "用户问", "user asked", "user asks", "user inquired", "the user asked"} {
		if strings.Contains(s, p) {
			inquiry = true
			break
		}
	}
	if !inquiry {
		return false
	}
	for _, p := range []string{
		"暂无", "尚无", "没有相关记录", "无相关记录", "没有记录", "不在记忆", "未找到", "未存储", "没有保存",
		"需要用户提供", "待用户补充", "需要用户补充",
		"no record", "not in memory", "not on file", "don't have", "do not have", "no information", "not stored",
	} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// LooksLikePreferenceStatement reports durable likes, favorites, and residence.
func LooksLikePreferenceStatement(statement string) bool {
	s := strings.TrimSpace(statement)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "喜欢") || strings.Contains(low, "偏好") ||
		strings.Contains(low, "prefer") || strings.Contains(low, "favorite") ||
		strings.Contains(low, "favourite") {
		return true
	}
	return likeStatementRe.MatchString(s) || favoriteStatementRe.MatchString(s) || liveStatementRe.MatchString(s)
}

// LooksLikeConstraintStatement reports explicit rules. "always" alone is not
// a constraint ("I always drink coffee" is a preference/habit).
func LooksLikeConstraintStatement(statement string) bool {
	s := strings.ToLower(strings.TrimSpace(statement))
	if s == "" {
		return false
	}
	for _, p := range []string{"必须", "禁止", "不要", "must ", "never ", "do not ", "don't ", "required", "rule:"} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// UserScopedFactKind reports whether the kind belongs on the user recall
// scope (cross-session), aligned with memory_remember.
func UserScopedFactKind(factKind string) bool {
	switch CanonicalizeFactKind(factKind, "") {
	case "user_identity", "preference", "constraint":
		return true
	}
	return false
}

// PreferenceSlotKey is a coarse attribute key used to detect same-slot
// updates (favorite color, residence, name). Empty means the statement is
// not a governable slot.
func PreferenceSlotKey(statement string) string {
	s := strings.TrimSpace(statement)
	if m := favoriteStatementRe.FindStringSubmatch(s); len(m) > 0 {
		// Re-parse the attribute word: `favorite <attr> is`.
		low := strings.ToLower(s)
		idx := strings.Index(low, "favorite")
		if idx < 0 {
			idx = strings.Index(low, "favourite")
		}
		if idx >= 0 {
			rest := strings.TrimSpace(s[idx:])
			fields := strings.Fields(rest)
			if len(fields) >= 2 {
				return "favorite:" + strings.ToLower(fields[1])
			}
		}
		return "favorite"
	}
	if liveStatementRe.MatchString(s) {
		return "live"
	}
	if LooksLikeUserIdentityStatement(s) || nameStatementRe.MatchString(s) {
		return "name"
	}
	if m := chineseFavoriteRe.FindStringSubmatch(s); len(m) > 1 {
		return "favorite:" + strings.ToLower(strings.TrimSpace(m[1]))
	}
	if strings.Contains(s, "住在") || strings.Contains(s, "居住在") {
		return "live"
	}
	if likeStatementRe.MatchString(s) {
		return "like"
	}
	return ""
}

// HasPreferenceUpdateCue reports an explicit correction, not an additional
// preference (coffee and tea can coexist unless the user says "now/instead").
func HasPreferenceUpdateCue(statement string) bool {
	low := strings.ToLower(statement)
	for _, n := range []string{" now", "now ", "instead", "no longer", "not anymore", "changed", "updated", "used to", "from now", "anymore", "现在", "改成", "换成", "不再"} {
		if strings.Contains(low, n) {
			return true
		}
	}
	return false
}

// ShouldSupersedeSameSlotFact reports whether an existing governable fact
// should be invalidated before writing the new one. Same-slot favorite /
// live / name updates always supersede; generic "like" updates require an
// update cue so "I like coffee" and "I like tea" can coexist.
func ShouldSupersedeSameSlotFact(oldKind, oldStmt, newKind, newStmt string) bool {
	if !governableMemoryKind(oldKind) || !governableMemoryKind(newKind) {
		return false
	}
	if NormalizeForDedup(oldStmt) == NormalizeForDedup(newStmt) {
		return false
	}
	oldSlot, newSlot := PreferenceSlotKey(oldStmt), PreferenceSlotKey(newStmt)
	if oldSlot == "" || oldSlot != newSlot {
		return false
	}
	if oldSlot == "like" {
		return HasPreferenceUpdateCue(newStmt)
	}
	return true
}

func governableMemoryKind(kind string) bool {
	switch CanonicalizeFactKind(kind, "") {
	case "user_identity", "preference", "constraint", "agent_instruction":
		return true
	default:
		return false
	}
}
