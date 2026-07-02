package team

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/telemetry/turntrace"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"go.opentelemetry.io/otel/trace"
)

const defaultGraphWatchTimeout = 30 * time.Minute

const defaultHITLSLATimeout = 24 * time.Hour

const maxHITLSLAExtensions = 3

const sessionMaxAge = 2 * time.Hour

const defaultCleanupInterval = 10 * time.Minute

type CoordinatorConfig struct {
	WatchTimeout    time.Duration
	HITLSLATimeout  time.Duration
	SessionMaxAge   time.Duration
	CleanupInterval time.Duration
}

func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		WatchTimeout:    defaultGraphWatchTimeout,
		HITLSLATimeout:  defaultHITLSLATimeout,
		SessionMaxAge:   sessionMaxAge,
		CleanupInterval: defaultCleanupInterval,
	}
}

// TeamGraphExecutionBackend indexes and resumes team-linked graph executions.
type TeamGraphExecutionBackend interface {
	RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID string, ct *biz.CompiledTeam) error
	MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error
	ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*biz.GraphExecution, error)
	GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error)
}

// TeamGraphTaskResumeHandler resumes team Graph runs after Kanban task completion.
// Stability:evolving
type TeamGraphTaskResumeHandler interface {
	HandleTeamGraphTaskCompleted(ctx context.Context, task *biz.GraphTask, resume map[string]any) (handled bool, err error)
}

// TeamGraphRunCoordinator unifies team graph execution register, HITL defer, and task resume (M53 P1).
type TeamGraphRunCoordinator struct {
	graphs          TeamGraphExecutionBackend
	teamRunReader   biz.TeamRunReader
	teamRunWriter   biz.TeamRunWriter
	runTransitioner biz.TeamRunStatusTransitioner
	activityBus     biz.ActivityEventBus
	finisher        *TeamRunMediator
	sessionRepo     biz.TeamGraphSessionRepo
	cfg             CoordinatorConfig
	lg              loggateway.Logger
	agentKeyFn      func(agentID string) string

	mu       sync.RWMutex
	sessions map[string]*teamGraphRunSession
}

type teamGraphRunSession struct {
	teamRunID       string
	teamID          string
	sessionID       string
	spiritSessionID string
	execID          string
	inputPreview    string
	definitionJSON  string
	watchStop       context.CancelFunc

	stepDedup     *graphStepDedup
	memberByNode  map[string]MemberDef
	stepSortIndex map[string]int
	obsReg        biz.OrchestrationRegistry
	obsStore      *biz.OrchestrationStatusStore
	registeredAt  time.Time
}

type graphWatchMode int

const (
	graphWatchStepsOnly graphWatchMode = iota
	graphWatchStepsAndFinalize
)

func NewTeamGraphRunCoordinator(graphs TeamGraphExecutionBackend, teamRunReader biz.TeamRunReader, teamRunWriter biz.TeamRunWriter, runTransitioner biz.TeamRunStatusTransitioner, activityBus biz.ActivityEventBus, sessionRepo biz.TeamGraphSessionRepo, agentKeyFn func(agentID string) string, lg loggateway.Logger) *TeamGraphRunCoordinator {
	if agentKeyFn == nil {
		agentKeyFn = func(agentID string) string { return strings.TrimSpace(agentID) }
	}
	return &TeamGraphRunCoordinator{
		graphs:          graphs,
		teamRunReader:   teamRunReader,
		teamRunWriter:   teamRunWriter,
		runTransitioner: runTransitioner,
		activityBus:     activityBus,
		sessionRepo:     sessionRepo,
		agentKeyFn:      agentKeyFn,
		sessions:        make(map[string]*teamGraphRunSession),
		cfg:             DefaultCoordinatorConfig(),
		lg:              lg,
	}
}

// SetFinisher wires the mediator for step persistence and team run finalization.
func (c *TeamGraphRunCoordinator) SetFinisher(m *TeamRunMediator) {
	if c == nil {
		return
	}
	c.finisher = m
}

