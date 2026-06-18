package trpcmem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
	memlink "aranea-agents/internal/memory"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmemtool "trpc.group/trpc-go/trpc-agent-go/memory/tool"
	"trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	factScopeTypeAgent      = "agent"
	factSourceTRPC          = "trpc_memory"
	vectorMinSimilarity     = 0.5 // minimum cosine similarity for vector recall hits
	defaultListEntriesLimit = 50  // fallback limit when caller provides none

	// proactiveRecallMaxQueries caps the number of search queries issued
	// during proactive recall to avoid excessive DB load.
	proactiveRecallMaxQueries = 8
	// contradictionBoost is the score increment applied to memories that
	// share keywords with the user statement (potential contradictions).
	contradictionBoost = 0.1
	// minKeywordLen is the minimum token length to be considered a keyword.
	minKeywordLen = 3
	// linkEvolutionTimeout caps the overall duration of an async link
	// evolution triggered after AddMemory. The LLM call inside EvolveLinks
	// has its own shorter timeout (evolutionLLMTimeout); this budget covers
	// DB reads/writes plus the LLM round-trip.
	linkEvolutionTimeout = 90 * time.Second
)

var memoryReadConsistencyCheck = os.Getenv("MEMORY_READ_CONSISTENCY_CHECK") == "1"

// vecHit is a vector search hit carrying the fact entry ID, memory text, and similarity score.
type vecHit struct {
	factID  string
	memText string
	score   float64
}

type memoryService struct {
	factReader      biz.L3FactReader
	factWriter      biz.L3FactWriter
	indexSync       biz.MemoryFactIndexSyncer
	autoMemoryQueue AutoMemoryQueue
	vector          vectorFactSearcher
	settingsLoader  AgentRuntimeSettingsLoader
	consistency     factConsistencyChecker
	resyncFlight    singleflightGroup
	linkEvolver     memlink.LinkEvolutionService
	lg              loggateway.Logger
}

// vectorFactSearcher optional pgvector recall for SearchMemories.
type vectorFactSearcher interface {
	RecallWithUser(ctx context.Context, agentID, userID, query string, topK int) ([]*biz.AgentMemory, error)
}

// factConsistencyChecker checks fact row status for read-consistency validation.
type factConsistencyChecker interface {
	GetFactConsistencyRow(ctx context.Context, factID string) (status, indexStatus, statement string, err error)
	GetFactResyncRow(ctx context.Context, factID string) (agentID, userID, statement string, err error)
}

var _ trpcmemory.Service = (*memoryService)(nil)

type singleflightGroup struct {
	mu sync.Mutex
	m  map[string]struct{}
}

func (g *singleflightGroup) TryStart(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.m == nil {
		g.m = make(map[string]struct{})
	}
	if _, ok := g.m[key]; ok {
		return false
	}
	g.m[key] = struct{}{}
	return true
}

func (g *singleflightGroup) Done(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.m, key)
}

func NewMemoryService(factReader biz.L3FactReader, factWriter biz.L3FactWriter, indexSync biz.MemoryFactIndexSyncer, queue AutoMemoryQueue, vector vectorFactSearcher, settingsLoader AgentRuntimeSettingsLoader, consistency factConsistencyChecker, linkEvolver memlink.LinkEvolutionService, lg loggateway.Logger) trpcmemory.Service {
	if factReader == nil {
		return nil
	}
	return &memoryService{
		factReader:      factReader,
		factWriter:      factWriter,
		indexSync:       indexSync,
		autoMemoryQueue: queue,
		vector:          vector,
		settingsLoader:  settingsLoader,
		consistency:     consistency,
		linkEvolver:     linkEvolver,
		lg:              lg,
	}
}

func (s *memoryService) requireStore() error {
	if s == nil || s.factReader == nil {
		return errors.New("sqlite memory service: fact reader not wired")
	}
	return nil
}

