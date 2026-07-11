// Package memory provides the Link Evolution Service that asynchronously
// builds A-MEM style memory links. When a new memory is added, the service
// retrieves related historical memories via keyword search, asks an LLM
// whether to establish links, and updates the links/keywords metadata of
// both the new and the related historical memories.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	// evolutionQueueDefaultSize is the default buffer size for the
	// in-memory evolution queue.
	evolutionQueueDefaultSize = 100
	// evolutionLLMTimeout caps the LLM call duration for link analysis.
	evolutionLLMTimeout = 30 * time.Second
	// evolutionHistoryLimit caps the number of historical memories
	// retrieved for link analysis.
	evolutionHistoryLimit = 20
	// evolutionKeywordLimit caps the number of keywords extracted per memory.
	evolutionKeywordLimit = 10
	// evolutionFactScopeTypeAgent is the scope type for agent-scoped facts,
	// matching the constant used in internal/memory/trpc/sqlite_adapter.go.
	evolutionFactScopeTypeAgent = "agent"
)

// Link type constants for A-MEM style memory evolution (Phase 6A-03).
const (
	// LinkTypeRelatedTo indicates a bidirectional association between two
	// memories that share a topic, entity, or concept. Both memories remain
	// valid. This is the default link type when the LLM does not specify one.
	LinkTypeRelatedTo = "RELATED_TO"
	// LinkTypeEvolvedFrom indicates the new memory evolves from / supersedes
	// the historical memory. The historical memory is invalidated (valid_until
	// set) because the new memory represents a more current or refined version.
	LinkTypeEvolvedFrom = "EVOLVED_FROM"
)

// LinkEvolutionService analyses newly added memories and evolves the memory
// link graph by establishing links between related memories.
type LinkEvolutionService interface {
	// EvolveLinks analyses a newly added memory and updates links/keywords
	// of related historical memories. Returns the links generated for the
	// new memory (IDs of related historical memories).
	EvolveLinks(ctx context.Context, uk trpcmemory.UserKey, newEntry *trpcmemory.Entry) ([]string, error)
}

// EvolutionJobRequest represents a queued link evolution job.
type EvolutionJobRequest struct {
	UserKey    trpcmemory.UserKey
	NewEntry   *trpcmemory.Entry
	EnqueuedAt time.Time
}

// EvolutionQueue is the queue interface consumed by LinkEvolutionServiceImpl.
// Implementations must be safe for concurrent use.
type EvolutionQueue interface {
	Enqueue(r EvolutionJobRequest)
	Chan() <-chan EvolutionJobRequest
}

