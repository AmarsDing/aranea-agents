package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
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
	// BeginSessionRunLifecycle starts a session run and stores the binding.
	BeginSessionRunLifecycle(ctx context.Context, p SessionRunStartParams) (context.Context, string)
	// FinishSessionRunLifecycle ends a session run and cleans up the binding.
	FinishSessionRunLifecycle(ctx context.Context, sessionID, sessionRunID string, turnErr error)
	// EscalateToDurableByUser escalates a session run from interactive to durable mode by user action.
	EscalateToDurableByUser(ctx context.Context, sessionID, sessionRunID string)
	// EscalateToDurableOnShutdown escalates an interactive session run to durable mode
	// during server shutdown, so the durable worker auto-resumes it after restart.
	EscalateToDurableOnShutdown(ctx context.Context, sessionID, sessionRunID string)
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
	plans        biz.TaskPlannerPort
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
	Plans        biz.TaskPlannerPort
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
		plans:        d.Plans,
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

// BeginSessionRunLifecycle starts a session run and stores the binding.
func (l *chatSessionRunLifecycle) BeginSessionRunLifecycle(
	ctx context.Context, p SessionRunStartParams,
) (context.Context, string) {
	if l == nil || l.sessionRuns == nil {
		return ctx, ""
	}
	sessionID := strings.TrimSpace(p.Session.ID)
	ltCfg := l.ResolveChannelLongTaskConfig(ctx, p.Session)
	run, err := l.sessionRuns.StartInteractive(
		ctx,
		sessionID,
		p.TurnID,
		p.RuntimeRunID,
		event.EnvelopeSourceFromContext(ctx),
		strings.TrimSpace(p.Agent.ID),
	)
	if err != nil || run.ID == "" {
		return ctx, ""
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
	return ctx, run.ID
}

// EscalateToDurableByUser escalates a session run from interactive to durable mode by user action.
func (l *chatSessionRunLifecycle) EscalateToDurableByUser(ctx context.Context, sessionID, sessionRunID string) {
	l.escalateToDurable(ctx, sessionID, sessionRunID, sessstatus.StatusReasonUserEscalated)
}

// EscalateToDurableOnShutdown escalates an interactive session run to durable mode
// during server shutdown, so the durable worker auto-resumes it after restart.
func (l *chatSessionRunLifecycle) EscalateToDurableOnShutdown(ctx context.Context, sessionID, sessionRunID string) {
	l.escalateToDurable(ctx, sessionID, sessionRunID, sessstatus.StatusReasonServerShutdown)
}

// escalateToDurable is the shared escalation path: snapshot checkpoint, transition
// phase, cancel the runtime runner, and mark the session interrupted with reason.
func (l *chatSessionRunLifecycle) escalateToDurable(ctx context.Context, sessionID, sessionRunID string, reason sessstatus.SessionStatusReason) {
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
		Org:              l.orgCheckpointForSession(ctx, sessionID),
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
	l.applyDurableTransition(ctx, sessionID, sessionRunID, run, cp, reason)
}

func (l *chatSessionRunLifecycle) orgCheckpointForSession(ctx context.Context, sessionID string) biz.OrgCheckpointFields {
	if l == nil || l.plans == nil {
		return biz.OrgCheckpointFields{}
	}
	plans, err := l.plans.ListPlans(ctx, sessionID)
	if err != nil || len(plans) == 0 {
		return biz.OrgCheckpointFields{}
	}
	return biz.OrgCheckpointFromPlans(plans)
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
	reason sessstatus.SessionStatusReason,
) {
	ok, err := l.sessionRuns.TransitionPhase(ctx, sessionRunID, biz.PhaseEventUserEscalate)
	if err != nil || !ok {
		l.lg.Warn("session run transition to durable failed",
			loggateway.StepID(flowStepRunEscalate),
			loggateway.Str("session_run_id", sessionRunID),
			loggateway.Bool("cas_ok", ok),
			loggateway.Err(err),
		)
		return
	}
	stopped, runID := l.runs.Cancel(sessionID, "durable_escalate")
	if stopped {
		if err := l.runStatus.SetRunStatus(ctx, sessionID, runID, biz.SessionRunPhaseCancelled, ""); err != nil {
			l.lg.Warn("set run status failed on durable escalate cancel",
				loggateway.StepID(flowStepRunEscalate),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("run_id", runID),
				loggateway.Err(err))
		}
	}
	l.sessionState.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, reason)
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
	// run 终态迁移是计费/恢复语义的一部分：客户端断连时仍须落库，
	// 否则 run 永远停在 running，被判为中断（P1，2026-08-20）。
	ctx, cancel := appctx.Detach(ctx)
	defer cancel()
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
	if err := l.sessionRuns.Complete(ctx, sessionRunID); err != nil {
		l.lg.Error("session run complete transition failed",
			loggateway.StepID("chat.session_run_complete"),
			loggateway.Str("session_run_id", sessionRunID),
			loggateway.Err(err))
	}
}
