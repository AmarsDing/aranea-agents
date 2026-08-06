package biz

import "context"

// EvalMessage is one inbound chat message accepted through the Agent Memory
// Challenge Add contract. Content carries the plain-text body.
// Stability:internal
type EvalMessage struct {
	Role      string
	Content   string
	MessageID string
	Timestamp string
}

// EvalMemoryItem is one memory-evidence row returned through the Agent Memory
// Challenge Search contract. JSON field names follow the platform contract.
// Stability:internal
type EvalMemoryItem struct {
	ID        string  `json:"id"`
	Content   string  `json:"content"`
	Score     float64 `json:"score"`
	Timestamp string  `json:"timestamp"`
}

// EvalMemoryStore bridges the Agent Memory Challenge Add/Search contract onto
// the L3 fact memory store. userID is the sole retrieval-isolation boundary:
// implementations must never return rows outside the caller's user scope, and
// Search must return stored memory evidence only (never generated answers).
// Stability:internal
type EvalMemoryStore interface {
	// AddMessages persists one conversation batch under the user scope and
	// returns the number of memory rows stored.
	AddMessages(ctx context.Context, userID, sessionID string, msgs []EvalMessage) (int, error)
	// SearchMemories recalls memory evidence for (userID, query), ranked by
	// the hybrid score, capped at topK rows.
	SearchMemories(ctx context.Context, userID, query string, topK int32) ([]EvalMemoryItem, error)
}
