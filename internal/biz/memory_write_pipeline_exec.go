package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// FactAdjudicationNeighbor is one existing fact shown to the LLM adjudicator
// as a candidate's neighbor (id + statement + kind).
type FactAdjudicationNeighbor struct {
	FactID    string
	Statement string
	FactKind  string
}

// FactAdjudicationItem pairs one contested candidate with its neighbors for
// a single batch adjudication call.
type FactAdjudicationItem struct {
	Candidate FactWriteCandidate
	Neighbors []FactAdjudicationNeighbor
}

// FactAdjudicationVerdict is the LLM adjudicator's per-candidate operation
// verdict. Statement matches the verdict back to its candidate.
type FactAdjudicationVerdict struct {
	Statement    string
	Operation    FactWriteOperation
	TargetFactID string
}

// FactWriteAdjudicator is the LLM operation-semantics port (P1-3). The
// service-layer implementation resolves a model via ModelCatalog and asks
// the LLM to verdict each contested candidate ADD/UPDATE/DELETE/NOOP.
// Stability:evolving
type FactWriteAdjudicator interface {
	AdjudicateFactWrites(ctx context.Context, agentID, userID string, items []FactAdjudicationItem) ([]FactAdjudicationVerdict, error)
}

// FactAccessCounter bumps recalled_count/last_used_at for dedup-merged facts
// (gate ③: a ≥0.92 same-kind neighbor already carries the fact — merge by
// recall-count increment instead of inserting a duplicate). FR-12.6: the
// merge target was "hit" by the write path, which maps to the recalled stage.
// Stability:evolving
type FactAccessCounter interface {
	IncrementFactRecalledCount(ctx context.Context, factIDs []string) error
}

// FactWriteBatchResult summarizes one Apply run for observability.
type FactWriteBatchResult struct {
	Added     int
	Updated   int
	Deleted   int
	Merged    int
	Dropped   int
	WriteErrs int
	// FactRows holds the raw JSON rows of newly written facts (add + update),
	// in decision order. Callers use them for downstream index sync — the
	// same contract as ConsolidationResult.FactRows.
	FactRows [][]byte
}

// FactWritePipelineDeps wires the unified write pipeline (biz struct
// dependency budget: exactly 8).
type FactWritePipelineDeps struct {
	Searcher    MemoryConflictNeighborSearcher // optional: nil → all candidates ADD
	Embedder    EmbeddingService               // optional: nil → all candidates ADD
	Reader      conflictFactRowReader          // optional: neighbor kind/statement enrichment
	Writer      L3FactWriter                   // required
	Access      FactAccessCounter              // optional: merges become plain noops
	Adjudicator FactWriteAdjudicator           // optional: contested → heuristic ADD
	ActionLog   MemoryActionLogWriter          // optional: audit trail
	LG          loggateway.Logger
}

// FactWritePipeline is the unified write pipeline (P1-3 §6.3): all automatic
// fact write sources (auto_memory worker, sleep-time episode consolidator)
// funnel candidates through gates → adjudication → bi-temporal writes →
// audit. Never blocks callers: every infrastructure failure degrades to the
// safest write (ADD) or a logged skip.
type FactWritePipeline struct {
	searcher    MemoryConflictNeighborSearcher
	embedder    EmbeddingService
	reader      conflictFactRowReader
	writer      L3FactWriter
	access      FactAccessCounter
	adjudicator FactWriteAdjudicator
	actionLog   MemoryActionLogWriter
	lg          loggateway.Logger
}

// NewFactWritePipeline creates the pipeline. Returns nil when the required
// writer is missing.
func NewFactWritePipeline(deps FactWritePipelineDeps) *FactWritePipeline {
	if deps.Writer == nil {
		return nil
	}
	lg := deps.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &FactWritePipeline{
		searcher:    deps.Searcher,
		embedder:    deps.Embedder,
		reader:      deps.Reader,
		writer:      deps.Writer,
		access:      deps.Access,
		adjudicator: deps.Adjudicator,
		actionLog:   deps.ActionLog,
		lg:          lg.With(loggateway.Domain("fact_write_pipeline")),
	}
}

