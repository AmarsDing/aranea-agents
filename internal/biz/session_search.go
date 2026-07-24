package biz

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// M71: sessionaccess — 精灵只读检索全工作区 session 会话内容
// 统一编排：spirit 身份校验 → 限流（20/min）→ 检索/读取 → 审计（fail-closed）。
// 设计：docs/development/71-agent-resource-sharing.design.md §4.3
// ---------------------------------------------------------------------------

const domainSessionSearch = "SESSION_SEARCH"

const (
	// sessionSearchRateLimit 是每 spirit Agent 的访问配额（FR-11）。
	sessionSearchRateLimit  = 20
	sessionSearchRateWindow = time.Minute

	defaultSessionListLimit = 20
	maxSessionListLimit     = 50
	defaultSearchLimit      = 20
	maxSearchLimit          = 50
	defaultHistoryLimit     = 50
	maxHistoryLimit         = 200
	defaultHistoryMaxChars  = 50000
	// MaxHistoryMaxChars 对齐 read_upstream_deliverable 上限。
	MaxHistoryMaxChars = 200000
)

// GlobalMessageHit is one row of cross-session message search output.
type GlobalMessageHit struct {
	ID             string `json:"id"`
	SessionID      string `json:"session_id"`
	Kind           string `json:"kind"` // task | reply
	AuthorAgentKey string `json:"author_agent_key,omitempty"`
	Snippet        string `json:"snippet"`
	StartedAt      string `json:"started_at"`
}

// SessionMeta is the session summary returned by list_agent_sessions.
type SessionMeta struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	AgentID      string `json:"agent_id"`
	SessionType  string `json:"session_type,omitempty"`
	MessageCount int    `json:"message_count"`
	Status       string `json:"status"`
	UpdatedAt    string `json:"updated_at"`
}

// HistoryMessage is one message row returned by read_session_history.
type HistoryMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// ---------------------------------------------------------------------------
// 端口（Stability:evolving）
// ---------------------------------------------------------------------------

// GlobalMessageSearcher performs cross-session content search over steps_v2
// (kind IN task/reply, content LIKE, started_at desc). Implemented in data.
// workspaceID scopes the search to one tenant workspace; empty means no
// workspace filter (system callers only — the usecase always resolves one
// from ctx via workspace.IDFromContext).
// Stability:evolving
type GlobalMessageSearcher interface {
	SearchGlobalMessages(ctx context.Context, keyword, agentID, workspaceID string, limit int) ([]GlobalMessageHit, error)
}

// ---------------------------------------------------------------------------
// SessionSearchUsecase
// ---------------------------------------------------------------------------

// SessionSearchUsecaseDeps groups the dependencies for SessionSearchUsecase.
type SessionSearchUsecaseDeps struct {
	Agents   AgentReader
	Sessions SessionReader
	Messages MessageReader
	Searcher GlobalMessageSearcher
	Auditor  AccessAuditor
	Lg       loggateway.Logger
}

// SessionSearchUsecase implements sessionaccess (FR-08~FR-11).
type SessionSearchUsecase struct {
	agents   AgentReader
	sessions SessionReader
	messages MessageReader
	searcher GlobalMessageSearcher
	auditor  AccessAuditor
	lg       loggateway.Logger

	mu      sync.Mutex
	buckets map[string]*rateBucket
}

// rateBucket is a fixed-window counter per spirit agent.
type rateBucket struct {
	windowStart time.Time
	count       int
}

// NewSessionSearchUsecase creates the usecase.
func NewSessionSearchUsecase(deps SessionSearchUsecaseDeps) *SessionSearchUsecase {
	lg := deps.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SessionSearchUsecase{
		agents:   deps.Agents,
		sessions: deps.Sessions,
		messages: deps.Messages,
		searcher: deps.Searcher,
		auditor:  deps.Auditor,
		lg:       lg.With(loggateway.Domain("session_search")),
		buckets:  make(map[string]*rateBucket),
	}
}

