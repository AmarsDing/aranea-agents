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

// AutoMemoryJobRequest is exported so the cron worker can type-assert queue items.
type AutoMemoryJobRequest struct {
	AppName    string
	SessionID  string
	UserID     string
	EnqueuedAt time.Time
}

// globalAutoMemoryQueue is an in-process queue for auto-memory extraction jobs.
// The cron worker (internal/cronrunner/jobs/auto_memory.go) drains this queue.
var autoMemoryQueue = &memoryJobQueue{ch: make(chan AutoMemoryJobRequest, 256)}

var recentAutoMemoryEnqueue sync.Map // sessionID -> time.Time

type memoryJobQueue struct {
	ch chan AutoMemoryJobRequest
}

// EnqueueAutoMemory schedules a job on the global queue (deduped within 30s per session).
func EnqueueAutoMemory(r AutoMemoryJobRequest) {
	GlobalAutoMemoryQueue().enqueue(r)
}

func (q *memoryJobQueue) enqueue(r AutoMemoryJobRequest) {
	if r.EnqueuedAt.IsZero() {
		r.EnqueuedAt = time.Now()
	}
	if sid := strings.TrimSpace(r.SessionID); sid != "" {
		if t, ok := recentAutoMemoryEnqueue.Load(sid); ok {
			if time.Since(t.(time.Time)) < 30*time.Second {
				return
			}
		}
		recentAutoMemoryEnqueue.Store(sid, time.Now())
	}
	select {
	case q.ch <- r:
	default:
		// Drop silently when channel is full; auto-memory is best-effort.
	}
}

func (q *memoryJobQueue) Chan() <-chan AutoMemoryJobRequest { return q.ch }

// GlobalAutoMemoryQueue exposes the process-wide auto-memory job queue for the cron worker.
func GlobalAutoMemoryQueue() *memoryJobQueue { return autoMemoryQueue }

type sqliteMemoryService struct {
	store *sessionmemory.Store
}

var _ trpcmemory.Service = (*sqliteMemoryService)(nil)

func NewSQLiteMemoryService(store *sessionmemory.Store) trpcmemory.Service {
	if store == nil {
		return nil
	}
	return &sqliteMemoryService{
		store: store,
	}
}

func (s *sqliteMemoryService) AddMemory(ctx context.Context, uk trpcmemory.UserKey, mem string, topics []string, opts ...trpcmemory.AddOption) error {
	if s == nil || s.store == nil {
		return nil
	}
	meta := trpcmemory.ResolveAddOptions(opts)
	now := time.Now()
	id := fmt.Sprintf("mem-%d", now.UnixNano())
	var kind string
	if meta != nil {
		kind = string(meta.Kind)
	}
	params := sessionmemory.EventEntityParams{
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
	_ = kind
	return s.store.UpsertEventEntity(ctx, params)
}

func (s *sqliteMemoryService) UpdateMemory(ctx context.Context, mk trpcmemory.Key, mem string, topics []string, opts ...trpcmemory.UpdateOption) error {
	if s == nil || s.store == nil {
		return nil
	}
	params := sessionmemory.EventEntityParams{
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
	return s.store.UpsertEventEntity(ctx, params)
}

func (s *sqliteMemoryService) DeleteMemory(ctx context.Context, mk trpcmemory.Key) error {
	if s == nil {
		return nil
	}
	return s.store.DeleteEventEntityByID(ctx, mk.MemoryID)
}

func (s *sqliteMemoryService) ClearMemories(ctx context.Context, uk trpcmemory.UserKey) error {
	if s == nil {
		return nil
	}
	return s.store.ClearMemoryEntities(ctx, "trpc_memory", uk.AppName, uk.UserID)
}

func (s *sqliteMemoryService) ReadMemories(ctx context.Context, uk trpcmemory.UserKey, limit int) ([]*trpcmemory.Entry, error) {
	if s == nil {
		return nil, nil
	}
	limit32 := int32(limit)
	if limit32 <= 0 {
		limit32 = 50
	}
	rows, _, err := s.store.ListEntityRows(ctx, "trpc_memory", uk.AppName, "", uk.UserID, "memory_fact", "", "", limit32, 0)
	if err != nil {
		return nil, err
	}
	var out []*trpcmemory.Entry
	for _, raw := range rows {
		e, convErr := entityRowToEntry(raw, uk)
		if convErr != nil {
			continue
		}
		out = append(out, e)
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

func (s *sqliteMemoryService) EnqueueAutoMemoryJob(_ context.Context, sess *session.Session) error {
	if s == nil || sess == nil {
		return nil
	}
	// Best-effort: publish to in-memory job queue if one is registered.
	autoMemoryQueue.enqueue(AutoMemoryJobRequest{
		AppName:   sess.AppName,
		SessionID: sess.ID,
		UserID:    sess.UserID,
	})
	return nil
}

func (s *sqliteMemoryService) Close() error {
	return nil
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