// TxProvider provides transactional execution for atomic link evolution.
// When tx is nil, EvolveLinks falls back to non-atomic best-effort updates
// (preserving the legacy behaviour). When tx is wired (e.g. *data.Data),
// the backlink batch + new-memory update are wrapped in a single transaction
// so a mid-loop failure rolls back all partial backlinks.
type TxProvider interface {
	ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// L4RelationWriter is the narrow interface for writing L4 memory relations
// during link evolution (Phase 6A-03). nil-safe: when not wired, EVOLVED_FROM
// links skip L4 relation creation (graceful degradation).
type L4RelationWriter interface {
	UpsertRelation(ctx context.Context, params biz.L4RelationWrite) error
}

// LinkEvolutionServiceImpl is the default LinkEvolutionService implementation.
// It uses an LLM to extract keywords and judge links, and persists the
// resulting links/keywords via the fact reader/writer ports.
type LinkEvolutionServiceImpl struct {
	llm             trpcmodel.Model
	factReader      biz.L3FactReader
	factWriter      biz.L3FactWriter
	queue           EvolutionQueue
	tx              TxProvider
	relationWriter  L4RelationWriter  // optional: nil → skip L4 relation creation
	actionLogWriter biz.MemoryActionLogWriter // optional: nil → skip action_log
	lg              loggateway.Logger

	// Phase 6A-03 T8: per-agent throttle. When throttleInterval > 0, LLM
	// analysis is skipped if the last call for the same agent (uk.AppName)
	// was less than throttleInterval ago. Prevents LLM cost spikes when
	// many memories are added in rapid succession. 0 = no throttle.
	throttleInterval time.Duration
	lastLLMCall      sync.Map // agentKey(string) → time.Time

	// Phase 6A-03 T9: global evolution kill switch. When false, EvolveLinks
	// is a no-op. Per-agent control via MemoryRuntimePolicy.LinkEvolutionEnabled
	// is resolved at the adapter call site (future enhancement).
	linkEvolutionEnabled bool
}

// NewLinkEvolutionService creates a LinkEvolutionServiceImpl.
//
// Parameters:
//   - llm:        the LLM used for keyword extraction and link judgment. May be nil (no-op).
//   - factReader: the reader for retrieving historical memories.
//   - factWriter: the writer for updating memory links/keywords.
//   - queue:      the queue for async evolution jobs. May be nil.
//   - tx:         the transaction provider for atomic backlink updates. May be nil
//     (falls back to non-atomic best-effort updates).
//   - lg:         the logger. Falls back to a no-op logger if nil.
func NewLinkEvolutionService(
	llm trpcmodel.Model,
	factReader biz.L3FactReader,
	factWriter biz.L3FactWriter,
	queue EvolutionQueue,
	tx TxProvider,
	lg loggateway.Logger,
) *LinkEvolutionServiceImpl {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &LinkEvolutionServiceImpl{
		llm:                  llm,
		factReader:           factReader,
		factWriter:           factWriter,
		queue:                queue,
		tx:                   tx,
		lg:                   lg.With(loggateway.Domain("link_evolution")),
		linkEvolutionEnabled: true, // T9: enabled by default
	}
}

// SetRelationWriter wires the L4 relation writer for EVOLVED_FROM link
// processing. When nil (default), EVOLVED_FROM links skip L4 relation creation.
func (s *LinkEvolutionServiceImpl) SetRelationWriter(rw L4RelationWriter) {
	if s == nil {
		return
	}
	s.relationWriter = rw
}

// SetActionLogWriter wires the action log writer for auditing evolution
// decisions. When nil (default), evolution actions are not logged to
// memory_action_log.
func (s *LinkEvolutionServiceImpl) SetActionLogWriter(aw biz.MemoryActionLogWriter) {
	if s == nil {
		return
	}
	s.actionLogWriter = aw
}

// SetThrottleInterval sets the minimum interval between LLM analysis calls
// per agent (Phase 6A-03 T8). When d > 0, EvolveLinks skips LLM analysis if
// the last call for the same agent (uk.AppName) was less than d ago.
// When d == 0 (default), throttling is disabled.
func (s *LinkEvolutionServiceImpl) SetThrottleInterval(d time.Duration) {
	if s == nil {
		return
	}
	s.throttleInterval = d
}

// SetLinkEvolutionEnabled toggles link evolution globally (Phase 6A-03 T9).
// When false, EvolveLinks is a no-op (returns nil, nil). Default: true.
// Per-agent control via MemoryRuntimePolicy.LinkEvolutionEnabled should be
// enforced at the adapter call site; this setter is a global kill switch.
func (s *LinkEvolutionServiceImpl) SetLinkEvolutionEnabled(enabled bool) {
	if s == nil {
		return
	}
	s.linkEvolutionEnabled = enabled
}

// EnqueueEvolution enqueues a link evolution job for the given user key and
// new entry. Returns nil when no queue is wired (no-op). Respects context
// cancellation. Non-blocking: drops the job if the queue is full.
func (s *LinkEvolutionServiceImpl) EnqueueEvolution(ctx context.Context, uk trpcmemory.UserKey, newEntry *trpcmemory.Entry) error {
	if s == nil || s.queue == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.queue.Enqueue(EvolutionJobRequest{
		UserKey:    uk,
		NewEntry:   newEntry,
		EnqueuedAt: time.Now(),
	})
	return nil
}

// Start runs the worker loop that drains the evolution queue and calls
// EvolveLinks for each job. Blocks until ctx is cancelled.
func (s *LinkEvolutionServiceImpl) Start(ctx context.Context) {
	if s == nil || s.queue == nil {
		return
	}
	ch := s.queue.Chan()
	if ch == nil {
		return
	}
	safego.Go(ctx, "link_evolution.worker", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case req := <-ch:
				s.processJob(ctx, req)
			}
		}
	})
}

