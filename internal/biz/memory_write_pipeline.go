package biz

import "strings"

// FactWriteOperation is the operation-semantics verdict for one candidate
// fact passing through the unified write pipeline (P1-3, Mem0-style
// ADD/UPDATE/DELETE/NOOP).
type FactWriteOperation string

const (
	// FactWriteOpAdd inserts a brand-new fact row.
	FactWriteOpAdd FactWriteOperation = "add"
	// FactWriteOpUpdate supersedes an existing fact: the old row is
	// invalidated (valid_until=now) and the new row is inserted atomically.
	FactWriteOpUpdate FactWriteOperation = "update"
	// FactWriteOpDelete invalidates an existing fact without a replacement
	// (never a physical delete — bi-temporal history is preserved).
	FactWriteOpDelete FactWriteOperation = "delete"
	// FactWriteOpNoop writes nothing. Two sub-cases: LLM-declared noop
	// (nothing worth storing) and dedup-merge (≥0.92 same-kind neighbor
	// already carries the fact — only its access counter is bumped).
	FactWriteOpNoop FactWriteOperation = "noop"
)

// Drop reasons recorded in FactWriteDecision.DropReason when a candidate is
// filtered out by the noise-reduction gates (P1-3 ③).
const (
	FactWriteDropEmptyStatement = "empty_statement"
	FactWriteDropKindWhitelist  = "kind_whitelist"
	FactWriteDropConfidence     = "confidence"
	// FactWriteDropAbsenceMeta drops conversation meta-observations such as
	// "用户询问 X，但暂无此信息" — not durable facts. Persisted, they outrank
	// the true fact on recency and seed a recursive "not found" pollution
	// loop (2026-08-26 domain-B regression). Genuine negative facts
	// ("原号码已作废") carry no inquiry marker and pass.
	FactWriteDropAbsenceMeta = "absence_meta_statement"
)

const (
	// FactWriteMinConfidence is gate ②: candidates below this confidence are
	// dropped (Mem0 #4573 lesson — without a confidence floor garbage
	// accumulates). The heuristic regex extractor is assigned exactly this
	// value so its high-precision matches survive the gate.
	FactWriteMinConfidence = 0.6
	// FactWriteMergeScore is gate ③: a same-kind neighbor at or above this
	// cosine similarity already carries the fact — merge (access count +1)
	// instead of inserting a duplicate. Matches the pre-existing conflict
	// supersede threshold (0.92).
	FactWriteMergeScore = memoryConflictSupersedeScore
	// FactWriteContestedScore is the lower bound of the ambiguous band:
	// neighbors in [0.80, 0.92) — or ≥0.92 of a different kind — make a
	// candidate "contested" and eligible for LLM adjudication.
	FactWriteContestedScore = memoryConflictMarkScore
)

// factKindWhitelist is gate ①: only durable, governance-worthy kinds enter
// the semantic store. Ephemeral event/knowledge/fact rows are dropped —
// "what the user did" is carried by L2 episodes, not L3 facts.
var factKindWhitelist = map[string]struct{}{
	"preference":   {},
	"profile":      {},
	"goal":         {},
	"constraint":   {},
	"decision":     {},
	"relationship": {},
}

// IsFactKindWhitelisted reports whether a fact kind passes gate ①.
func IsFactKindWhitelisted(kind string) bool {
	_, ok := factKindWhitelist[strings.TrimSpace(kind)]
	return ok
}

// FactKindForSubjectType maps the extractor subject_type vocabulary (V3
// schema: person|preference|constraint|goal|decision|relationship|other) to
// the storage fact_kind column. Durable kinds map to their whitelisted
// counterpart; unknown/ephemeral values fall back to "fact" — which gate ①
// then drops ("what the user did" belongs to L2 episodes, not L3 facts).
func FactKindForSubjectType(subjectType string) string {
	switch strings.TrimSpace(subjectType) {
	case "person":
		return "profile"
	case "preference":
		return "preference"
	case "constraint":
		return "constraint"
	case "goal":
		return "goal"
	case "decision":
		return "decision"
	case "relationship":
		return "relationship"
	case "event":
		return "event"
	case "concept":
		return "knowledge"
	default:
		return "fact"
	}
}

