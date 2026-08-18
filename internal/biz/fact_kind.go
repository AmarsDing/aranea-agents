package biz

import "strings"

// CanonicalizeFactKind maps extractor / tool kinds onto the remember-tool
// vocabulary so identity facts are not split across preference / profile /
// user_identity / domain_knowledge. Statement heuristics override a wrong
// LLM label (e.g. 工号 tagged as preference).
func CanonicalizeFactKind(kind, statement string) string {
	if LooksLikeUserIdentityStatement(statement) {
		return "user_identity"
	}
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

// LooksLikeUserIdentityStatement reports stable user-identity statements
// (employee id, name, role) that must not be stored as preference.
func LooksLikeUserIdentityStatement(statement string) bool {
	s := strings.ToLower(strings.TrimSpace(statement))
	if s == "" {
		return false
	}
	for _, p := range []string{"工号", "我叫", "负责", "employee id", "employee number", "my name is"} {
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
	case "user_identity", "preference", "user_preference", "constraint":
		return true
	}
	return false
}
