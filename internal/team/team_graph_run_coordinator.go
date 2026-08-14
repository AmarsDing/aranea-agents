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
	rt "aranea-agents/internal/runtime"
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

// defaultSuspendIdleThreshold is the P3-3 (ADR-D) idle window after which a
// waiting_human session is suspended: evicted from memory while its DB row is
// retained (status unchanged) so a later resume signal can wake it.
const defaultSuspendIdleThreshold = 30 * time.Minute

type CoordinatorConfig struct {
	WatchTimeout    time.Duration
	HITLSLATimeout  time.Duration
	SessionMaxAge   time.Duration
	CleanupInterval time.Duration
	// SuspendIdleThreshold gates SuspendIdleWaits (P3-3); <=0 falls back to
	// defaultSuspendIdleThreshold.
	SuspendIdleThreshold time.Duration
}

func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		WatchTimeout:         defaultGraphWatchTimeout,
		HITLSLATimeout:       defaultHITLSLATimeout,
		SessionMaxAge:        sessionMaxAge,
		CleanupInterval:      defaultCleanupInterval,
		SuspendIdleThreshold: defaultSuspendIdleThreshold,
	}
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
	// v2 EventBus: subscribe SystemNoticeEvent for graph watch; publish via seq.
	eventBus    biz.EventBus
	seq         rt.EventPublisher // Publish via v2 Sequencer (FIFO + retry); eventBus retained for Subscribe
	finisher    *TeamRunMediator
	sessionRepo biz.TeamGraphSessionRepo
	cfg         CoordinatorConfig
	lg          loggateway.Logger
	agentKeyFn  func(agentID string) string

	mu       sync.RWMutex
	sessions map[string]*teamGraphRunSession
}

type teamGraphRunSession struct {
	teamRunID       string
	teamID          string
	sessionID       string
	spiritSessionID string
	rootTaskID      string
	execID          string
	inputPreview    string
	definitionJSON  string
	watchStop       context.CancelFunc
	// emitter is the run-scoped TraceEmitter captured at registration, used to
	// emit team.member.* flow logs on the resume path where the incoming ctx
	// (from the task-completion handler) carries no emitter.
	emitter *event.TraceEmitter

	stepDedup     *graphStepDedup
	nodeStarts    *graphNodeStartTracker
	memberByNode  map[string]MemberDef
	stepSortIndex map[string]int
	obsReg        biz.OrchestrationRegistry
	obsStore      *biz.OrchestrationStatusStore
	registeredAt  time.Time
	// status mirrors the TeamRun status (P3-3 / ADR-D): SuspendIdleWaits only
	// evicts waiting_human sessions — running sessions have inflight steps.
	status string
	// lastActivityAt is the suspend/stale aging basis (D3): registration time
	// is only the fallback for sessions with no recorded activity yet.
	lastActivityAt time.Time
}

type graphWatchMode int

const (
	graphWatchStepsOnly graphWatchMode = iota
	graphWatchStepsAndFinalize
)

func NewTeamGraphRunCoordinator(graphs TeamGraphExecutionBackend, teamRunReader biz.TeamRunReader, teamRunWriter biz.TeamRunWriter, runTransitioner biz.TeamRunStatusTransitioner, eventBus biz.EventBus, seq rt.EventPublisher, sessionRepo biz.TeamGraphSessionRepo, agentKeyFn func(agentID string) string, lg loggateway.Logger) *TeamGraphRunCoordinator {
	if agentKeyFn == nil {
		agentKeyFn = func(agentID string) string { return strings.TrimSpace(agentID) }
	}
	return &TeamGraphRunCoordinator{
		graphs:          graphs,
		teamRunReader:   teamRunReader,
		teamRunWriter:   teamRunWriter,
		runTransitioner: runTransitioner,
		eventBus:        eventBus,
		seq:             seq,
		sessionRepo:     sessionRepo,
		agentKeyFn:      agentKeyFn,
		sessions:        make(map[string]*teamGraphRunSession),
		cfg:             DefaultCoordinatorConfig(),
		lg:              lg,
	}
}

// publishEvent routes a v2 Event through the Sequencer when available;
// falls back to EventBus.Publish when Sequencer is nil.
func (c *TeamGraphRunCoordinator) publishEvent(ctx context.Context, e biz.Event) {
	if c == nil {
		return
	}
	if c.seq != nil {
		c.seq.Publish(ctx, e)
		return
	}
	if c.eventBus != nil {
		c.eventBus.Publish(ctx, e)
	}
}