func (s *LinkEvolutionServiceImpl) processJob(ctx context.Context, req EvolutionJobRequest) {
	if _, err := s.EvolveLinks(ctx, req.UserKey, req.NewEntry); err != nil {
		s.lg.Warn("link evolution job failed",
			loggateway.Str("app", req.UserKey.AppName),
			loggateway.Str("user", req.UserKey.UserID),
			loggateway.Err(err))
	}
}

// EvolveLinks analyses a newly added memory and updates links/keywords of
// related historical memories.
//
// Behaviour:
//   - nil newEntry → returns nil, nil (red line #26).
//   - nil LLM → returns nil, nil (graceful degradation, warn log).
//   - LLM failure (function error, API error, malformed JSON) → returns nil, nil.
//   - No related historical memories → returns nil, nil.
//   - factReader/factWriter failure → logs warn and returns nil (best-effort).
func (s *LinkEvolutionServiceImpl) EvolveLinks(ctx context.Context, uk trpcmemory.UserKey, newEntry *trpcmemory.Entry) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	// T9: global kill switch.
	if !s.linkEvolutionEnabled {
		return nil, nil
	}
	// Red line #26: nil defense for external input.
	if newEntry == nil || newEntry.Memory == nil {
		s.lg.Warn("link evolution skipped: nil new entry",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID))
		return nil, nil
	}
	if s.llm == nil {
		s.lg.Warn("link evolution skipped: nil LLM",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID))
		return nil, nil
	}
	if s.factReader == nil || s.factWriter == nil {
		s.lg.Warn("link evolution skipped: nil fact reader or writer",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID))
		return nil, nil
	}
	// T8: per-agent throttle. Skip LLM analysis if called too soon.
	if s.throttleInterval > 0 {
		agentKey := uk.AppName
		now := time.Now()
		if last, ok := s.lastLLMCall.Load(agentKey); ok {
			if lastTime, ok := last.(time.Time); ok && now.Sub(lastTime) < s.throttleInterval {
				remaining := s.throttleInterval - now.Sub(lastTime)
				s.lg.Debug("link evolution throttled: too soon since last call",
					loggateway.Str("app", uk.AppName),
					loggateway.Str("user", uk.UserID),
					loggateway.Str("remaining", remaining.String()))
				return nil, nil
			}
		}
		s.lastLLMCall.Store(agentKey, now)
	}

	newMemText := strings.TrimSpace(newEntry.Memory.Memory)
	if newMemText == "" || newEntry.ID == "" {
		return nil, nil
	}

	// 1. Retrieve historical memories for the user.
	historicalRows, err := s.factReader.ListFactRowsForUser(ctx,
		evolutionFactScopeTypeAgent, uk.AppName, uk.UserID, "", evolutionHistoryLimit, 0)
	if err != nil {
		s.lg.Warn("link evolution read historical memories failed",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID),
			loggateway.Err(err))
		return nil, nil
	}
	// Filter out the new memory itself and parse rows.
	var historical []factRowData
	for _, raw := range historicalRows {
		row, parseErr := parseFactRowData(raw)
		if parseErr != nil {
			continue
		}
		if row.ID == newEntry.ID || strings.TrimSpace(row.Statement) == "" {
			continue
		}
		historical = append(historical, row)
	}
	if len(historical) == 0 {
		// No historical memories to link — return empty links.
		return nil, nil
	}

	// 2. LLM analysis: extract keywords and judge links.
	result, err := s.llmAnalyzeLinks(ctx, newEntry.ID, newMemText, historical)
	if err != nil {
		// Graceful degradation: log warn, return empty links.
		s.lg.Warn("link evolution LLM analysis failed, skipping",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID),
			loggateway.Err(err))
		return nil, nil
	}

	// 3+4. Apply backlinks (historical memories) and forward links (new memory).
	//
	// When s.tx is wired (production: *data.Data), the batch is wrapped in a
	// single transaction so a mid-loop failure rolls back all partial
	// backlinks (atomicity). When s.tx is nil (tests / legacy), the batch
	// runs without a transaction — the first failure aborts the remaining
	// updates but already-applied writes are not rolled back.
	//
	// Note: UpsertFactRow internally calls Data.ExecInTx for INSERT + read-back.
	// When this method wraps the batch in ExecInTx, the nested ExecInTx inside
	// UpsertFactRow detects the in-progress transaction via ctx and reuses it
	// (see tx.go: txClientKey detection), so all writes participate in the
	// same transaction.
	applyBacklinks := func(ctx context.Context) ([]string, error) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		linkedIDs := make([]string, 0, len(result.Links))
		for _, link := range result.Links {
			targetID := strings.TrimSpace(link.TargetID)
			if targetID == "" || targetID == newEntry.ID {
				continue
			}
			// Verify the target ID exists in historical memories.
			var targetRow *factRowData
			for i := range historical {
				if historical[i].ID == targetID {
					targetRow = &historical[i]
					break
				}
			}
			if targetRow == nil {
				continue
			}
			// Add the new memory's ID to the historical memory's links.
			updatedLinks := appendUnique(decodeStringArray(targetRow.LinksJSON), newEntry.ID)
			updatedKeywords := mergeUnique(decodeStringArray(targetRow.KeywordsJSON), result.Keywords)
			upsert := factRowToUpsert(*targetRow)
			upsert.LinksJSON = encodeStringArray(updatedLinks)
			upsert.KeywordsJSON = encodeStringArray(updatedKeywords)

			// EVOLVED_FROM: invalidate the old fact (set valid_until) and
			// attach the LLM-generated context_note explaining the evolution.
			// RELATED_TO: optionally attach context_note for the association.
			if link.LinkType == LinkTypeEvolvedFrom {
				if strings.TrimSpace(targetRow.ValidUntil) == "" {
					upsert.ValidUntil = now
				}
				if link.ContextNote != "" {
					upsert.ContextNote = link.ContextNote
				}
			} else if link.ContextNote != "" {
				// For RELATED_TO, append context_note if the old fact doesn't already have one.
				if strings.TrimSpace(targetRow.ContextNote) == "" {
					upsert.ContextNote = link.ContextNote
				}
			}

			if _, upErr := s.factWriter.UpsertFactRow(ctx, upsert); upErr != nil {
				return nil, fmt.Errorf("update historical memory %s: %w", targetID, upErr)
			}

			// EVOLVED_FROM: create L4 memory_relations entry and action_log.
			if link.LinkType == LinkTypeEvolvedFrom {
				s.applyEvolvedFromSideEffects(ctx, newEntry.ID, targetRow, link, now)
			}
			linkedIDs = append(linkedIDs, targetID)
		}

		// 4. Update the new memory's links and keywords.
		if len(linkedIDs) > 0 {
			if err := s.updateNewMemoryLinks(ctx, uk, newEntry.ID, linkedIDs, result.Keywords); err != nil {
				return nil, fmt.Errorf("update new memory %s: %w", newEntry.ID, err)
			}
		}
		return linkedIDs, nil
	}

	var linkedIDs []string
	var applyErr error
	if s.tx != nil {
		applyErr = s.tx.ExecInTx(ctx, func(txCtx context.Context) error {
			var err error
			linkedIDs, err = applyBacklinks(txCtx)
			return err
		})
	} else {
		linkedIDs, applyErr = applyBacklinks(ctx)
	}
	if applyErr != nil {
		// Best-effort: log warn and return empty links. A failed batch leaves
		// no partial backlinks when tx is wired (rolled back), or may leave
		// partial writes when tx is nil (acceptable — links can be rebuilt on
		// the next analysis pass).
		s.lg.Warn("link evolution apply backlinks failed",
			loggateway.Str("app", uk.AppName),
			loggateway.Str("user", uk.UserID),
			loggateway.Str("new_fact_id", newEntry.ID),
			loggateway.Err(applyErr))
		return nil, nil
	}

	s.lg.Info("link evolution completed",
		loggateway.Str("app", uk.AppName),
		loggateway.Str("user", uk.UserID),
		loggateway.Str("new_fact_id", newEntry.ID),
		loggateway.Int("links", len(linkedIDs)),
		loggateway.Int("keywords", len(result.Keywords)))
	return linkedIDs, nil
}