func (s *memoryService) AddMemory(ctx context.Context, uk trpcmemory.UserKey, mem string, topics []string, opts ...trpcmemory.AddOption) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	meta := trpcmemory.ResolveAddOptions(opts)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	factKind := "fact"
	if meta != nil && meta.Kind != "" {
		factKind = string(meta.Kind)
	}
	upsert := trpcFactUpsert(uk, "", mem, topics, factKind, now, now)
	// Bi-temporal (P3-8): new memories are valid from creation time.
	// ValidUntil stays empty (= currently valid).
	upsert.ValidFrom = now
	applyPIIScan(&upsert, mem)
	raw, err := s.factWriter.UpsertFactRow(ctx, upsert)
	if err != nil {
		return err
	}
	s.syncIndexBestEffort(ctx, raw)
	s.triggerLinkEvolution(ctx, uk, raw)
	return nil
}

func (s *memoryService) UpdateMemory(ctx context.Context, mk trpcmemory.Key, mem string, topics []string, opts ...trpcmemory.UpdateOption) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := strings.TrimSpace(mk.MemoryID)
	if id == "" {
		return errors.New("memory id is required")
	}
	uk := trpcmemory.UserKey{AppName: mk.AppName, UserID: mk.UserID}
	// Resolve update options to extract Kind and UpdateResult sink.
	meta := trpcmemory.ResolveUpdateOptions(opts)
	factKind := "fact"
	if meta != nil && meta.Kind != "" {
		factKind = string(meta.Kind)
	}
	// Bi-temporal conflict detection (P3-8): when the new content differs
	// from the existing fact, invalidate the old fact (set ValidUntil)
	// rather than overwriting it. This preserves history for temporal
	// reconstruction queries.
	upsertID := id
	if oldID := s.detectContentConflict(ctx, id, mem); oldID != "" {
		if _, invErr := s.factWriter.InvalidateFact(ctx, oldID); invErr != nil {
			s.lg.Warn("invalidate old fact on update conflict failed",
				loggateway.StepID("memory.update.invalidate"),
				loggateway.Str("fact_id", oldID),
				loggateway.Err(invErr))
		}
		// Create a new fact with a fresh ID so the old row is preserved.
		upsertID = ""
	}
	upsert := trpcFactUpsert(uk, upsertID, mem, topics, factKind, now, now)
	upsert.ValidFrom = now
	applyPIIScan(&upsert, mem)
	raw, err := s.factWriter.UpsertFactRow(ctx, upsert)
	if err != nil {
		return err
	}
	// Propagate the effective memory ID back to the caller so that
	// subsequent operations reference the correct row (upsert may
	// rotate the ID when the fingerprint changes).
	result := trpcmemory.ResolveUpdateResult(opts)
	if result != nil && len(raw) > 0 {
		var row map[string]any
		if json.Unmarshal(raw, &row) == nil {
			if effectiveID, _ := row["id"].(string); effectiveID != "" {
				result.MemoryID = effectiveID
			}
		}
	}
	s.syncIndexBestEffort(ctx, raw)
	return nil
}

// detectContentConflict checks whether the existing fact with the given ID
// has a different statement than the new memory content. Returns the fact ID
// if a conflict is detected (content differs), or empty string otherwise.
// When the fact reader is unavailable or the fact doesn't exist, no conflict
// is reported and the update proceeds as a normal upsert.
func (s *memoryService) detectContentConflict(ctx context.Context, factID, newMem string) string {
	if s.factReader == nil || factID == "" {
		return ""
	}
	newMem = strings.TrimSpace(newMem)
	if newMem == "" {
		return ""
	}
	rows, err := s.factReader.GetFactRowsByIDs(ctx, []string{factID})
	if err != nil || len(rows) == 0 {
		return ""
	}
	var row factRow
	if json.Unmarshal(rows[0], &row) != nil {
		return ""
	}
	if strings.TrimSpace(row.Statement) != newMem {
		return factID
	}
	return ""
}

