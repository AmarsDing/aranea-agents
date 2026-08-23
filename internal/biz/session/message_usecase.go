package session

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// SessionMessageUsecase is a permanent sub-usecase of SessionUsecase, following the
// project's sub-usecase decomposition pattern (see usecase.go TECH-DEBT(COG) note —
// SessionUsecase struct_fields=14/15, so further inlining would violate AS-COG-01).
//
// Responsibilities (sibling to compressionUsecase / timelineUsecase / metricsUsecase):
//   - Message-shaped reads via ActivityMessageReader (Activity → ChatMessage adapter)
//   - Message writes (currently NoopMessageWriter — ActivityProjector owns persistence)
//   - Session title generation (snippet + async LLM generation)
//   - SessionMetricsDelta accumulation on chat append
//   - Revision counter management (BumpSessionRevision / GetSessionRevision)
//   - Delegation to state/turn/participant sub-usecases (orthogonal concerns grouped
//     here to keep SessionUsecase under the complexity budget)
//
// The messages table was DROPPED in Phase 1c (migration 20260902); this struct now
// operates purely on Activity-backed data and exists as a legitimate Facade, not as
// pending-deletion legacy code.
//
// Stability:evolving
type SessionMessageUsecase struct {
	messageReader       MessageReader
	messageSearchReader MessageSearchReader
	messageWriter       MessageWriter
	messageStatusWriter MessageStatusWriter
	titleGenerator      SessionTitleGenerator
	sessionReader       SessionReader
	sessionWriter       SessionWriter
	lg                  loggateway.Logger
	metricsUsecase      *SessionMetricsUsecase
	stateUsecase        *SessionStateUsecase
	turnUsecase         *SessionTurnUsecase
	participantUsecase  *SessionParticipantUsecase
	flowLog             FlowLogWriter
}

// NewSessionMessageUsecase creates a new SessionMessageUsecase.
func NewSessionMessageUsecase(
	messageReader MessageReader,
	messageSearchReader MessageSearchReader,
	messageWriter MessageWriter,
	messageStatusWriter MessageStatusWriter,
	titleGenerator SessionTitleGenerator,
	sessionReader SessionReader,
	sessionWriter SessionWriter,
	lg loggateway.Logger,
	metricsUsecase *SessionMetricsUsecase,
	stateRepo StateRepo,
	turnRepo TurnRepo,
	participants SessionParticipantRepository,
	flowLog FlowLogWriter,
) *SessionMessageUsecase {
	if titleGenerator == nil {
		titleGenerator = NewNoopSessionTitleGenerator()
	}
	return &SessionMessageUsecase{
		messageReader:       messageReader,
		messageSearchReader: messageSearchReader,
		messageWriter:       messageWriter,
		messageStatusWriter: messageStatusWriter,
		titleGenerator:      titleGenerator,
		sessionReader:       sessionReader,
		sessionWriter:       sessionWriter,
		lg:                  lg,
		metricsUsecase:      metricsUsecase,
		stateUsecase:        NewSessionStateUsecase(stateRepo),
		turnUsecase:         NewSessionTurnUsecase(turnRepo, metricsUsecase),
		participantUsecase:  NewSessionParticipantUsecase(participants, sessionReader, messageReader),
		flowLog:             flowLog,
	}
}

// SearchMessages full-text search within one session.
func (uc *SessionMessageUsecase) SearchMessages(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error) {
	if strings.TrimSpace(q.SessionID) == "" {
		return MessageSearchResult{}, validationErr("session_id is required")
	}
	if strings.TrimSpace(q.Keyword) == "" {
		return MessageSearchResult{}, validationErr("keyword is required")
	}
	return uc.messageSearchReader.SearchMessages(ctx, q)
}

func (uc *SessionMessageUsecase) ListMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	res, err := uc.ListMessagesPaged(ctx, sessionID, 0, 0)
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

// ListMessagesPaged returns messages with DB pagination (default limit when limit<=0).
func (uc *SessionMessageUsecase) ListMessagesPaged(ctx context.Context, sessionID string, limit, offset int) (MessageListResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return MessageListResult{}, validationErr("session id is required")
	}
	total, err := uc.messageReader.CountMessagesBySession(ctx, sessionID)
	if err != nil {
		return MessageListResult{}, err
	}
	limit = clampMessageListLimit(limit)
	if offset < 0 {
		offset = 0
	}
	items, err := uc.messageReader.ListMessagesBySession(ctx, sessionID, limit, offset)
	if err != nil {
		return MessageListResult{}, err
	}
	return MessageListResult{Items: items, Total: total}, nil
}

// ListMessagesAfterTurn loads rows with turn_number > afterTurn (compression path).
func (uc *SessionMessageUsecase) ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	return uc.messageReader.ListMessagesAfterTurn(ctx, sessionID, afterTurn)
}

// ListMessagesByStatus loads recent rows matching status (e.g. tool_running cancel path).
func (uc *SessionMessageUsecase) ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	return uc.messageSearchReader.ListMessagesByStatus(ctx, sessionID, status, limit)
}

