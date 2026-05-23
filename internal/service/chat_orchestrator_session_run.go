package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

type sessionRunTurnBinding struct {
	sessionRunID string
	turnID       string
	agentID      string
	userContent  string
	dialogMode   string
	provider     string
	model        string
	runtimeRunID string
	ltCfg        biz.ChannelLongTaskConfig
}

func (o *ChatOrchestrator) resolveChannelLongTaskConfig(ctx context.Context, sess biz.Session) biz.ChannelLongTaskConfig {
	if o == nil || o.td.Sessions == nil {
		return biz.ChannelLongTaskConfig{}
	}
	meta, ok := biz.ParseChannelSessionMeta(sess.MetadataJSON)
	if !ok || strings.TrimSpace(meta.ChannelID) == "" || o.chTurn.Channels == nil {
		return biz.ChannelLongTaskConfig{}
	}
	ch, err := o.chTurn.Channels.Get(ctx, meta.ChannelID)
	if err != nil {
		return biz.ChannelLongTaskConfig{}
	}
	return biz.ParseChannelLongTaskConfig(ch.ConfigJSON)
}

func (o *ChatOrchestrator) beginSessionRunLifecycle(
	ctx context.Context,
	emitter *event.TraceEmitter,
	sess biz.Session,
	ag biz.Agent,
	turnID, runtimeRunID, userContent, dialogMode, provider, model string,
) (sessionRunID string, stopBudget context.CancelFunc) {
	stopBudget = func() {}
	if o == nil || o.chTurn.SessionRuns == nil {
		return "", stopBudget
	}
	sessionID := strings.TrimSpace(sess.ID)
	ltCfg := o.resolveChannelLongTaskConfig(ctx, sess)
	budget := ltCfg.RunPolicy()
	run, err := o.chTurn.SessionRuns.StartInteractive(
		ctx,
		sessionID,
		turnID,
		runtimeRunID,
		event.EnvelopeSourceFromContext(ctx),
		strings.TrimSpace(ag.ID),
		budget,
	)
	if err != nil || run.ID == "" {
		return "", stopBudget
	}
	ctx = event.WithSessionRunID(ctx, run.ID)
	o.sessionRunBindings.Store(sessionID, sessionRunTurnBinding{
		sessionRunID: run.ID,
		turnID:       turnID,
		agentID:      strings.TrimSpace(ag.ID),
		userContent:  strings.TrimSpace(userContent),
		dialogMode:   strings.TrimSpace(dialogMode),
		provider:     strings.TrimSpace(provider),
		model:        strings.TrimSpace(model),
		runtimeRunID: strings.TrimSpace(runtimeRunID),
		ltCfg:        ltCfg,
	})
	if emitter != nil {
		emitter.LogDone("run.start", "Session Run 已创建",
			event.P("session_run_id", run.ID),
			event.P("run.phase", run.Phase),
			event.P("turn_id", turnID),
		)
	}
	stopBudget = o.chTurn.SessionRuns.StartBudgetWatcher(ctx, run.ID, budget, biz.BudgetPhaseCallbacks{
		OnSoftBudget: func(phase string) {
			if emitter != nil {
				emitter.Log("run.budget.soft", event.FlowPhaseDone, "软预算到达",
					event.P("session_run_id", run.ID), event.P("run.phase", phase))
			}
			o.onSessionRunSoftBudget(ctx, run, ltCfg)
		},
		OnHardBudget: func(phase string) {
			if emitter != nil {
				emitter.Log("run.budget.hard", event.FlowPhaseDone, "硬预算到达",
					event.P("session_run_id", run.ID), event.P("run.phase", phase))
			}
			o.escalateSessionRunToDurable(ctx, sessionID, run.ID)
		},
	})
	return run.ID, stopBudget
}

func (o *ChatOrchestrator) onSessionRunSoftBudget(ctx context.Context, run biz.SessionRun, ltCfg biz.ChannelLongTaskConfig) {
	if o == nil {
		return
	}
	auto := ltCfg.AutoEscalateAfterSoftBudget
	if o.chTurn.RunEscalation != nil {
		_ = o.chTurn.RunEscalation.NotifySoftBudget(ctx, run, auto)
	}
	if !auto {
		return
	}
	wait := time.Duration(ltCfg.SoftEscalateConfirmSecOrDefault()) * time.Second
	runID := run.ID
	sessionID := run.SessionID
	safego.Go(ctx, "session-run-auto-escalate", func() {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			cur, err := o.chTurn.SessionRuns.Get(context.Background(), runID)
			if err != nil || cur.ID == "" {
				return
			}
			if cur.Phase != biz.SessionRunPhaseEscalating {
				return
			}
			o.escalateSessionRunToDurable(context.Background(), sessionID, runID)
		}
	})
}