// SearchMessages searches message content across sessions (FR-09).
func (u *SessionSearchUsecase) SearchMessages(ctx context.Context, callerAgentID, keyword, agentID string, limit int) ([]GlobalMessageHit, error) {
	if err := u.guardSpirit(ctx, callerAgentID, ActionSearchMessages, "query:"+truncateForAudit(keyword)); err != nil {
		return nil, err
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, apierror.BadRequest(domainSessionSearch, "query is required")
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	hits, err := u.searcher.SearchGlobalMessages(ctx, keyword, strings.TrimSpace(agentID), workspace.IDFromContext(ctx), limit)
	if err != nil {
		return nil, apierror.Internal(domainSessionSearch, "search failed: %s", err)
	}
	return hits, nil
}

// ListAgentSessions lists session metadata, optionally filtered by agent (FR-10).
func (u *SessionSearchUsecase) ListAgentSessions(ctx context.Context, callerAgentID, agentID string, limit int) ([]SessionMeta, error) {
	if err := u.guardSpirit(ctx, callerAgentID, ActionListSessions, "agent:"+strings.TrimSpace(agentID)); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultSessionListLimit
	}
	if limit > maxSessionListLimit {
		limit = maxSessionListLimit
	}
	res, err := u.sessions.SearchSessions(ctx, SessionSearchQuery{
		AgentID:     strings.TrimSpace(agentID),
		Status:      "active",
		WorkspaceID: workspace.IDFromContext(ctx),
		Limit:       limit,
		SortBy:      "updated_at",
		SortOrder:   "desc",
	})
	if err != nil {
		return nil, apierror.Internal(domainSessionSearch, "list sessions failed: %s", err)
	}
	out := make([]SessionMeta, 0, len(res.Items))
	for _, s := range res.Items {
		out = append(out, SessionMeta{
			ID:           s.ID,
			Title:        s.Title,
			AgentID:      s.AgentID,
			SessionType:  s.SessionType,
			MessageCount: s.MessageCount,
			Status:       s.Status,
			UpdatedAt:    s.UpdatedAt,
		})
	}
	return out, nil
}

// ReadSessionHistory returns paginated chat messages of one session (FR-08).
// beforeMessageID paginates backwards from a known message; truncation is
// char-budget based (maxChars, capped at MaxHistoryMaxChars).
func (u *SessionSearchUsecase) ReadSessionHistory(ctx context.Context, callerAgentID, sessionID, beforeMessageID string, limit, maxChars int) ([]HistoryMessage, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, false, apierror.BadRequest(domainSessionSearch, "session_id is required")
	}
	if err := u.guardSpirit(ctx, callerAgentID, ActionReadSession, "session:"+sessionID); err != nil {
		return nil, false, err
	}
	// P2-C workspace 隔离（IDOR 防护）：session 是 tenant-owned 私有数据。
	// 查询失败或 workspace 不匹配一律返回 NotFound，不泄露会话存在性
	// （镜像 SessionService.assertSessionAccess 语义）。
	sess, err := u.sessions.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, false, apierror.NotFound(domainSessionSearch, "session not found")
	}
	if err := workspace.AssertWorkspace(workspace.IDFromContext(ctx), sess.WorkspaceID); err != nil {
		u.lg.Warn("sessionaccess denied: workspace mismatch",
			loggateway.StepID("session_search.idor"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("caller_ws", workspace.IDFromContext(ctx)))
		return nil, false, apierror.NotFound(domainSessionSearch, "session not found")
	}
	if limit <= 0 {
		limit = defaultHistoryLimit
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}
	if maxChars <= 0 {
		maxChars = defaultHistoryMaxChars
	}
	if maxChars > MaxHistoryMaxChars {
		maxChars = MaxHistoryMaxChars
	}

	msgs, err := u.messages.ListMessagesBySession(ctx, sessionID, 0, 0)
	if err != nil {
		return nil, false, apierror.Internal(domainSessionSearch, "read history failed: %s", err)
	}
	// beforeMessageID: 只保留该消息之前（不含）的消息。
	if beforeMessageID = strings.TrimSpace(beforeMessageID); beforeMessageID != "" {
		cut := len(msgs)
		for i, m := range msgs {
			if m.ID == beforeMessageID {
				cut = i
				break
			}
		}
		msgs = msgs[:cut]
	}
	// 取最后 limit 条（倒序分页语义：最近的一页）。
	if len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	out := make([]HistoryMessage, 0, len(msgs))
	total := 0
	truncated := false
	for _, m := range msgs {
		content := m.ContentMarkdown
		if total+len(content) > maxChars {
			remain := maxChars - total
			if remain <= 0 {
				truncated = true
				break
			}
			content = content[:remain]
			truncated = true
		}
		total += len(content)
		out = append(out, HistoryMessage{
			ID:        m.ID,
			Role:      m.Role,
			Content:   content,
			CreatedAt: m.CreatedAt,
		})
		if truncated {
			break
		}
	}
	return out, truncated, nil
}

