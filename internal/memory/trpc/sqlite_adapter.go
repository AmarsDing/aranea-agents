package trpcmem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/pgvector"
	"aranea-agents/internal/data/sessionmemory"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmemtool "trpc.group/trpc-go/trpc-agent-go/memory/tool"
	"trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	factScopeTypeAgent = "agent"
	factSourceTRPC     = "trpc_memory"
)

type sqliteMemoryService struct {
	store           *sessionmemory.Store
	indexSync       biz.MemoryFactIndexSyncer
	autoMemoryQueue AutoMemoryQueue
	vector          vectorFactSearcher
	settingsLoader  AgentRuntimeSettingsLoader
}

// vectorFactSearcher optional pgvector recall for SearchMemories.
type vectorFactSearcher interface {
	RecallWithUser(ctx context.Context, agentID, userID, query string, topK int) ([]*biz.AgentMemory, error)
}

var _ trpcmemory.Service = (*sqliteMemoryService)(nil)

func NewSQLiteMemoryService(store *sessionmemory.Store, indexSync biz.MemoryFactIndexSyncer, queue AutoMemoryQueue, vector vectorFactSearcher, settingsLoader AgentRuntimeSettingsLoader) trpcmemory.Service {
	if store == nil {
		return nil
	}
	return &sqliteMemoryService{
		store:           store,
		indexSync:       indexSync,
		autoMemoryQueue: queue,
		vector:          vector,
		settingsLoader:  settingsLoader,
	}
}

func (s *sqliteMemoryService) requireStore() error {
	if s == nil || s.store == nil {
		return errors.New("sqlite memory service: store not wired")
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
	// Empty id: UpsertFactRow dedupes via fingerprint(scope, statement).
	raw, err := s.store.UpsertFactRow(ctx, trpcFactUpsert(uk, "", mem, topics, factKind, now, now))
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
	raw, err := s.store.UpsertFactRow(ctx, trpcFactUpsert(uk, id, mem, topics, "fact", now, now))
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
	return s.store.DeleteFactByID(ctx, mk.MemoryID)
}

func (s *sqliteMemoryService) ClearMemories(ctx context.Context, uk trpcmemory.UserKey) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	return s.store.ClearFacts(ctx, factScopeTypeAgent, uk.AppName, uk.UserID)
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
				now := time.Now()
				out = append(out, &trpcmemory.Entry{
					ID:      entryID,
					AppName: uk.AppName,
					UserID:  uk.UserID,
					Memory: &trpcmemory.Memory{
						Memory:      memText,
						LastUpdated: &now,
					},
					Score: 1.0,
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

func (s *sqliteMemoryService) listEntries(ctx context.Context, uk trpcmemory.UserKey, keyword string, limit int, minImportance float64) ([]*trpcmemory.Entry, error) {
	limit32 := int32(limit)
	if limit32 <= 0 {
		limit32 = 50
	}
	rows, err := s.store.ListFactRowsForUser(ctx, factScopeTypeAgent, uk.AppName, uk.UserID, keyword, limit32, 0)
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
	_ = s.indexSync.SyncFactIndexFromRow(ctx, raw)
}

func trpcFactUpsert(uk trpcmemory.UserKey, id, mem string, topics []string, factKind, createdAt, updatedAt string) sessionmemory.MemoryFactUpsert {
	mem = strings.TrimSpace(mem)
	return sessionmemory.MemoryFactUpsert{
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
	parts := make([]string, 0, len(topics))
	for _, t := range topics {
		t = strings.TrimSpace(t)
		if t != "" {
			parts = append(parts, `"`+t+`"`)
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

type factRow struct {
	ID          string  `json:"id"`
	Statement   string  `json:"statement"`
	TagsJSON    string  `json:"tags_json"`
	Importance  float64 `json:"importance"`
	MetadataJSON string `json:"metadata_json"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
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
