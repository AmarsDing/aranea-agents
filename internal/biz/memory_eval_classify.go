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

// EvalMaxStatementAtoms bounds the per-message atom split so one long dump
// cannot fan out into unbounded fact rows.
const EvalMaxStatementAtoms = 8

// evalAtomBreakers are the rune-level sentence delimiters for atom split:
// CJK full stops/exclamations/questions/semicolons, ASCII !?;, and newlines.
const evalAtomBreakers = "。！？!?；;\n"

// SplitEvalStatements deterministically splits one eval message into atomic
// statements (AMC C3-A Q4). Delimiters are CJK 。！？；, ASCII !?;, newline,
// and an English ". " followed by an uppercase letter. Trailing delimiters
// stay attached to their atom. Fragments shorter than 4 runes (after
// trimming) are dropped; when the split would exceed maxAtoms the overflow
// is merged back into the last atom. A message without any delimiter is
// returned verbatim as exactly one atom (zero behavior change).
func SplitEvalStatements(content string, maxAtoms int) []string {
	if maxAtoms <= 0 {
		maxAtoms = EvalMaxStatementAtoms
	}
	if strings.TrimSpace(content) == "" {
		return []string{content}
	}
	runes := []rune(content)
	var segs []string
	start := 0
	pos := 0
	for i, r := range runes {
		w := len(string(r))
		end := pos + w
		isBreak := strings.ContainsRune(evalAtomBreakers, r)
		if !isBreak && r == '.' {
			// English ". " followed by an uppercase letter.
			rest := runes[i+1:]
			if len(rest) >= 2 && rest[0] == ' ' && rest[1] >= 'A' && rest[1] <= 'Z' {
				isBreak = true
				end++ // swallow the separating space into this segment
			}
		}
		if isBreak {
			segs = append(segs, content[start:end])
			start = end
		}
		pos = end
	}
	if start < len(content) {
		segs = append(segs, content[start:])
	}
	if len(segs) <= 1 {
		return []string{content}
	}
	atoms := make([]string, 0, len(segs))
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if len([]rune(seg)) < 4 {
			continue
		}
		atoms = append(atoms, seg)
	}
	if len(atoms) == 0 {
		return []string{strings.TrimSpace(content)}
	}
	if len(atoms) > maxAtoms {
		tail := strings.Join(atoms[maxAtoms-1:], "")
		atoms = append(atoms[:maxAtoms-1:maxAtoms-1], tail)
	}
	return atoms
}
