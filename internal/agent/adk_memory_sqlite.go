package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/provider"

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

// AddSessionToMemory persists assistant/user-visible text from the session snapshot into memory_entities (keyword recall).
func (s *SessionSQLiteMemoryService) AddSessionToMemory(ctx context.Context, sess session.Session) error {
	if ctx.Err() != nil || s == nil || s.store == nil || sess == nil {
		return nil
	}
	var idx int
	for ev := range sess.Events().All() {
		if ev == nil || ev.LLMResponse.Partial {
			continue
		}
		main, rsn := provider.TextsFromLLMResponse(&ev.LLMResponse)
		desc := strings.TrimSpace(main)
		if rsn != "" {
			if desc != "" {
				desc += "\n\n"
			}
			desc += "[reasoning]\n" + strings.TrimSpace(rsn)
		}
		desc = strings.TrimSpace(desc)
		if desc == "" {
			continue
		}
		eid := strings.TrimSpace(ev.ID)
		if eid == "" {
			eid = fmt.Sprintf("idx-%d", idx)
		}
		idx++

		entityID := fmt.Sprintf("adkevt:%s:%s", sess.ID(), eid)
		nameNorm := normalizeMemoryEntityKey(eid)
		metaObj := map[string]any{"event_id": eid, "author": strings.TrimSpace(ev.Author)}
		metaJSON, _ := json.Marshal(metaObj)

		name := strings.TrimSpace(ev.Author)
		if name == "" {
			name = "message"
		}

		evTime := ""
		if !ev.Timestamp.IsZero() {
			evTime = ev.Timestamp.UTC().Format(time.RFC3339)
		}

		params := sessionmemory.ADKEventEntityParams{
			ID:               entityID,
			ScopeType:        "",
			ScopeID:          sess.ID(),
			UserID:           sess.UserID(),
			EntityType:       "",
			Name:             name,
			NameNormalized:   nameNorm,
			Description:      desc,
			Importance:       0.5,
			Confidence:       0.7,
			UseCount:         0,
			MetadataJSON:     string(metaJSON),
			CreatedAtRFC3339: evTime,
			UpdatedAtRFC3339: evTime,
		}
		if err := s.store.UpsertADKEventEntity(ctx, params); err != nil {
			return err
		}
	}
	return nil
}

func normalizeMemoryEntityKey(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "event"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "event"
	}
	return out
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
		id := jsonStringMem(m, "id")
		if id == "" {
			id = fmt.Sprintf("mem-%d", i)
		}
		etype := jsonStringMem(m, "entity_type")
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

func jsonStringMem(m map[string]any, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}
