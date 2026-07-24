package biz

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// M71: deptmail — 部门主管之间的异步消息信箱（含 Turn 唤醒防抖）
// 设计：docs/development/71-agent-resource-sharing.design.md §4.2/§6
// ---------------------------------------------------------------------------

const domainDeptMail = "DEPT_MAIL"

// 消息状态（3 态，无需显式状态机文件）。
const (
	DeptMailStatusUnread  = "unread"
	DeptMailStatusRead    = "read"
	DeptMailStatusReplied = "replied"
)

// mailboxWakeDebounce 是同一 (from,to) 主管对的唤醒合并窗口（US-05）。
const mailboxWakeDebounce = 5 * time.Minute

const defaultInboxLimit = 20
const maxInboxLimit = 100

// maxSubjectRunes bounds the subject length in runes (not bytes) so multi-byte
// subjects are never cut mid-rune into invalid UTF-8.
const maxSubjectRunes = 200

// DeptLeadMessage is the biz model for one mailbox message.
type DeptLeadMessage struct {
	ID          string
	FromAgentID string
	FromDeptID  string
	ToAgentID   string
	ToDeptID    string
	Subject     string
	Body        string
	RefsJSON    string
	Status      string
	ReplyToID   string
	CreatedAt   time.Time
	ReadAt      *time.Time
}

// ---------------------------------------------------------------------------
// 端口（Stability:evolving）
// ---------------------------------------------------------------------------

// DeptLeadMailboxRepo persists mailbox messages.
// Stability:evolving
type DeptLeadMailboxRepo interface {
	CreateMessage(ctx context.Context, msg DeptLeadMessage) (DeptLeadMessage, error)
	ListInbox(ctx context.Context, toAgentID, status string, limit int) ([]DeptLeadMessage, error)
	GetMessage(ctx context.Context, id string) (DeptLeadMessage, error)
	MarkRead(ctx context.Context, id string, readAt time.Time) error
	MarkReplied(ctx context.Context, id string) error
}

// MailboxWaker wakes the recipient dept lead with a lightweight system turn.
// Implemented in the service layer via TurnExecutorGateway. Wake failures must
// not affect the persisted message (NFR-05).
// Stability:evolving
type MailboxWaker interface {
	WakeDeptLead(ctx context.Context, agentID, hint string) error
}

// ---------------------------------------------------------------------------
// DeptMailboxUsecase
// ---------------------------------------------------------------------------

// DeptMailboxUsecaseDeps groups the dependencies for DeptMailboxUsecase.
type DeptMailboxUsecaseDeps struct {
	Repo    DeptLeadMailboxRepo
	Agents  AgentReader
	Org     OrganizationReader
	Auditor AccessAuditor
	Waker   MailboxWaker
	Lg      loggateway.Logger
}

// DeptMailboxUsecase implements the dept lead mailbox (FR-05~FR-07).
type DeptMailboxUsecase struct {
	repo    DeptLeadMailboxRepo
	agents  AgentReader
	org     OrganizationReader
	auditor AccessAuditor
	waker   MailboxWaker
	lg      loggateway.Logger

	mu         sync.Mutex
	lastWakeAt map[string]time.Time
}

// NewDeptMailboxUsecase creates the usecase.
func NewDeptMailboxUsecase(deps DeptMailboxUsecaseDeps) *DeptMailboxUsecase {
	lg := deps.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &DeptMailboxUsecase{
		repo:       deps.Repo,
		agents:     deps.Agents,
		org:        deps.Org,
		auditor:    deps.Auditor,
		waker:      deps.Waker,
		lg:         lg.With(loggateway.Domain("dept_mail")),
		lastWakeAt: make(map[string]time.Time),
	}
}

