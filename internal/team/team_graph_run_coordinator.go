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
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const defaultGraphWatchTimeout = 30 * time.Minute

const defaultHITLSLATimeout = 24 * time.Hour

const maxHITLSLAExtensions = 3

const sessionMaxAge = 2 * time.Hour

const defaultCleanupInterval = 10 * time.Minute

type CoordinatorConfig struct {
	WatchTimeout   time.Duration
	HITLSLATimeout time.Duration
	SessionMaxAge  time.Duration
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
	RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, ct *biz.CompiledTeam) error
	MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error
	ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*biz.GraphExecution, error)
	GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error)
}

// TeamGraphTaskResumeHandler resumes team Graph runs after Kanban task completion.
type TeamGraphTaskResumeHandler interface {
	HandleTeamGraphTaskCompleted(ctx context.Context, task *biz.GraphTask, resume map[string]any) (handled bool, err error)
}

// TeamGraphRunCoordinator unifies team graph execution register, HITL defer, and task resume (M53 P1).
type TeamGraphRunCoordinator struct {
	graphs         TeamGraphExecutionBackend
	teams          biz.TeamRunRepo
	bus            event.Bus
	finisher       *TeamRunMediator
	sessionRepo    biz.TeamGraphSessionRepo
	cfg            CoordinatorConfig
	lg             loggateway.Logger
	agentKeyFn     func(agentID string) string

	mu       sync.RWMutex
	sessions map[string]*teamGraphRunSession
}

