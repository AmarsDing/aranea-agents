package adkadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/data/sessionmemory"

	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// SessionSQLiteMemoryService bridges Aranea SQLite session-memory tables to [memory.Service].
type SessionSQLiteMemoryService struct {
	store *sessionmemory.Store
}

var _ memory.Service = (*SessionSQLiteMemoryService)(nil)

// NewSessionSQLiteMemoryService wraps Ent-backed session memory for ADK load_memory / preload_memory.
// Returns nil if store is nil (caller should fall back to [NewADKMemoryService]).
func NewSessionSQLiteMemoryService(store *sessionmemory.Store) memory.Service {
	if store == nil {
		return nil
	}
	return &SessionSQLiteMemoryService{store: store}
}

// AddSessionToMemory is a no-op until full session→memory ingestion is wired.
func (s *SessionSQLiteMemoryService) AddSessionToMemory(ctx context.Context, sess session.Session) error {
	_, _ = ctx, sess
	return nil
}

// SearchMemory returns entity rows loosely matching the query (keyword filter on memory_entities).
func (s *SessionSQLiteMemoryService) SearchMemory(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
	if req == nil || s.store == nil {
		return &memory.SearchResponse{Memories: nil}, nil
	}
	q := strings.TrimSpace(req.Query)
	rows, _, err := s.store.ListEntityRows(ctx, "", "", "", "", "", "active", q, 20, 0)
	if err != nil {
		return nil, err
	}
	var entries []memory.Entry
	for i, row := range rows {
		var m map[string]any
		if err := json.Unmarshal(row, &m); err != nil {
			continue
		}
		name, _ := m["name"].(string)
		desc, _ := m["description"].(string)
		body := strings.TrimSpace(name + "\n" + desc)
		if body == "" {
			body = string(row)
		}
		id := jsonString(m, "id")
		if id == "" {
			id = fmt.Sprintf("mem-%d", i)
		}
		etype := jsonString(m, "entity_type")
		if etype == "" {
			etype = "memory"
		}
		entries = append(entries, memory.Entry{
			ID:      id,
			Content: genai.NewContentFromText(body, genai.RoleUser),
			Author:  etype,
		})
	}
	return &memory.SearchResponse{Memories: entries}, nil
}

func jsonString(m map[string]any, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}