// Apply runs the full pipeline for one batch of candidates from a single
// source. Safe to call with nil receiver or empty input.
func (p *FactWritePipeline) Apply(ctx context.Context, candidates []FactWriteCandidate) FactWriteBatchResult {
	var res FactWriteBatchResult
	if p == nil || len(candidates) == 0 {
		return res
	}

	// 1. Gates (pure): whitelist kind + confidence floor.
	passed := make([]FactWriteCandidate, 0, len(candidates))
	for _, c := range candidates {
		d := GateFactWriteCandidate(c)
		if d.DropReason != "" {
			res.Dropped++
			p.audit(ctx, "fact_write.drop", c, d.DropReason)
			continue
		}
		passed = append(passed, c)
	}
	if len(passed) == 0 {
		return res
	}

	// 2. Neighbor search per candidate (embedding); failures degrade to no
	// neighbors (→ ADD). Kinds/statements enriched from the fact rows.
	neighborsByCand := make([][]MemoryConflictNeighbor, len(passed))
	statementsByID := map[string]string{}
	for i, c := range passed {
		neighborsByCand[i] = p.findNeighbors(ctx, c, statementsByID)
	}

	// 3. Partition: contested candidates go to the LLM adjudicator (one
	// batch call); the rest are decided heuristically.
	decisions := make([]FactWriteDecision, len(passed))
	var contestedIdx []int
	var adjItems []FactAdjudicationItem
	for i, c := range passed {
		neighbors := neighborsByCand[i]
		if FactWriteIsContested(c.FactKind, neighbors) && p.adjudicator != nil {
			contestedIdx = append(contestedIdx, i)
			adjItems = append(adjItems, FactAdjudicationItem{
				Candidate: c,
				Neighbors: adjudicationNeighbors(neighbors, statementsByID),
			})
			continue
		}
		decisions[i] = DecideFactWriteHeuristic(c, neighbors)
	}
	if len(contestedIdx) > 0 {
		p.applyAdjudication(ctx, passed, neighborsByCand, decisions, contestedIdx, adjItems)
	}

	// 4. Execute decisions (bi-temporal writes; merges batched).
	var mergeIDs []string
	for _, d := range decisions {
		if d.Operation == "" {
			continue
		}
		p.execute(ctx, d, &res, &mergeIDs)
	}
	if len(mergeIDs) > 0 && p.access != nil {
		if err := p.access.IncrementFactRecalledCount(ctx, mergeIDs); err != nil {
			p.lg.Warn("fact write pipeline: merge recalled bump failed", loggateway.Err(err))
		}
	}
	return res
}

// findNeighbors embeds the candidate statement and recalls scored neighbors,
// enriching kinds/statements from fact rows. Any failure degrades to nil
// (candidate will be ADDed).
func (p *FactWritePipeline) findNeighbors(ctx context.Context, c FactWriteCandidate, statementsByID map[string]string) []MemoryConflictNeighbor {
	if p.searcher == nil || p.embedder == nil {
		return nil
	}
	embedding, err := p.embedder.Embed(ctx, c.Statement)
	if err != nil || len(embedding) == 0 {
		return nil
	}
	neighbors, err := p.searcher.SearchFactNeighbors(ctx, c.AgentID, c.UserID, embedding, memoryConflictNeighborLimit, FactWriteContestedScore)
	if err != nil || len(neighbors) == 0 {
		return nil
	}
	if p.reader == nil {
		return nil // without kind enrichment no band decision is safe
	}
	ids := make([]string, 0, len(neighbors))
	for _, n := range neighbors {
		if n.FactID != "" {
			ids = append(ids, n.FactID)
		}
	}
	rows, err := p.reader.GetFactRowsByIDs(ctx, ids)
	if err != nil {
		return nil
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
		if stmt, _ := m["statement"].(string); stmt != "" {
			statementsByID[id] = stmt
		}
	}
	out := make([]MemoryConflictNeighbor, 0, len(neighbors))
	for _, n := range neighbors {
		kind, ok := kindByID[n.FactID]
		if !ok {
			continue // inactive/deleted rows are excluded
		}
		n.FactKind = kind
		out = append(out, n)
	}
	return out
}

// applyAdjudication runs the batch LLM adjudication for contested candidates
// and folds verdicts into decisions. Unknown statements, invalid targets and
// adjudicator errors all fall back to the heuristic decision.
func (p *FactWritePipeline) applyAdjudication(ctx context.Context, passed []FactWriteCandidate, neighborsByCand [][]MemoryConflictNeighbor, decisions []FactWriteDecision, contestedIdx []int, items []FactAdjudicationItem) {
	agentID, userID := "", ""
	if len(passed) > 0 {
		agentID, userID = passed[0].AgentID, passed[0].UserID
	}
	verdicts, err := p.adjudicator.AdjudicateFactWrites(ctx, agentID, userID, items)
	if err != nil {
		p.lg.Warn("fact write pipeline: adjudicator failed, heuristic fallback", loggateway.Err(err))
		for _, i := range contestedIdx {
			decisions[i] = DecideFactWriteHeuristic(passed[i], neighborsByCand[i])
		}
		return
	}
	verdictByStmt := make(map[string]FactAdjudicationVerdict, len(verdicts))
	for _, v := range verdicts {
		verdictByStmt[strings.TrimSpace(v.Statement)] = v
	}
	for _, i := range contestedIdx {
		c := passed[i]
		v, ok := verdictByStmt[strings.TrimSpace(c.Statement)]
		if !ok {
			decisions[i] = DecideFactWriteHeuristic(c, neighborsByCand[i])
			continue
		}
		d := FactWriteDecision{Candidate: c, Operation: v.Operation, TargetFactID: strings.TrimSpace(v.TargetFactID)}
		switch v.Operation {
		case FactWriteOpUpdate, FactWriteOpDelete:
			if !neighborIDSet(neighborsByCand[i])[d.TargetFactID] {
				// LLM pointed at a fact that is not among the candidate's
				// neighbors (hallucinated or stale id) — safest is ADD.
				d = FactWriteDecision{Candidate: c, Operation: FactWriteOpAdd}
			}
		case FactWriteOpNoop:
			// LLM-declared noop: nothing to write, no access bump.
		default:
			d = FactWriteDecision{Candidate: c, Operation: FactWriteOpAdd}
		}
		decisions[i] = d
	}
}