func (c *TeamGraphRunCoordinator) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID string, ct *biz.CompiledTeam) error {
	if c == nil || c.graphs == nil {
		return nil
	}
	// Create graph execution child span (P3-2): Trace propagation across Spirit→Team→Graph.
	if bridge := turntrace.FromContext(ctx); bridge != nil {
		var span trace.Span
		ctx, span = bridge.StartChild(ctx, "graph.execute.register")
		defer turntrace.EndChild(span, nil)
	}
	if err := c.graphs.RegisterTeamGraphExecution(ctx, execID, sessionID, spiritSessionID, teamID, teamRunID, ct); err != nil {
		return err
	}
	execID = strings.TrimSpace(execID)
	sess := &teamGraphRunSession{
		teamRunID:       strings.TrimSpace(teamRunID),
		teamID:          strings.TrimSpace(teamID),
		sessionID:       strings.TrimSpace(sessionID),
		spiritSessionID: strings.TrimSpace(spiritSessionID),
		execID:          execID,
		stepDedup:       newGraphStepDedup(),
		registeredAt:    time.Now(),
	}
	if c.teamRunReader != nil {
		if run, err := c.teamRunReader.GetTeamRunByID(ctx, sess.teamRunID); err == nil {
			sess.inputPreview = strings.TrimSpace(run.InputPreview)
			sess.definitionJSON = strings.TrimSpace(run.DefinitionSnapshotJSON)
			reg, memberByNode, stepSortIndex := buildResumeSessionContext(run.DefinitionSnapshotJSON, sess.inputPreview, c.agentKeyFn, c.lg)
			sess.obsReg = reg
			sess.obsStore = biz.NewOrchestrationStatusStore(reg)
			sess.memberByNode = memberByNode
			sess.stepSortIndex = stepSortIndex
		}
	}
	c.mu.Lock()
	c.sessions[execID] = sess
	c.mu.Unlock()
	c.persistSession(ctx, sess, biz.TeamRunStatusRunning)
	return nil
}

func (c *TeamGraphRunCoordinator) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	if c == nil || c.graphs == nil {
		return nil
	}
	if err := c.graphs.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID); err != nil {
		return err
	}
	sess := c.session(execID)
	if sess == nil || c.teamRunReader == nil {
		return nil
	}
	run, err := c.teamRunReader.GetTeamRunByID(ctx, sess.teamRunID)
	if err != nil {
		return err
	}
	if run.Status == biz.TeamRunStatusWaitingHuman {
		return nil
	}
	sm := biz.NewTeamRunStateMachine()
	if !sm.CanTransition(biz.TeamRunState(run.Status), biz.TeamRunState(biz.TeamRunStatusWaitingHuman)) {
		return nil
	}
	_, transitionErr := c.runTransitioner.TransitionRunStatus(ctx, run.ID, biz.TeamRunStatusWaitingHuman)
	if transitionErr != nil {
		c.lg.Error("TransitionRunStatus failed in MarkTeamGraphInterrupt",
			loggateway.StepID("team.run.transition_fail"),
			loggateway.Str("team_run_id", run.ID), loggateway.Err(transitionErr))
		return transitionErr
	}
	c.updateSessionStatus(ctx, sess.execID, biz.TeamRunStatusWaitingHuman)
	return nil
}

// DeferTeamRunSuccessIfHITL keeps the team run open when graph execution paused for human task.
func (c *TeamGraphRunCoordinator) DeferTeamRunSuccessIfHITL(ctx context.Context, graphExecID string, run *biz.TeamRunRecord) (bool, error) {
	if c == nil || c.graphs == nil || run == nil {
		return false, nil
	}
	exec, err := c.graphs.GetExecution(ctx, strings.TrimSpace(graphExecID))
	if err != nil {
		return false, err
	}
	if exec.Status != biz.TeamRunStatusWaitingHuman {
		return false, nil
	}
	if strings.TrimSpace(exec.GetInterruptNode()) == "" {
		return false, nil
	}
	sm := biz.NewTeamRunStateMachine()
	if !sm.CanTransition(biz.TeamRunState(run.Status), biz.TeamRunState(biz.TeamRunStatusWaitingHuman)) {
		return false, nil
	}
	updatedRun, transitionErr := c.runTransitioner.TransitionRunStatus(ctx, run.ID, biz.TeamRunStatusWaitingHuman)
	if transitionErr != nil {
		c.lg.Error("TransitionRunStatus failed in DeferTeamRunSuccessIfHITL",
			loggateway.StepID("team.run.transition_fail"),
			loggateway.Str("team_run_id", run.ID), loggateway.Err(transitionErr))
		return false, transitionErr
	}
	*run = updatedRun
	return true, nil
}

