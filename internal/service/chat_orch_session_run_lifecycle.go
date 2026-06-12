package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// SessionRunStartParams aggregates the parameters for starting a session run lifecycle.
// Introduced to satisfy BI1 (parameter count ≤ 5) while preserving call-site clarity.
// Stability:evolving
type SessionRunStartParams struct {
	Emitter      *event.TraceEmitter
	Session      biz.Session
	Agent        biz.Agent
	TurnID       string
	RuntimeRunID string
	UserContent  string
	DialogMode   string
	Provider     string
	Model        string
}

// sessionRunLifecycle manages the session run lifecycle: begin, finish, and escalate to durable.
// Stability:evolving
type sessionRunLifecycle interface {
	// BeginSessionRunLifecycle starts a session run, creates budget watchers, and stores the binding.
	BeginSessionRunLifecycle(ctx context.Context, p SessionRunStartParams) (context.Context, string, context.CancelFunc)
	// FinishSessionRunLifecycle ends a session run and cleans up the binding.
	FinishSessionRunLifecycle(ctx context.Context, sessionID, sessionRunID string, turnErr error)
	// EscalateSessionRunToDurable escalates a session run from interactive to durable mode.
	EscalateSessionRunToDurable(ctx context.Context, sessionID, sessionRunID string)
	// ResolveChannelLongTaskConfig resolves the channel long task config for a session.
	ResolveChannelLongTaskConfig(ctx context.Context, sess biz.Session) biz.ChannelLongTaskConfig
}

// chatSessionRunLifecycle implements sessionRunLifecycle.
//
// Part of the TECH-DEBT(BL8) resolution: extracting session run lifecycle
// management from ChatOrchestrator to reduce cognitive complexity (AS-COG-01).
type chatSessionRunLifecycle struct {
	sessionRuns  *biz.SessionRunUsecase
	channels     *biz.ChannelUsecase
	sessions     biz.SessionStatePort
	runStatus    runStatusTracker
	sessionState sessionStateTransitor
	runs         *rt.RunRegistry
	escalation   SessionRunEscalationNotifier
	lg           loggateway.Logger
}

// chatSessionRunLifecycleDeps aggregates constructor dependencies for chatSessionRunLifecycle.
// Introduced to satisfy BI1 (parameter count ≤ 5) for the constructor.
// Stability:internal
type chatSessionRunLifecycleDeps struct {
	SessionRuns  *biz.SessionRunUsecase
	Channels     *biz.ChannelUsecase
	Sessions     biz.SessionStatePort
	RunStatus    runStatusTracker
	SessionState sessionStateTransitor
	Runs         *rt.RunRegistry
	Escalation   SessionRunEscalationNotifier
	Logger       loggateway.Logger
}

func newChatSessionRunLifecycle(d chatSessionRunLifecycleDeps) *chatSessionRunLifecycle {
	return &chatSessionRunLifecycle{
		sessionRuns:  d.SessionRuns,
		channels:     d.Channels,
		sessions:     d.Sessions,
		runStatus:    d.RunStatus,
		sessionState: d.SessionState,
		runs:         d.Runs,
		escalation:   d.Escalation,
		lg:           d.Logger,
	}
}

// Compile-time interface check.
var _ sessionRunLifecycle = (*chatSessionRunLifecycle)(nil)

// ResolveChannelLongTaskConfig resolves the channel long task config for a session.
func (l *chatSessionRunLifecycle) ResolveChannelLongTaskConfig(ctx context.Context, sess biz.Session) biz.ChannelLongTaskConfig {
	if l == nil || l.channels == nil {
		return biz.ChannelLongTaskConfig{}
	}
	meta, ok := biz.ParseChannelSessionMeta(sess.MetadataJSON)
	if !ok || strings.TrimSpace(meta.ChannelID) == "" {
		return biz.ChannelLongTaskConfig{}
	}
	ch, err := l.channels.Get(ctx, meta.ChannelID)
	if err != nil {
		return biz.ChannelLongTaskConfig{}
	}
	return biz.ParseChannelLongTaskConfig(ch.ConfigJSON)
}