// updateNewMemoryLinks reads the new memory's row, updates its links/keywords,
// and writes it back via factWriter.
func (s *LinkEvolutionServiceImpl) updateNewMemoryLinks(ctx context.Context, uk trpcmemory.UserKey, newID string, linkedIDs, keywords []string) error {
	rows, err := s.factReader.GetFactRowsByIDs(ctx, []string{newID})
	if err != nil {
		return fmt.Errorf("read new memory row: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	row, err := parseFactRowData(rows[0])
	if err != nil {
		return fmt.Errorf("parse new memory row: %w", err)
	}
	existingLinks := decodeStringArray(row.LinksJSON)
	mergedLinks := mergeUnique(existingLinks, linkedIDs)
	existingKeywords := decodeStringArray(row.KeywordsJSON)
	mergedKeywords := mergeUnique(existingKeywords, keywords)
	upsert := factRowToUpsert(row)
	upsert.LinksJSON = encodeStringArray(mergedLinks)
	upsert.KeywordsJSON = encodeStringArray(mergedKeywords)
	_, err = s.factWriter.UpsertFactRow(ctx, upsert)
	return err
}

// applyEvolvedFromSideEffects creates the L4 memory_relations entry and
// action_log record for an EVOLVED_FROM link. Both are best-effort:
// failures are logged as warnings and do not abort the evolution batch.
func (s *LinkEvolutionServiceImpl) applyEvolvedFromSideEffects(ctx context.Context, newFactID string, oldFact *factRowData, link linkDecision, now string) {
	// L4 memory_relations entry: new fact EVOLVED_FROM old fact.
	if s.relationWriter != nil && oldFact != nil {
		rel := biz.L4RelationWrite{
			ScopeType:    oldFact.ScopeType,
			ScopeID:      oldFact.ScopeID,
			SourceID:     newFactID,
			TargetID:     oldFact.ID,
			RelationType: LinkTypeEvolvedFrom,
			Weight:       1.0,
			Confidence:   0.8,
		}
		if err := s.relationWriter.UpsertRelation(ctx, rel); err != nil {
			s.lg.Warn("link evolution: L4 relation creation failed",
				loggateway.Str("source", newFactID),
				loggateway.Str("target", oldFact.ID),
				loggateway.Err(err))
		}
	}

	// action_log: audit trail for the evolution decision.
	if s.actionLogWriter != nil {
		rec := biz.MemoryPolicyRecord{
			Action:        "evolve_link",
			TargetKind:    "memory_fact",
			TargetID:      oldFact.ID,
			Reason:        link.Reason,
			PolicyVersion: "link_evolution_v1",
		}
		if err := s.actionLogWriter.WriteMemoryActionLog(ctx, rec); err != nil {
			s.lg.Warn("link evolution: action_log write failed",
				loggateway.Str("target", oldFact.ID),
				loggateway.Err(err))
		}
	}
}

// linkAnalysisResult holds the LLM-produced keywords and link decisions.
type linkAnalysisResult struct {
	Keywords []string       `json:"keywords"`
	Links    []linkDecision `json:"links"`
}

// linkDecision represents a single link decision from the LLM.
type linkDecision struct {
	TargetID    string `json:"target_id"`
	LinkType    string `json:"link_type,omitempty"` // RELATED_TO (default) | EVOLVED_FROM
	Reason      string `json:"reason,omitempty"`
	ContextNote string `json:"context_note,omitempty"` // LLM-generated evolution context annotation
}

// llmAnalyzeLinks calls the LLM with the new memory and historical memories,
// asking it to extract keywords and decide which historical memories to link.
func (s *LinkEvolutionServiceImpl) llmAnalyzeLinks(ctx context.Context, newID, newMemText string, historical []factRowData) (*linkAnalysisResult, error) {
	prompt := buildLinkEvolutionPrompt(newID, newMemText, historical)
	callCtx, cancel := context.WithTimeout(ctx, evolutionLLMTimeout)
	defer cancel()

	req := trpcmodel.NewRequest([]trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: linkEvolutionSystemPrompt},
		{Role: trpcmodel.RoleUser, Content: prompt},
	})

	respCh, err := s.llm.GenerateContent(callCtx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM generate content: %w", err)
	}

	content, err := consumeLLMResponse(respCh)
	if err != nil {
		return nil, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return &linkAnalysisResult{}, nil
	}

	var result linkAnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse LLM link evolution response: %w", err)
	}
	// Cap keywords to prevent unbounded growth.
	if len(result.Keywords) > evolutionKeywordLimit {
		result.Keywords = result.Keywords[:evolutionKeywordLimit]
	}
	// Normalize link_type: default to RELATED_TO for empty/unknown values.
	for i := range result.Links {
		lt := strings.ToUpper(strings.TrimSpace(result.Links[i].LinkType))
		if lt != LinkTypeEvolvedFrom {
			lt = LinkTypeRelatedTo
		}
		result.Links[i].LinkType = lt
		result.Links[i].ContextNote = strings.TrimSpace(result.Links[i].ContextNote)
	}
	return &result, nil
}

