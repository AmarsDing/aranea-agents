// 实现见 arenea/backend/internal/conversation/application/session_service.go（迁移 #23）。
package service

import (
	"arenea/backend/internal/conversation/application"
	"arenea/backend/internal/repository"
)

// SessionService 为 Conversation 会话用例；与 application.SessionService 类型等价。
type SessionService = application.SessionService

// NewSessionService 转发至 conversation/application。
func NewSessionService(repo repository.Store) *SessionService {
	return application.NewSessionService(repo)
}
