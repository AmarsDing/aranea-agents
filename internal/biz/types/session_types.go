package types

// SessionTreeNode represents a node in the session hierarchy tree.
// Used for rendering the session sidebar tree and tracking
// parent-child relationships between orchestration sessions.
//
// Note: SessionStatus and SessionStatusReason remain in
// internal/biz/session/status.go due to Go's circular import
// restriction (biz/types is a sub-package of biz).
//
// SessionTreeNode is a value object; direct construction via &SessionTreeNode{} is acceptable.
type SessionTreeNode struct {
	SessionID string            `json:"session_id"`
	Title     string            `json:"title"`
	Status    string            `json:"status"` // references biz.SessionStatus values
	AgentKey  string            `json:"agent_key,omitempty"`
	Children  []SessionTreeNode `json:"children,omitempty"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}