// guardSpirit verifies the caller is the spirit agent, enforces the rate
// limit, and audits the attempt (fail-closed, NFR-06).
func (u *SessionSearchUsecase) guardSpirit(ctx context.Context, callerAgentID, action, uri string) error {
	entry := AuditEntry{
		ActorAgentID: callerAgentID,
		ActorRole:    RoleSpirit,
		Action:       action,
		Relation:     RelationNone,
		ResourceURI:  uri,
	}
	caller, err := u.agents.GetAgentByID(ctx, callerAgentID)
	if err != nil || strings.TrimSpace(caller.AgentKey) != SpiritAgentKey {
		entry.Result = ResultDenied
		entry.DenyReason = "only the spirit agent may use sessionaccess tools"
		if err != nil {
			entry.DenyReason = "caller agent not found"
		}
		if aErr := u.auditSessionAccess(ctx, entry); aErr != nil {
			return aErr
		}
		return apierror.Forbidden(domainSessionSearch, "%s", entry.DenyReason)
	}
	if !u.allowRate(callerAgentID) {
		entry.Result = ResultDenied
		entry.DenyReason = "rate limit exceeded"
		if aErr := u.auditSessionAccess(ctx, entry); aErr != nil {
			return aErr
		}
		return apierror.BadRequest(domainSessionSearch, "rate limit exceeded: %d requests per %s; retry later", sessionSearchRateLimit, sessionSearchRateWindow)
	}
	entry.Result = ResultAllowed
	return u.auditSessionAccess(ctx, entry)
}

// allowRate enforces the fixed-window rate limit (FR-11). Process restart
// resets counters: acceptable for an in-memory guard.
func (u *SessionSearchUsecase) allowRate(agentID string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	now := time.Now()
	b, ok := u.buckets[agentID]
	if !ok || now.Sub(b.windowStart) >= sessionSearchRateWindow {
		u.buckets[agentID] = &rateBucket{windowStart: now, count: 1}
		return true
	}
	if b.count >= sessionSearchRateLimit {
		return false
	}
	b.count++
	return true
}

// auditSessionAccess records an audit entry; audit failure denies access
// (fail-closed, NFR-06).
func (u *SessionSearchUsecase) auditSessionAccess(ctx context.Context, e AuditEntry) error {
	if u.auditor == nil {
		return apierror.Internal(domainSessionSearch, "auditor not configured")
	}
	if err := u.auditor.Record(ctx, e); err != nil {
		return apierror.Internal(domainSessionSearch, "audit failed: %s", err)
	}
	return nil
}

// truncateForAudit bounds user-supplied text written into the audit row.
// Truncates by runes (not bytes) so multi-byte text is never cut mid-rune.
func truncateForAudit(s string) string {
	s = strings.TrimSpace(s)
	if rs := []rune(s); len(rs) > 120 {
		return string(rs[:120])
	}
	return s
}