func (c *TeamGraphRunCoordinator) HandleTeamGraphTaskCompleted(ctx context.Context, task *biz.GraphTask, resume map[string]any) (bool, error) {
	if c == nil || c.graphs == nil || task == nil {
		return false, nil
	}
	// Create graph execution child span (P3-2): Trace propagation across Spirit→Team→Graph.
	if bridge := turntrace.FromContext(ctx); bridge != nil {
		var span trace.Span
		ctx, span = bridge.StartChild(ctx, "graph.execute.resume")
		defer turntrace.EndChild(span, nil)
	}
	sess := c.session(task.ExecutionID)
	if sess == nil {
		return false, nil
	}
	exec, err := c.graphs.GetExecution(ctx, task.ExecutionID)
	if err != nil {
		return true, err
	}
	if !shouldResumeTeamGraph(exec, task.NodeID) {
		return true, nil
	}
	if _, err := c.graphs.ResumeExecution(ctx, task.ExecutionID, resume); err != nil {
		if c.teamRunReader != nil {
			if run, rerr := c.teamRunReader.GetTeamRunByID(ctx, sess.teamRunID); rerr == nil {
				if !biz.IsTeamRunTerminalStatus(run.Status) {
					sm := biz.NewTeamRunStateMachine()
					if !sm.CanTransition(biz.TeamRunState(run.Status), biz.TeamRunState(biz.TeamRunStatusFailed)) {
						c.lg.Warn("HandleTeamGraphTaskCompleted: illegal transition to failed skipped",
							loggateway.StepID("team.run.fsm_skip"),
							loggateway.Str("team_run_id", run.ID),
							loggateway.Str("from", run.Status))
					} else {
						updatedRun, transitionErr := c.runTransitioner.TransitionRunStatus(ctx, run.ID, biz.TeamRunStatusFailed)
						if transitionErr != nil {
							c.lg.Error("TransitionRunStatus failed in HandleTeamGraphTaskCompleted",
								loggateway.StepID("team.run.transition_fail"),
								loggateway.Str("team_run_id", run.ID), loggateway.Err(transitionErr))
						} else {
							updatedRun.ErrorMessage = fmt.Sprintf("ResumeExecution failed: %s", err.Error())
							if uerr := c.teamRunWriter.UpdateTeamRun(ctx, updatedRun); uerr != nil {
								c.lg.Warn("UpdateTeamRun failed after ResumeExecution error",
									loggateway.StepID("team.graph.resume_fail_update"),
									loggateway.Str("team_run_id", updatedRun.ID), loggateway.Str("update_error", uerr.Error()))
							}
							run = updatedRun
						}
					}
				}
				if c.activityBus != nil {
					ev := biz.ActivityEvent{
						Event: biz.ActivityEventFailed,
						Activity: biz.Activity{
							ID:               agent.TeamStageActivityID(sess.teamID),
							Kind:             biz.ActivityKindTeamStage,
							Status:           biz.ActivityStatusFailed,
							SessionID:        sess.sessionID,
							SpiritSessionID:  sess.spiritSessionID,
							TeamID:           sess.teamID,
							Timestamp:        time.Now().UTC(),
							Stage:            "failed",
							ParentActivityID: agent.GraphStageActivityID(sess.spiritSessionID),
							Meta: map[string]any{
								"run_id":        run.ID,
								"error_message": err.Error(),
							},
						},
						Domain: biz.ActivityDomainChat,
					}
					c.activityBus.Publish(ctx, ev)
				}
			}
		}
		c.evictSession(sess.execID)
		c.lg.Error("HandleTeamGraphTaskCompleted: ResumeExecution failed",
			loggateway.StepID("team.graph.resume_fail"),
			loggateway.Str("execution_id", task.ExecutionID),
			loggateway.Err(err))
		return true, err
	}
	run, err := c.teamRunReader.GetTeamRunByID(ctx, sess.teamRunID)
	if err != nil {
		return true, err
	}
	if run.Status == biz.TeamRunStatusWaitingHuman {
		sm := biz.NewTeamRunStateMachine()
		if !sm.CanTransition(biz.TeamRunState(run.Status), biz.TeamRunState(biz.TeamRunStatusRunning)) {
			c.lg.Warn("HandleTeamGraphTaskCompleted: invalid transition",
				loggateway.StepID("team.transition_invalid"),
				loggateway.Str("team_run_id", run.ID),
				loggateway.Str("from", run.Status),
				loggateway.Str("to", biz.TeamRunStatusRunning))
		} else {
			_, transitionErr := c.runTransitioner.TransitionRunStatus(ctx, run.ID, biz.TeamRunStatusRunning)
			if transitionErr != nil {
				c.lg.Error("TransitionRunStatus failed in HandleTeamGraphTaskCompleted",
					loggateway.StepID("team.run.transition_fail"),
					loggateway.Str("team_run_id", run.ID), loggateway.Err(transitionErr))
			}
			c.updateSessionStatus(ctx, sess.execID, biz.TeamRunStatusRunning)
		}
	}
	c.startGraphWatch(ctx, sess, graphWatchStepsAndFinalize)
	return true, nil
}