// applyPIIScan scans the statement for PII and updates the FactUpsert accordingly.
// If PII is detected, the original text is preserved in OriginalStatement so that
// ApprovePIIFact can restore it. The statement and details are replaced with the
// redacted version for safe storage and display.
func applyPIIScan(u *biz.FactUpsert, original string) {
	result := biz.ScanPII(original)
	if !result.PIIFlag {
		return
	}
	u.PIIFlag = true
	u.PIITypes = result.PIITypes
	if result.RedactedStatement != "" {
		u.OriginalStatement = original // preserve original for ApprovePIIFact recovery
		u.Statement = result.RedactedStatement
		u.DetailsMarkdown = result.RedactedStatement
	}
}

func (s *memoryService) DeleteMemory(ctx context.Context, mk trpcmemory.Key) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	return s.factWriter.DeleteFactRow(ctx, mk.MemoryID)
}

func (s *memoryService) ClearMemories(ctx context.Context, uk trpcmemory.UserKey) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	// Delegate to L3FactWriter so that both SQLite rows and pgvector
	// embeddings are cleaned up atomically. The previous implementation
	// only soft-deleted SQLite rows, leaving stale vectors in pgvector
	// that would still be recalled by SearchMemories.
	_, err := s.factWriter.ClearFactsByScope(ctx, factScopeTypeAgent, uk.AppName, uk.UserID)
	return err
}

func (s *memoryService) ReadMemories(ctx context.Context, uk trpcmemory.UserKey, limit int) ([]*trpcmemory.Entry, error) {
	if s == nil {
		return nil, nil
	}
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	topK, minScore := resolveMemoryToolSearchLimits(ctx, s.settingsLoader, uk.AppName, 0)
	// When MasterEnabled is false, topK is 0 — respect the policy and return empty.
	if topK <= 0 {
		return nil, nil
	}
	// Cap limit by policy topK so callers cannot exceed the allowed ceiling.
	if limit <= 0 || limit > int(topK) {
		limit = int(topK)
	}
	return s.listEntries(ctx, uk, "", limit, minScore, false)
}

func (s *memoryService) SearchMemories(ctx context.Context, uk trpcmemory.UserKey, query string, opts ...trpcmemory.SearchOption) ([]*trpcmemory.Entry, error) {
	if s == nil {
		return nil, nil
	}
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	searchOpts := trpcmemory.ResolveSearchOptions(query, opts)
	q := strings.TrimSpace(query)
	if q == "" {
		q = strings.TrimSpace(searchOpts.Query)
	}
	topK, minScore := resolveMemoryToolSearchLimits(ctx, s.settingsLoader, uk.AppName, int32(searchOpts.MaxResults))
	if topK <= 0 {
		return nil, nil
	}

	if q != "" && s.vector != nil {
		hits, err := s.vector.RecallWithUser(ctx, uk.AppName, uk.UserID, q, int(topK))
		if err != nil {
			if !errors.Is(err, biz.ErrMemoryUnavailable) {
				s.lg.Warn("vector recall failed", loggateway.StepID("memory.search"), loggateway.Str("agent", uk.AppName), loggateway.Err(err))
			}
		}
		if err == nil && len(hits) > 0 {
			// Collect fact IDs for batch enrichment from the authoritative
			// SQLite store. Vector hits only carry ID + content + score;
			// topics, kind, event_time etc. must be backfilled from the
			// fact rows to avoid returning incomplete entries.
			var factIDs []string
			hitMap := make(map[string]vecHit, len(hits))
			for _, h := range hits {
				if h == nil || strings.TrimSpace(h.Content) == "" {
					continue
				}
				if h.Score < vectorMinSimilarity {
					continue
				}
				factID, memText := vector.ParseFactVectorContent(h.Content)
				if memText == "" {
					memText = strings.TrimSpace(h.Content)
				}
				entryID := factID
				if entryID == "" {
					entryID = fmt.Sprintf("%d", h.ID)
				}
				if memoryReadConsistencyCheck && factID != "" && s.consistency != nil {
					row, consistencyErr := s.getFactConsistencyRow(ctx, factID)
					if consistencyErr != nil {
						s.lg.Warn("read consistency check failed, skipping validation",
							loggateway.StepID("memory.search"),
							loggateway.Str("fact_id", factID),
							loggateway.Err(consistencyErr))
					} else if row.Status == "" || row.Status != "active" || row.IndexStatus == "disabled" {
						continue
					} else if row.IndexStatus == "stale" {
						s.asyncResyncFact(ctx, factID)
						continue
					}
				}
				if factID != "" {
					factIDs = append(factIDs, factID)
					hitMap[factID] = vecHit{factID: entryID, memText: memText, score: h.Score}
				}
			}
			// Enrich vector hits with full metadata from SQLite.
			if len(factIDs) > 0 && s.factReader != nil {
				enriched := s.enrichVectorHits(ctx, uk, factIDs, hitMap)
				if len(enriched) > 0 {
					return enriched, nil
				}
			}
			// Fallback: build minimal entries from vector data when
			// enrichment is unavailable (factReader nil or no matches).
			out := make([]*trpcmemory.Entry, 0, len(hitMap))
			for fid, hit := range hitMap {
				now := time.Now()
				memContent := hit.memText
				if memContent == "" {
					memContent = fid
				}
				out = append(out, &trpcmemory.Entry{
					ID:      hit.factID,
					AppName: uk.AppName,
					UserID:  uk.UserID,
					Memory: &trpcmemory.Memory{
						Memory:      memContent,
						LastUpdated: &now,
					},
					Score: hit.score,
				})
			}
			if len(out) > 0 {
				return out, nil
			}
		}
	}

	entries, err := s.listEntries(ctx, uk, q, int(topK), minScore, searchOpts.IncludeInvalidated)
	if err != nil {
		return nil, err
	}
	// When a query was provided, do NOT fall back to unfiltered ReadMemories.
	// Returning unrelated random memories is worse than returning nothing.
	return entries, nil
}