func (o *ChatOrchestrator) escalateSessionRunToDurable(ctx context.Context, sessionID, sessionRunID string) {
	if o == nil || o.chTurn.SessionRuns == nil {
		return
	}
	sessionRunID = strings.TrimSpace(sessionRunID)
	sessionID = strings.TrimSpace(sessionID)
	if sessionRunID == "" || sessionID == "" {
		return
	}
	bind, hasBind := o.sessionRunBinding(sessionID)
	run, err := o.chTurn.SessionRuns.Get(ctx, sessionRunID)
	if err != nil || run.ID == "" {
		event.SysLogWarn(flowStepRunEscalate, "session run not found for escalate",
			event.P("session_run_id", sessionRunID),
			event.P("session_id", sessionID),
		)
		return
	}
	if run.Phase == biz.SessionRunPhaseCompleted || run.Phase == biz.SessionRunPhaseFailed {
		return
	}
	if run.Phase == biz.SessionRunPhaseDurable && strings.TrimSpace(run.CheckpointID) != "" {
		return
	}
	agentID := bind.agentID
	userContent := bind.userContent
	dialogMode := bind.dialogMode
	provider := bind.provider
	model := bind.model
	runtimeRunID := bind.runtimeRunID
	if !hasBind || (userContent == "" && dialogMode == "") {
		if cp, cpErr := o.chTurn.SessionRuns.GetCheckpoint(ctx, sessionRunID); cpErr == nil && strings.TrimSpace(cp.PayloadJSON) != "" {
			if p, pErr := biz.ParseDurableCheckpointPayload(cp.PayloadJSON); pErr == nil {
				userContent = firstNonEmptyString(userContent, p.UserContent)
				dialogMode = firstNonEmptyString(dialogMode, p.DialogMode)
				provider = firstNonEmptyString(provider, p.Provider)
				model = firstNonEmptyString(model, p.Model)
				runtimeRunID = firstNonEmptyString(runtimeRunID, p.RuntimeRunID)
				agentID = firstNonEmptyString(agentID, p.AgentID)
			}
		}
	}
	if agentID == "" {
		agentID = run.AgentID
	}
	var sessionRevision int64
	if o.td.Sessions != nil {
		if rev, err := o.td.Sessions.GetSessionRevision(ctx, sessionID); err == nil {
			sessionRevision = rev
		}
	}
	cp, err := o.chTurn.SessionRuns.CreateDurableCheckpoint(ctx, biz.DurableCheckpointSnapshot{
		Run:              run,
		AgentID:          agentID,
		UserContent:      userContent,
		SessionRevision:  sessionRevision,
		DialogMode:       dialogMode,
		Provider:         provider,
		Model:            model,
		TrpcInvocationID: firstNonEmptyString(runtimeRunID, run.RuntimeRunID),
	})
	if err != nil || cp.ID == "" {
		event.SysLogWarn(flowStepRunEscalate, "durable checkpoint create failed",
			event.P("session_run_id", sessionRunID),
			event.P("session_id", sessionID),
			event.P("error", errString(err)),
		)
		return
	}
	if err := o.chTurn.SessionRuns.MarkPhase(ctx, sessionRunID, biz.SessionRunPhaseDurable); err != nil {
		event.SysLogWarn(flowStepRunEscalate, "session run mark durable failed",
			event.P("session_run_id", sessionRunID),
			event.P("error", err.Error()),
		)
		return
	}
	o.cancelActiveRun(ctx, sessionID)
	run.Phase = biz.SessionRunPhaseDurable
	run.CheckpointID = cp.ID
	if o.chTurn.RunEscalation != nil {
		if err := o.chTurn.RunEscalation.NotifyDurableEscalated(ctx, run); err != nil {
			event.SysLogWarn(flowStepRunEscalate, "durable escalation notify failed",
				event.P("session_run_id", sessionRunID),
				event.P("error", err.Error()),
			)
		}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (o *ChatOrchestrator) sessionRunBinding(sessionID string) (sessionRunTurnBinding, bool) {
	if v, ok := o.sessionRunBindings.Load(strings.TrimSpace(sessionID)); ok {
		b, ok := v.(sessionRunTurnBinding)
		return b, ok
	}
	return sessionRunTurnBinding{}, false
}

func (o *ChatOrchestrator) finishSessionRunLifecycle(ctx context.Context, sessionID, sessionRunID string, turnErr error) {
	if o == nil || o.chTurn.SessionRuns == nil || sessionRunID == "" {
		return
	}
	o.sessionRunBindings.Delete(strings.TrimSpace(sessionID))
	if turnErr != nil {
		_ = o.chTurn.SessionRuns.Fail(ctx, sessionRunID, turnErr.Error())
		return
	}
	cur, err := o.chTurn.SessionRuns.Get(ctx, sessionRunID)
	if err == nil && cur.Phase == biz.SessionRunPhaseDurable {
		return
	}
	_ = o.chTurn.SessionRuns.Complete(ctx, sessionRunID)
}
