package trpcmem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/pgvector"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmemtool "trpc.group/trpc-go/trpc-agent-go/memory/tool"
	"trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	factScopeTypeAgent = "agent"
	factSourceTRPC     = "trpc_memory"
)

var memoryReadConsistencyCheck = os.Getenv("MEMORY_READ_CONSISTENCY_CHECK") == "1"

type sqliteMemoryService struct {
	data            *data.Data
	factWriter      biz.L3FactWriter
	indexSync       biz.MemoryFactIndexSyncer
	autoMemoryQueue AutoMemoryQueue
	vector          vectorFactSearcher
	settingsLoader  AgentRuntimeSettingsLoader
	resyncFlight    singleflightGroup
	lg              loggateway.Logger
}

// vectorFactSearcher optional pgvector recall for SearchMemories.
type vectorFactSearcher interface {
	RecallWithUser(ctx context.Context, agentID, userID, query string, topK int) ([]*biz.AgentMemory, error)
}

var _ trpcmemory.Service = (*sqliteMemoryService)(nil)

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

func NewSQLiteMemoryService(d *data.Data, factWriter biz.L3FactWriter, indexSync biz.MemoryFactIndexSyncer, queue AutoMemoryQueue, vector vectorFactSearcher, settingsLoader AgentRuntimeSettingsLoader, lg loggateway.Logger) trpcmemory.Service {
	if d == nil {
		return nil
	}
	return &sqliteMemoryService{
		data:            d,
		factWriter:      factWriter,
		indexSync:       indexSync,
		autoMemoryQueue: queue,
		vector:          vector,
		settingsLoader:  settingsLoader,
		lg:              lg,
	}
}

func (s *sqliteMemoryService) requireStore() error {
	if s == nil || s.data == nil {
		return errors.New("sqlite memory service: data not wired")
	}
	return nil
}

func (s *sqliteMemoryService) AddMemory(ctx context.Context, uk trpcmemory.UserKey, mem string, topics []string, opts ...trpcmemory.AddOption) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	meta := trpcmemory.ResolveAddOptions(opts)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	factKind := "fact"
	if meta != nil && meta.Kind != "" {
		factKind = string(meta.Kind)
	}
	raw, err := s.factWriter.UpsertFactRow(ctx, trpcFactUpsert(uk, "", mem, topics, factKind, now, now))
	if err != nil {
		return err
	}
	s.syncIndexBestEffort(ctx, raw)
	return nil
}

func (s *sqliteMemoryService) UpdateMemory(ctx context.Context, mk trpcmemory.Key, mem string, topics []string, opts ...trpcmemory.UpdateOption) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := strings.TrimSpace(mk.MemoryID)
	if id == "" {
		return errors.New("memory id is required")
	}
	uk := trpcmemory.UserKey{AppName: mk.AppName, UserID: mk.UserID}
	raw, err := s.factWriter.UpsertFactRow(ctx, trpcFactUpsert(uk, id, mem, topics, "fact", now, now))
	if err != nil {
		return err
	}
	s.syncIndexBestEffort(ctx, raw)
	return nil
}

func (s *sqliteMemoryService) DeleteMemory(ctx context.Context, mk trpcmemory.Key) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	return s.factWriter.DeleteFactRow(ctx, mk.MemoryID)
}

