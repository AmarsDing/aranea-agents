package session

import (
	"context"
	"sort"
	"strings"
	"time"
)

// ActivityMessageReader adapts ActivityLister to the SessionMessageUsecase's
// message listing API, allowing the messages table to be deleted while keeping
// the ListSessionMessages RPC and frontend messageStore working.
//
// Activities are converted to ChatMessage shape:
//   - kind=task    → role=user    (user message)
//   - kind=reply   → role=assistant (agent reply)
//   - kind=action  → role=tool    (tool result, attached to preceding assistant)
//   - kind=notice  → role=system  (system notification)
//   - kind=thinking → skipped (reasoning is merged into assistant OptionsJSON)
//
// Phase 1c-3: replaces the messages-table-backed MessageReader.
type ActivityMessageReader struct {
	activities ActivityLister
}

// NewActivityMessageReader creates a new ActivityMessageReader.
func NewActivityMessageReader(activities ActivityLister) *ActivityMessageReader {
	if activities == nil {
		return nil
	}
	return &ActivityMessageReader{activities: activities}
}

// ListMessagesBySession returns all chat-shaped messages for a session.
func (r *ActivityMessageReader) ListMessagesBySession(ctx context.Context, sessionID string, limit, offset int) ([]ChatMessage, error) {
	acts, err := r.activities.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	msgs := activitiesToChatMessages(acts)
	return paginateMessages(msgs, limit, offset), nil
}

// ListMessagesRecent returns the latest N messages in chronological order.
func (r *ActivityMessageReader) ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	acts, err := r.activities.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	msgs := activitiesToChatMessages(acts)
	if limit <= 0 || limit >= len(msgs) {
		return msgs, nil
	}
	return msgs[len(msgs)-limit:], nil
}

// ListMessagesAfterTurn returns messages with turn_number > afterTurn.
// Note: Activity doesn't have turn_number; this uses TurnID correlation.
// For now, returns all messages (turn-based filtering is handled at the Activity level).
func (r *ActivityMessageReader) ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]ChatMessage, error) {
	acts, err := r.activities.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return activitiesToChatMessages(acts), nil
}

// ListMessagesByIDs returns messages matching the given IDs.
func (r *ActivityMessageReader) ListMessagesByIDs(ctx context.Context, sessionID string, ids []string) ([]ChatMessage, error) {
	acts, err := r.activities.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	var filtered []ActivityEntry
	for _, a := range acts {
		if _, ok := idSet[a.ID]; ok {
			filtered = append(filtered, a)
		}
	}
	return activitiesToChatMessages(filtered), nil
}

// CountMessagesBySession returns the total message count for a session.
func (r *ActivityMessageReader) CountMessagesBySession(ctx context.Context, sessionID string) (int, error) {
	acts, err := r.activities.ListBySession(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	return len(activitiesToChatMessages(acts)), nil
}

// ListMessagesAfterRevision returns messages after the given revision.
// Activities don't have a revision counter; this returns all messages.
// The session_revision mechanism is kept for WS sync; full replay is safe here.
func (r *ActivityMessageReader) ListMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int64) ([]ChatMessage, error) {
	acts, err := r.activities.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return activitiesToChatMessages(acts), nil
}

// ListMessagesByStatus returns messages matching a status filter.
func (r *ActivityMessageReader) ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]ChatMessage, error) {
	acts, err := r.activities.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var filtered []ActivityEntry
	for _, a := range acts {
		if a.Status == status {
			filtered = append(filtered, a)
		}
	}
	msgs := activitiesToChatMessages(filtered)
	if limit > 0 && limit < len(msgs) {
		msgs = msgs[:limit]
	}
	return msgs, nil
}

// SearchMessages performs a simple substring search over Activity content.
// This is a fallback for the removed FTS5 messages_fts; Phase 3 will introduce
// Activity-based search if needed.
func (r *ActivityMessageReader) SearchMessages(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error) {
	acts, err := r.activities.ListBySession(ctx, q.SessionID)
	if err != nil {
		return MessageSearchResult{}, err
	}
	keyword := q.Keyword
	var hits []MessageSearchHit
	for _, a := range acts {
		if matchesKeyword(a, keyword) {
			hits = append(hits, activityToSearchHit(a))
		}
	}
	if q.Limit > 0 && q.Limit < len(hits) {
		hits = hits[:q.Limit]
	}
	return MessageSearchResult{Items: hits, Total: len(hits)}, nil
}