// execute applies one decision to storage and updates the batch counters.
func (p *FactWritePipeline) execute(ctx context.Context, d FactWriteDecision, res *FactWriteBatchResult, mergeIDs *[]string) {
	switch d.Operation {
	case FactWriteOpAdd:
		row, err := p.writer.UpsertFactRow(ctx, buildFactUpsertFromCandidate(d.Candidate))
		if err != nil {
			res.WriteErrs++
			p.lg.Warn("fact write pipeline: add failed", loggateway.Err(err))
			return
		}
		res.Added++
		if len(row) > 0 {
			res.FactRows = append(res.FactRows, row)
		}
		p.audit(ctx, "fact_write.add", d.Candidate, "")
	case FactWriteOpUpdate:
		row, err := p.writer.InvalidateAndUpsertFactTx(ctx, d.TargetFactID, buildFactUpsertFromCandidate(d.Candidate))
		if err != nil {
			res.WriteErrs++
			p.lg.Warn("fact write pipeline: update failed", loggateway.Str("target", d.TargetFactID), loggateway.Err(err))
			return
		}
		res.Updated++
		if len(row) > 0 {
			res.FactRows = append(res.FactRows, row)
		}
		p.audit(ctx, "fact_write.update", d.Candidate, "target="+d.TargetFactID)
	case FactWriteOpDelete:
		if _, err := p.writer.InvalidateFact(ctx, d.TargetFactID); err != nil {
			res.WriteErrs++
			p.lg.Warn("fact write pipeline: delete failed", loggateway.Str("target", d.TargetFactID), loggateway.Err(err))
			return
		}
		res.Deleted++
		p.audit(ctx, "fact_write.delete", d.Candidate, "target="+d.TargetFactID)
	case FactWriteOpNoop:
		if d.TargetFactID != "" {
			res.Merged++
			*mergeIDs = append(*mergeIDs, d.TargetFactID)
			p.audit(ctx, "fact_write.merge", d.Candidate, "target="+d.TargetFactID)
		} else {
			p.audit(ctx, "fact_write.noop", d.Candidate, "")
		}
	}
}

// audit writes one action-log record per decision (best-effort).
func (p *FactWritePipeline) audit(ctx context.Context, action string, c FactWriteCandidate, reason string) {
	if p.actionLog == nil {
		return
	}
	rec := MemoryPolicyRecord{
		Action:        action,
		TargetKind:    "memory_fact",
		TargetID:      c.SourceEpisodeID,
		Reason:        strings.TrimSpace(reason + " kind=" + c.FactKind + " stmt=" + truncateRunes(c.Statement, 80)),
		PolicyVersion: "fact_write_pipeline_v1",
	}
	if err := p.actionLog.WriteMemoryActionLog(ctx, rec); err != nil {
		p.lg.Debug("fact write pipeline: audit write failed", loggateway.Err(err))
	}
}

// buildFactUpsertFromCandidate maps a pipeline candidate to the storage DTO.
func buildFactUpsertFromCandidate(c FactWriteCandidate) FactUpsert {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return FactUpsert{
		ID:              uuid.NewString(),
		ScopeType:       c.ScopeType,
		ScopeID:         c.ScopeID,
		UserID:          c.UserID,
		AgentID:         c.AgentID,
		Statement:       strings.TrimSpace(c.Statement),
		DetailsMarkdown: strings.TrimSpace(c.Statement),
		FactKind:        c.FactKind,
		TagsJSON:        c.TagsJSON,
		Confidence:      c.Confidence,
		Importance:      c.Importance,
		SourceKind:      c.SourceKind,
		SourceEpisodeID: c.SourceEpisodeID,
		SourceSessionID: c.SourceSessionID,
		SourceMessageID: c.SourceMessageID,
		Version:         1,
		Status:          "active",
		CreatedAt:       now,
		UpdatedAt:       now,
		ValidFrom:       now,
	}
}

// adjudicationNeighbors converts scored neighbors to the adjudicator DTO.
func adjudicationNeighbors(neighbors []MemoryConflictNeighbor, statementsByID map[string]string) []FactAdjudicationNeighbor {
	out := make([]FactAdjudicationNeighbor, 0, len(neighbors))
	for _, n := range neighbors {
		if n.FactID == "" {
			continue
		}
		out = append(out, FactAdjudicationNeighbor{
			FactID:    n.FactID,
			Statement: statementsByID[n.FactID],
			FactKind:  n.FactKind,
		})
	}
	return out
}

// neighborIDSet builds a lookup set of neighbor fact IDs.
func neighborIDSet(neighbors []MemoryConflictNeighbor) map[string]bool {
	set := make(map[string]bool, len(neighbors))
	for _, n := range neighbors {
		if n.FactID != "" {
			set[n.FactID] = true
		}
	}
	return set
}