type factConsistencyRow struct {
	Status      string
	IndexStatus string
	Statement   string
}

func (s *memoryService) getFactConsistencyRow(ctx context.Context, factID string) (factConsistencyRow, error) {
	if s.consistency == nil {
		return factConsistencyRow{}, errors.New("consistency checker not wired")
	}
	status, indexStatus, statement, err := s.consistency.GetFactConsistencyRow(ctx, factID)
	if err != nil {
		return factConsistencyRow{}, err
	}
	return factConsistencyRow{Status: status, IndexStatus: indexStatus, Statement: statement}, nil
}

func (s *memoryService) asyncResyncFact(ctx context.Context, factID string) {
	if s.indexSync == nil || s.consistency == nil {
		return
	}
	if !s.resyncFlight.TryStart(factID) {
		return
	}
	syncer := s.indexSync
	consistency := s.consistency
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	safego.Go(bgCtx, "memory.index_resync_on_hit", func() {
		defer cancel()
		defer s.resyncFlight.Done(factID)
		agentID, userID, statement, err := consistency.GetFactResyncRow(bgCtx, factID)
		if err != nil {
			return
		}
		raw, _ := json.Marshal(map[string]string{
			"agent_id":  agentID,
			"user_id":   userID,
			"id":        factID,
			"statement": statement,
		})
		if err := syncer.SyncFactIndexFromRow(bgCtx, raw); err != nil {
			s.lg.Warn("async resync on stale hit failed",
				loggateway.StepID("memory.index_resync"),
				loggateway.Str("fact_id", factID),
				loggateway.Err(err))
		}
	})
}

// enrichVectorHits backfills topics, kind, event_time and other metadata
// from the authoritative SQLite fact rows for vector search hits.
// This ensures SearchMemories returns complete entries regardless of
// whether the hit came from vector similarity or keyword search.
func (s *memoryService) enrichVectorHits(ctx context.Context, uk trpcmemory.UserKey, factIDs []string, hitMap map[string]vecHit) []*trpcmemory.Entry {
	// Fetch fact rows directly by ID to avoid limit/offset truncation
	// that could occur with ListFactRowsForUser.
	rows, err := s.factReader.GetFactRowsByIDs(ctx, factIDs)
	if err != nil {
		return nil
	}
	var out []*trpcmemory.Entry
	for _, raw := range rows {
		e, convErr := factRowToEntry(raw, uk)
		if convErr != nil {
			continue
		}
		if hit, ok := hitMap[e.ID]; ok {
			e.Score = hit.score
		}
		out = append(out, e)
	}
	return out
}

