package service

import (
	"context"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// durableResumeTurnCtx holds durable checkpoint resume state for a single agent turn (CC-R-OPT-05).
type durableResumeTurnCtx struct {
	spec       event.DurableResumeSpec
	active     bool
	runID      string
	dialogMode string
	provider   string
	model      string
}

func durableResumeTurnCtxFrom(ctx context.Context, runID, dialogMode, provider, model string) durableResumeTurnCtx {
	out := durableResumeTurnCtx{
		runID: runID, dialogMode: dialogMode, provider: provider, model: model,
	}
	spec, ok := event.DurableResumeFromContext(ctx)
	if !ok {
		return out
	}
	out.active = true
	out.spec = spec
	if rid := firstNonEmptyString(spec.RuntimeRunID, spec.TrpcInvocationID); rid != "" {
		out.runID = rid
	}
	if dm := firstNonEmptyString(spec.DialogMode); dm != "" {
		out.dialogMode = dm
	}
	if p := firstNonEmptyString(spec.Provider); p != "" {
		out.provider = p
	}
	if m := firstNonEmptyString(spec.Model); m != "" {
		out.model = m
	}
	return out
}

func (d durableResumeTurnCtx) buildUserMessage(sessionID, userOpts string, attN int, emitter *event.TraceEmitter) biz.ChatMessage {
	now := chatagent.RFC3339Now()
	msg := biz.ChatMessage{
		ID:               d.spec.TurnID,
		SessionID:        sessionID,
		Role:             "user",
		ContentMarkdown:  d.spec.UserContent,
		Status:           "ok",
		OptionsJSON:      userOpts,
		CreatedAt:        now,
		AttachmentsCount: attN,
	}
	// AF-correlation: TurnID 必须等于 msg.ID（= d.spec.TurnID），使前端通过 API
	// 加载的 user message 的 turn_id 非空，useConversationTimeline 才能将 Activity
	// 记录关联到此 UserTurn。
	msg.TurnID = msg.ID
	if emitter != nil {
		emitter.LogDone("chat.user_msg_persist", "Durable checkpoint 续跑（复用 turn_id，跳过 biz 用户行）",
			event.P("turn_id", d.spec.TurnID),
			event.P("session_run_id", d.spec.SessionRunID),
			event.P("session_revision", d.spec.SessionRevision),
			event.P("trpc_invocation_id", d.spec.TrpcInvocationID),
			event.P("dialog_mode", d.spec.DialogMode),
		)
	}
	return msg
}

func (o *ChatOrchestrator) durableSessionRunLifecycle(
	ctx context.Context,
	emitter *event.TraceEmitter,
	sess biz.Session,
	ag biz.Agent,
	d durableResumeTurnCtx,
	userMsg biz.ChatMessage,
	userContent string,
) (context.Context, string) {
	ctx = event.WithTurnID(ctx, userMsg.ID)
	if sess.DefaultContextWindowTokens > 0 {
		ctx = event.WithSessionDefaultContextWindow(ctx, sess.DefaultContextWindowTokens)
	}
	if d.active && d.spec.SessionRunID != "" {
		ctx = event.WithSessionRunID(ctx, d.spec.SessionRunID)
		if tid := strings.TrimSpace(d.spec.TurnID); tid != "" {
			ctx = event.WithTurnID(ctx, tid)
		}
		return ctx, d.spec.SessionRunID
	}
	return o.sessionRunLC().BeginSessionRunLifecycle(ctx, SessionRunStartParams{
		Emitter:      emitter,
		Session:      sess,
		Agent:        ag,
		TurnID:       userMsg.ID,
		RuntimeRunID: d.runID,
		UserContent:  userContent,
		DialogMode:   d.dialogMode,
		Provider:     d.provider,
		Model:        d.model,
	})
}

func durableResumeRunOpts(active bool, base []trpcagent.RunOption) []trpcagent.RunOption {
	if !active {
		return base
	}
	return append(base,
		trpcagent.WithDetachedCancel(true),
		trpcagent.WithPersistInterruptedAssistant(true),
	)
}