// SendMessage sends a message to another department's lead (FR-05).
// refsJSON must be a JSON array (DeliverableRef envelopes) or empty.
func (u *DeptMailboxUsecase) SendMessage(ctx context.Context, fromAgentID, toDeptID, subject, body, refsJSON string) (DeptLeadMessage, error) {
	from, fromDeptID, err := u.requireDeptLead(ctx, fromAgentID)
	if err != nil {
		return DeptLeadMessage{}, err
	}
	toDeptID = strings.TrimSpace(toDeptID)
	if toDeptID == "" {
		return DeptLeadMessage{}, apierror.BadRequest(domainDeptMail, "to_dept_id is required")
	}
	toDept, err := u.org.GetOrgNode(ctx, toDeptID)
	if err != nil || toDept.Level != "department" {
		u.auditMail(ctx, fromAgentID, "", ActionSendMail, "dept:"+toDeptID, ResultDenied, "target department not found")
		return DeptLeadMessage{}, apierror.NotFound(domainDeptMail, "target department not found: %s", toDeptID)
	}
	if toDept.ID == fromDeptID {
		u.auditMail(ctx, fromAgentID, "", ActionSendMail, "dept:"+toDeptID, ResultDenied, "cannot send to own department")
		return DeptLeadMessage{}, apierror.BadRequest(domainDeptMail, "cannot send to your own department; use internal communication instead")
	}
	if toDept.DeptLeadAgentID == "" {
		u.auditMail(ctx, fromAgentID, toDeptID, ActionSendMail, "dept:"+toDeptID, ResultDenied, "target department has no lead")
		return DeptLeadMessage{}, apierror.NotFound(domainDeptMail, "target department has no lead agent")
	}

	subject = strings.TrimSpace(subject)
	if subject == "" {
		return DeptLeadMessage{}, apierror.BadRequest(domainDeptMail, "subject is required")
	}
	if rs := []rune(subject); len(rs) > maxSubjectRunes {
		subject = string(rs[:maxSubjectRunes])
	}
	refs, err := normalizeRefsJSON(refsJSON)
	if err != nil {
		return DeptLeadMessage{}, err
	}

	msg := DeptLeadMessage{
		ID:          uuid.NewString(),
		FromAgentID: from.ID,
		FromDeptID:  fromDeptID,
		ToAgentID:   toDept.DeptLeadAgentID,
		ToDeptID:    toDept.ID,
		Subject:     subject,
		Body:        body,
		RefsJSON:    refs,
		Status:      DeptMailStatusUnread,
		CreatedAt:   time.Now().UTC(),
	}
	// 先落库再唤醒（NFR-05）。
	saved, err := u.repo.CreateMessage(ctx, msg)
	if err != nil {
		return DeptLeadMessage{}, apierror.Internal(domainDeptMail, "persist message failed: %s", err)
	}
	u.auditMail(ctx, fromAgentID, toDept.ID, ActionSendMail, "msg:"+saved.ID, ResultAllowed, "")
	u.wakeDebounced(ctx, from.ID, saved.ToAgentID, u.wakeHint(ctx, fromDeptID, subject))
	return saved, nil
}

// ListInbox lists the caller's own inbox (FR-06).
func (u *DeptMailboxUsecase) ListInbox(ctx context.Context, callerAgentID, status string, limit int) ([]DeptLeadMessage, error) {
	if _, _, err := u.requireDeptLead(ctx, callerAgentID); err != nil {
		return nil, err
	}
	status = strings.TrimSpace(status)
	if status != "" && status != DeptMailStatusUnread && status != DeptMailStatusRead && status != DeptMailStatusReplied {
		return nil, apierror.BadRequest(domainDeptMail, "invalid status filter: %s", status)
	}
	if limit <= 0 {
		limit = defaultInboxLimit
	}
	if limit > maxInboxLimit {
		limit = maxInboxLimit
	}
	items, err := u.repo.ListInbox(ctx, callerAgentID, status, limit)
	if err != nil {
		return nil, apierror.Internal(domainDeptMail, "list inbox failed: %s", err)
	}
	return items, nil
}