type teamGraphRunSession struct {
	teamRunID      string
	teamID         string
	sessionID      string
	execID         string
	inputPreview   string
	definitionJSON string
	watchStop      context.CancelFunc

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

func NewTeamGraphRunCoordinator(graphs TeamGraphExecutionBackend, teams biz.TeamRunRepo, bus event.Bus, sessionRepo biz.TeamGraphSessionRepo, agentKeyFn func(agentID string) string, lg loggateway.Logger) *TeamGraphRunCoordinator {
	if agentKeyFn == nil {
		agentKeyFn = func(agentID string) string { return strings.TrimSpace(agentID) }
	}
	return &TeamGraphRunCoordinator{
		graphs:         graphs,
		teams:          teams,
		bus:            bus,
		sessionRepo:    sessionRepo,
		agentKeyFn:     agentKeyFn,
		sessions:       make(map[string]*teamGraphRunSession),
		cfg:            DefaultCoordinatorConfig(),
		lg:             lg,
	}
}

// SetFinisher wires the mediator for step persistence and team run finalization.
func (c *TeamGraphRunCoordinator) SetFinisher(m *TeamRunMediator) {
	if c == nil {
		return
	}
	c.finisher = m
}

func (c *TeamGraphRunCoordinator) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, ct *biz.CompiledTeam) error {
	if c == nil || c.graphs == nil {
		return nil
	}
	if err := c.graphs.RegisterTeamGraphExecution(ctx, execID, sessionID, teamID, teamRunID, ct); err != nil {
		return err
	}
	execID = strings.TrimSpace(execID)
	sess := &teamGraphRunSession{
		teamRunID:    strings.TrimSpace(teamRunID),
		teamID:       strings.TrimSpace(teamID),
		sessionID:    strings.TrimSpace(sessionID),
		execID:       execID,
		stepDedup:    newGraphStepDedup(),
		registeredAt: time.Now(),
	}
	if c.teams != nil {
		if run, err := c.teams.GetTeamRunByID(ctx, sess.teamRunID); err == nil {
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
	if sess == nil || c.teams == nil {
		return nil
	}
	run, err := c.teams.GetTeamRunByID(ctx, sess.teamRunID)
	if err != nil {
		return err
	}
	if run.Status == biz.TeamRunStatusWaitingHuman {
		return nil
	}
	if !biz.ValidateTeamRunTransition(run.Status, biz.TeamRunStatusWaitingHuman) {
		return nil
	}
	run.Status = biz.TeamRunStatusWaitingHuman
	if err := c.teams.UpdateTeamRun(ctx, run); err != nil {
		return err
	}
	c.updateSessionStatus(ctx, sess.execID, biz.TeamRunStatusWaitingHuman)
	return nil
}

// DeferTeamRunSuccessIfHITL keeps the team run open when graph execution paused for human task.
func (c *TeamGraphRunCoordinator) DeferTeamRunSuccessIfHITL(ctx context.Context, graphExecID string, run *biz.TeamRun) (bool, error) {
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
	if !biz.ValidateTeamRunTransition(run.Status, biz.TeamRunStatusWaitingHuman) {
		return false, nil
	}
	run.Status = biz.TeamRunStatusWaitingHuman
	if err := c.teams.UpdateTeamRun(ctx, *run); err != nil {
		return false, err
	}
	return true, nil
}

func (c *TeamGraphRunCoordinator) HandleTeamGraphTaskCompleted(ctx context.Context, task *biz.GraphTask, resume map[string]any) (bool, error) {
	if c == nil || c.graphs == nil || task == nil {
		return false, nil
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
		if c.teams != nil {
			if run, rerr := c.teams.GetTeamRunByID(ctx, sess.teamRunID); rerr == nil {
				if !biz.IsTeamRunTerminalStatus(run.Status) {
					run.Status = biz.TeamRunStatusFailed
					run.ErrorMessage = fmt.Sprintf("ResumeExecution failed: %s", err.Error())
					run.FinishedAt = agent.RFC3339Now()
					if uerr := c.teams.UpdateTeamRun(ctx, run); uerr != nil {
						c.lg.Warn("UpdateTeamRun failed after ResumeExecution error",
						loggateway.StepID("team.graph.resume_fail_update"),
						loggateway.Str("team_run_id", run.ID), loggateway.Str("update_error", uerr.Error()))
					}
				}
				if c.bus != nil {
					failEnv := event.NewEnvelope(event.EnvelopeTypeTeamRunFailed, "team-coordinator", sess.sessionID)
					failEnv.TeamID = sess.teamID
					failEnv.Metadata = map[string]any{"run_id": run.ID, "error_message": err.Error()}
					c.bus.Publish(ctx, failEnv)
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
	run, err := c.teams.GetTeamRunByID(ctx, sess.teamRunID)
	if err != nil {
		return true, err
	}
	if run.Status == biz.TeamRunStatusWaitingHuman {
		if !biz.ValidateTeamRunTransition(run.Status, biz.TeamRunStatusRunning) {
			c.lg.Warn("HandleTeamGraphTaskCompleted: invalid transition",
				loggateway.StepID("team.transition_invalid"),
				loggateway.Str("team_run_id", run.ID),
				loggateway.Str("from", run.Status),
				loggateway.Str("to", biz.TeamRunStatusRunning))
		} else {
			run.Status = biz.TeamRunStatusRunning
			if err := c.teams.UpdateTeamRun(ctx, run); err != nil {
				c.lg.Warn("HandleTeamGraphTaskCompleted: UpdateTeamRun failed",
					loggateway.StepID("team.usage_record_fail"),
					loggateway.Str("team_run_id", run.ID),
					loggateway.Err(err))
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
	if c == nil || c.bus == nil || sess == nil {
		return func() {}
	}
	c.stopWatch(sess.execID)
	watchCtx, cancel := context.WithCancel(ctx)
	sess.watchStop = cancel
	c.mu.Lock()
	c.sessions[sess.execID] = sess
	c.mu.Unlock()

	safego.Go(watchCtx, "team.graph.run.watch", func() {
		defer cancel()
		ch, unsub := c.bus.Subscribe(event.SubscribeOptions{
			SessionID:  sess.sessionID,
			BufferSize: 32,
			DropPolicy: event.DropNewest,
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
					sessRun, runErr := c.teams.GetTeamRunByID(watchCtx, sess.teamRunID)
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
			case env, ok := <-ch:
				if !ok {
					return
				}
				if done, failed, errMsg := c.handleGraphWatchEnvelope(watchCtx, sess, env, mode); done {
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

func (c *TeamGraphRunCoordinator) handleGraphWatchEnvelope(ctx context.Context, sess *teamGraphRunSession, env event.Envelope, mode graphWatchMode) (done, failed bool, errMsg string) {
	if execID := trackerMetaString(env.Metadata, "execution_id"); execID != "" && execID != sess.execID {
		return false, false, ""
	}
	stepCtx := sess.stepContext()
	if sess.obsStore != nil {
		changed := sess.obsStore.ApplyEnvelope(env, sess.obsReg)
		for _, st := range changed {
			switch env.Type {
			case event.EnvelopeTypeMemberMessageDone, event.EnvelopeTypeGraphNodeEnd, event.EnvelopeTypeTeamStepFinished:
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
	if env.Type == event.EnvelopeTypeTeamStepFinished && stepCtx != nil {
		if nodeID := resumeStepNodeID(env.Metadata, sess.obsReg); nodeID != "" {
			stepCtx.MarkPersisted(nodeID)
		}
	}
	if mode == graphWatchStepsOnly {
		return false, false, ""
	}
	switch env.Type {
	case event.EnvelopeTypeGraphNodeError:
		if resumeMetaBool(env.Metadata, "retrying") {
			return false, false, ""
		}
		msg := trackerMetaString(env.Metadata, "error")
		if env.Error != nil && msg == "" {
			msg = env.Error.Message
		}
		return true, true, msg
	case event.EnvelopeTypeGraphExecutionDone:
		if trackerMetaString(env.Metadata, "execution_id") != sess.execID {
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

func (c *TeamGraphRunCoordinator) finalizeTeamRun(ctx context.Context, sess *teamGraphRunSession, failed bool, errMsg string) {
	if c == nil || sess == nil {
		return
	}
	if c.finisher != nil {
		c.finisher.FinalizeGraphTeamRun(ctx, sess.stepContext(), failed, errMsg)
		c.evictSession(sess.execID)
		return
	}
	if c.teams == nil {
		return
	}
	run, err := c.teams.GetTeamRunByID(ctx, sess.teamRunID)
	if err != nil {
		return
	}
	if run.Status != biz.TeamRunStatusWaitingHuman && run.Status != biz.TeamRunStatusRunning {
		return
	}
	now := agent.RFC3339Now()
	run.FinishedAt = now
	run.UpdatedAt = now
	if failed {
		run.Status = biz.TeamRunStatusFailed
		run.ErrorMessage = errMsg
	} else {
		run.Status = biz.TeamRunStatusSuccess
	}
	if err := c.teams.UpdateTeamRun(ctx, run); err != nil {
		c.lg.Warn("UpdateTeamRun failed in finalizeTeamRun",
			loggateway.StepID("team.graph.finalize_update_fail"),
			loggateway.Str("team_run_id", run.ID), loggateway.Str("update_error", err.Error()))
	}
	if c.bus != nil {
		typ := event.EnvelopeTypeTeamRunFinished
		if failed {
			typ = event.EnvelopeTypeTeamRunFailed
		}
		env := event.NewEnvelope(typ, "team-graph-coordinator", sess.sessionID)
		env.TeamID = sess.teamID
		env.Metadata = map[string]any{"run_id": run.ID, "run": run}
		c.bus.Publish(ctx, env)
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
func (c *TeamGraphRunCoordinator) CleanupStaleSessions() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	maxAge := c.cfg.SessionMaxAge
	if maxAge <= 0 {
		maxAge = sessionMaxAge
	}
	now := time.Now()
	for id, sess := range c.sessions {
		if !sess.registeredAt.IsZero() && now.Sub(sess.registeredAt) > maxAge {
			if sess.watchStop != nil {
				sess.watchStop()
				sess.watchStop = nil
			}
			delete(c.sessions, id)
			c.deleteSessionFromDB(id)
			c.lg.Warn("CleanupStaleSessions: evicted stale session",
				loggateway.StepID("team.session.stale_evicted"),
				loggateway.Str("exec_id", id),
				loggateway.Str("age", now.Sub(sess.registeredAt).String()))
		}
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