// BeginSessionRunLifecycle starts a session run, creates budget watchers, and stores the binding.
func (l *chatSessionRunLifecycle) BeginSessionRunLifecycle(
	ctx context.Context, p SessionRunStartParams,
) (context.Context, string, context.CancelFunc) {
	stopBudget := func() {}
	if l == nil || l.sessionRuns == nil {
		return ctx, "", stopBudget
	}
	sessionID := strings.TrimSpace(p.Session.ID)
	ltCfg := l.ResolveChannelLongTaskConfig(ctx, p.Session)
	budget := ltCfg.RunPolicy()
	run, err := l.sessionRuns.StartInteractive(
		ctx,
		sessionID,
		p.TurnID,
		p.RuntimeRunID,
		event.EnvelopeSourceFromContext(ctx),
		strings.TrimSpace(p.Agent.ID),
		budget,
	)
	if err != nil || run.ID == "" {
		return ctx, "", stopBudget
	}
	ctx = event.WithSessionRunID(ctx, run.ID)
	ctx = event.WithTurnID(ctx, p.TurnID)
	if p.Session.DefaultContextWindowTokens > 0 {
		ctx = event.WithSessionDefaultContextWindow(ctx, p.Session.DefaultContextWindowTokens)
	}
	l.runStatus.StoreBinding(sessionID, sessionRunTurnBinding{
		sessionRunID: run.ID,
		turnID:       p.TurnID,
		agentID:      strings.TrimSpace(p.Agent.ID),
		userContent:  strings.TrimSpace(p.UserContent),
		dialogMode:   strings.TrimSpace(p.DialogMode),
		provider:     strings.TrimSpace(p.Provider),
		model:        strings.TrimSpace(p.Model),
		runtimeRunID: strings.TrimSpace(p.RuntimeRunID),
		ltCfg:        ltCfg,
	})
	if p.Emitter != nil {
		p.Emitter.LogDone("run.start", "Session Run 已创建",
			event.P("session_run_id", run.ID),
			event.P("run.phase", run.Phase),
			event.P("turn_id", p.TurnID),
		)
	}
	stopBudget = l.sessionRuns.StartBudgetWatcher(ctx, run.ID, budget, biz.BudgetPhaseCallbacks{
		OnSoftBudget: func(phase string) {
			if p.Emitter != nil {
				p.Emitter.Log("run.budget.soft", event.FlowPhaseDone, "软预算到达",
					event.P("session_run_id", run.ID), event.P("run.phase", phase))
			}
			l.onSessionRunSoftBudget(ctx, run, ltCfg)
		},
		OnHardBudget: func(phase string) {
			if p.Emitter != nil {
				p.Emitter.Log("run.budget.hard", event.FlowPhaseDone, "硬预算到达",
					event.P("session_run_id", run.ID), event.P("run.phase", phase))
			}
			l.EscalateSessionRunToDurable(ctx, sessionID, run.ID)
		},
	})
	return ctx, run.ID, stopBudget
}

func (l *chatSessionRunLifecycle) onSessionRunSoftBudget(ctx context.Context, run biz.SessionRun, ltCfg biz.ChannelLongTaskConfig) {
	if l == nil {
		return
	}
	auto := ltCfg.AutoEscalateAfterSoftBudget
	if l.escalation != nil {
		_ = l.escalation.NotifySoftBudget(ctx, run, auto)
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
		fromTimer := false
		select {
		case <-ctx.Done():
		case <-timer.C:
			fromTimer = true
		}
		escalateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cur, err := l.sessionRuns.Get(escalateCtx, runID)
		if err != nil || cur.ID == "" {
			return
		}
		if cur.Phase != biz.SessionRunPhaseEscalating {
			if fromTimer {
				l.lg.Warn("skipping auto-escalate, run no longer escalating",
					loggateway.StepID("session-run-auto-escalate"),
					loggateway.Str("session_run_id", runID),
					loggateway.Any("current_phase", cur.Phase),
				)
			}
			return
		}
		l.EscalateSessionRunToDurable(escalateCtx, sessionID, runID)
	})
}

