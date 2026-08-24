package biz

import "strings"

// ClassifyEvalFactKind is the eval-adapter view of CanonicalizeFactKind.
// Raw dumped messages without an extractor still get a durable kind when
// the statement is a preference, identity, or constraint; everything else
// stays event so it does not participate in evergreen scoring.
func ClassifyEvalFactKind(statement string) string {
	k := CanonicalizeFactKind("general", stripRolePrefix(statement))
	if k == "general" {
		return "event"
	}
	return k
}

// EvalFactSlot is the eval-adapter alias for PreferenceSlotKey.
func EvalFactSlot(statement string) string {
	return PreferenceSlotKey(stripRolePrefix(statement))
}

// HasEvalUpdateCue is the eval-adapter alias for HasPreferenceUpdateCue.
func HasEvalUpdateCue(statement string) bool {
	return HasPreferenceUpdateCue(statement)
}

// ShouldSupersedeEvalFact is the eval-adapter alias for
// ShouldSupersedeSameSlotFact.
func ShouldSupersedeEvalFact(oldKind, oldStmt, newKind, newStmt string) bool {
	return ShouldSupersedeSameSlotFact(oldKind, stripRolePrefix(oldStmt), newKind, stripRolePrefix(newStmt))
}

func stripRolePrefix(statement string) string {
	s := strings.TrimSpace(statement)
	if i := strings.Index(s, ": "); i >= 0 && i <= 16 {
		return strings.TrimSpace(s[i+2:])
	}
	return s
}