// StartGraphStepWatch persists team_run_steps from graph events during the initial Team Graph run (BL-03).
func (c *TeamGraphRunCoordinator) StartGraphStepWatch(ctx context.Context, execID string) context.CancelFunc {
	sess := c.session(execID)
	if sess == nil {
		return func() {}
	}
	return c.startGraphWatch(ctx, sess, graphWatchStepsOnly)
}

func (c *TeamGraphRunCoordinator) startGraphWatch(ctx context.Context, sess *teamGraphRunSession, mode graphWatchMode) context.CancelFunc {
	if c == nil || c.activityBus == nil || sess == nil {
		return func() {}
	}
	c.stopWatch(sess.execID)
	watchCtx, cancel := context.WithCancel(ctx)
	// Assign watchStop and update sessions map inside the same lock scope
	// to prevent concurrent startGraphWatch/stopWatch from observing a nil watchStop.
	c.mu.Lock()
	sess.watchStop = cancel
	c.sessions[sess.execID] = sess
	c.mu.Unlock()

	safego.Go(watchCtx, "team.graph.run.watch", func() {
		defer cancel()
		ch, unsub := c.activityBus.Subscribe(biz.ActivityEventSubscribeOptions{
			SessionID:  sess.sessionID,
			BufferSize: 32,
		})
		defer unsub()
		watchTimeout := c.cfg.WatchTimeout
		if watchTimeout <= 0 {
			watchTimeout = defaultGraphWatchTimeout
		}
		hitlSLA := c.cfg.HITLSLATimeout
		if hitlSLA <= 0 {
			hitlSLA = defaultHITLSLATimeout
		}
		deadline := time.After(watchTimeout)
		hitlExtensions := 0
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-deadline:
				if mode == graphWatchStepsAndFinalize {
					sessRun, runErr := c.teamRunReader.GetTeamRunByID(watchCtx, sess.teamRunID)
					if runErr == nil && sessRun.Status == biz.TeamRunStatusWaitingHuman && hitlExtensions < maxHITLSLAExtensions {
						hitlExtensions++
						c.lg.Warn("HITL SLA expired, extending deadline for manual resolution",
							loggateway.StepID("team.hitl_sla_expired"),
							loggateway.Str("exec_id", sess.execID),
							loggateway.Str("hitl_sla", hitlSLA.String()),
							loggateway.Int("extension", hitlExtensions),
							loggateway.Int("max_extensions", maxHITLSLAExtensions))
						deadline = time.After(hitlSLA)
						continue
					}
					c.finalizeTeamRun(watchCtx, sess, true, fmt.Sprintf("graph resume watch timed out after %s", watchTimeout))
				}
				return
			case aev, ok := <-ch:
				if !ok {
					return
				}
				if done, failed, errMsg := c.handleGraphWatchActivity(watchCtx, sess, aev, mode); done {
					c.finalizeTeamRun(watchCtx, sess, failed, errMsg)
					return
				}
			}
		}
	})
	return cancel
}