func (s *memoryService) listEntries(ctx context.Context, uk trpcmemory.UserKey, keyword string, limit int, minImportance float64, includeInvalidated bool) ([]*trpcmemory.Entry, error) {
	limit32 := int32(limit)
	if limit32 <= 0 {
		limit32 = defaultListEntriesLimit
	}
	var rows [][]byte
	var err error
	if includeInvalidated {
		rows, err = s.factReader.ListFactRowsForUserAll(ctx, factScopeTypeAgent, uk.AppName, uk.UserID, keyword, limit32, 0)
	} else {
		rows, err = s.factReader.ListFactRowsForUser(ctx, factScopeTypeAgent, uk.AppName, uk.UserID, keyword, limit32, 0)
	}
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(rows))
	out := make([]*trpcmemory.Entry, 0, len(rows))
	for _, raw := range rows {
		if minImportance > 0 {
			var row map[string]any
			if json.Unmarshal(raw, &row) == nil {
				if imp, ok := row["importance"].(float64); ok && imp < minImportance {
					continue
				}
			}
		}
		e, convErr := factRowToEntry(raw, uk)
		if convErr != nil {
			continue
		}
		if e.ID == "" {
			continue
		}
		if _, ok := seen[e.ID]; ok {
			continue
		}
		seen[e.ID] = struct{}{}
		out = append(out, e)
	}
	return out, nil
}

func (s *memoryService) syncIndexBestEffort(ctx context.Context, raw []byte) {
	if s == nil || s.indexSync == nil || len(raw) == 0 {
		return
	}
	if err := s.indexSync.SyncFactIndexFromRow(ctx, raw); err != nil {
		s.lg.Warn("sqlite_adapter index sync failed", loggateway.StepID("memory.index_sync"), loggateway.Err(err))
	}
}

// triggerLinkEvolution asynchronously invokes the link evolver for a newly
// added memory. Best-effort: failures are logged as Warn and do not affect
// the AddMemory result. The evolver runs in a detached context so that
// cancellation of the caller's context (e.g. HTTP request completion) does
// not abort the evolution work. Red line #13: goroutine started via safego.Go.
func (s *memoryService) triggerLinkEvolution(ctx context.Context, uk trpcmemory.UserKey, raw []byte) {
	if s == nil || s.linkEvolver == nil || len(raw) == 0 {
		return
	}
	entry, err := factRowToEntry(raw, uk)
	if err != nil || entry == nil || entry.ID == "" {
		return
	}
	evolver := s.linkEvolver
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), linkEvolutionTimeout)
	safego.Go(bgCtx, "memory.link_evolution_on_add", func() {
		defer cancel()
		if _, err := evolver.EvolveLinks(bgCtx, uk, entry); err != nil {
			s.lg.Warn("async link evolution failed",
				loggateway.StepID("memory.link_evolution"),
				loggateway.Str("fact_id", entry.ID),
				loggateway.Err(err))
		}
	})
}