// FactWriteCandidate is the unified pipeline input. All automatic write
// sources (auto_memory worker, sleep-time episode consolidator) normalize
// their extracted proposals into this shape before entering the pipeline.
type FactWriteCandidate struct {
	Statement  string
	FactKind   string
	Confidence float64
	Importance float64
	TagsJSON   string

	ScopeType string
	ScopeID   string
	UserID    string
	AgentID   string

	SourceKind      string
	SourceEpisodeID string
	SourceSessionID string
	SourceMessageID string
}

// FactWriteDecision records the pipeline outcome for one candidate: the
// operation to execute, the target fact for update/delete/merge, and — when
// dropped by a gate — the drop reason for audit.
type FactWriteDecision struct {
	Candidate    FactWriteCandidate
	Operation    FactWriteOperation
	TargetFactID string
	DropReason   string
	// Contested marks candidates with conflict-band neighbors (R3 gate input).
	Contested bool
	// Adjudicated marks decisions taken as-is from an explicit LLM verdict
	// (add / noop / valid-target update|delete). Heuristic fallbacks —
	// adjudicator error, missing verdict, unknown op, hallucinated target —
	// leave this false so the R3 gate pends them as CONTESTED.
	Adjudicated bool
	// DecisionReason explains the verdict origin for pending records/audit
	// (e.g. adjudicator_error, verdict_missing, target_not_neighbor).
	DecisionReason string
}

// GateFactWriteCandidate applies gates ① and ② (pure). A passed candidate
// returns a decision with empty DropReason; a dropped candidate returns the
// gate's drop reason and no operation.
func GateFactWriteCandidate(c FactWriteCandidate) FactWriteDecision {
	d := FactWriteDecision{Candidate: c}
	if strings.TrimSpace(c.Statement) == "" {
		d.DropReason = FactWriteDropEmptyStatement
		return d
	}
	// Absence meta-statements ("用户询问 X，但暂无此信息") are conversation
	// meta-observations, not durable facts: persisted they outrank the true
	// fact on recency and seed a recursive "not found" pollution loop
	// (2026-08-26 domain-B regression). The immediate-fact writer already
	// drops them at its own gate; this covers the auto_memory / sleep-time
	// consolidation paths so the loop cannot re-enter through them.
	if LooksLikeAbsenceMetaStatement(c.Statement) {
		d.DropReason = FactWriteDropAbsenceMeta
		return d
	}
	if !IsFactKindWhitelisted(c.FactKind) {
		d.DropReason = FactWriteDropKindWhitelist
		return d
	}
	if c.Confidence < FactWriteMinConfidence {
		d.DropReason = FactWriteDropConfidence
		return d
	}
	return d
}

// DecideFactWriteHeuristic arbitrates a (gate-passed) candidate against its
// scored neighbors without an LLM call (pure):
//
//   - same-kind neighbor ≥ 0.92 → noop-merge (highest score wins)
//   - everything else          → add
//
// Contested candidates (ambiguous band / cross-kind high similarity) fall
// back to add in heuristic mode — only the LLM adjudicator may emit
// update/delete.
func DecideFactWriteHeuristic(c FactWriteCandidate, neighbors []MemoryConflictNeighbor) FactWriteDecision {
	d := FactWriteDecision{Candidate: c, Operation: FactWriteOpAdd}
	var merge *MemoryConflictNeighbor
	for i := range neighbors {
		n := neighbors[i]
		if n.FactID == "" || n.FactKind != c.FactKind || n.Score < FactWriteMergeScore {
			continue
		}
		if merge == nil || n.Score > merge.Score {
			cp := n
			merge = &cp
		}
	}
	if merge != nil {
		d.Operation = FactWriteOpNoop
		d.TargetFactID = merge.FactID
	}
	return d
}

// FactWriteIsContested reports whether a candidate's neighbors put it in the
// ambiguous band eligible for LLM adjudication (pure):
//
//   - any neighbor in [0.80, 0.92)               → contested
//   - any ≥0.92 neighbor of a DIFFERENT kind      → contested
//   - ≥0.92 same-kind only (auto-merge) or none   → not contested
func FactWriteIsContested(factKind string, neighbors []MemoryConflictNeighbor) bool {
	for _, n := range neighbors {
		if n.FactID == "" {
			continue
		}
		if n.Score >= FactWriteMergeScore && n.FactKind != factKind {
			return true
		}
		if n.Score >= FactWriteContestedScore && n.Score < FactWriteMergeScore {
			return true
		}
	}
	return false
}