func (c *TeamGraphRunCoordinator) startCompletionWatch(ctx context.Context, sess *teamGraphRunSession) {
	c.startGraphWatch(ctx, sess, graphWatchStepsAndFinalize)
}

func shouldResumeTeamGraph(exec *biz.GraphExecution, nodeID string) bool {
	if exec == nil {
		return false
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false
	}
	if exec.Status == biz.TeamRunStatusWaitingHuman && (exec.GetInterruptNode() == nodeID || exec.CurrentNode == nodeID) {
		return true
	}
	return strings.HasPrefix(exec.GraphID, "team:") && exec.GetInterruptNode() == nodeID
}

func (c *TeamGraphRunCoordinator) handleGraphWatchActivity(ctx context.Context, sess *teamGraphRunSession, aev biz.ActivityEvent, mode graphWatchMode) (done, failed bool, errMsg string) {
	if aev.Activity.Kind != biz.ActivityKindGraphStage {
		return false, false, ""
	}
	execID := metaString(aev.Activity.Meta, "execution_id")
	if execID != "" && execID != sess.execID {
		return false, false, ""
	}
	stepCtx := sess.stepContext()
	isNodeEnd := aev.Activity.Stage == "node_end"
	isNodeStart := aev.Activity.Stage == "node_start"
	isStepFinished := aev.Activity.Kind == biz.ActivityKindTeamStage && aev.Event == biz.ActivityEventCompleted
	if sess.obsStore != nil {
		changed := sess.obsStore.ApplyActivityEvent(aev, sess.obsReg)
		for _, st := range changed {
			if isNodeStart && c.finisher != nil && stepCtx != nil {
				c.finisher.PublishTeamStepStarted(ctx, stepCtx, st.NodeID)
			}
			if isNodeEnd || isStepFinished {
				if biz.IsTerminalAgentNodeStatus(st.Status) && c.finisher != nil && stepCtx != nil {
					skipped := st.Status == biz.AgentNodeStatusSkipped
					errText := st.ErrorMessage
					if st.Status == biz.AgentNodeStatusFailed && errText == "" {
						errText = "graph node failed"
					}
					c.finisher.PersistGraphRunStep(ctx, stepCtx, st.NodeID, st.OutputPreview, errText, skipped, 0)
				}
			}
		}
	}
	if isStepFinished && stepCtx != nil {
		if nodeID := resumeStepNodeID(aev.Activity.Meta, sess.obsReg); nodeID != "" {
			stepCtx.MarkPersisted(nodeID)
		}
	}
	if mode == graphWatchStepsOnly {
		return false, false, ""
	}
	switch aev.Activity.Stage {
	case "node_error":
		if aev.Event != biz.ActivityEventFailed {
			return false, false, ""
		}
		if resumeMetaBool(aev.Activity.Meta, "retrying") {
			return false, false, ""
		}
		msg := metaString(aev.Activity.Meta, "error")
		if strings.TrimSpace(aev.Activity.Content) != "" && msg == "" {
			msg = strings.TrimSpace(aev.Activity.Content)
		}
		return true, true, msg
	case "execution_done":
		if aev.Event != biz.ActivityEventCompleted {
			return false, false, ""
		}
		return true, false, ""
	}
	return false, false, ""
}

