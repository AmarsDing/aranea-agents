package service

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/preview"
	"aranea-agents/pkg/loggateway"
)

const channelOutboundEmptyFallback = "（暂无文本回复）"

// TECH-DEBT: notification content assembly (phase/source guards, message matching in
// assistantReplyPartsForRun, error message fallback selection) should be extracted to
// biz-layer helper functions. The Service layer should only orchestrate: call biz
// helpers for content assembly → call preview for IM formatting → call channel for
// delivery. Blocked by: preview package coupling (internal/channel/preview is a
// presentation concern that must not be imported from biz) and the cross-cutting
// nature of enqueueForSession which inherently mixes biz data queries with channel
// service calls. Tracked in issue TBD.

// SessionRunEscalationNotifier sends IM notices when a run hits soft/hard budgets (CC-R-02).
type SessionRunEscalationNotifier interface {
	NotifySoftBudget(ctx context.Context, run biz.SessionRun, autoEscalate bool) error
	NotifyDurableEscalated(ctx context.Context, run biz.SessionRun) error
	NotifyRunCompleted(ctx context.Context, run biz.SessionRun, replyMarkdown string) error
	NotifyRunFailed(ctx context.Context, run biz.SessionRun, errMsg string) error
}

type channelRunEscalationNotifier struct {
	channels *biz.ChannelUsecase
	sessions *biz.SessionUsecase
	lg       loggateway.Logger
}

type channelOutboundEnqueueOpts struct {
	skipFormat bool
}

func NewChannelRunEscalationNotifier(channels *biz.ChannelUsecase, sessions *biz.SessionUsecase, lg loggateway.Logger) SessionRunEscalationNotifier {
	if channels == nil || sessions == nil {
		return nil
	}
	return &channelRunEscalationNotifier{channels: channels, sessions: sessions, lg: lg}
}

func (n *channelRunEscalationNotifier) NotifySoftBudget(ctx context.Context, run biz.SessionRun, autoEscalate bool) error {
	if n == nil {
		return nil
	}
	text := "任务处理时间较长，仍在执行中。"
	if autoEscalate {
		text += "若无进一步操作，将自动转入后台继续。"
	} else {
		text += "如需后台继续，请回复 /background。"
	}
	cardJSON := ""
	if card, err := preview.BuildFeishuEscalateCardJSON(run.ID, run.SessionID, ""); err != nil {
		n.lg.Warn("Channel escalate card build failed",
			loggateway.StepID(flowStepChannelOutbound),
			loggateway.Str("session_run_id", run.ID),
			loggateway.Str("session_id", run.SessionID),
			loggateway.Err(err),
		)
	} else {
		cardJSON = card
	}
	return n.enqueueForSession(ctx, run.SessionID, "run:"+run.ID+":soft", text, cardJSON)
}

func (n *channelRunEscalationNotifier) NotifyDurableEscalated(ctx context.Context, run biz.SessionRun) error {
	if n == nil {
		return nil
	}
	text := "任务较长，已转入后台继续执行；完成后将在此会话通知你。"
	return n.enqueueForSession(ctx, run.SessionID, "run:"+run.ID+":durable", text, "")
}

func (n *channelRunEscalationNotifier) NotifyRunCompleted(ctx context.Context, run biz.SessionRun, replyMarkdown string) error {
	if n == nil || n.sessions == nil {
		return nil
	}
	if run.Phase != biz.SessionRunPhaseCompleted {
		return nil
	}
	if strings.TrimSpace(run.Source) != "" && !strings.EqualFold(strings.TrimSpace(run.Source), "channel") {
		return nil
	}
	reply := strings.TrimSpace(replyMarkdown)
	reasoning := ""
	if reply == "" {
		reply, reasoning = assistantReplyPartsForRun(ctx, n.sessions, run.SessionID, run.StartedAt, run.TurnID)
	} else if reasoning == "" {
		_, reasoning = assistantReplyPartsForRun(ctx, n.sessions, run.SessionID, run.StartedAt, run.TurnID)
	}
	if reply == "" {
		reply = "任务已完成。"
	}
	platform, err := n.sessionPlatform(ctx, run.SessionID)
	if err != nil {
		return err
	}
	formattedReasoning := strings.TrimSpace(preview.FormatRenderedTranscriptForIM(platform, reasoning))
	formattedBody := strings.TrimSpace(preview.FormatAssistantReplyForIM(platform, reply))
	if formattedBody == "" {
		formattedBody = "任务已完成。"
	}

	if formattedReasoning != "" && (strings.EqualFold(platform, "feishu") || strings.EqualFold(platform, "lark")) {
		if card, err := preview.BuildFeishuChannelReplyCardJSON(formattedReasoning, formattedBody); err == nil && strings.TrimSpace(card) != "" {
			summary := preview.FormatIMSectionedReply(platform, formattedReasoning, formattedBody)
			if summary == "" {
				summary = formattedBody
			}
			pages := preview.SplitPages(summary, preview.PlatformTextLimit(platform))
			text := formattedBody
			if len(pages) > 0 {
				text = pages[0]
			}
			if err := n.enqueueForSession(ctx, run.SessionID, "run:"+run.ID+":completed:card", text, card, channelOutboundEnqueueOpts{skipFormat: true}); err != nil {
				return err
			}
			for i, page := range pages[1:] {
				key := fmt.Sprintf("run:%s:completed:%d", run.ID, i+2)
				if err := n.enqueueForSession(ctx, run.SessionID, key, page, "", channelOutboundEnqueueOpts{skipFormat: true}); err != nil {
					return err
				}
			}
			return nil
		}
	}

	formatted := preview.FormatIMSectionedReply(platform, formattedReasoning, formattedBody)
	if formatted == "" {
		formatted = formattedBody
	}
	pages := preview.SplitPages(formatted, preview.PlatformTextLimit(platform))
	for i, page := range pages {
		key := "run:" + run.ID + ":completed"
		if len(pages) > 1 {
			key = fmt.Sprintf("%s:%d", key, i+1)
		}
		if err := n.enqueueForSession(ctx, run.SessionID, key, page, "", channelOutboundEnqueueOpts{skipFormat: true}); err != nil {
			return err
		}
	}
	return nil
}