// activitiesToChatMessages converts Activity records to ChatMessage shape,
// filtering to chat-relevant kinds (task/reply/action/notice) and sorting
// by timestamp ascending.
func activitiesToChatMessages(acts []ActivityEntry) []ChatMessage {
	msgs := make([]ChatMessage, 0, len(acts))
	for _, a := range acts {
		msg, ok := activityToChatMessage(a)
		if !ok {
			continue
		}
		msgs = append(msgs, msg)
	}
	sort.SliceStable(msgs, func(i, j int) bool {
		return msgs[i].CreatedAt < msgs[j].CreatedAt
	})
	return msgs
}

// systemInternalNoticeTypes mirrors the frontend SYSTEM_NOTICE_TYPES set
// (web/src/features/chat/noticeFilter.ts): machine-payload notices that carry
// metrics/recall internals, not user-facing text. They are rendered as
// dedicated UI affordances in the v2 chat stream (e.g. MemoryRecallChips) and
// must not leak into the plain message view as raw JSON system messages.
// Keep in sync with the frontend set.
var systemInternalNoticeTypes = map[string]struct{}{
	"context_usage":   {},
	"context_window":  {},
	"metrics_updated": {},
	"token_usage":     {},
	"memory_recalled": {},
}

// activityToChatMessage converts a single Activity to ChatMessage.
// Returns ok=false for kinds that don't map to chat messages (thinking/session/etc.)
// and for system-internal notices (see systemInternalNoticeTypes).
func activityToChatMessage(a ActivityEntry) (ChatMessage, bool) {
	var role string
	switch a.Kind {
	case "task":
		role = "user"
	case "reply":
		role = "assistant"
	case "action":
		role = "tool"
	case "notice":
		if _, internal := systemInternalNoticeTypes[a.NoticeType]; internal {
			return ChatMessage{}, false
		}
		role = "system"
	default:
		return ChatMessage{}, false
	}
	content := a.Content
	if role == "tool" {
		// Tool messages show the result, not the arguments
		content = a.ToolResult
	}
	status := "ok"
	if a.Status == "failed" || a.Status == "cancelled" {
		status = "error"
	} else if a.Status == "running" || a.Status == "pending" {
		status = "pending"
	}
	return ChatMessage{
		ID:              a.ID,
		SessionID:       a.SessionID,
		TurnID:          a.TurnID,
		Role:            role,
		ContentMarkdown: content,
		Status:          status,
		OptionsJSON:     buildOptionsJSON(a),
		CreatedAt:       formatActivityTimestamp(a.Timestamp),
	}, true
}

// buildOptionsJSON builds the options_json field for the ChatMessage from Activity metadata.
// Currently minimal; Phase 3 will enrich this with tool/reasoning details.
func buildOptionsJSON(a ActivityEntry) string {
	// Preserve tool name for tool messages so the frontend can render tool cards.
	if a.Kind == "action" && a.ToolName != "" {
		return `{"tool_name":"` + a.ToolName + `"}`
	}
	return ""
}

// formatActivityTimestamp formats a time.Time to RFC3339 string.
func formatActivityTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// matchesKeyword checks if an Activity's content contains the keyword (case-insensitive).
func matchesKeyword(a ActivityEntry, keyword string) bool {
	if keyword == "" {
		return true
	}
	return strings.Contains(strings.ToLower(a.Content), strings.ToLower(keyword)) ||
		strings.Contains(strings.ToLower(a.Reasoning), strings.ToLower(keyword)) ||
		strings.Contains(strings.ToLower(a.ToolResult), strings.ToLower(keyword)) ||
		strings.Contains(strings.ToLower(a.ToolName), strings.ToLower(keyword))
}

// activityToSearchHit converts an Activity to a MessageSearchHit.
func activityToSearchHit(a ActivityEntry) MessageSearchHit {
	return MessageSearchHit{
		ID:              a.ID,
		SessionID:       a.SessionID,
		Role:            a.Kind,
		ContentMarkdown: a.Content,
		Highlight:       a.Content,
		CreatedAt:       formatActivityTimestamp(a.Timestamp),
	}
}

// paginateMessages applies limit/offset to a message slice.
func paginateMessages(msgs []ChatMessage, limit, offset int) []ChatMessage {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(msgs) {
		return nil
	}
	msgs = msgs[offset:]
	if limit > 0 && limit < len(msgs) {
		msgs = msgs[:limit]
	}
	return msgs
}