// buildLinkEvolutionPrompt constructs the user prompt containing the new
// memory and historical memories for LLM analysis.
func buildLinkEvolutionPrompt(newID, newMemText string, historical []factRowData) string {
	type histItem struct {
		ID       string   `json:"id"`
		Memory   string   `json:"memory"`
		Keywords []string `json:"keywords,omitempty"`
	}
	items := make([]histItem, 0, len(historical))
	for _, h := range historical {
		items = append(items, histItem{
			ID:       h.ID,
			Memory:   h.Statement,
			Keywords: decodeStringArray(h.KeywordsJSON),
		})
	}
	histJSON, _ := json.Marshal(items)
	return fmt.Sprintf(`New memory:
{"id": %q, "memory": %q}

Historical memories:
%s

Analyse the new memory and decide which historical memories should be linked to it. Return JSON only.`,
		newID, newMemText, string(histJSON))
}

const linkEvolutionSystemPrompt = `You are a memory linking agent. Given a new memory and a list of historical memories, your task is to:
1. Extract 3-10 keywords that capture the key concepts of the new memory.
2. Decide which historical memories are related to the new memory and should be linked.
3. For each link, classify the relationship type and provide a context annotation.

Return a JSON object with this schema:
{"keywords": ["keyword1", "keyword2"], "links": [{"target_id": "memory-id", "link_type": "RELATED_TO|EVOLVED_FROM", "reason": "brief reason", "context_note": "how this memory relates to or evolves from the target"}]}

Link types:
- RELATED_TO: the new memory is associated with the historical memory (same topic, entity, or concept). Both memories remain valid. Use this for most links.
- EVOLVED_FROM: the new memory supersedes, refines, or contradicts the historical memory. The historical memory is no longer the current truth. Use this when the new memory represents a more recent or corrected version of the same fact.

Rules:
- Only link memories that are genuinely related (same topic, entity, or concept).
- Do not link a memory to itself.
- For EVOLVED_FROM links, context_note should explain how the new memory supersedes the old one (e.g., "User's location updated from Beijing to Shanghai").
- For RELATED_TO links, context_note should explain the nature of the association.
- If no historical memories are related, return {"keywords": [...], "links": []}.
- Output ONLY the JSON object, no markdown fences or explanations.`

