package trpcmem

import (
	"aranea-agents/internal/biz"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

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
