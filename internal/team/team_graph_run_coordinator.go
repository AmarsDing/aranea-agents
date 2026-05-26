package team

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

const teamRunStatusWaitingHuman = biz.TeamRunStatusWaitingHuman

const defaultGraphWatchTimeout = 30 * time.Minute

const sessionMaxAge = 2 * time.Hour

// TeamGraphExecutionBackend indexes and resumes team-linked graph executions.
type TeamGraphExecutionBackend interface {
	RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, cfg biz.GraphBuildConfig) error
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
	graphs        TeamGraphExecutionBackend
	teams         biz.TeamRepository
	bus           event.Bus
	finisher      TeamGraphRunFinisher
	watchTimeout  time.Duration

	mu       sync.RWMutex
	sessions map[string]*teamGraphRunSession
}

type teamGraphRunSession struct {
	teamRunID    string
	teamID       string
	sessionID    string
	execID       string
	inputPreview string
	definitionJSON string
	watchStop    context.CancelFunc

	stepDedup      *graphStepDedup
	memberByNode   map[string]MemberDef
	stepSortIndex  map[string]int
	obsReg         biz.OrchestrationRegistry
	obsStore       *biz.OrchestrationStatusStore
	registeredAt   time.Time
}

type graphWatchMode int

const (
	graphWatchStepsOnly graphWatchMode = iota
	graphWatchStepsAndFinalize
)

func NewTeamGraphRunCoordinator(graphs TeamGraphExecutionBackend, teams biz.TeamRepository, bus event.Bus) *TeamGraphRunCoordinator {
	return &TeamGraphRunCoordinator{
		graphs:       graphs,
		teams:        teams,
		bus:          bus,
		sessions:     make(map[string]*teamGraphRunSession),
		watchTimeout: defaultGraphWatchTimeout,
	}
}

func (c *TeamGraphRunCoordinator) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, cfg biz.GraphBuildConfig) error {
	if c == nil || c.graphs == nil {
		return nil
	}
	if err := c.graphs.RegisterTeamGraphExecution(ctx, execID, sessionID, teamID, teamRunID, cfg); err != nil {
		return err
	}
	execID = strings.TrimSpace(execID)
	sess := &teamGraphRunSession{
		teamRunID:      strings.TrimSpace(teamRunID),
		teamID:         strings.TrimSpace(teamID),
		sessionID:      strings.TrimSpace(sessionID),
		execID:         execID,
		stepDedup:      newGraphStepDedup(),
		registeredAt:   time.Now(),
	}
	if c.teams != nil {
		if run, err := c.teams.GetTeamRunByID(ctx, sess.teamRunID); err == nil {
			sess.inputPreview = strings.TrimSpace(run.InputPreview)
			sess.definitionJSON = strings.TrimSpace(run.DefinitionSnapshotJSON)
			reg, memberByNode, stepSortIndex := buildResumeSessionContext(run.DefinitionSnapshotJSON, sess.inputPreview)
			sess.obsReg = reg
			sess.obsStore = biz.NewOrchestrationStatusStore(reg)
			sess.memberByNode = memberByNode
			sess.stepSortIndex = stepSortIndex
		}
	}
	c.mu.Lock()
	c.sessions[execID] = sess
	c.mu.Unlock()
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
	if run.Status == teamRunStatusWaitingHuman {
		return nil
	}
	run.Status = teamRunStatusWaitingHuman
	return c.teams.UpdateTeamRun(ctx, run)
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
	if exec.Status != teamRunStatusWaitingHuman {
		return false, nil
	}
	if strings.TrimSpace(exec.InterruptNode) == "" {
		return false, nil
	}
	run.Status = teamRunStatusWaitingHuman
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
		if c.bus != nil {
			run, rerr := c.teams.GetTeamRunByID(ctx, sess.teamRunID)
			if rerr == nil {
				failEnv := event.NewEnvelope(event.EnvelopeTypeTeamRunFailed, "team-coordinator", sess.sessionID)
				failEnv.TeamID = sess.teamID
				failEnv.Metadata = map[string]any{"run_id": run.ID, "error_message": err.Error()}
				c.bus.Publish(ctx, failEnv)
			}
		}
		slog.Error("HandleTeamGraphTaskCompleted: ResumeExecution failed",
			"execution_id", task.ExecutionID, "error", err)
		return true, err
	}
	run, err := c.teams.GetTeamRunByID(ctx, sess.teamRunID)
	if err != nil {
		return true, err
	}
	if run.Status == teamRunStatusWaitingHuman {
		run.Status = biz.TeamRunStatusRunning
		if err := c.teams.UpdateTeamRun(ctx, run); err != nil {
			slog.Warn("HandleTeamGraphTaskCompleted: UpdateTeamRun failed", "team_run_id", run.ID, "error", err)
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
		watchTimeout := c.watchTimeout
		if watchTimeout <= 0 {
			watchTimeout = defaultGraphWatchTimeout
		}
		deadline := time.After(watchTimeout)
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-deadline:
				if mode == graphWatchStepsAndFinalize {
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
	if exec.Status == teamRunStatusWaitingHuman && (exec.InterruptNode == nodeID || exec.CurrentNode == nodeID) {
		return true
	}
	return strings.HasPrefix(exec.GraphID, "team:") && exec.InterruptNode == nodeID
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
	if run.Status != teamRunStatusWaitingHuman && run.Status != biz.TeamRunStatusRunning {
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
	_ = c.teams.UpdateTeamRun(ctx, run)
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
}

// CleanupStaleSessions removes sessions older than sessionMaxAge (e.g. after process restart).
func (c *TeamGraphRunCoordinator) CleanupStaleSessions() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for id, sess := range c.sessions {
		if !sess.registeredAt.IsZero() && now.Sub(sess.registeredAt) > sessionMaxAge {
			if sess.watchStop != nil {
				sess.watchStop()
				sess.watchStop = nil
			}
			delete(c.sessions, id)
			slog.Warn("CleanupStaleSessions: evicted stale session", "exec_id", id, "age", now.Sub(sess.registeredAt))
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