func resumeStepNodeID(meta map[string]any, reg biz.OrchestrationRegistry) string {
	if meta == nil {
		return ""
	}
	step, ok := meta["step"].(map[string]any)
	if !ok {
		return ""
	}
	if agentID, ok := step["agent_id"].(string); ok && strings.TrimSpace(agentID) != "" {
		for nodeID, entry := range reg.ByNodeID {
			if strings.EqualFold(entry.AgentID, strings.TrimSpace(agentID)) {
				return nodeID
			}
		}
	}
	return ""
}

func resumeMetaBool(meta map[string]any, key string) bool {
	if meta == nil {
		return false
	}
	v, ok := meta[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func metaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func (c *TeamGraphRunCoordinator) finalizeTeamRun(ctx context.Context, sess *teamGraphRunSession, failed bool, errMsg string) {
	if c == nil || sess == nil {
		return
	}
	if c.finisher != nil {
		c.finisher.FinalizeGraphTeamRun(ctx, sess.stepContext(), failed, errMsg)
		c.evictSession(sess.execID)
		return
	}
	if c.teamRunReader == nil {
		return
	}
	run, err := c.teamRunReader.GetTeamRunByID(ctx, sess.teamRunID)
	if err != nil {
		return
	}
	if run.Status != biz.TeamRunStatusWaitingHuman && run.Status != biz.TeamRunStatusRunning {
		return
	}
	newStatus := biz.TeamRunStatusSuccess
	if failed {
		newStatus = biz.TeamRunStatusFailed
	}
	// Bounded retry loop: re-read current state and re-attempt transition.
	// This replaces the previous TECH-DEBT(S8) fallback that bypassed the FSM
	// and CAS guard via direct mutation. If all retries fail, we log and return
	// without mutating — a reconciler or manual intervention must resolve it.
	const maxRetries = 3
	var lastErr error
	updatedRun, transitionErr := c.runTransitioner.TransitionRunStatus(ctx, run.ID, newStatus)
	if transitionErr != nil {
		sm := biz.NewTeamRunStateMachine()
		for attempt := 1; attempt <= maxRetries; attempt++ {
			// Re-read current state to handle concurrent updates.
			currentRun, rerr := c.teamRunReader.GetTeamRunByID(ctx, run.ID)
			if rerr != nil {
				lastErr = rerr
				break
			}
			if !sm.CanTransition(biz.TeamRunState(currentRun.Status), biz.TeamRunState(newStatus)) {
				// State machine rejects this transition — do NOT fall back to direct
				// mutation. Log and return so the caller sees the inconsistency.
				c.lg.Warn("TransitionRunStatus rejected by state machine in finalizeTeamRun",
					loggateway.StepID("team.run.transition_rejected"),
					loggateway.Str("team_run_id", run.ID),
					loggateway.Str("from_status", currentRun.Status),
					loggateway.Str("to_status", newStatus),
					loggateway.Err(transitionErr))
				return
			}
			updatedRun, transitionErr = c.runTransitioner.TransitionRunStatus(ctx, run.ID, newStatus)
			if transitionErr == nil {
				lastErr = nil
				break
			}
			lastErr = transitionErr
		}
		if lastErr != nil {
			c.lg.Error("TransitionRunStatus exhausted retries in finalizeTeamRun",
				loggateway.StepID("team.run.transition_exhausted"),
				loggateway.Str("team_run_id", run.ID),
				loggateway.Str("to_status", newStatus),
				loggateway.Int("attempts", maxRetries),
				loggateway.Err(lastErr))
			return
		}
	}
	// Preserve extra fields that TransitionRunStatus doesn't set.
	if failed {
		updatedRun.ErrorMessage = errMsg
	}
	if updatedRun.ErrorMessage != "" || !failed {
		if err := c.teamRunWriter.UpdateTeamRun(ctx, updatedRun); err != nil {
			c.lg.Warn("UpdateTeamRun failed in finalizeTeamRun",
				loggateway.StepID("team.graph.finalize_update_fail"),
				loggateway.Str("team_run_id", updatedRun.ID), loggateway.Str("update_error", err.Error()))
		}
	}
	run = updatedRun
	if c.activityBus != nil {
		var eventType biz.ActivityEventType
		var status biz.ActivityStatus
		var stage string
		if failed {
			eventType = biz.ActivityEventFailed
			status = biz.ActivityStatusFailed
			stage = "failed"
		} else {
			eventType = biz.ActivityEventCompleted
			status = biz.ActivityStatusCompleted
			stage = "completed"
		}
		ev := biz.ActivityEvent{
			Event: eventType,
			Activity: biz.Activity{
				ID:               agent.TeamStageActivityID(sess.teamID),
				Kind:             biz.ActivityKindTeamStage,
				Status:           status,
				SessionID:        sess.sessionID,
				SpiritSessionID:  sess.spiritSessionID,
				TeamID:           sess.teamID,
				Timestamp:        time.Now().UTC(),
				Stage:            stage,
				ParentActivityID: agent.GraphStageActivityID(sess.spiritSessionID),
				Meta:             map[string]any{"run_id": run.ID, "run": run},
			},
			Domain: biz.ActivityDomainChat,
		}
		c.activityBus.Publish(ctx, ev)
	}
	c.evictSession(sess.execID)
}

func (c *TeamGraphRunCoordinator) evictSession(execID string) {
	c.stopWatch(execID)
	c.mu.Lock()
	delete(c.sessions, strings.TrimSpace(execID))
	c.mu.Unlock()
	c.deleteSessionFromDB(execID)
}

// CleanupStaleSessions removes sessions older than sessionMaxAge (e.g. after process restart).
// DB deletion is performed outside the lock to avoid blocking concurrent access.
func (c *TeamGraphRunCoordinator) CleanupStaleSessions() {
	if c == nil {
		return
	}
	maxAge := c.cfg.SessionMaxAge
	if maxAge <= 0 {
		maxAge = sessionMaxAge
	}
	now := time.Now()
	// Phase 1: collect stale sessions under lock.
	var stale []struct {
		execID string
		age    time.Duration
	}
	c.mu.Lock()
	for id, sess := range c.sessions {
		if !sess.registeredAt.IsZero() && now.Sub(sess.registeredAt) > maxAge {
			if sess.watchStop != nil {
				sess.watchStop()
				sess.watchStop = nil
			}
			delete(c.sessions, id)
			stale = append(stale, struct {
				execID string
				age    time.Duration
			}{execID: id, age: now.Sub(sess.registeredAt)})
		}
	}
	c.mu.Unlock()
	// Phase 2: delete from DB outside the lock.
	for _, s := range stale {
		c.deleteSessionFromDB(s.execID)
		c.lg.Warn("CleanupStaleSessions: evicted stale session",
			loggateway.StepID("team.session.stale_evicted"),
			loggateway.Str("exec_id", s.execID),
			loggateway.Str("age", s.age.String()))
	}
}

func (c *TeamGraphRunCoordinator) session(execID string) *teamGraphRunSession {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessions[strings.TrimSpace(execID)]
}

func (c *TeamGraphRunCoordinator) stopWatch(execID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sess := c.sessions[strings.TrimSpace(execID)]
	if sess != nil && sess.watchStop != nil {
		sess.watchStop()
		sess.watchStop = nil
	}
}

func (c *TeamGraphRunCoordinator) persistSession(ctx context.Context, sess *teamGraphRunSession, status string) {
	if c == nil || c.sessionRepo == nil || sess == nil {
		return
	}
	now := agent.RFC3339Now()
	dbSess := biz.TeamGraphSession{
		ExecID:         sess.execID,
		TeamRunID:      sess.teamRunID,
		TeamID:         sess.teamID,
		SessionID:      sess.sessionID,
		InputPreview:   sess.inputPreview,
		DefinitionJSON: sess.definitionJSON,
		Status:         status,
		RegisteredAt:   now,
		LastActivityAt: now,
		UpdatedAt:      now,
	}
	if existing, err := c.sessionRepo.GetSession(ctx, sess.execID); err == nil {
		dbSess.CreatedAt = existing.CreatedAt
	} else {
		dbSess.CreatedAt = now
	}
	if err := c.sessionRepo.SaveSession(ctx, dbSess); err != nil {
		c.lg.Warn("persistSession: SaveSession failed",
			loggateway.StepID("team.session.persist_fail"),
			loggateway.Str("exec_id", sess.execID),
			loggateway.Err(err))
	}
}

func (c *TeamGraphRunCoordinator) updateSessionStatus(ctx context.Context, execID, status string) {
	if c == nil || c.sessionRepo == nil {
		return
	}
	if err := c.sessionRepo.UpdateSessionStatus(ctx, execID, status); err != nil {
		c.lg.Warn("updateSessionStatus failed",
			loggateway.StepID("team.session.update_fail"),
			loggateway.Str("exec_id", execID),
			loggateway.Str("status", status),
			loggateway.Err(err))
	}
}

func (c *TeamGraphRunCoordinator) deleteSessionFromDB(execID string) {
	if c == nil || c.sessionRepo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.sessionRepo.DeleteSession(ctx, execID); err != nil {
		c.lg.Warn("deleteSessionFromDB failed",
			loggateway.StepID("team.session.delete_fail"),
			loggateway.Str("exec_id", execID),
			loggateway.Err(err))
	}
}

// RecoverSessions rebuilds in-memory sessions from DB after process restart (BL-04b).
// Running sessions whose graph runtime was lost are cancelled; waiting_human sessions
// are re-registered so that HITL task completion can resume them.
func (c *TeamGraphRunCoordinator) RecoverSessions(ctx context.Context) {
	if c == nil || c.sessionRepo == nil {
		return
	}
	cancelled, err := c.sessionRepo.MarkOrphanedSessionsTerminal(ctx)
	if err != nil {
		c.lg.Warn("RecoverSessions: MarkOrphanedSessionsTerminal failed",
			loggateway.StepID("team.session.recover_fail"),
			loggateway.Err(err))
	}
	if cancelled > 0 {
		c.lg.Warn("RecoverSessions: cancelled orphaned running sessions",
			loggateway.StepID("team.session.orphan_cancelled"),
			loggateway.Int("count", cancelled))
	}
	active, err := c.sessionRepo.ListActiveSessions(ctx)
	if err != nil {
		c.lg.Warn("RecoverSessions: ListActiveSessions failed",
			loggateway.StepID("team.session.recover_fail"),
			loggateway.Err(err))
		return
	}
	if len(active) == 0 {
		return
	}
	recovered := 0
	for _, dbSess := range active {
		sess := &teamGraphRunSession{
			teamRunID:      dbSess.TeamRunID,
			teamID:         dbSess.TeamID,
			sessionID:      dbSess.SessionID,
			execID:         dbSess.ExecID,
			inputPreview:   dbSess.InputPreview,
			definitionJSON: dbSess.DefinitionJSON,
			stepDedup:      newGraphStepDedup(),
			registeredAt:   time.Now(),
		}
		reg, memberByNode, stepSortIndex := buildResumeSessionContext(dbSess.DefinitionJSON, dbSess.InputPreview, c.agentKeyFn, c.lg)
		sess.obsReg = reg
		sess.obsStore = biz.NewOrchestrationStatusStore(reg)
		sess.memberByNode = memberByNode
		sess.stepSortIndex = stepSortIndex
		c.mu.Lock()
		c.sessions[dbSess.ExecID] = sess
		c.mu.Unlock()
		if dbSess.Status == biz.TeamRunStatusWaitingHuman {
			c.startCompletionWatch(ctx, sess)
		}
		recovered++
	}
	c.lg.Warn("RecoverSessions: recovered sessions from DB",
		loggateway.StepID("team.session.recovered"),
		loggateway.Int("recovered", recovered))
}

// BuildTaskResumeValue builds the resume payload for graph checkpoint continuation.
func BuildTaskResumeValue(task *biz.GraphTask) map[string]any {
	if task == nil {
		return map[string]any{}
	}
	resume := map[string]any{
		"task_id": task.TaskID,
		"node_id": task.NodeID,
		"output":  task.Output,
		"summary": task.Summary,
	}
	if b, err := json.Marshal(resume); err == nil {
		resume["task_result_json"] = string(b)
	}
	return resume
}