func trpcFactUpsert(uk trpcmemory.UserKey, id, mem string, topics []string, factKind, createdAt, updatedAt string) biz.FactUpsert {
	mem = strings.TrimSpace(mem)
	return biz.FactUpsert{
		ID:              id,
		ScopeType:       factScopeTypeAgent,
		ScopeID:         strings.TrimSpace(uk.AppName),
		UserID:          strings.TrimSpace(uk.UserID),
		AgentID:         strings.TrimSpace(uk.AppName),
		Statement:       mem,
		DetailsMarkdown: mem,
		FactKind:        factKind,
		TagsJSON:        topicsJSON(topics),
		Confidence:      1.0,
		Importance:      0.5,
		SourceKind:      factSourceTRPC,
		Status:          "active",
		MetadataJSON:    `{"source":"trpc_memory"}`,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
}

func (s *memoryService) Tools() []trpctool.Tool {
	return []trpctool.Tool{
		trpcmemtool.NewAddTool(),
		trpcmemtool.NewUpdateTool(),
		trpcmemtool.NewDeleteTool(),
		trpcmemtool.NewClearTool(),
		trpcmemtool.NewSearchTool(),
		trpcmemtool.NewLoadTool(),
	}
}

func (s *memoryService) Close() error {
	return nil
}

// ProactiveRecall retrieves associated memories based on the conversation
// context (mentioned entities, current topic, user statement) without
// requiring an explicit query. It is intended to be called before each
// conversation turn to surface relevant memories.
//
// Behaviour:
//   - Empty conversation context → returns empty list, no error.
//   - Invalidated memories (Bi-temporal P3-8) are filtered out by default.
//   - Contradiction detection: when UserStatement potentially conflicts
//     with a stored memory, that memory is prioritised (boosted score).
//   - Results are deduplicated by memory ID and sorted by score descending.
func (s *memoryService) ProactiveRecall(ctx context.Context, uk trpcmemory.UserKey,
	convCtx trpcmemory.ConversationContext) ([]*trpcmemory.Entry, error) {
	if s == nil {
		return nil, nil
	}
	if err := s.requireStore(); err != nil {
		return nil, err
	}

	queries := collectProactiveQueries(convCtx)
	if len(queries) == 0 {
		return nil, nil
	}

	// Determine search limits from agent runtime settings.
	topK, _ := resolveMemoryToolSearchLimits(ctx, s.settingsLoader, uk.AppName, 0)
	if topK <= 0 {
		return nil, nil
	}

	// Collect candidate entries from each query, deduplicating by ID.
	seen := make(map[string]*trpcmemory.Entry, topK)
	for _, q := range queries {
		entries, err := s.SearchMemories(ctx, uk, q)
		if err != nil {
			// Degrade gracefully on search errors — log and continue.
			s.lg.Warn("proactive recall search failed",
				loggateway.StepID("memory.proactive_recall"),
				loggateway.Str("query", q),
				loggateway.Err(err))
			continue
		}
		for _, e := range entries {
			if e == nil || e.ID == "" {
				continue
			}
			// Bi-temporal filter: skip invalidated memories.
			if e.ValidUntil != nil && e.ValidUntil.Before(time.Now()) {
				continue
			}
			if existing, ok := seen[e.ID]; ok {
				// Keep the higher score.
				if e.Score > existing.Score {
					seen[e.ID] = e
				}
			} else {
				seen[e.ID] = e
			}
		}
	}

	if len(seen) == 0 {
		return nil, nil
	}

	// Apply contradiction detection: boost entries that share keywords
	// with the user statement (potential conflicts should be surfaced).
	statementKeywords := extractKeywords(convCtx.UserStatement)
	if len(statementKeywords) > 0 {
		for _, e := range seen {
			if e.Memory != nil && hasKeywordOverlap(e.Memory.Memory, statementKeywords) {
				e.Score += contradictionBoost
			}
		}
	}

	// Sort by score descending and cap at topK.
	out := make([]*trpcmemory.Entry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	sortProactiveEntries(out)
	if int32(len(out)) > topK {
		out = out[:topK]
	}
	return out, nil
}

// collectProactiveQueries extracts search keywords from the conversation
// context. Returns nil when the context carries no usable signal.
// For UserStatement, individual keywords are extracted (not the full
// statement) so contradiction detection can find memories sharing
// conceptual keywords (e.g. "live" matches "lives in London").
func collectProactiveQueries(convCtx trpcmemory.ConversationContext) []string {
	var queries []string
	for _, e := range convCtx.MentionedEntities {
		e = strings.TrimSpace(e)
		if e != "" {
			queries = append(queries, e)
		}
	}
	if topic := strings.TrimSpace(convCtx.CurrentTopic); topic != "" {
		queries = append(queries, topic)
	}
	// For contradiction detection, extract keywords from the user statement
	// so we can find potentially conflicting memories by shared concepts.
	for _, kw := range extractKeywords(convCtx.UserStatement) {
		queries = append(queries, kw)
	}
	// Cap the number of queries to avoid excessive DB load.
	if len(queries) > proactiveRecallMaxQueries {
		queries = queries[:proactiveRecallMaxQueries]
	}
	return queries
}

// extractKeywords splits a text into lowercase keyword tokens for overlap
// comparison. Very simple tokenisation (YAGNI: no NLP, no stemming).
func extractKeywords(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	// Split on whitespace and common punctuation.
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '!' || r == '?' ||
			r == ';' || r == ':' || r == '\'' || r == '"' || r == '\n' || r == '\t'
	})
	// Filter out very short tokens (noise).
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) >= minKeywordLen {
			out = append(out, f)
		}
	}
	return out
}