// factRowData is a minimal struct for parsing fact row JSON returned by
// L3FactReader. It captures the fields needed for link evolution.
type factRowData struct {
	ID              string  `json:"id"`
	ScopeType       string  `json:"scope_type"`
	ScopeID         string  `json:"scope_id"`
	WorkspaceID     string  `json:"workspace_id"`
	UserID          string  `json:"user_id"`
	TeamID          string  `json:"team_id"`
	AgentID         string  `json:"agent_id"`
	Statement       string  `json:"statement"`
	Fingerprint     string  `json:"fingerprint"`
	DetailsMarkdown string  `json:"details_markdown"`
	FactKind        string  `json:"fact_kind"`
	TagsJSON        string  `json:"tags_json"`
	Confidence      float64 `json:"confidence"`
	Importance      float64 `json:"importance"`
	SourceKind      string  `json:"source_kind"`
	Status          string  `json:"status"`
	MetadataJSON    string  `json:"metadata_json"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	ValidFrom       string  `json:"valid_from"`
	ValidUntil      string  `json:"valid_until"`
	LinksJSON       string  `json:"links"`
	KeywordsJSON    string  `json:"keywords"`
	ContextNote     string  `json:"context_note"`
}

// parseFactRowData parses a raw JSON byte slice into factRowData.
func parseFactRowData(raw []byte) (factRowData, error) {
	var row factRowData
	if err := json.Unmarshal(raw, &row); err != nil {
		return factRowData{}, err
	}
	return row, nil
}

// factRowToUpsert converts a factRowData into a FactUpsert suitable for
// writing back via L3FactWriter.UpsertFactRow. Preserves existing fields
// so the upsert does not clobber unrelated columns.
func factRowToUpsert(row factRowData) biz.FactUpsert {
	return biz.FactUpsert{
		ID:              row.ID,
		ScopeType:       row.ScopeType,
		ScopeID:         row.ScopeID,
		WorkspaceID:     row.WorkspaceID,
		UserID:          row.UserID,
		TeamID:          row.TeamID,
		AgentID:         row.AgentID,
		Statement:       row.Statement,
		Fingerprint:     row.Fingerprint,
		DetailsMarkdown: row.DetailsMarkdown,
		FactKind:        row.FactKind,
		TagsJSON:        row.TagsJSON,
		Confidence:      row.Confidence,
		Importance:      row.Importance,
		SourceKind:      row.SourceKind,
		Status:          row.Status,
		MetadataJSON:    row.MetadataJSON,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		ValidFrom:       row.ValidFrom,
		ValidUntil:      row.ValidUntil,
		LinksJSON:       row.LinksJSON,
		KeywordsJSON:    row.KeywordsJSON,
		ContextNote:     row.ContextNote,
	}
}

// consumeLLMResponse drains the response channel and concatenates content.
func consumeLLMResponse(respCh <-chan *trpcmodel.Response) (string, error) {
	var sb strings.Builder
	for resp := range respCh {
		if resp == nil {
			continue
		}
		if resp.Error != nil {
			return "", fmt.Errorf("LLM API error: %s", resp.Error.Message)
		}
		for _, choice := range resp.Choices {
			if choice.Delta.Content != "" {
				sb.WriteString(choice.Delta.Content)
			}
			if choice.Message.Content != "" {
				sb.WriteString(choice.Message.Content)
			}
		}
	}
	return sb.String(), nil
}

// decodeStringArray parses a JSON string array. Returns nil for empty/invalid input.
func decodeStringArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// encodeStringArray serialises a string slice to JSON. Returns "[]" for nil/empty.
func encodeStringArray(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	b, err := json.Marshal(arr)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// appendUnique appends item to arr if not already present. Returns the
// (possibly extended) slice.
func appendUnique(arr []string, item string) []string {
	for _, existing := range arr {
		if existing == item {
			return arr
		}
	}
	return append(arr, item)
}

// mergeUnique merges two string slices, removing duplicates. Preserves order.
func mergeUnique(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, item := range a {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	for _, item := range b {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

// evolutionQueue is a simple in-memory implementation of EvolutionQueue.
type evolutionQueue struct {
	ch chan EvolutionJobRequest
}

// NewEvolutionQueue creates a new in-memory evolution queue with the given
// buffer size. Falls back to a default size when bufferSize <= 0.
func NewEvolutionQueue(bufferSize int) EvolutionQueue {
	if bufferSize <= 0 {
		bufferSize = evolutionQueueDefaultSize
	}
	return &evolutionQueue{
		ch: make(chan EvolutionJobRequest, bufferSize),
	}
}

func (q *evolutionQueue) Enqueue(r EvolutionJobRequest) {
	if q == nil {
		return
	}
	if r.EnqueuedAt.IsZero() {
		r.EnqueuedAt = time.Now()
	}
	select {
	case q.ch <- r:
	default:
		// Queue full — drop the job. A dropped evolution job is
		// acceptable: links can be rebuilt on the next analysis pass.
	}
}

func (q *evolutionQueue) Chan() <-chan EvolutionJobRequest {
	if q == nil {
		return nil
	}
	return q.ch
}
