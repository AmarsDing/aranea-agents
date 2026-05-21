package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/event"
)

const teamMemberOptionsSchema = "chat.team_member/v1"

// messageStoreConsumer persists selected envelopes to messages (team member replies).
type messageStoreConsumer struct {
	bus      event.Bus
	sessions *SessionUsecase
}

func newMessageStoreConsumer(bus event.Bus, sessions *SessionUsecase) *messageStoreConsumer {
	if sessions == nil {
		return nil
	}
	return &messageStoreConsumer{bus: bus, sessions: sessions}
}

func (c *messageStoreConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	runTypedConsumer(ctx, "event-bus-message-store", c.bus, event.SubscribeOptions{
		EventTypes: []event.EnvelopeType{event.EnvelopeTypeMemberMessageDone},
		BufferSize: 256,
		Reliable:   true,
	}, c.handle)
}

func (c *messageStoreConsumer) handle(ctx context.Context, env event.Envelope) {
	if c == nil || c.sessions == nil {
		return
	}
	if strings.TrimSpace(env.TeamID) == "" {
		return
	}
	text := ""
	if env.Content != nil {
		text = strings.TrimSpace(env.Content.Text)
	}
	if text == "" {
		return
	}
	author := strings.TrimSpace(env.Author)
	if author == "" {
		author = "member"
	}
	now := env.Timestamp
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339Nano)
	}
	opts, _ := json.Marshal(map[string]any{
		"schema":           teamMemberOptionsSchema,
		"team_member":      map[string]any{"agent_key": author, "name": author, "role": ""},
		"member_agent_key": author,
		"team_id":          env.TeamID,
		"branch":           env.Branch,
		"filter_key":       env.FilterKey,
	})
	modelName := "team/member"
	msg := ChatMessage{
		ID:              teamMemberMessageID(author, env.ID, text, now),
		SessionID:       env.SessionID,
		Role:            "member",
		ContentMarkdown: text,
		ModelName:       modelName,
		Status:          "ok",
		OptionsJSON:     string(opts),
		CreatedAt:       now,
	}
	if err := c.sessions.AppendChatMessage(ctx, env.SessionID, msg, false); err != nil {
		event.SessionSysLogWarn(ctx, env.SessionID, "event_bus.message.store", "团队成员消息落库失败",
			event.P("author", author), event.P("error", err))
	}
}

func teamMemberMessageID(author, envelopeID, text, createdAt string) string {
	suffix := strings.TrimSpace(envelopeID)
	if suffix == "" {
		sum := sha256.Sum256([]byte(author + "\x00" + text + "\x00" + createdAt))
		suffix = hex.EncodeToString(sum[:8])
	}
	return "msg-team-" + author + "-" + suffix
}