// hasKeywordOverlap returns true if the memory text contains any of the
// provided keywords. Used for contradiction detection.
func hasKeywordOverlap(memoryText string, keywords []string) bool {
	if memoryText == "" || len(keywords) == 0 {
		return false
	}
	low := strings.ToLower(memoryText)
	for _, kw := range keywords {
		if kw != "" && strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

// sortProactiveEntries sorts entries by score descending. Entries with
// equal scores retain their relative order (stable sort).
func sortProactiveEntries(entries []*trpcmemory.Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Score > entries[j].Score
	})
}

// ProactiveRecallAdapter wraps a trpcmemory.Service and exposes it as a
// biz.ProactiveRecaller. This adapter is necessary because the framework
// Service.ProactiveRecall and biz.ProactiveRecaller.ProactiveRecall methods
// have different signatures (framework uses UserKey + ConversationContext,
// biz uses agentID/userID + ProactiveRecallContext), and Go does not allow
// two methods with the same name on the same type.
type ProactiveRecallAdapter struct {
	svc trpcmemory.Service
}

// NewProactiveRecallAdapter creates a biz.ProactiveRecaller backed by the
// given framework memory Service. Returns nil if svc is nil.
func NewProactiveRecallAdapter(svc trpcmemory.Service) biz.ProactiveRecaller {
	if svc == nil {
		return nil
	}
	return &ProactiveRecallAdapter{svc: svc}
}

// ProactiveRecall implements biz.ProactiveRecaller. It converts biz-level
// types to framework types and delegates to the framework ProactiveRecall
// method. Results are converted to biz.CompositeRecallHit for consumption
// by the composite recall usecase.
func (a *ProactiveRecallAdapter) ProactiveRecall(ctx context.Context, agentID, userID string, convCtx biz.ProactiveRecallContext) ([]biz.CompositeRecallHit, error) {
	if a == nil || a.svc == nil {
		return nil, nil
	}
	uk := trpcmemory.UserKey{AppName: agentID, UserID: userID}
	entries, err := a.svc.ProactiveRecall(ctx, uk, trpcmemory.ConversationContext{
		MentionedEntities: convCtx.MentionedEntities,
		CurrentTopic:      convCtx.CurrentTopic,
		UserStatement:     convCtx.UserStatement,
	})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	out := make([]biz.CompositeRecallHit, 0, len(entries))
	for _, e := range entries {
		if e == nil || e.Memory == nil {
			continue
		}
		out = append(out, biz.CompositeRecallHit{
			Layer: "L3",
			Line:  e.Memory.Memory,
			Score: e.Score,
		})
	}
	return out, nil
}

func (s *memoryService) EnqueueAutoMemoryJob(ctx context.Context, sess *session.Session) error {
	if s == nil || sess == nil || s.autoMemoryQueue == nil {
		return nil
	}
	// Respect context cancellation — do not enqueue if the caller's context
	// is already done (e.g. client disconnected or timeout exceeded).
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.autoMemoryQueue.Enqueue(AutoMemoryJobRequest{
		AppName:   sess.AppName,
		SessionID: sess.ID,
		UserID:    sess.UserID,
		Priority:  MemoryJobPriorityNormal,
		TenantID:  sess.AppName,
	})
	return nil
}

