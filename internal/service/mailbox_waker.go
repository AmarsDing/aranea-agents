package service

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// mailboxWakeTimeout bounds one asynchronous wake turn. The wake hint is a
// lightweight system message, but the LLM reply can take tens of seconds —
// keep this comfortably above FirstByteTimeout.
const mailboxWakeTimeout = 2 * time.Minute

// mailboxWaker implements biz.MailboxWaker (M71 deptmail wake-up).
type mailboxWaker struct {
	// turnGw is injected post-construction via SetTurnGateway to break the
	// Wire cycle: ChatService → RuntimeTooling → DeptMailboxUsecase →
	// MailboxWaker → TurnExecutorGateway → ChatService (same pattern as
	// TeamStarter.SetTurnGateway).
	turnGw   biz.TurnExecutorGateway
	sessions biz.SessionReader
	writer   biz.SessionWriter
	lg       loggateway.Logger
}

var _ biz.MailboxWaker = (*mailboxWaker)(nil)

// NewMailboxWaker creates the waker. The turn gateway is backfilled by
// ProvideChatService once ChatService (its implementation) exists.
func NewMailboxWaker(sessions biz.SessionReader, writer biz.SessionWriter, lg loggateway.Logger) biz.MailboxWaker {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &mailboxWaker{
		sessions: sessions,
		writer:   writer,
		lg:       lg.With(loggateway.Domain("mailbox_waker")),
	}
}

// SetTurnGateway injects the turn gateway after construction (startup-time,
// before traffic). See TeamStarter.SetTurnGateway for the same pattern.
func (w *mailboxWaker) SetTurnGateway(gw biz.TurnExecutorGateway) {
	w.turnGw = gw
}

// WakeDeptLead schedules a lightweight system turn to the dept lead's most
// recent active session (creating one when none exists). The wake is
// asynchronous: ExecuteTurn runs the full LLM pipeline, so running it inline
// would block the sender lead's own turn inside send_dept_message. Delivery
// is best-effort — failures are logged, never propagated (NFR-05).
func (w *mailboxWaker) WakeDeptLead(ctx context.Context, agentID, hint string) error {
	if w.turnGw == nil {
		return nil
	}
	wakeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mailboxWakeTimeout)
	safego.Go(wakeCtx, "mailbox-waker.wake", func() {
		defer cancel()
		w.wake(wakeCtx, agentID, hint)
	})
	return nil
}

// wake resolves the target session and submits the hint turn.
func (w *mailboxWaker) wake(ctx context.Context, agentID, hint string) {
	sessionID, err := w.resolveLeadSession(ctx, agentID)
	if err != nil {
		w.lg.Warn("dept lead wake session resolve failed",
			loggateway.StepID("mailbox_waker.wake"),
			loggateway.Str("agent_id", agentID),
			loggateway.Err(err))
		return
	}
	// The turn pipeline's own queueing semantics prevent interrupting an
	// in-flight turn (zero-value EntryConfig allows the pending queue).
	if _, err := w.turnGw.ExecuteTurn(ctx, biz.TurnInput{
		SessionID: sessionID,
		Content:   hint,
		Timeouts: biz.TurnTimeouts{
			TurnTimeout:      biz.DefaultTurnTimeout,
			FirstByteTimeout: biz.DefaultFirstByteTimeout,
		},
	}); err != nil {
		w.lg.Warn("dept lead wake turn failed",
			loggateway.StepID("mailbox_waker.wake"),
			loggateway.Str("agent_id", agentID),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err))
	}
}

// resolveLeadSession returns the lead's latest active session, creating a
// dedicated mailbox session when none exists.
func (w *mailboxWaker) resolveLeadSession(ctx context.Context, agentID string) (string, error) {
	res, err := w.sessions.SearchSessions(ctx, biz.SessionSearchQuery{
		AgentID:     agentID,
		WorkspaceID: workspace.IDFromContext(ctx),
		Status:      "active",
		Limit:       1,
		SortBy:      "last_message_at",
		SortOrder:   "desc",
	})
	if err == nil && len(res.Items) > 0 {
		return res.Items[0].ID, nil
	}
	if err != nil {
		w.lg.Warn("search lead session failed, creating new one",
			loggateway.StepID("mailbox_waker.resolve"),
			loggateway.Str("agent_id", agentID),
			loggateway.Err(err))
	}
	now := time.Now().UTC().Format(time.RFC3339)
	created, cErr := w.writer.CreateSession(ctx, biz.Session{
		ID:          uuid.NewString(),
		WorkspaceID: workspace.IDFromContext(ctx),
		OwnerType:   "agent",
		AgentID:     agentID,
		Title:       "部门主管信箱",
		Status:      "idle",
		SessionType: "standalone",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if cErr != nil {
		return "", cErr
	}
	return created.ID, nil
}
