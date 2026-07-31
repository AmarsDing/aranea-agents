package biz

import (
	"context"
	"encoding/json"
	"strings"
)

// ConflictAction describes the governance action taken for a conflicting fact.
type ConflictAction string

const (
	// ConflictActionNone means no conflict governance is needed.
	ConflictActionNone ConflictAction = ""
	// ConflictActionSupersede means the old fact is replaced by the new one
	// (same kind, very high similarity): old fact is marked superseded.
	ConflictActionSupersede ConflictAction = "supersede"
	// ConflictActionMarkConflict means a potential conflict is recorded for
	// human review: old fact conflict_count is incremented and the new fact
	// metadata records the candidate.
	ConflictActionMarkConflict ConflictAction = "mark_conflict"
)

// Conflict arbitration thresholds.
const (
	// memoryConflictSupersedeScore: same-kind neighbor at or above this score
	// is treated as a newer statement of the same fact → auto supersede.
	memoryConflictSupersedeScore = 0.92
	// memoryConflictMarkScore: any neighbor at or above this score (but not
	// superseded) is recorded as a conflict candidate.
	memoryConflictMarkScore = 0.80
	// memoryConflictNeighborLimit caps neighbor recall for arbitration.
	memoryConflictNeighborLimit = 8
)

// MemoryConflictNeighbor is a scored candidate fact near a proposed memory.
type MemoryConflictNeighbor struct {
	FactID   string
	Score    float64
	FactKind string
}

// MemoryConflictDecision is the outcome of conflict arbitration for one
// proposed fact.
type MemoryConflictDecision struct {
	Action       ConflictAction
	TargetFactID string
	Score        float64
}

// IsConflictGovernableFactKind reports whether a fact kind participates in
// conflict governance. Only long-lived preference/constraint/profile facts
// are governed; event/knowledge/fact rows accumulate naturally.
func IsConflictGovernableFactKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "preference", "constraint", "profile":
		return true
	default:
		return false
	}
}

// DecideMemoryConflict arbitrates a proposed fact of the given kind against
// its scored neighbors. Pure function: no I/O, fully testable.
//
// Rules (evaluated per neighbor, empty FactID skipped):
//   - score ≥ 0.92 AND same kind → supersede (highest score wins)
//   - score ≥ 0.80 (any kind, not superseded above) → mark conflict
//   - otherwise → no action
//
// Supersede always wins over mark.
func DecideMemoryConflict(kind string, neighbors []MemoryConflictNeighbor) MemoryConflictDecision {
	if !IsConflictGovernableFactKind(kind) {
		return MemoryConflictDecision{Action: ConflictActionNone}
	}
	var supersede *MemoryConflictNeighbor
	var mark *MemoryConflictNeighbor
	for i := range neighbors {
		n := neighbors[i]
		if n.FactID == "" {
			continue
		}
		if n.Score >= memoryConflictSupersedeScore && n.FactKind == kind {
			if supersede == nil || n.Score > supersede.Score {
				cp := n
				supersede = &cp
			}
			continue
		}
		if n.Score >= memoryConflictMarkScore {
			if mark == nil || n.Score > mark.Score {
				cp := n
				mark = &cp
			}
		}
	}
	if supersede != nil {
		return MemoryConflictDecision{Action: ConflictActionSupersede, TargetFactID: supersede.FactID, Score: supersede.Score}
	}
	if mark != nil {
		return MemoryConflictDecision{Action: ConflictActionMarkConflict, TargetFactID: mark.FactID, Score: mark.Score}
	}
	return MemoryConflictDecision{Action: ConflictActionNone}
}

// MemoryConflictNeighborSearcher finds scored candidate facts near an
// embedding within the agent+user partition.
// Stability:evolving
type MemoryConflictNeighborSearcher interface {
	SearchFactNeighbors(ctx context.Context, agentID, userID string, embedding []float32, limit int, minScore float64) ([]MemoryConflictNeighbor, error)
}

// conflictFactRowReader reads raw fact rows for active-status validation.
// Satisfied by L3FactReader (GetFactRowsByIDs filters to active rows).
type conflictFactRowReader interface {
	GetFactRowsByIDs(ctx context.Context, factIDs []string) ([][]byte, error)
}

// MemoryConflictDetector detects conflicts for a proposed memory fact before
// it is written.
// Stability:evolving
type MemoryConflictDetector interface {
	DetectConflict(ctx context.Context, agentID, userID, factKind, statement string) (MemoryConflictDecision, error)
}

type memoryConflictDetector struct {
	searcher MemoryConflictNeighborSearcher
	embedder EmbeddingService
	reader   conflictFactRowReader
}

// NewMemoryConflictDetector wires a detector. Returns nil when any dependency
// is missing (conflict governance is best-effort and must never block writes).
func NewMemoryConflictDetector(searcher MemoryConflictNeighborSearcher, embedder EmbeddingService, reader conflictFactRowReader) MemoryConflictDetector {
	if searcher == nil || embedder == nil || reader == nil {
		return nil
	}
	return &memoryConflictDetector{searcher: searcher, embedder: embedder, reader: reader}
}

// DetectConflict embeds the statement, recalls scored neighbors, drops
// inactive rows, and arbitrates via DecideMemoryConflict. All infrastructure
// failures degrade to ConflictActionNone with a nil error — conflict
// governance must never fail the memory write path.
func (d *memoryConflictDetector) DetectConflict(ctx context.Context, agentID, userID, factKind, statement string) (MemoryConflictDecision, error) {
	none := MemoryConflictDecision{Action: ConflictActionNone}
	if !IsConflictGovernableFactKind(factKind) {
		return none, nil
	}
	stmt := strings.TrimSpace(statement)
	if stmt == "" {
		return none, nil
	}
	embedding, err := d.embedder.Embed(ctx, stmt)
	if err != nil || len(embedding) == 0 {
		return none, nil
	}
	neighbors, err := d.searcher.SearchFactNeighbors(ctx, agentID, userID, embedding, memoryConflictNeighborLimit, memoryConflictMarkScore)
	if err != nil || len(neighbors) == 0 {
		return none, nil
	}
	ids := make([]string, 0, len(neighbors))
	for _, n := range neighbors {
		if n.FactID != "" {
			ids = append(ids, n.FactID)
		}
	}
	if len(ids) == 0 {
		return none, nil
	}
	// Validate neighbors against the fact table: superseded/deleted rows are
	// excluded from arbitration and fact_kind is enriched from the row of
	// record (vector hits carry no kind).
	rows, err := d.reader.GetFactRowsByIDs(ctx, ids)
	if err != nil {
		return none, nil
	}
	kindByID := make(map[string]string, len(rows))
	for _, raw := range rows {
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		id, _ := m["id"].(string)
		if id == "" {
			continue
		}
		kindByID[id], _ = m["fact_kind"].(string)
	}
	enriched := make([]MemoryConflictNeighbor, 0, len(neighbors))
	for _, n := range neighbors {
		kind, ok := kindByID[n.FactID]
		if !ok {
			continue
		}
		n.FactKind = kind
		enriched = append(enriched, n)
	}
	return DecideMemoryConflict(factKind, enriched), nil
}
