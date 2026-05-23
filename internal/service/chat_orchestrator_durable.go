package service

import (
	"context"

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
) (sessionRunID string, stopBudget context.CancelFunc) {
	stopBudget = func() {}
	if d.active && d.spec.SessionRunID != "" {
		return d.spec.SessionRunID, stopBudget
	}
	return o.beginSessionRunLifecycle(ctx, emitter, sess, ag, userMsg.ID, d.runID, userContent, d.dialogMode, d.provider, d.model)
}

func durableResumeRunOpts(active bool, base []trpcagent.RunOption) []trpcagent.RunOption {
	if !active {
		return base
	}
	return append(base, trpcagent.WithDetachedCancel(true))
}
