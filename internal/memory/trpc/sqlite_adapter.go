package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/data/sessionmemory"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmemtool "trpc.group/trpc-go/trpc-agent-go/memory/tool"
	"trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type sqliteMemoryService struct {
	store *sessionmemory.Store
	mu    sync.RWMutex
	cache map[string]map[string]*trpcmemory.Entry
}

var _ trpcmemory.Service = (*sqliteMemoryService)(nil)

func NewSQLiteMemoryService(store *sessionmemory.Store) trpcmemory.Service {
	if store == nil {
		return nil
	}
	return &sqliteMemoryService{
		store: store,
		cache: map[string]map[string]*trpcmemory.Entry{},
	}
}

func (s *sqliteMemoryService) AddMemory(ctx context.Context, uk trpcmemory.UserKey, mem string, topics []string, opts ...trpcmemory.AddOption) error {
	if s == nil || s.store == nil {
		return nil
	}
	meta := trpcmemory.ResolveAddOptions(opts)
	now := time.Now()
	id := fmt.Sprintf("mem-%d", now.UnixNano())
	entry := &trpcmemory.Entry{
		ID:      id,
		AppName: uk.AppName,
		Memory: &trpcmemory.Memory{
			Memory:      mem,
			Topics:      topics,
			LastUpdated: &now,
		},
		UserID:    uk.UserID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if meta != nil {
		entry.Memory.Kind = meta.Kind
		entry.Memory.EventTime = meta.EventTime
		entry.Memory.Participants = meta.Participants
		entry.Memory.Location = meta.Location
	}
	s.mu.Lock()
	key := cacheKey(uk)
	if s.cache[key] == nil {
		s.cache[key] = map[string]*trpcmemory.Entry{}
	}
	s.cache[key][id] = entry
	s.mu.Unlock()

	params := sessionmemory.ADKEventEntityParams{
		ID:               id,
		ScopeType:        "trpc_memory",
		ScopeID:          uk.AppName,
		UserID:           uk.UserID,
		EntityType:       "memory_fact",
		Name:             truncate(mem, 200),
		NameNormalized:   strings.ToLower(truncate(mem, 200)),
		Description:      mem,
		Importance:       0.5,
		Confidence:       1.0,
		MetadataJSON:     topicsJSON(topics),
		CreatedAtRFC3339: now.Format(time.RFC3339),
		UpdatedAtRFC3339: now.Format(time.RFC3339),
	}
	return s.store.UpsertADKEventEntity(ctx, params)
}

func (s *sqliteMemoryService) UpdateMemory(ctx context.Context, mk trpcmemory.Key, mem string, topics []string, opts ...trpcmemory.UpdateOption) error {
	if s == nil || s.store == nil {
		return nil
	}
	uk := trpcmemory.UserKey{AppName: mk.AppName, UserID: mk.UserID}
	s.mu.Lock()
	key := cacheKey(uk)
	entries := s.cache[key]
	if entries != nil {
		if e, ok := entries[mk.MemoryID]; ok {
			e.Memory.Memory = mem
			e.Memory.Topics = topics
			now := time.Now()
			e.Memory.LastUpdated = &now
			e.UpdatedAt = now
		}
	}
	s.mu.Unlock()

	params := sessionmemory.ADKEventEntityParams{
		ID:               mk.MemoryID,
		ScopeType:        "trpc_memory",
		ScopeID:          mk.AppName,
		UserID:           mk.UserID,
		EntityType:       "memory_fact",
		Name:             truncate(mem, 200),
		NameNormalized:   strings.ToLower(truncate(mem, 200)),
		Description:      mem,
		MetadataJSON:     topicsJSON(topics),
		UpdatedAtRFC3339: time.Now().Format(time.RFC3339),
	}
	return s.store.UpsertADKEventEntity(ctx, params)
}

func (s *sqliteMemoryService) DeleteMemory(_ context.Context, mk trpcmemory.Key) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	key := cacheKey(trpcmemory.UserKey{AppName: mk.AppName, UserID: mk.UserID})
	if entries := s.cache[key]; entries != nil {
		delete(entries, mk.MemoryID)
	}
	s.mu.Unlock()
	return nil
}

func (s *sqliteMemoryService) ClearMemories(_ context.Context, uk trpcmemory.UserKey) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	key := cacheKey(uk)
	s.cache[key] = map[string]*trpcmemory.Entry{}
	s.mu.Unlock()
	return nil
}

func (s *sqliteMemoryService) ReadMemories(_ context.Context, uk trpcmemory.UserKey, limit int) ([]*trpcmemory.Entry, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := cacheKey(uk)
	entries := s.cache[key]
	if len(entries) == 0 {
		return nil, nil
	}
	out := make([]*trpcmemory.Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *sqliteMemoryService) SearchMemories(ctx context.Context, uk trpcmemory.UserKey, query string, opts ...trpcmemory.SearchOption) ([]*trpcmemory.Entry, error) {
	if s == nil {
		return nil, nil
	}
	searchOpts := trpcmemory.ResolveSearchOptions(query, opts)
	_ = searchOpts
	rows, _, err := s.store.ListEntityRows(ctx, "trpc_memory", uk.AppName, "", uk.UserID, "memory_fact", "", strings.TrimSpace(query), 20, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return s.ReadMemories(ctx, uk, 10)
	}
	var out []*trpcmemory.Entry
	for _, raw := range rows {
		e, convErr := entityRowToEntry(raw, uk)
		if convErr != nil {
			continue
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return s.ReadMemories(ctx, uk, 10)
	}
	return out, nil
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

func (s *sqliteMemoryService) EnqueueAutoMemoryJob(_ context.Context, _ *session.Session) error {
	return nil
}

func (s *sqliteMemoryService) Close() error {
	return nil
}

func cacheKey(uk trpcmemory.UserKey) string {
	return uk.AppName + ":" + uk.UserID
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
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

type entityRow struct {
	ID           string  `json:"id"`
	ScopeID      string  `json:"scope_id"`
	UserID       string  `json:"user_id"`
	EntityType   string  `json:"entity_type"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Importance   float64 `json:"importance"`
	MetadataJSON string  `json:"metadata_json"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

func entityRowToEntry(raw []byte, uk trpcmemory.UserKey) (*trpcmemory.Entry, error) {
	var row entityRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	topics := decodeMetadataTopics(row.MetadataJSON)
	now := time.Now()
	lastUpdated := &now
	if t, err := time.Parse(time.RFC3339, row.UpdatedAt); err == nil {
		lastUpdated = &t
	}
	createdAt := now
	if t, err := time.Parse(time.RFC3339, row.CreatedAt); err == nil {
		createdAt = t
	}
	updatedAt := now
	if t, err := time.Parse(time.RFC3339, row.UpdatedAt); err == nil {
		updatedAt = t
	}
	return &trpcmemory.Entry{
		ID:      row.ID,
		AppName: uk.AppName,
		Memory: &trpcmemory.Memory{
			Memory:      row.Description,
			Topics:      topics,
			LastUpdated: lastUpdated,
		},
		UserID:    uk.UserID,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Score:     row.Importance,
	}, nil
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