func topicsJSON(topics []string) string {
	if len(topics) == 0 {
		return "[]"
	}
	filtered := make([]string, 0, len(topics))
	for _, t := range topics {
		t = strings.TrimSpace(t)
		if t != "" {
			filtered = append(filtered, t)
		}
	}
	b, _ := json.Marshal(filtered)
	return string(b)
}

type factRow struct {
	ID           string  `json:"id"`
	Statement    string  `json:"statement"`
	TagsJSON     string  `json:"tags_json"`
	Importance   float64 `json:"importance"`
	MetadataJSON string  `json:"metadata_json"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	ValidFrom    string  `json:"valid_from"`
	ValidUntil   string  `json:"valid_until"`
	LinksJSON    string  `json:"links"`
	KeywordsJSON string  `json:"keywords"`
}

func factRowToEntry(raw []byte, uk trpcmemory.UserKey) (*trpcmemory.Entry, error) {
	var row factRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	topics := decodeTagsJSON(row.TagsJSON)
	if len(topics) == 0 {
		topics = decodeMetadataTopics(row.MetadataJSON)
	}
	memText := strings.TrimSpace(row.Statement)
	if memText == "" {
		return nil, errors.New("empty fact statement")
	}
	now := time.Now()
	lastUpdated := parseRFC3339(row.UpdatedAt, now)
	createdAt := parseRFC3339(row.CreatedAt, now)
	var validFrom, validUntil *time.Time
	if vf := strings.TrimSpace(row.ValidFrom); vf != "" {
		if t, err := time.Parse(time.RFC3339Nano, vf); err == nil {
			validFrom = &t
		} else if t, err := time.Parse(time.RFC3339, vf); err == nil {
			validFrom = &t
		}
	}
	if vu := strings.TrimSpace(row.ValidUntil); vu != "" {
		if t, err := time.Parse(time.RFC3339Nano, vu); err == nil {
			validUntil = &t
		} else if t, err := time.Parse(time.RFC3339, vu); err == nil {
			validUntil = &t
		}
	}
	return &trpcmemory.Entry{
		ID:      row.ID,
		AppName: uk.AppName,
		Memory: &trpcmemory.Memory{
			Memory:      memText,
			Topics:      topics,
			LastUpdated: &lastUpdated,
			ValidFrom:   validFrom,
			ValidUntil:  validUntil,
		},
		UserID:     uk.UserID,
		CreatedAt:  createdAt,
		UpdatedAt:  lastUpdated,
		Score:      row.Importance,
		ValidFrom:  validFrom,
		ValidUntil: validUntil,
		Links:      decodeTagsJSON(row.LinksJSON),
		Keywords:   decodeTagsJSON(row.KeywordsJSON),
		Tags:       topics,
	}, nil
}

func parseRFC3339(s string, fallback time.Time) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s)); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(s)); err == nil {
		return t
	}
	return fallback
}

func decodeTagsJSON(tags string) []string {
	tags = strings.TrimSpace(tags)
	if tags == "" || tags == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(tags), &out); err != nil {
		return nil
	}
	return out
}

func decodeMetadataTopics(meta string) []string {
	meta = strings.TrimSpace(meta)
	if meta == "" || meta == "[]" || meta == "{}" {
		return nil
	}
	var topics []string
	if err := json.Unmarshal([]byte(meta), &topics); err != nil {
		var m map[string]any
		if err2 := json.Unmarshal([]byte(meta), &m); err2 == nil {
			if t, ok := m["topics"]; ok {
				if arr, ok := t.([]any); ok {
					for _, v := range arr {
						if s, ok := v.(string); ok {
							topics = append(topics, s)
						}
					}
				}
			}
		}
	}
	return topics
}