// ReadMessage returns one message; marks unread → read when the caller is the recipient (FR-06).
func (u *DeptMailboxUsecase) ReadMessage(ctx context.Context, callerAgentID, messageID string) (DeptLeadMessage, error) {
	if _, _, err := u.requireDeptLead(ctx, callerAgentID); err != nil {
		return DeptLeadMessage{}, err
	}
	msg, err := u.repo.GetMessage(ctx, strings.TrimSpace(messageID))
	if err != nil {
		return DeptLeadMessage{}, apierror.NotFound(domainDeptMail, "message not found")
	}
	if msg.ToAgentID != callerAgentID && msg.FromAgentID != callerAgentID {
		u.auditMail(ctx, callerAgentID, msg.ToDeptID, ActionReadMail, "msg:"+msg.ID, ResultDenied, "not a party of this message")
		return DeptLeadMessage{}, apierror.Forbidden(domainDeptMail, "message does not belong to your mailbox")
	}
	if msg.ToAgentID == callerAgentID && msg.Status == DeptMailStatusUnread {
		readAt := time.Now().UTC()
		if err := u.repo.MarkRead(ctx, msg.ID, readAt); err != nil {
			return DeptLeadMessage{}, apierror.Internal(domainDeptMail, "mark read failed: %s", err)
		}
		msg.Status = DeptMailStatusRead
		msg.ReadAt = &readAt
	}
	u.auditMail(ctx, callerAgentID, msg.ToDeptID, ActionReadMail, "msg:"+msg.ID, ResultAllowed, "")
	return msg, nil
}

// ReplyMessage replies to a received message, forming a thread (FR-06).
func (u *DeptMailboxUsecase) ReplyMessage(ctx context.Context, callerAgentID, messageID, body string) (DeptLeadMessage, error) {
	from, fromDeptID, err := u.requireDeptLead(ctx, callerAgentID)
	if err != nil {
		return DeptLeadMessage{}, err
	}
	orig, err := u.repo.GetMessage(ctx, strings.TrimSpace(messageID))
	if err != nil {
		return DeptLeadMessage{}, apierror.NotFound(domainDeptMail, "message not found")
	}
	if orig.ToAgentID != callerAgentID {
		u.auditMail(ctx, callerAgentID, orig.ToDeptID, ActionReplyMail, "msg:"+orig.ID, ResultDenied, "only the recipient may reply")
		return DeptLeadMessage{}, apierror.Forbidden(domainDeptMail, "only the recipient may reply to this message")
	}
	if strings.TrimSpace(body) == "" {
		return DeptLeadMessage{}, apierror.BadRequest(domainDeptMail, "body is required")
	}

	reply := DeptLeadMessage{
		ID:          uuid.NewString(),
		FromAgentID: from.ID,
		FromDeptID:  fromDeptID,
		ToAgentID:   orig.FromAgentID,
		ToDeptID:    orig.FromDeptID,
		Subject:     "Re: " + orig.Subject,
		Body:        body,
		RefsJSON:    "[]",
		Status:      DeptMailStatusUnread,
		ReplyToID:   orig.ID,
		CreatedAt:   time.Now().UTC(),
	}
	saved, err := u.repo.CreateMessage(ctx, reply)
	if err != nil {
		return DeptLeadMessage{}, apierror.Internal(domainDeptMail, "persist reply failed: %s", err)
	}
	if err := u.repo.MarkReplied(ctx, orig.ID); err != nil {
		u.lg.Warn("mark original message replied failed",
			loggateway.StepID("dept_mail.reply"), loggateway.Str("message_id", orig.ID), loggateway.Err(err))
	}
	u.auditMail(ctx, callerAgentID, orig.FromDeptID, ActionReplyMail, "msg:"+saved.ID, ResultAllowed, "")
	u.wakeDebounced(ctx, from.ID, saved.ToAgentID, u.wakeHint(ctx, fromDeptID, reply.Subject))
	return saved, nil
}