func (s *sqliteMemoryService) ClearMemories(ctx context.Context, uk trpcmemory.UserKey) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	// Delete all facts for this agent+user
	_, err := s.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_facts SET deleted_at = ?, status = 'deleted' WHERE scope_type = ? AND scope_id = ? AND user_id = ? AND status = 'active' AND deleted_at = ''`,
		time.Now().UTC().Format(time.RFC3339Nano), factScopeTypeAgent, uk.AppName, uk.UserID)
	return err
}

func (s *sqliteMemoryService) ReadMemories(ctx context.Context, uk trpcmemory.UserKey, limit int) ([]*trpcmemory.Entry, error) {
	if s == nil {
		return nil, nil
	}
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	defaultLimit, _ := resolveMemoryToolSearchLimits(ctx, s.settingsLoader, uk.AppName, 0)
	if limit <= 0 {
		limit = int(defaultLimit)
	}
	return s.listEntries(ctx, uk, "", limit, 0)
}

func (s *sqliteMemoryService) SearchMemories(ctx context.Context, uk trpcmemory.UserKey, query string, opts ...trpcmemory.SearchOption) ([]*trpcmemory.Entry, error) {
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
			out := make([]*trpcmemory.Entry, 0, len(hits))
			for _, h := range hits {
				if h == nil || strings.TrimSpace(h.Content) == "" {
					continue
				}
				factID, memText := pgvector.ParseFactVectorContent(h.Content)
				if memText == "" {
					memText = strings.TrimSpace(h.Content)
				}
				entryID := factID
				if entryID == "" {
					entryID = fmt.Sprintf("%d", h.ID)
				}
				if memoryReadConsistencyCheck && factID != "" && s.data != nil {
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
					} else if row.Statement != "" && row.Statement != memText {
						memText = row.Statement
					}
				}
				now := time.Now()
				out = append(out, &trpcmemory.Entry{
					ID:      entryID,
					AppName: uk.AppName,
					UserID:  uk.UserID,
					Memory: &trpcmemory.Memory{
						Memory:      memText,
						LastUpdated: &now,
					},
					Score: h.Score,
				})
			}
			if len(out) > 0 {
				return out, nil
			}
		}
	}

	entries, err := s.listEntries(ctx, uk, q, int(topK), minScore)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return s.ReadMemories(ctx, uk, int(topK))
	}
	return entries, nil
}

type factConsistencyRow struct {
	Status      string
	IndexStatus string
	Statement   string
}

func (s *sqliteMemoryService) getFactConsistencyRow(ctx context.Context, factID string) (factConsistencyRow, error) {
	var row factConsistencyRow
	err := data.QueryRowScan(ctx, s.data.RWDB().ReadDB(ctx),
		`SELECT status, embedding_status, statement FROM memory_facts WHERE id = ?`,
		[]any{factID}, &row.Status, &row.IndexStatus, &row.Statement)
	return row, err
}

func (s *sqliteMemoryService) asyncResyncFact(ctx context.Context, factID string) {
	if s.indexSync == nil || s.data == nil {
		return
	}
	if !s.resyncFlight.TryStart(factID) {
		return
	}
	syncer := s.indexSync
	d := s.data
	bgCtx := context.WithoutCancel(ctx)
	safego.Go(bgCtx, "memory.index_resync_on_hit", func() {
		defer s.resyncFlight.Done(factID)
		// Read raw fact row
		rows, err := d.RWDB().ReadDB(bgCtx).QueryContext(bgCtx,
			`SELECT id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
			statement, statement_normalized, fingerprint, details_markdown,
			fact_kind, tags_json,
			confidence, importance, use_count, hit_count,
			positive_feedback_count, negative_feedback_count, conflict_count,
			source_kind, source_episode_id, source_session_id, source_message_id, source_external,
			version, status, superseded_by,
			embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
			pii_flag, redacted_statement,
			ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
			metadata_json, quality_score, created_at, updated_at, archived_at, deleted_at
			FROM memory_facts WHERE id = ?`, factID)
		if err != nil || !rows.Next() {
			if rows != nil {
				rows.Close()
			}
			return
		}
		var (
			id, stype, sid, wid, uid, tid, aid string
			stmt, snorm, fp, details           string
			fkind, tags                        string
			conf, imp                          float64
			uc, hc, pfc, nfc, cc               int
			srcKind, epID, sessID, msgID, ext  string
			ver                                int
			st, sup                            string
			embSt, embModel                    string
			embDim                             int
			embBlob                            []byte
			embNorm                            float64
			pii                                int
			redacted                           string
			ttlD                               int
			decay                              float64
			nextD, lastU, exp                  string
			meta, ca, ua, arch, del            string
			qScore                             float64
		)
		err = rows.Scan(
			&id, &stype, &sid, &wid, &uid, &tid, &aid,
			&stmt, &snorm, &fp, &details,
			&fkind, &tags,
			&conf, &imp, &uc, &hc, &pfc, &nfc, &cc,
			&srcKind, &epID, &sessID, &msgID, &ext,
			&ver, &st, &sup,
			&embSt, &embModel, &embDim, &embBlob, &embNorm,
			&pii, &redacted,
			&ttlD, &decay, &nextD, &lastU, &exp,
			&meta, &qScore, &ca, &ua, &arch, &del,
		)
		rows.Close()
		if err != nil {
			return
		}
		// Build raw JSON
		m := map[string]any{
			"id": id, "scope_type": stype, "scope_id": sid, "workspace_id": wid,
			"user_id": uid, "team_id": tid, "agent_id": aid,
			"statement": stmt, "statement_normalized": snorm, "fingerprint": fp,
			"details_markdown": details, "fact_kind": fkind, "tags_json": tags,
			"confidence": conf, "importance": imp,
			"use_count": uc, "hit_count": hc,
			"positive_feedback_count": pfc, "negative_feedback_count": nfc, "conflict_count": cc,
			"source_kind": srcKind, "source_episode_id": epID,
			"source_session_id": sessID, "source_message_id": msgID, "source_external": ext,
			"version": ver, "status": st, "superseded_by": sup,
			"embedding_status": embSt, "embedding_model": embModel, "embedding_dim": embDim,
			"embedding_norm":     embNorm,
			"pii_flag":           pii != 0,
			"redacted_statement": redacted,
			"ttl_days":           ttlD, "decay_factor": decay,
			"next_decay_at": nextD, "last_used_at": lastU, "expires_at": exp,
			"metadata_json": meta, "quality_score": qScore, "created_at": ca, "updated_at": ua,
			"archived_at": arch, "deleted_at": del,
		}
		raw, _ := json.Marshal(m)
		if err := syncer.SyncFactIndexFromRow(bgCtx, raw); err != nil {
			s.lg.Warn("async resync on stale hit failed",
				loggateway.StepID("memory.index_resync"),
				loggateway.Str("fact_id", factID),
				loggateway.Err(err))
		}
	})
}

func (s *sqliteMemoryService) listEntries(ctx context.Context, uk trpcmemory.UserKey, keyword string, limit int, minImportance float64) ([]*trpcmemory.Entry, error) {
	limit32 := int32(limit)
	if limit32 <= 0 {
		limit32 = 50
	}
	// Use L3FactReader to list facts
	reader := data.NewL3FactReaderForUser(s.data)
	rows, err := reader.ListFactRowsForUser(ctx, factScopeTypeAgent, uk.AppName, uk.UserID, keyword, limit32, 0)
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
		key := strings.ToLower(strings.TrimSpace(e.Memory.Memory))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, e)
	}
	return out, nil
}

func (s *sqliteMemoryService) syncIndexBestEffort(ctx context.Context, raw []byte) {
	if s == nil || s.indexSync == nil || len(raw) == 0 {
		return
	}
	if err := s.indexSync.SyncFactIndexFromRow(ctx, raw); err != nil {
		s.lg.Warn("sqlite_adapter index sync failed", loggateway.StepID("memory.index_sync"), loggateway.Err(err))
	}
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

func (s *sqliteMemoryService) Tools() []trpctool.Tool {
	return []trpctool.Tool{
		trpcmemtool.NewAddTool(),
		trpcmemtool.NewUpdateTool(),
		trpcmemtool.NewDeleteTool(),
		trpcmemtool.NewClearTool(),
		trpcmemtool.NewSearchTool(),
		trpcmemtool.NewLoadTool(),
	}
}

func (s *sqliteMemoryService) Close() error {
	return nil
}

func (s *sqliteMemoryService) EnqueueAutoMemoryJob(_ context.Context, sess *session.Session) error {
	if s == nil || sess == nil || s.autoMemoryQueue == nil {
		return nil
	}
	s.autoMemoryQueue.Enqueue(AutoMemoryJobRequest{
		AppName:   sess.AppName,
		SessionID: sess.ID,
		UserID:    sess.UserID,
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
	return &trpcmemory.Entry{
		ID:      row.ID,
		AppName: uk.AppName,
		Memory: &trpcmemory.Memory{
			Memory:      memText,
			Topics:      topics,
			LastUpdated: &lastUpdated,
		},
		UserID:    uk.UserID,
		CreatedAt: createdAt,
		UpdatedAt: lastUpdated,
		Score:     row.Importance,
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
