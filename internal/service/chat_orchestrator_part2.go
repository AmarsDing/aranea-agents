package service

import (
	"aranea-agents/internal/biz"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/pkg/ctxuser"
	"context"
	"time"
)

func (o *ChatOrchestrator) DeleteAwaitChannel(sessionID string) {
	o.awaitCoord().DeleteAwaitChannel(sessionID)
}

func (o *ChatOrchestrator) LoadAwaitChannel(sessionID string) (biz.AwaitChannel, bool) {
	return o.awaitCoord().LoadAwaitChannel(sessionID)
}

func (o *ChatOrchestrator) TrySendAwaitChannel(sessionID string, msg biz.AwaitReplyMsg) bool {
	return o.awaitCoord().TrySendAwaitChannel(sessionID, msg)
}

// Tool-scoped await channel operations (BUG-02, chat-e2e-20260823): address
// one specific parallel tool confirmation instead of the shared session slot.
func (o *ChatOrchestrator) LoadAwaitChannelForTool(sessionID, toolCallID string) (biz.AwaitChannel, bool) {
	return o.awaitCoord().LoadAwaitChannelForTool(sessionID, toolCallID)
}

func (o *ChatOrchestrator) TrySendAwaitChannelForTool(sessionID, toolCallID string, msg biz.AwaitReplyMsg) bool {
	return o.awaitCoord().TrySendAwaitChannelForTool(sessionID, toolCallID, msg)
}

// hydrateRunStatusFromSession delegates to the runStatus sub-manager.
func (o *ChatOrchestrator) hydrateRunStatusFromSession(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	return o.runStatus().HydrateRunStatusFromSession(ctx, sessionID)
}

func (o *ChatOrchestrator) resolveAwaitMeta(ctx context.Context, sessionID, status string) biz.ChatAwaitMeta {
	return o.runStatus().ResolveAwaitMeta(ctx, sessionID, status)
}

// Close is idempotent (review #1): closing sweepStop is detected by sweepLoop
// immediately, unlike the previous non-blocking send which was lost whenever
// the loop was not parked in select.
func (o *ChatOrchestrator) Close() {
	if o == nil {
		return
	}
	o.closeOnce.Do(func() {
		if o.sweepStop != nil {
			close(o.sweepStop)
		}
	})
}

func (o *ChatOrchestrator) sweepLoop() {
	ticker := time.NewTicker(orchMapSweepPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			o.sweepStaleMaps()
		case <-o.sweepStop:
			return
		}
	}
}

func (o *ChatOrchestrator) sweepStaleMaps() {
	o.runStatus().Sweep()
	o.pendingQ().Sweep()
	o.awaitCoord().Sweep()
	o.pendingClarifications.Sweep()
	o.sweepStuckRuns()
}

// canResumeAwait delegates to the awaitCoord sub-manager.
func (o *ChatOrchestrator) canResumeAwait(ctx context.Context, sessionID string) (runID string, ok bool) {
	return o.awaitCoord().CanResumeAwait(ctx, sessionID)
}

func (o *ChatOrchestrator) sessionRuntime() *araneasession.Runtime {
	if o == nil {
		return nil
	}
	if o.td().SessionRT != nil {
		return o.td().SessionRT
	}
	if o.td().Persist.Session == nil {
		return nil
	}
	return araneasession.NewRuntime(o.td().Persist.Session, o.lg())
}

func (o *ChatOrchestrator) resolveUserID(ctx context.Context, sessionID string) string {
	return ctxuser.TRPCUserKey(ctx)
}