// requireDeptLead verifies the caller is a dept lead and returns (agent, deptID).
func (u *DeptMailboxUsecase) requireDeptLead(ctx context.Context, agentID string) (Agent, string, error) {
	a, err := u.agents.GetAgentByID(ctx, agentID)
	if err != nil {
		return Agent{}, "", apierror.Forbidden(domainDeptMail, "caller agent not found")
	}
	if !IsDeptLeadAgent(a) {
		return Agent{}, "", apierror.Forbidden(domainDeptMail, "only department lead agents may use the mailbox")
	}
	deptID, err := resolveAgentDepartment(ctx, u.org, a)
	if err != nil {
		return Agent{}, "", apierror.Internal(domainDeptMail, "department lookup failed: %s", err)
	}
	if deptID == "" {
		return Agent{}, "", apierror.Forbidden(domainDeptMail, "caller is not attached to a department")
	}
	return a, deptID, nil
}

// wakeDebounced wakes the recipient at most once per (from,to) pair per window (US-05).
// Process restart loses debounce state: acceptable (worst case one extra wake).
// The waker delivers asynchronously; its failure never affects the persisted
// message (NFR-05).
func (u *DeptMailboxUsecase) wakeDebounced(ctx context.Context, fromAgentID, toAgentID, hint string) {
	if u.waker == nil {
		return
	}
	key := fromAgentID + "->" + toAgentID
	u.mu.Lock()
	last, ok := u.lastWakeAt[key]
	if ok && time.Since(last) < mailboxWakeDebounce {
		u.mu.Unlock()
		return
	}
	u.lastWakeAt[key] = time.Now()
	u.mu.Unlock()

	if err := u.waker.WakeDeptLead(ctx, toAgentID, hint); err != nil {
		u.lg.Warn("wake dept lead failed",
			loggateway.StepID("dept_mail.wake"), loggateway.Str("to_agent_id", toAgentID), loggateway.Err(err))
	}
}

// wakeHint builds the system-turn hint text for the recipient lead.
func (u *DeptMailboxUsecase) wakeHint(ctx context.Context, fromDeptID, subject string) string {
	fromName := fromDeptID
	if fromNode, err := u.org.GetOrgNode(ctx, fromDeptID); err == nil && fromNode.Name != "" {
		fromName = fromNode.Name
	}
	return "【部门信箱】你收到来自「" + fromName + "」主管的消息《" + subject + "》。请使用 list_inbox 工具查收并处理。"
}

// auditMail records a mailbox audit entry; failures are logged but never block
// the caller for send (message already persisted) — fail-closed applies to
// reads/denied paths where audit precedes the error return.
func (u *DeptMailboxUsecase) auditMail(ctx context.Context, actorAgentID, targetDeptID, action, uri, result, denyReason string) {
	if u.auditor == nil {
		u.lg.Warn("mailbox auditor not configured", loggateway.StepID("dept_mail.audit"))
		return
	}
	if err := u.auditor.Record(ctx, AuditEntry{
		ActorAgentID: actorAgentID,
		ActorRole:    RoleDeptLead,
		Action:       action,
		TargetDeptID: targetDeptID,
		Relation:     RelationNone,
		ResourceURI:  uri,
		Result:       result,
		DenyReason:   denyReason,
	}); err != nil {
		u.lg.Warn("mailbox audit write failed",
			loggateway.StepID("dept_mail.audit"), loggateway.Str("action", action), loggateway.Err(err))
	}
}

// normalizeRefsJSON validates refs as a JSON array; empty input becomes "[]".
func normalizeRefsJSON(refsJSON string) (string, error) {
	s := strings.TrimSpace(refsJSON)
	if s == "" {
		return "[]", nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return "", apierror.BadRequest(domainDeptMail, "refs must be a JSON array")
	}
	return s, nil
}