// SetFinisher wires the mediator for step persistence and team run finalization.
func (c *TeamGraphRunCoordinator) SetFinisher(m *TeamRunMediator) {
	if c == nil {
		return
	}
	c.finisher = m
}

func (c *TeamGraphRunCoordinator) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID string, ct *biz.CompiledTeam) error {
	if c == nil || c.graphs == nil {
		return nil
	}
	// Create graph execution child span (P3-2): Trace propagation across Spirit→Team→Graph.
	if bridge := turntrace.FromContext(ctx); bridge != nil {
		var span trace.Span
		ctx, span = bridge.StartChild(ctx, "graph.execute.register")
		defer turntrace.EndChild(span, nil)
	}
	if err := c.graphs.RegisterTeamGraphExecution(ctx, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID, ct); err != nil {
		return err
	}
	execID = strings.TrimSpace(execID)
	now := time.Now()
	sess := &teamGraphRunSession{
		teamRunID:       strings.TrimSpace(teamRunID),
		teamID:          strings.TrimSpace(teamID),
		sessionID:       strings.TrimSpace(sessionID),
		spiritSessionID: strings.TrimSpace(spiritSessionID),
		// S-3: capture the run-dimension at registration; the finisher's ctx
		// (resume/finalize path) never carries RootTaskActivityID.
		rootTaskID:     string(agent.RootTaskActivityIDFromCtx(ctx)),
		execID:         execID,
		emitter:        event.TraceEmitterFromContext(ctx),
		stepDedup:      newGraphStepDedup(),
		nodeStarts:     newGraphNodeStartTracker(),
		registeredAt:   now,
		status:         biz.TeamRunStatusRunning,
		lastActivityAt: now,
	}
	if c.teamRunReader != nil {
		if run, err := c.teamRunReader.GetTeamRunByID(ctx, sess.teamRunID); err == nil {
			sess.inputPreview = strings.TrimSpace(run.InputPreview)
			sess.definitionJSON = strings.TrimSpace(run.DefinitionSnapshotJSON)
			sess.restoreAttribution(ct, c.agentKeyFn, c.lg)
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
	sess := c.ensureSessionResident(ctx, task.ExecutionID)
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
				if c.seq != nil || c.eventBus != nil {
					now := time.Now().UTC()
					ts := biz.TeamStage{
						// S-3: use the run-dimension captured at registration; this
						// resume-failure ctx (task-completion handler) never carries
						// RootTaskActivityID, so a ctx lookup would derive the wrong
						// stage ID and upsert a second team_stages_v2 row.
						ID:        string(agent.NewTeamStageActivityID(sess.teamID, sess.rootTaskID)),
						TeamID:    sess.teamID,
						SessionID: sess.spiritSessionID,
						Status:    biz.TeamStageStatusFailed,
						Stage:     biz.TeamStageStageFailed,
						StartedAt: now,
						Version:   1,
					}
					if ts.SessionID == "" {
						ts.SessionID = sess.sessionID
					}
					c.publishEvent(ctx, biz.NewTeamStageFailedEvent(ts))
					c.publishEvent(ctx, biz.NewSystemNoticeEvent(ts.SessionID, "team_run_failed", err.Error(), map[string]any{
						"run_id":        run.ID,
						"team_id":       sess.teamID,
						"error_message": err.Error(),
					}))
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
	if c == nil || sess == nil {
		return func() {}
	}
	c.stopWatch(sess.execID)
	watchCtx, cancel := context.WithCancel(ctx)
	// Ensure the watch goroutine carries the run's TraceEmitter so member-node
	// flow logs (team.member.*) are emitted even on the resume path, whose
	// incoming ctx originates from the task-completion handler without one.
	if event.TraceEmitterFromContext(watchCtx) == nil && sess.emitter != nil {
		watchCtx = event.WithTraceEmitter(watchCtx, sess.emitter)
	}
	// Assign watchStop and update sessions map inside the same lock scope
	// to prevent concurrent startGraphWatch/stopWatch from observing a nil watchStop.
	c.mu.Lock()
	sess.watchStop = cancel
	c.sessions[sess.execID] = sess
	c.mu.Unlock()
	// P3-3: the watch lifecycle handle (watchStop) is assigned even without an
	// EventBus so woken sessions are observably watched; only the goroutine is
	// event-driven.
	if c.eventBus == nil {
		return cancel
	}

	safego.Go(watchCtx, "team.graph.run.watch", func() {
		defer cancel()
		// Subscribe via v2 EventBus; graph node lifecycle arrives as system.notice
		// (activity_kind=graph_stage) from graph EventBridge.
		opts := biz.EventSubscribeOptions{
			SpiritSessionID: sess.spiritSessionID,
		}
		if opts.SpiritSessionID == "" {
			opts.SpiritSessionID = sess.sessionID
		}
		ch, unsub := c.eventBus.Subscribe(opts)
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
			case e, ok := <-ch:
				if !ok {
					return
				}
				notice, ok := e.(*biz.SystemNoticeEvent)
				if !ok {
					continue
				}
				if done, failed, errMsg := c.handleGraphWatchNotice(watchCtx, sess, notice, mode); done {
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
	// B9（C1 全量物化）：team 执行的 graph_id 已是真实资产 ID，不再有 team: 前缀。
	// 调用方 HandleTeamGraphTaskCompleted 已保证 exec 是本 coordinator 注册的 team
	// 执行（sess != nil），此前缀代理条件不再需要；只需排除运行中状态，避免对活跃
	// 执行重复 resume。
	return exec.Status != biz.TeamRunStatusRunning && exec.GetInterruptNode() == nodeID
}

func (c *TeamGraphRunCoordinator) handleGraphWatchNotice(ctx context.Context, sess *teamGraphRunSession, notice *biz.SystemNoticeEvent, mode graphWatchMode) (done, failed bool, errMsg string) {
	if notice == nil {
		return false, false, ""
	}
	meta := notice.Meta
	kind := biz.ActivityKind(metaString(meta, "activity_kind"))
	if kind == "" {
		kind = biz.ActivityKindNotice
	}
	eventType := biz.ActivityEventType(metaString(meta, "activity_event"))
	if kind != biz.ActivityKindGraphStage {
		return false, false, ""
	}
	execID := metaString(meta, "execution_id")
	if execID != "" && execID != sess.execID {
		return false, false, ""
	}
	// P3-3 (D3): a graph_stage notice addressed to this session is activity —
	// it must reset the suspend/stale aging clock.
	c.touchSessionActivity(sess.execID)
	// Y1/N3: a HITL interrupt arrives as a graph_stage "step" notice carrying
	// interrupt metadata (the Pregel interrupt is the only reachable carrier —
	// checkpoint interrupt events require StreamModeCheckpoints which is not
	// enabled). Mark the run waiting_human instead of silently dropping it;
	// otherwise the run stays "running" until the watch times out.
	if metaString(meta, "interrupt_key") != "" {
		nodeID := metaString(meta, "node_id")
		lineageID := metaString(meta, "lineage_id")
		if err := c.MarkTeamGraphInterrupt(ctx, sess.execID, nodeID, lineageID); err != nil {
			c.lg.Warn("handleGraphWatchNotice: MarkTeamGraphInterrupt failed",
				loggateway.StepID("team.graph.interrupt_mark_fail"),
				loggateway.Str("exec_id", sess.execID),
				loggateway.Str("node_id", nodeID),
				loggateway.Err(err))
		}
		return false, false, ""
	}
	stepCtx := sess.stepContext()
	stage := notice.NoticeType
	isNodeEnd := stage == "node_end"
	isNodeStart := stage == "node_start"
	// 2026-08-08 问题4b：记录 node_start 真实开始时刻，node_end persistStep
	// 时取出计算真实 DurationMS（否则 StartedAt≈FinishedAt≈落库时刻，耗时恒 0）。
	// tracker first-write-wins：节点重试保留最早开始，窗口覆盖全部尝试。
	if isNodeStart && stepCtx != nil {
		stepCtx.MarkNodeStarted(metaString(meta, "node_id"), notice.OccurredAt())
	}
	isStepFinished := kind == biz.ActivityKindTeamStage && eventType == biz.ActivityEventCompleted
	if sess.obsStore != nil {
		changed := sess.obsStore.ApplySystemNotice(notice, sess.obsReg)
		for _, st := range changed {
			if isNodeStart && c.finisher != nil && stepCtx != nil {
				c.finisher.PublishTeamStepStarted(ctx, stepCtx, st.NodeID)
			}
			if isNodeEnd || isStepFinished {
				if biz.IsTerminalAgentNodeStatus(st.Status) {
					skipped := st.Status == biz.AgentNodeStatusSkipped
					errText := st.ErrorMessage
					if st.Status == biz.AgentNodeStatusFailed && errText == "" {
						errText = "graph node failed"
					}
					if c.finisher != nil && stepCtx != nil {
						c.finisher.PersistGraphRunStep(ctx, stepCtx, st.NodeID, st.OutputPreview, errText, skipped, 0)
					}
					// F-B：team 路径无 consumeRuntimeEvents 消费者，steps_json
					// 由 watch 增量落库（对齐 standalone 路径 node_end 行为）。
					c.recordGraphNodeEnd(ctx, sess.execID, st, meta)
				}
			}
		}
	}
	if isStepFinished && stepCtx != nil {
		if nodeID := resumeStepNodeID(meta, sess.obsReg); nodeID != "" {
			stepCtx.MarkPersisted(nodeID)
		}
	}
	if mode == graphWatchStepsOnly {
		return false, false, ""
	}
	// N3: a graph-level fatal error (Pregel error: max steps / panic /
	// executeGraph failure) arrives as a graph_stage "step" notice carrying an
	// error key. Finalize as failed immediately instead of waiting for the
	// watch timeout.
	if stage == "step" {
		if errText := metaString(meta, "error"); errText != "" {
			return true, true, errText
		}
	}
	switch stage {
	case "node_error":
		if eventType != biz.ActivityEventFailed {
			return false, false, ""
		}
		if resumeMetaBool(meta, "retrying") {
			return false, false, ""
		}
		msg := metaString(meta, "error")
		if strings.TrimSpace(notice.Message) != "" && msg == "" {
			msg = strings.TrimSpace(notice.Message)
		}
		return true, true, msg
	case "execution_done":
		if eventType != biz.ActivityEventCompleted {
			return false, false, ""
		}
		return true, false, ""
	}
	return false, false, ""
}

func (c *TeamGraphRunCoordinator) finalizeTeamRun(ctx context.Context, sess *teamGraphRunSession, failed bool, errMsg string) {
	if c == nil || sess == nil {
		return
	}
	// F-B：graph 运行已到达终局（execution_done / node_error / 超时），先收敛
	// graph_executions 行（幂等），再做 team run 终态记账。
	_ = c.FinalizeTeamGraphExecution(ctx, sess.execID, failed, errMsg)
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
	if c.seq != nil || c.eventBus != nil {
		now := time.Now().UTC()
		ts := biz.TeamStage{
			// S-3: run-dimension captured at registration (watch/finalize ctx
			// originates from the completion watcher, not the run ctx).
			ID:        string(agent.NewTeamStageActivityID(sess.teamID, sess.rootTaskID)),
			TeamID:    sess.teamID,
			SessionID: sess.spiritSessionID,
			StartedAt: now,
			Version:   1,
		}
		if ts.SessionID == "" {
			ts.SessionID = sess.sessionID
		}
		noticeType := "team_stage_completed"
		if failed {
			ts.Status = biz.TeamStageStatusFailed
			ts.Stage = biz.TeamStageStageFailed
			c.publishEvent(ctx, biz.NewTeamStageFailedEvent(ts))
			noticeType = "team_run_failed"
		} else {
			ts.Status = biz.TeamStageStatusCompleted
			ts.Stage = biz.TeamStageStageCompleted
			c.publishEvent(ctx, biz.NewTeamStageCompletedEvent(ts))
		}
		c.publishEvent(ctx, biz.NewSystemNoticeEvent(ts.SessionID, noticeType, errMsg, map[string]any{
			"run_id":  run.ID,
			"run":     run,
			"team_id": sess.teamID,
		}))
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

// CleanupStaleSessions removes sessions whose last activity is older than
// sessionMaxAge (P3-3 / ADR-D D3: aging basis is lastActivityAt, falling back
// to registeredAt for sessions with no recorded activity — a session registered
// long ago but still receiving events is NOT stale).
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
		last := sess.lastActivityAt
		if last.IsZero() {
			last = sess.registeredAt
		}
		if last.IsZero() {
			continue
		}
		if age := now.Sub(last); age > maxAge {
			if sess.watchStop != nil {
				sess.watchStop()
				sess.watchStop = nil
			}
			delete(c.sessions, id)
			stale = append(stale, struct {
				execID string
				age    time.Duration
			}{execID: id, age: age})
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
		ExecID:          sess.execID,
		TeamRunID:       sess.teamRunID,
		TeamID:          sess.teamID,
		SessionID:       sess.sessionID,
		SpiritSessionID: sess.spiritSessionID,
		InputPreview:    sess.inputPreview,
		DefinitionJSON:  sess.definitionJSON,
		Status:          status,
		RegisteredAt:    now,
		LastActivityAt:  now,
		UpdatedAt:       now,
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
	if c == nil {
		return
	}
	// P3-3: mirror the transition into the in-memory session (suspend gate +
	// activity aging) regardless of DB availability.
	c.setSessionActivity(execID, status)
	if c.sessionRepo == nil {
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

// restoreAttribution rebuilds the observability/attribution maps from the
// persisted definition, preferring the actually-executed CompiledTeam (C1:
// attribution shares the execution graph's source; def-derived maps are only
// the fallback when ct is nil or has no agent member nodes). Shared by
// registration, RecoverSessions, and the P3-3 wake path (restoreSession).
func (s *teamGraphRunSession) restoreAttribution(ct *biz.CompiledTeam, agentKeyFn func(agentID string) string, lg loggateway.Logger) {
	reg, memberByNode, stepSortIndex := buildResumeSessionContext(s.definitionJSON, s.inputPreview, agentKeyFn, lg)
	if m, idx, r, ok := buildAttributionFromCompiledTeam(ct, s.definitionJSON, agentKeyFn); ok {
		memberByNode, stepSortIndex, reg = m, idx, r
	}
	s.obsReg = reg
	s.obsStore = biz.NewOrchestrationStatusStore(reg)
	s.memberByNode = memberByNode
	s.stepSortIndex = stepSortIndex
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
		sess := c.restoreSession(dbSess)
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

// ---------------------------------------------------------------------------
// P3-3 / ADR-D: TeamRun graph execution suspend / wake
// ---------------------------------------------------------------------------

// restoreSession rebuilds an in-memory session from its DB row. Shared by
// RecoverSessions (process restart) and ensureSessionResident (wake path).
// The rebuilt session carries no emitter/rootTaskID (absent from the DB row);
// step persistence degrades gracefully without them.
func (c *TeamGraphRunCoordinator) restoreSession(dbSess biz.TeamGraphSession) *teamGraphRunSession {
	sess := &teamGraphRunSession{
		teamRunID:      dbSess.TeamRunID,
		teamID:         dbSess.TeamID,
		sessionID:      dbSess.SessionID,
		execID:         dbSess.ExecID,
		inputPreview:   dbSess.InputPreview,
		definitionJSON: dbSess.DefinitionJSON,
		// Y5: restore the watch subscription filter key — without it a
		// recovered waiting_human session silently drops every graph_stage
		// notice (EventSubscribeOptions.SpiritSessionID mismatch).
		spiritSessionID: dbSess.SpiritSessionID,
		status:          dbSess.Status,
		stepDedup:       newGraphStepDedup(),
		nodeStarts:      newGraphNodeStartTracker(),
		registeredAt:    parseSessionActivityTime(dbSess.RegisteredAt),
		lastActivityAt:  parseSessionActivityTime(dbSess.LastActivityAt),
	}
	// ct is unavailable on this path: def-derived attribution is the
	// documented fallback (C1), identical to the pre-P3-3 RecoverSessions.
	sess.restoreAttribution(nil, c.agentKeyFn, c.lg)
	return sess
}

// parseSessionActivityTime parses an RFC3339 DB timestamp; empty/invalid
// falls back to now (activity timestamps are aging hints, not correctness).
func parseSessionActivityTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(s)); err == nil {
		return t
	}
	return time.Now()
}

// SuspendIdleWaits evicts idle waiting_human sessions from memory (P3-3 /
// ADR-D D1). The DB row is preserved with its status unchanged, so a later
// resume signal wakes the session via ensureSessionResident. Running sessions
// (inflight steps) and recently active sessions are never suspended. Driven by
// the existing cleanup ticker in provider.go — no new background task.
func (c *TeamGraphRunCoordinator) SuspendIdleWaits(now time.Time, idleThreshold time.Duration) int {
	if c == nil {
		return 0
	}
	if idleThreshold <= 0 {
		idleThreshold = c.cfg.SuspendIdleThreshold
	}
	if idleThreshold <= 0 {
		idleThreshold = defaultSuspendIdleThreshold
	}
	var suspended []struct {
		execID  string
		emitter *event.TraceEmitter
	}
	c.mu.Lock()
	for id, sess := range c.sessions {
		if sess.status != biz.TeamRunStatusWaitingHuman {
			continue
		}
		last := sess.lastActivityAt
		if last.IsZero() {
			last = sess.registeredAt
		}
		if last.IsZero() || now.Sub(last) <= idleThreshold {
			continue
		}
		if sess.watchStop != nil {
			sess.watchStop()
			sess.watchStop = nil
		}
		delete(c.sessions, id)
		suspended = append(suspended, struct {
			execID  string
			emitter *event.TraceEmitter
		}{execID: id, emitter: sess.emitter})
	}
	c.mu.Unlock()
	for _, s := range suspended {
		// D4 双轨：流程日志 best-effort（emitter 仅存于本进程注册的会话；
		// 重启恢复/唤醒重建的会话 emitter 为空，仅进程日志）。
		if s.emitter != nil {
			s.emitter.Log("team.session.suspended", event.FlowPhaseDone, "挂起空闲等待会话", event.P("exec_id", s.execID))
		}
		c.lg.Info("SuspendIdleWaits: suspended idle waiting_human session (DB row retained)",
			loggateway.StepID("team.session.suspended"),
			loggateway.Str("exec_id", s.execID))
	}
	return len(suspended)
}

// ensureSessionResident returns the in-memory session, rebuilding it from the
// DB when absent (P3-3 / ADR-D D2 wake path). Returns nil when neither memory
// nor DB holds the session (caller keeps the "not ours" contract). A memory
// hit is a zero-cost pointer return; a DB hit re-registers the session and
// restarts the completion watch for waiting_human sessions.
func (c *TeamGraphRunCoordinator) ensureSessionResident(ctx context.Context, execID string) *teamGraphRunSession {
	if c == nil {
		return nil
	}
	execID = strings.TrimSpace(execID)
	if sess := c.session(execID); sess != nil {
		return sess
	}
	if c.sessionRepo == nil {
		return nil
	}
	dbSess, err := c.sessionRepo.GetSession(ctx, execID)
	if err != nil {
		return nil
	}
	sess := c.restoreSession(dbSess)
	c.mu.Lock()
	// Double-checked insert: a concurrent wake may have restored it already.
	if existing := c.sessions[execID]; existing != nil {
		c.mu.Unlock()
		return existing
	}
	c.sessions[execID] = sess
	c.mu.Unlock()
	if dbSess.Status == biz.TeamRunStatusWaitingHuman {
		c.startCompletionWatch(ctx, sess)
	}
	// D4 双轨：流程日志依赖调用方 ctx emitter（task-completion 链路当前无
	// emitter，故唤醒通常仅进程日志——与 Batch-2 偏差同模式，待调用方补
	// emitter 后自然生效）。
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.Log("team.session.resumed", event.FlowPhaseDone, "唤醒挂起会话", event.P("exec_id", execID))
	}
	c.lg.Info("ensureSessionResident: woke session from DB",
		loggateway.StepID("team.session.resumed"),
		loggateway.Str("exec_id", execID),
		loggateway.Str("status", dbSess.Status))
	return sess
}

// touchSessionActivity records last-activity for suspend/stale aging (P3-3 D3).
func (c *TeamGraphRunCoordinator) touchSessionActivity(execID string) {
	c.mu.Lock()
	if sess := c.sessions[strings.TrimSpace(execID)]; sess != nil {
		sess.lastActivityAt = time.Now()
	}
	c.mu.Unlock()
}

// setSessionActivity mirrors a status transition into the in-memory session
// and bumps last-activity (P3-3 D1/D3).
func (c *TeamGraphRunCoordinator) setSessionActivity(execID, status string) {
	c.mu.Lock()
	if sess := c.sessions[strings.TrimSpace(execID)]; sess != nil {
		sess.status = status
		sess.lastActivityAt = time.Now()
	}
	c.mu.Unlock()
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