// ListMessagesRecent loads the latest N messages in chronological order (timeline / cron).
func (uc *SessionMessageUsecase) ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	return uc.messageReader.ListMessagesRecent(ctx, sessionID, limit)
}

// AppendChatTurn persists a user + assistant pair (native chat).
func (uc *SessionMessageUsecase) AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error {
	if err := uc.messageWriter.AppendChatTurn(ctx, sessionID, user, assistant); err != nil {
		return err
	}
	delta := SessionMetricsDelta{SessionID: sessionID, MessageCount: 2, LastMessageAt: assistant.CreatedAt}
	if uc.metricsUsecase != nil {
		uc.metricsUsecase.AccumulateMetricsDelta(delta)
	}
	if strings.EqualFold(strings.TrimSpace(user.Role), "user") {
		if err := uc.AutoTitleFromUserMessage(ctx, sessionID, user.ContentMarkdown); err != nil {
			uc.lg.Warn("AutoTitleFromUserMessage failed", loggateway.StepID("session.auto_title"), loggateway.SessionID(sessionID), loggateway.Err(err))
		}
	}
	return nil
}

// AppendChatMessage persists one chat row (streamed native turns).
func (uc *SessionMessageUsecase) AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error {
	if err := uc.messageWriter.AppendChatMessage(ctx, sessionID, msg, bumpModelCall); err != nil {
		return err
	}
	delta := SessionMetricsDelta{SessionID: sessionID, MessageCount: 1, LastMessageAt: msg.CreatedAt}
	if uc.metricsUsecase != nil {
		uc.metricsUsecase.AccumulateMetricsDelta(delta)
	}
	if strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
		if err := uc.AutoTitleFromUserMessage(ctx, sessionID, msg.ContentMarkdown); err != nil {
			uc.lg.Warn("AutoTitleFromUserMessage failed", loggateway.StepID("session.auto_title"), loggateway.SessionID(sessionID), loggateway.Err(err))
		}
	}
	return nil
}

func (uc *SessionMessageUsecase) UpdateChatMessageStatus(ctx context.Context, sessionID, messageID, status, errorMessage string) error {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	status = strings.TrimSpace(status)
	if sessionID == "" || messageID == "" {
		return validationErr("session_id and message_id are required")
	}
	if status == "" {
		return validationErr("status is required")
	}
	return uc.messageStatusWriter.UpdateChatMessageStatus(ctx, sessionID, messageID, status, strings.TrimSpace(errorMessage))
}

// UpdateMessageFeedback records thumbs up/down on an assistant message (options_json.feedback).
func (uc *SessionMessageUsecase) UpdateMessageFeedback(ctx context.Context, sessionID, messageID, rating, comment string) error {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	rating = strings.TrimSpace(strings.ToLower(rating))
	if sessionID == "" || messageID == "" {
		return validationErr("session_id and message_id are required")
	}
	if rating != "positive" && rating != "negative" {
		return validationErr("rating must be positive or negative")
	}
	return uc.messageWriter.UpdateMessageFeedbackJSON(ctx, sessionID, messageID, rating, strings.TrimSpace(comment))
}

// ListMessagesAfterRevision returns messages with turn_number > afterRevision (M55 session sync).
func (uc *SessionMessageUsecase) ListMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int64) ([]ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	if _, err := uc.sessionReader.GetSessionByID(ctx, sessionID); err != nil {
		return nil, err
	}
	return uc.messageSearchReader.ListMessagesAfterRevision(ctx, sessionID, afterRevision)
}

// BumpSessionRevision atomically increments session_revision after a completed turn.
func (uc *SessionMessageUsecase) BumpSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, validationErr("session id is required")
	}
	return uc.sessionWriter.BumpSessionRevision(ctx, sessionID)
}

// GetSessionRevision returns the current session_revision counter.
func (uc *SessionMessageUsecase) GetSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, validationErr("session id is required")
	}
	return uc.sessionReader.GetSessionRevision(ctx, sessionID)
}

// UpsertChatActivityMessage persists a tool/MCP/Skill execution card for chat history restore.
func (uc *SessionMessageUsecase) UpsertChatActivityMessage(ctx context.Context, sessionID string, msg ChatMessage) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return validationErr("session id is required")
	}
	if strings.TrimSpace(msg.ID) == "" {
		return validationErr("message id is required")
	}
	if _, err := uc.sessionReader.GetSessionByID(ctx, sessionID); err != nil {
		return err
	}
	inserted, err := uc.messageWriter.UpsertChatActivityMessage(ctx, sessionID, msg)
	if err != nil {
		return err
	}
	if inserted && uc.metricsUsecase != nil {
		uc.metricsUsecase.AccumulateMetricsDelta(SessionMetricsDelta{SessionID: sessionID, MessageCount: 1, LastMessageAt: msg.CreatedAt})
	}
	return nil
}

