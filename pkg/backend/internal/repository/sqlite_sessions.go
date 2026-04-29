package repository

import (
	"arenea/backend/internal/conversation/adapters/sqlite"
	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) conversationSQL() *sqlite.Store {
	return sqlite.NewStore(r.db)
}

func (r *SQLiteRepository) CreateSession(s domain.Session) (domain.Session, error) {
	return r.conversationSQL().CreateSession(s)
}

func (r *SQLiteRepository) GetSessionByID(id string) (domain.Session, error) {
	return r.conversationSQL().GetSessionByID(id)
}

func (r *SQLiteRepository) ListSessions(agentID string) ([]domain.Session, error) {
	return r.conversationSQL().ListSessions(agentID)
}

func (r *SQLiteRepository) ListTeamSessions(teamID string) ([]domain.Session, error) {
	return r.conversationSQL().ListTeamSessions(teamID)
}

func (r *SQLiteRepository) SearchSessions(query domain.SessionSearchQuery) (domain.SessionListResult, error) {
	return r.conversationSQL().SearchSessions(query)
}

func (r *SQLiteRepository) UpdateSessionTitle(id string, title string) (domain.Session, error) {
	return r.conversationSQL().UpdateSessionTitle(id, title)
}

func (r *SQLiteRepository) UpdateSessionContextUsedRatio(sessionID string, ratio float64) error {
	return r.conversationSQL().UpdateSessionContextUsedRatio(sessionID, ratio)
}

func (r *SQLiteRepository) UpdateSessionL0Context(sessionID string, promptTokens int, contextWindow int, ratio float64) error {
	return r.conversationSQL().UpdateSessionL0Context(sessionID, promptTokens, contextWindow, ratio)
}

func (r *SQLiteRepository) ArchiveSession(id string) error {
	return r.conversationSQL().ArchiveSession(id)
}

func (r *SQLiteRepository) DeleteSession(id string) error {
	return r.conversationSQL().DeleteSession(id)
}

func (r *SQLiteRepository) DeleteSessionsByAgentID(agentID string) error {
	return r.conversationSQL().DeleteSessionsByAgentID(agentID)
}

func (r *SQLiteRepository) AddMessage(m domain.Message) (domain.Message, error) {
	return r.conversationSQL().AddMessage(m)
}

func (r *SQLiteRepository) ListMessages(sessionID string) ([]domain.Message, error) {
	return r.conversationSQL().ListMessages(sessionID)
}

func (r *SQLiteRepository) ListLatestMessagesByTokens(sessionID string, maxTokens int, hardCap int) ([]domain.Message, error) {
	return r.conversationSQL().ListLatestMessagesByTokens(sessionID, maxTokens, hardCap)
}
