package biz

import "strings"

// ClassifyEvalFactKind is the eval-adapter view of CanonicalizeFactKind.
// Raw dumped messages without an extractor still get a durable kind when
// the statement is a preference, identity, or constraint; everything else
// stays event so it does not participate in evergreen scoring.
func ClassifyEvalFactKind(statement string) string {
	role, body := evalSpeaker(statement)
	if role == "system" || role == "developer" {
		return "agent_instruction"
	}
	// Commit-shaped evidence keeps its own durable kinds. The structural
	// prefix beats text cues below it (planned/historical/slot heuristics)
	// and is non-governable, so supersession semantics match "event".
	if k := EvalCaseFactKind(body); k != "" {
		return k
	}
	if IsEvalPlannedStatement(body) || IsEvalHistoricalStatement(body) {
		return "event"
	}
	if EvalInstructionSlot(statement) != "" {
		return "constraint"
	}
	if EvalHealthSlot(statement) != "" {
		return "user_identity"
	}
	if LooksLikeConstraintStatement(body) {
		return "constraint"
	}
	switch EvalFactSlot(statement) {
	case "live", "job", "phone", "employee_id", "email", "timezone", "language":
		return "user_identity"
	}
	k := CanonicalizeFactKind("general", stripRolePrefix(statement))
	if k == "general" {
		return "event"
	}
	return k
}

// EvalFactSlot keeps identity attributes separate and supports multiword favorites.
func EvalFactSlot(statement string) string {
	_, s := evalSpeaker(statement)
	if IsEvalStructuredEvidence(s) {
		return ""
	}
	low := strings.ToLower(s)
	if slot := EvalInstructionSlot(statement); slot != "" {
		return slot
	}
	if slot := EvalHealthSlot(statement); slot != "" {
		return slot
	}
	if m := evalFavoriteAttribute.FindStringSubmatch(s); len(m) > 1 {
		return "favorite:" + strings.Join(strings.Fields(strings.ToLower(m[1])), " ")
	}
	for _, slot := range []struct {
		key  string
		cues []string
	}{
		{"employee_id", []string{"工号", "employee id", "employee number"}},
		{"phone", []string{"手机号", "电话号码", "phone number", "mobile number"}},
		{"email", []string{"邮箱是", "邮箱改", "email is", "e-mail is"}},
		{"timezone", []string{"时区是", "时区改", "timezone is", "time zone is"}},
		{"language", []string{"首选语言是", "首选语言改", "preferred language is"}},
		{"job", []string{"换工作", "入职", "新工作", "work at", "works at", "work for", "works for", "new job", "joined "}},
	} {
		for _, cue := range slot.cues {
			if strings.Contains(low, cue) {
				return slot.key
			}
		}
	}
	if strings.Contains(s, "负责") {
		return ""
	}
	if evalCNLikeObject.MatchString(s) && !chineseFavoriteRe.MatchString(s) {
		return "like"
	}
	return PreferenceSlotKey(s)
}

// HasEvalUpdateCue is the eval-adapter alias for HasPreferenceUpdateCue.
func HasEvalUpdateCue(statement string) bool {
	return HasPreferenceUpdateCue(statement)
}

// ShouldSupersedeEvalFact requires matching speaker, subject and attribute.
func ShouldSupersedeEvalFact(oldKind, oldStmt, newKind, newStmt string) bool {
	if !governableMemoryKind(oldKind) || !governableMemoryKind(newKind) ||
		NormalizeForDedup(oldStmt) == NormalizeForDedup(newStmt) || !evalSameSubject(oldStmt, newStmt) {
		return false
	}
	_, body := evalSpeaker(newStmt)
	if IsHypotheticalStatement(body) || IsEvalPlannedStatement(body) || IsEvalHistoricalStatement(newStmt) {
		return false
	}
	oldSlot, newSlot := EvalFactSlot(oldStmt), EvalFactSlot(newStmt)
	if oldSlot == "" || oldSlot != newSlot {
		return false
	}
	if oldSlot == "like" {
		return evalExplicitPreferenceCorrection(oldStmt, newStmt)
	}
	return true
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

// evalTrailingAbbrev are abbreviation stems that end at "." and must NOT be
// treated as an English sentence break even when followed by ". "+uppercase
// (AMC C3 修复 #5): "I met Dr. Lee yesterday." is one atom, not two.
// Honorifics and common abbreviations keep the sentence intact; the merge
// direction (fewer atoms) is recall-safe.
var evalTrailingAbbrev = map[string]bool{
	"mr": true, "mrs": true, "ms": true, "dr": true, "prof": true,
	"st": true, "sr": true, "jr": true, "misc": true, "vs": true,
	"etc": true, "approx": true, "no": true, "fig": true, "dept": true,
	"inc": true, "ltd": true, "co": true, "rev": true,
}

func isASCIILetterRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// evalEndsWithAbbrev reports whether the rune sequence ends with a known
// abbreviation stem (ASCII letters immediately before a trailing dot).
func evalEndsWithAbbrev(prev []rune) bool {
	i := len(prev) - 1
	for i >= 0 && isASCIILetterRune(prev[i]) {
		i--
	}
	word := strings.ToLower(string(prev[i+1:]))
	return evalTrailingAbbrev[word]
}

// SplitEvalStatements deterministically splits one eval message into atomic
// statements (AMC C3-A Q4). Delimiters are CJK 。！？；, ASCII !?;, newline,
// and an English ". " followed by an uppercase letter (skipped after known
// abbreviation stems, 修复 #5). Trailing delimiters stay attached to their
// atom. Nonempty short fragments are retained as possible labels/examples; when the
// split would exceed maxAtoms the overflow is merged back into the last atom.
// A message without any delimiter is returned verbatim as exactly one atom
// (zero behavior change).
func SplitEvalStatements(content string, maxAtoms int) []string {
	if maxAtoms <= 0 {
		maxAtoms = EvalMaxStatementAtoms
	}
	if strings.TrimSpace(content) == "" {
		return []string{content}
	}
	if IsEvalStructuredEvidence(content) {
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
				if !evalEndsWithAbbrev(runes[:i]) {
					isBreak = true
				}
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
		if seg == "" {
			continue
		}
		atoms = append(atoms, seg)
	}
	if len(atoms) == 0 {
		return []string{strings.TrimSpace(content)}
	}
	if len(atoms) > maxAtoms {
		tail := strings.Join(atoms[maxAtoms-1:], "\n")
		atoms = append(atoms[:maxAtoms-1:maxAtoms-1], tail)
	}
	return atoms
}