// listAllMessages loads all messages for a session (used by export and participants).
func (uc *SessionMessageUsecase) listAllMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	total, err := uc.messageReader.CountMessagesBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	out := make([]ChatMessage, 0, total)
	for offset := 0; offset < total; {
		limit := MessageListMaxLimit
		if remaining := total - offset; remaining < limit {
			limit = remaining
		}
		chunk, err := uc.messageReader.ListMessagesBySession(ctx, sessionID, limit, offset)
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
		offset += len(chunk)
		if len(chunk) == 0 {
			break
		}
	}
	return out, nil
}

// AutoTitleFromUserMessage renames a still-default session title from the
// user's first message (snippet synchronously + async LLM refinement).
// Triggers: (a) legacy AppendChatTurn/AppendChatMessage hooks above;
// (b) the service-layer task.created subscriber (session_auto_title_subscriber.go)
// for the v2 native chat path, where messages persist via ActivityProjector
// and never reach those hooks (BUG-01, chat-e2e-20260823).
func (uc *SessionMessageUsecase) AutoTitleFromUserMessage(ctx context.Context, sessionID, content string) error {
	sess, err := uc.sessionReader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if !shouldAutoNameSession(sess.Title) {
		return nil
	}
	snippet := sessionTitleFromUserSnippet(content)
	if snippet != "" {
		if _, err := uc.sessionWriter.UpdateSessionTitle(ctx, sessionID, snippet); err != nil {
			uc.lg.Warn("auto rename from snippet failed", loggateway.StepID("session.title"), loggateway.SessionID(sessionID), loggateway.Err(err))
		}
	}
	appCtx := appctx.Ctx()
	safego.Go(appCtx, "generate-title-async", func() {
		uc.generateTitleAsync(appCtx, sessionID, sess.AgentID, content)
	})
	return nil
}

// generateTitleAsync derives a 15s timeout context from parentCtx (app-lifecycle),
// so it cancels both on timeout and on application shutdown.
func (uc *SessionMessageUsecase) generateTitleAsync(parentCtx context.Context, sessionID, agentID, content string) {
	bgCtx, cancel := context.WithTimeout(parentCtx, 15*time.Second)
	defer cancel()

	title, err := uc.titleGenerator.Generate(bgCtx, TitleGenRequest{
		UserMessage: content,
		SessionID:   sessionID,
		AgentID:     agentID,
	})
	if err != nil {
		if uc.flowLog != nil {
			uc.flowLog.LogFlowError(bgCtx, sessionID, "system.session.title_fail", "会话标题生成失败",
				LogPair{Key: "session_id", Value: sessionID}, LogPair{Key: "error", Value: err.Error()})
		}
		uc.lg.Debug("title generation skipped", loggateway.StepID("session.title"), loggateway.SessionID(sessionID), loggateway.Err(err))
		return
	}
	if title == "" {
		return
	}
	if _, err := uc.sessionWriter.UpdateSessionTitle(bgCtx, sessionID, title); err != nil {
		uc.lg.Warn("auto rename from generated title failed", loggateway.StepID("session.title"), loggateway.SessionID(sessionID), loggateway.Err(err))
	}
}

// --- State delegation ---

// GetSessionState delegates to SessionStateUsecase.
func (uc *SessionMessageUsecase) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
	return uc.stateUsecase.GetSessionState(ctx, sessionID)
}

// SaveSessionState delegates to SessionStateUsecase.
func (uc *SessionMessageUsecase) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error {
	return uc.stateUsecase.SaveSessionState(ctx, sessionID, state)
}

// PatchSessionState delegates to SessionStateUsecase.
func (uc *SessionMessageUsecase) PatchSessionState(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error {
	return uc.stateUsecase.PatchSessionState(ctx, sessionID, sets, deletes)
}

// ApplyStateDelta delegates to SessionStateUsecase.
func (uc *SessionMessageUsecase) ApplyStateDelta(ctx context.Context, sessionID string, delta StateDelta) error {
	return uc.stateUsecase.ApplyStateDelta(ctx, sessionID, delta)
}

// --- Turn delegation ---

// CreateTurn delegates to SessionTurnUsecase.
func (uc *SessionMessageUsecase) CreateTurn(ctx context.Context, turn SessionTurn) (SessionTurn, error) {
	return uc.turnUsecase.CreateTurn(ctx, turn)
}

// UpdateTurn delegates to SessionTurnUsecase.
func (uc *SessionMessageUsecase) UpdateTurn(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error) {
	return uc.turnUsecase.UpdateTurn(ctx, id, fields)
}

// IncrementInvocationCounts delegates to SessionTurnUsecase.
func (uc *SessionMessageUsecase) IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error {
	return uc.turnUsecase.IncrementInvocationCounts(ctx, sessionID, toolDelta, mcpDelta, skillDelta)
}

// ListTurns delegates to SessionTurnUsecase.
func (uc *SessionMessageUsecase) ListTurns(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error) {
	return uc.turnUsecase.ListTurns(ctx, sessionID, limit, offset)
}

// --- Participant delegation ---

// ListParticipants delegates to SessionParticipantUsecase.
func (uc *SessionMessageUsecase) ListParticipants(ctx context.Context, sessionID string) ([]SessionParticipant, error) {
	return uc.participantUsecase.ListParticipants(ctx, sessionID)
}
