package session

import "context"

// SessionParticipant is one actor in a session (agent/user/team member).
type SessionParticipant struct {
	ID               string
	SessionID        string
	ParticipantType  string
	ParticipantID    string
	DisplayName      string
	RoleInSession    string
	Status           string
	FirstActiveAt    string
	LastActiveAt     string
	MessageCount     int
	RunStepCount     int
	InputTokens      int
	OutputTokens     int
	ContextUsedRatio float64
	MetadataJSON     string
	CreatedAt        string
	UpdatedAt        string
}

// SessionParticipantRepository persists derived participant rows for a session.
type SessionParticipantRepository interface {
	SyncFromSession(ctx context.Context, sess Session, messages []ChatMessage) error
	ListBySession(ctx context.Context, sessionID string) ([]SessionParticipant, error)
}
