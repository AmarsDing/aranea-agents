package biz

import "context"

// ── FR-12.7: resident profile card (report §6.4) ─────────────────────────
//
// One distilled profile card per (agent_id, user_id), maintained by the
// Sleep-time ProfileCardDistiller from active profile/preference/goal/
// constraint facts. The card is injected into the prompt unconditionally
// (100% inject rate, no recall scoring) at the first memory-block position —
// it is the shortest path for the user to feel that memory exists.

// ProfileCard is the resident profile card model.
type ProfileCard struct {
	AgentID   string
	UserID    string
	Content   string
	FactCount int
	Version   int
	UpdatedAt string
}

// MemoryProfileCardReader loads the resident profile card. Returns (nil, nil)
// when no card exists for the (agent, user) pair.
// Stability:evolving
type MemoryProfileCardReader interface {
	GetProfileCard(ctx context.Context, agentID, userID string) (*ProfileCard, error)
}

// MemoryProfileCardWriter maintains the resident profile card from Sleep-time
// distillation. Upsert bumps version; Delete removes a stale card when no
// active facts remain.
// Stability:evolving
type MemoryProfileCardWriter interface {
	UpsertProfileCard(ctx context.Context, card ProfileCard) error
	DeleteProfileCard(ctx context.Context, agentID, userID string) error
}