func (n *channelRunEscalationNotifier) NotifyRunFailed(ctx context.Context, run biz.SessionRun, errMsg string) error {
	if n == nil {
		return nil
	}
	if run.Phase != biz.SessionRunPhaseFailed {
		return nil
	}
	if strings.TrimSpace(run.Source) != "" && !strings.EqualFold(strings.TrimSpace(run.Source), "channel") {
		return nil
	}
	text := strings.TrimSpace(errMsg)
	if text == "" {
		text = strings.TrimSpace(run.ErrorMessage)
	}
	if text == "" {
		text = "后台任务执行失败，请稍后重试或联系管理员。"
	}
	return n.enqueueForSession(ctx, run.SessionID, "run:"+run.ID+":failed", text, "")
}

// assistantReplyPartsForRun finds the assistant reply belonging to a specific run.
// It uses turnID (the user message ID stored on the run) to precisely locate the
// paired assistant message via turn_id FK, avoiding the timestamp ambiguity
// that causes CHAT-03 (picking the wrong assistant message in busy sessions).
func assistantReplyPartsForRun(ctx context.Context, sessions *biz.SessionUsecase, sessionID, runStartedAt, turnID string) (body, reasoning string) {
	if sessions == nil {
		return "", ""
	}
	msgs, err := sessions.ListMessagesRecent(ctx, strings.TrimSpace(sessionID), 32)
	if err != nil || len(msgs) == 0 {
		return "", ""
	}

	// Primary path: locate the user message by ID, then find the assistant message
	// sharing the same turn_id (guaranteed by AppendChatTurn's FK assignment).
	if tid := strings.TrimSpace(turnID); tid != "" {
		var userTurnID string
		for _, m := range msgs {
			if strings.TrimSpace(m.ID) == tid {
				userTurnID = m.TurnID
				break
			}
		}
		if userTurnID != "" {
			for _, m := range msgs {
				if m.TurnID == userTurnID && strings.EqualFold(strings.TrimSpace(m.Role), "assistant") {
					candidateBody := strings.TrimSpace(m.ContentMarkdown)
					if candidateBody == "" && strings.TrimSpace(m.OptionsJSON) == "" {
						break
					}
					return candidateBody, preview.ReasoningMarkdownFromOptions(m.OptionsJSON)
				}
			}
		}
	}

	// Fallback: timestamp heuristic for runs without a TurnID or when the user
	// message has scrolled out of the recent-32 window.
	started := strings.TrimSpace(runStartedAt)
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if !strings.EqualFold(strings.TrimSpace(m.Role), "assistant") {
			continue
		}
		candidateBody := strings.TrimSpace(m.ContentMarkdown)
		if candidateBody == "" && strings.TrimSpace(m.OptionsJSON) == "" {
			continue
		}
		if started != "" && strings.TrimSpace(m.CreatedAt) != "" && m.CreatedAt < started {
			continue
		}
		return candidateBody, preview.ReasoningMarkdownFromOptions(m.OptionsJSON)
	}
	return "", ""
}

func (n *channelRunEscalationNotifier) sessionPlatform(ctx context.Context, sessionID string) (string, error) {
	sess, err := n.sessions.Get(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return "", err
	}
	meta, ok := biz.ParseChannelSessionMeta(sess.MetadataJSON)
	if !ok || strings.TrimSpace(meta.Platform) == "" {
		return "feishu", nil
	}
	return strings.TrimSpace(meta.Platform), nil
}

func (n *channelRunEscalationNotifier) enqueueForSession(
	ctx context.Context,
	sessionID, idempotencyKey, text, cardJSON string,
	opts ...channelOutboundEnqueueOpts,
) error {
	sess, err := n.sessions.Get(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return err
	}
	meta, ok := biz.ParseChannelSessionMeta(sess.MetadataJSON)
	if !ok || strings.TrimSpace(meta.ChannelID) == "" {
		return nil
	}
	recipient := strings.TrimSpace(meta.PeerID)
	if recipient == "" {
		recipient = strings.TrimSpace(meta.PeerKey)
	}
	if recipient == "" {
		return nil
	}
	platform := strings.TrimSpace(meta.Platform)
	skipFormat := len(opts) > 0 && opts[0].skipFormat
	if !skipFormat {
		text = preview.FormatAssistantReplyForIM(platform, text)
		if strings.TrimSpace(text) == "" && strings.TrimSpace(cardJSON) == "" {
			n.lg.Warn("Channel session outbound empty after format",
			loggateway.StepID(flowStepChannelOutbound),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("platform", platform),
			loggateway.Str("idempotency_key", idempotencyKey),
		)
			return nil
		}
	}
	payload := biz.ChannelOutboundPayload{
		Platform:       platform,
		Recipient:      recipient,
		Text:           text,
		IdempotencyKey: idempotencyKey,
	}
	if strings.TrimSpace(cardJSON) != "" && strings.EqualFold(strings.TrimSpace(meta.Platform), "feishu") {
		payload.Kind = biz.ChannelOutboundCardKind
		payload.CardJSON = cardJSON
	}
	_, _, err = n.channels.EnqueueOutboundDelivery(ctx, meta.ChannelID, payload)
	return err
}