// EscalateSessionRunToDurable escalates a session run from interactive to durable mode.
func (l *chatSessionRunLifecycle) EscalateSessionRunToDurable(ctx context.Context, sessionID, sessionRunID string) {
	if l == nil || l.sessionRuns == nil {
		return
	}
	sessionRunID = strings.TrimSpace(sessionRunID)
	sessionID = strings.TrimSpace(sessionID)
	if sessionRunID == "" || sessionID == "" {
		return
	}
	bind, hasBind := l.runStatus.LoadBinding(sessionID)
	run, err := l.sessionRuns.Get(ctx, sessionRunID)
	if err != nil || run.ID == "" {
		l.lg.Warn("session run not found for escalate",
			loggateway.StepID(flowStepRunEscalate),
			loggateway.Str("session_run_id", sessionRunID),
			loggateway.Str("session_id", sessionID),
		)
		return
	}
	if run.Phase == biz.SessionRunPhaseCompleted || run.Phase == biz.SessionRunPhaseFailed {
		return
	}
	if run.Phase == biz.SessionRunPhaseDurable && strings.TrimSpace(run.CheckpointID) != "" {
		return
	}
	agentID, userContent, dialogMode, provider, model, runtimeRunID := l.resolveEscalationFields(ctx, sessionRunID, run, bind, hasBind)
	var sessionRevision int64
	if l.sessions != nil {
		if rev, err := l.sessions.GetSessionRevision(ctx, sessionID); err == nil {
			sessionRevision = rev
		}
	}
	cp, err := l.sessionRuns.CreateDurableCheckpoint(ctx, biz.DurableCheckpointSnapshot{
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
		l.lg.Warn("durable checkpoint create failed",
			loggateway.StepID(flowStepRunEscalate),
			loggateway.Str("session_run_id", sessionRunID),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
		return
	}
	l.applyDurableTransition(ctx, sessionID, sessionRunID, run, cp)
}

// resolveEscalationFields fills in missing escalation fields from checkpoint when binding is absent.
func (l *chatSessionRunLifecycle) resolveEscalationFields(
	ctx context.Context, sessionRunID string, run biz.SessionRun,
	bind sessionRunTurnBinding, hasBind bool,
) (agentID, userContent, dialogMode, provider, model, runtimeRunID string) {
	agentID = bind.agentID
	userContent = bind.userContent
	dialogMode = bind.dialogMode
	provider = bind.provider
	model = bind.model
	runtimeRunID = bind.runtimeRunID
	if !hasBind || (userContent == "" && dialogMode == "") {
		if cp, cpErr := l.sessionRuns.GetCheckpoint(ctx, sessionRunID); cpErr == nil && strings.TrimSpace(cp.PayloadJSON) != "" {
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
	return
}

// applyDurableTransition marks the run as durable, cancels the runtime runner, and notifies escalation.
func (l *chatSessionRunLifecycle) applyDurableTransition(
	ctx context.Context, sessionID, sessionRunID string, run biz.SessionRun, cp biz.SessionRunCheckpoint,
) {
	if err := l.sessionRuns.MarkPhase(ctx, sessionRunID, biz.SessionRunPhaseDurable); err != nil {
		l.lg.Warn("session run mark durable failed",
			loggateway.StepID(flowStepRunEscalate),
			loggateway.Str("session_run_id", sessionRunID),
			loggateway.Err(err),
		)
		return
	}
	stopped, runID := l.runs.Cancel(sessionID)
	if stopped {
		l.runStatus.SetRunStatus(ctx, sessionID, runID, biz.SessionRunPhaseCancelled, "")
	}
	l.sessionState.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonBudgetEscalated)
	run.Phase = biz.SessionRunPhaseDurable
	run.CheckpointID = cp.ID
	if l.escalation != nil {
		if err := l.escalation.NotifyDurableEscalated(ctx, run); err != nil {
			l.lg.Warn("durable escalation notify failed",
				loggateway.StepID(flowStepRunEscalate),
				loggateway.Str("session_run_id", sessionRunID),
				loggateway.Err(err),
			)
		}
	}
}

// FinishSessionRunLifecycle ends a session run and cleans up the binding.
func (l *chatSessionRunLifecycle) FinishSessionRunLifecycle(ctx context.Context, sessionID, sessionRunID string, turnErr error) {
	if l == nil || l.sessionRuns == nil || sessionRunID == "" {
		return
	}
	l.runStatus.DeleteBinding(sessionID)
	if turnErr != nil {
		if err := l.sessionRuns.Fail(ctx, sessionRunID, turnErr.Error()); err != nil {
			l.lg.Error("session run fail transition failed",
				loggateway.StepID("chat.session_run_fail"),
				loggateway.Str("session_run_id", sessionRunID),
				loggateway.Err(err))
		}
		return
	}
	cur, err := l.sessionRuns.Get(ctx, sessionRunID)
	if err == nil && cur.Phase == biz.SessionRunPhaseDurable {
		return
	}
	// If the run is in escalating phase and the turn completed successfully,
	// mark it as completed (the turn finished before hard budget was reached).
	if err == nil && cur.Phase == biz.SessionRunPhaseEscalating {
		l.lg.Info("session run in escalating phase, marking as completed since turn finished",
			loggateway.StepID("chat.session_run_escalating_complete"),
			loggateway.Str("session_run_id", sessionRunID),
		)
	}
	if err := l.sessionRuns.Complete(ctx, sessionRunID); err != nil {
		l.lg.Error("session run complete transition failed",
			loggateway.StepID("chat.session_run_complete"),
			loggateway.Str("session_run_id", sessionRunID),
			loggateway.Err(err))
	}
}
