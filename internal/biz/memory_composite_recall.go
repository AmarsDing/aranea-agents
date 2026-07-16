package biz

import (
	"context"
	"strings"
)

// CompositeRecallQuery merges L2 episodes and L3 facts by fused score.
type CompositeRecallQuery struct {
	AgentID   string
	SessionID string
	UserID    string
	Query     string
	Limit     int32
}

// CompositeRecallHit is one ranked line for composite prompt injection.
type CompositeRecallHit struct {
	Layer string
	Line  string
	Score float64
	// P2-04: provenance metadata for L3 facts (empty for L2 episodes).
	FactID         string
	SourceSession  string
	Confidence     float64
	Version        int
}

// SessionCompositeRecallStore loads fused L2+L3 candidates (implemented by data memory_shim adapters).
type SessionCompositeRecallStore interface {
	CompositeSearchMemories(ctx context.Context, agentID, sessionID, userID, query string, limit int32) ([]CompositeRecallStoreRow, error)
}

// CompositeRecallStoreRow is the store-neutral composite recall row.
type CompositeRecallStoreRow struct {
	Layer     string
	Title     string
	Summary   string
	Statement string
	Score     float64
	// P2-04: provenance metadata for L3 facts (empty for L2 episodes).
	FactID        string
	SourceSession string
	Confidence    float64
	Version       int
}

// MemoryCompositeRecaller performs cross-layer L2+L3 recall for prompt injection.
type MemoryCompositeRecaller interface {
	RecallComposite(ctx context.Context, q CompositeRecallQuery) ([]CompositeRecallHit, error)
}

// ProactiveRecallContext captures the current conversation state for
// proactive recall. This is the biz-level mirror of the framework's
// ConversationContext type, defined here to avoid importing the framework
// package (red line #2).
type ProactiveRecallContext struct {
	// MentionedEntities are people, places, or topics mentioned in the
	// conversation. Each entity is used as a search keyword to retrieve
	// related memories.
	MentionedEntities []string

	// CurrentTopic is the topic of the current conversation turn.
	CurrentTopic string

	// UserStatement is the user's latest statement, used for contradiction
	// detection.
	UserStatement string
}

// ProactiveRecaller is the biz port for proactive memory recall.
// Implementations live in internal/memory/trpc and delegate to the
// framework's memory.Service.ProactiveRecall method.
type ProactiveRecaller interface {
	ProactiveRecall(ctx context.Context, agentID, userID string, convCtx ProactiveRecallContext) ([]CompositeRecallHit, error)
}

// MemoryCompositeRecallUsecase wraps SessionCompositeRecallStore and
// optionally a ProactiveRecaller for conversation-driven memory surfacing.
type MemoryCompositeRecallUsecase struct {
	store             SessionCompositeRecallStore
	proactiveRecaller ProactiveRecaller
}

// NewMemoryCompositeRecallUsecase wires the composite recall store.
// The proactive recaller is optional; use SetProactiveRecaller to inject it
// after construction (avoids breaking existing Wire providers).
func NewMemoryCompositeRecallUsecase(store SessionCompositeRecallStore) *MemoryCompositeRecallUsecase {
	if store == nil {
		return nil
	}
	return &MemoryCompositeRecallUsecase{store: store}
}

// SetProactiveRecaller injects a proactive recaller after construction.
// This avoids breaking the existing NewMemoryCompositeRecallUsecase signature
// and allows Wire to bind the proactive recaller separately.
func (uc *MemoryCompositeRecallUsecase) SetProactiveRecaller(r ProactiveRecaller) {
	if uc == nil {
		return
	}
	uc.proactiveRecaller = r
}

func (uc *MemoryCompositeRecallUsecase) RecallComposite(ctx context.Context, q CompositeRecallQuery) ([]CompositeRecallHit, error) {
	if uc == nil || uc.store == nil {
		return nil, nil
	}
	agentID := strings.TrimSpace(q.AgentID)
	if agentID == "" {
		return nil, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	rows, err := uc.store.CompositeSearchMemories(ctx, agentID, strings.TrimSpace(q.SessionID), strings.TrimSpace(q.UserID), strings.TrimSpace(q.Query), limit)
	if err != nil {
		return nil, err
	}
	out := make([]CompositeRecallHit, 0, len(rows))
	for _, row := range rows {
		line := formatCompositeRecallLine(row)
		if line == "" {
			continue
		}
		out = append(out, CompositeRecallHit{
			Layer:        row.Layer,
			Line:         line,
			Score:        row.Score,
			FactID:       row.FactID,
			SourceSession: row.SourceSession,
			Confidence:   row.Confidence,
			Version:      row.Version,
		})
	}
	return out, nil
}

// ProactiveRecall retrieves memories based on the conversation context
// (mentioned entities, current topic, user statement) without requiring an
// explicit query. It is intended to be called before each conversation turn
// to surface relevant memories that the agent should consider.
//
// Returns empty list (not error) when no proactive recaller is wired or
// when the conversation context carries no usable signal.
func (uc *MemoryCompositeRecallUsecase) ProactiveRecall(ctx context.Context, agentID, userID string, convCtx ProactiveRecallContext) ([]CompositeRecallHit, error) {
	if uc == nil || uc.proactiveRecaller == nil {
		return nil, nil
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, nil
	}
	return uc.proactiveRecaller.ProactiveRecall(ctx, agentID, strings.TrimSpace(userID), convCtx)
}

func formatCompositeRecallLine(row CompositeRecallStoreRow) string {
	layer := strings.ToUpper(strings.TrimSpace(row.Layer))
	switch layer {
	case "L2", "L2_EPISODE", "EPISODE":
		title := strings.TrimSpace(row.Title)
		summary := strings.TrimSpace(row.Summary)
		if title == "" {
			title = summary
		}
		if title == "" {
			return ""
		}
		if summary != "" && summary != title {
			return title + ": " + summary
		}
		return title
	default:
		stmt := strings.TrimSpace(row.Statement)
		if stmt == "" {
			stmt = strings.TrimSpace(row.Summary)
		}
		return stmt
	}
}
