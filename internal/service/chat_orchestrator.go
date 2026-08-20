package service

import (
	"context"
	"fmt"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/graph"
	"aranea-agents/internal/outbound"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/internal/runtime/turn"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/security"
	subagenttool "aranea-agents/internal/tools/subagent"
	"aranea-agents/internal/voice"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	orchMapMaxIdle     = 30 * time.Minute
	orchMapSweepPeriod = 5 * time.Minute
)

// sessionStateTransitor is the interface for session status transitions.
// *biz.TurnLifecycleUsecase satisfies this interface, allowing the service
// layer to delegate session state management to the biz layer.
type sessionStateTransitor interface {
	TransitionStatus(ctx context.Context, sessionID string, targetStatus sessstatus.SessionStatus, reason sessstatus.SessionStatusReason)
}

// Compile-time check: *biz.TurnLifecycleUsecase satisfies sessionStateTransitor.
var _ sessionStateTransitor = (*biz.TurnLifecycleUsecase)(nil)

// TeamOrchestrationDeps groups team execution and graph compilation dependencies.
// These are only used when a session is owned by a team or when graph execution
// is triggered from the chat orchestrator.
type TeamOrchestrationDeps struct {
	TeamUC           *biz.TeamUsecase
	TeamsNative      biz.TeamRunnerWirePort
	GraphFactory     biz.GraphBuilderFactory
	Graphs           *biz.GraphUsecase
	Tasks            *biz.TaskUsecase
	TeamGraphCoord   biz.TeamGraphCoordPort
	TeamMediator     biz.TeamMediatorPort
	SpiritUC         biz.SpiritTeamController
	TaskPlanner      biz.TaskPlannerPort
	AgentAllocator   biz.AgentAllocatorPort
	TaskOrchestrator biz.TaskOrchestratorPort
	// P1 fix (2026-06-18): Previously-orphan graph components now wired into
	// production via TeamOrchestrationDeps. NL2GraphConverter enables natural
	// language → graph build config conversion; RuntimeReplanner enables
	// automatic replanning on graph node failures.
	NL2GraphConverter graph.NL2GraphConverter
	RuntimeReplanner  graph.RuntimeReplanner
}

// ChannelTurnJobDeps groups channel turn job tracking and session run management.
// These are used for channel async job lifecycle.
type ChannelTurnJobDeps struct {
	TurnJobs    *biz.ChannelTurnJobUsecase
	SessionRuns *biz.SessionRunUsecase
	Channels    *biz.ChannelUsecase
}

// ChannelNotifierDeps groups notification dependencies for session run escalation.
type ChannelNotifierDeps struct {
	RunEscalation SessionRunEscalationNotifier
}

// ChatChannelDeps groups channel turn job and notification dependencies.
type ChatChannelDeps struct {
	ChJobs   ChannelTurnJobDeps
	ChNotify ChannelNotifierDeps
}

// chatTurnCoreDeps groups core turn execution dependencies: TurnDeps, RuntimeTooling,
// admission gate, admission usecase, and turn timeout. Consolidating these into a
// single struct reduces ChatOrchestrator's field count (AS-COG-01).
type chatTurnCoreDeps struct {
	TD          rt.TurnDeps
	RT          RuntimeTooling
	AdmitGate   *turn.AdmissionGate
	Admission   *biz.TurnAdmissionUsecase
	TurnTimeout time.Duration
	StepReader  biz.StepV2Reader
	StepWriter  biz.StepV2Writer
	TaskV2      biz.TaskV2Repo
}

// chatTurnLifecycle combines session state transition, turn metrics recording,
// and turn event publishing into a single composite interface.
// Merging 3 sub-manager interfaces into 1 reduces ChatOrchestrator's field count (AS-COG-01).
type chatTurnLifecycle interface {
	sessionStateTransitor
	turnRecorder
	turnEventPublisher
}

// chatRunManager combines run status tracking, pending queue management,
// await coordination, and session run lifecycle into a single composite interface.
// Merging 4 sub-manager interfaces into 1 reduces ChatOrchestrator's field count (AS-COG-01).
type chatRunManager interface {
	runStatusTracker
	pendingQueueManager
	awaitCoordinator
	sessionRunLifecycle
}

// ChatOrchestrator owns the turn lifecycle: admission, execution, status tracking,
// and post-turn side effects. ChatService delegates all orchestration work here.
//
// Field count: 13 (well under AS-COG-01 limit of 15).
type ChatOrchestrator struct {
	core         chatTurnCoreDeps
	channelDeps  ChatChannelDeps
	usageDeps    ChatUsageDeps
	teamExecDeps ChatTeamDeps
	evoDeps      ChatEvolutionDeps
	infraDeps    ChatInfraDeps

	runs       *rt.RunRegistry
	chatUC     *biz.ChatUsecase
	turnLC     chatTurnLifecycle
	runMgr     chatRunManager
	agentBuild agentBuildDirector

	// cmdSafetyChecker enforces command safety rules (protected paths like
	// .aws/.ssh/.env) on every tool call via the framework's per-run
	// permission policy. Non-protected tools pass through with zero overhead.
	cmdSafetyChecker *security.CommandSafetyPermissionChecker

	// v2Seq is the v2 Sequencer (persist + WS) extracted from V2ProjectorFactory.
	v2Seq rt.EventPublisher

	// planBoardOrch resolves check_progress/cancel by PlanBoard.ID when the
	// legacy OrchestrationRepository has no row (C-18). May be nil.
	planBoardOrch tools.PlanBoardOrchFallback

	// immediateFactWriter persists <fact> tags from agent responses to memory_fact
	// immediately after each turn, bridging the async gap to Sleep-time consolidation.
	immediateFactWriter *biz.ImmediateFactWriter

	// pendingClarifications is a process-local hot cache of in-flight
	// clarification resume state. Source of truth is the persisted clarify
	// Step envelope; cache miss (restart / other replica) rebuilds from it.
	pendingClarifications clarificationPendingCache

	// voiceDelegationGw 是 delegate_to_spirit 工具的 turn 提交网关（M74 V9）。
	// ProvideChatService 启动期回填（Wire 环：ChatService → orch → gateway →
	// ChatService；SetPlanBoardOrch 先例）。启动后只读，无需锁。
	voiceDelegationGw biz.TurnExecutorGateway

	// resumeAwaitFn is a test seam for submitAwaitReply's restart-recovery
	// branch. Nil in production → resumeAwaitAfterRestart.
	resumeAwaitFn func(ctx context.Context, sessionID, reply, runID string) error

	sweepStop chan struct{}
}

// chatTurnLifecycleImpl combines sessionStateTransitor, turnRecorder, and
// turnEventPublisher into a single struct satisfying chatTurnLifecycle.
type chatTurnLifecycleImpl struct {
	sessionStateTransitor
	turnRecorder
	turnEventPublisher
}

var _ chatTurnLifecycle = (*chatTurnLifecycleImpl)(nil)

// chatRunManagerImpl combines runStatusTracker, pendingQueueManager,
// awaitCoordinator, and sessionRunLifecycle into a single struct satisfying chatRunManager.
type chatRunManagerImpl struct {
	runStatusTracker
	pendingQueueManager
	awaitCoordinator
	sessionRunLifecycle
}

// Sweep resolves the ambiguity between runStatusTracker.Sweep and
// awaitCoordinator.Sweep by delegating to both.
func (m *chatRunManagerImpl) Sweep() {
	m.runStatusTracker.Sweep()
	m.awaitCoordinator.Sweep()
}

var _ chatRunManager = (*chatRunManagerImpl)(nil)

// Accessor methods preserve call-site compatibility after field grouping (AS-COG-01).
func (o *ChatOrchestrator) td() rt.TurnDeps                      { return o.core.TD }
func (o *ChatOrchestrator) tdPtr() *rt.TurnDeps                  { return &o.core.TD }
func (o *ChatOrchestrator) rt() RuntimeTooling                   { return o.core.RT }
func (o *ChatOrchestrator) admitGate() *turn.AdmissionGate       { return o.core.AdmitGate }
func (o *ChatOrchestrator) admission() *biz.TurnAdmissionUsecase { return o.core.Admission }
func (o *ChatOrchestrator) turnTimeout() time.Duration           { return o.core.TurnTimeout }
func (o *ChatOrchestrator) stepReader() biz.StepV2Reader         { return o.core.StepReader }
func (o *ChatOrchestrator) stepWriter() biz.StepV2Writer         { return o.core.StepWriter }
func (o *ChatOrchestrator) taskV2Writer() biz.TaskV2Writer       { return o.core.TaskV2 }

func (o *ChatOrchestrator) team() TeamOrchestrationDeps   { return o.teamExecDeps.Team }
func (o *ChatOrchestrator) chJobs() ChannelTurnJobDeps    { return o.channelDeps.ChJobs }
func (o *ChatOrchestrator) chNotify() ChannelNotifierDeps { return o.channelDeps.ChNotify }

func (o *ChatOrchestrator) artifacts() *biz.ArtifactUsecase            { return o.usageDeps.Artifacts }
func (o *ChatOrchestrator) skillStats() biz.SkillInvocationStatsReader { return o.usageDeps.SkillStats }
func (o *ChatOrchestrator) expAnalytics() *biz.ExperienceAnalyticsUsecase {
	return o.usageDeps.ExpAnalytics
}

func (o *ChatOrchestrator) spiritAssembler() *SpiritTeamAssembler {
	return o.teamExecDeps.SpiritAssembler
}
func (o *ChatOrchestrator) spiritSynthesis() *SpiritSynthesisService {
	return o.teamExecDeps.SpiritSynthesis
}
func (o *ChatOrchestrator) teamStarter() biz.TeamStarterPort { return o.teamExecDeps.TeamStarter }
func (o *ChatOrchestrator) graphExec() biz.GraphExecutor     { return o.teamExecDeps.GraphExec }

func (o *ChatOrchestrator) skillIntel() *biz.SkillIntelligenceUsecase { return o.evoDeps.SkillIntel }
func (o *ChatOrchestrator) evolution() *biz.EvolutionUsecase          { return o.evoDeps.Evolution }

func (o *ChatOrchestrator) a2aUC() *biz.A2AUsecase           { return o.infraDeps.A2AUC }
func (o *ChatOrchestrator) outboundRouter() *outbound.Router { return o.infraDeps.OutboundRouter }
func (o *ChatOrchestrator) subAgentService() *subagenttool.Service {
	return o.infraDeps.SubAgentService
}
func (o *ChatOrchestrator) lg() loggateway.Logger { return o.infraDeps.LG }

// immediateFactWriter returns the ImmediateFactWriter for persisting <fact> tags.
// Returns nil when memory consolidation writer is not wired (graceful degradation).
func (o *ChatOrchestrator) factWriter() *biz.ImmediateFactWriter { return o.immediateFactWriter }

// profileResolver returns the Wire-injected ProfileResolver, or nil when not
// configured. Callers must nil-check before use.
func (o *ChatOrchestrator) profileResolver() *chatagent.ProfileResolver {
	return o.infraDeps.ProfileResolver
}

// heartbeatEmitter returns the Wire-injected RunHeartbeatEmitter, or nil when
// not configured. Callers must nil-check before use.
func (o *ChatOrchestrator) heartbeatEmitter() *RunHeartbeatEmitter {
	return o.infraDeps.HeartbeatEmitter
}

// deadLetterQueue returns the Wire-injected DeadLetterQueue for pending-queue
// failures (A4), or nil when not configured. Callers must nil-check before use.
func (o *ChatOrchestrator) deadLetterQueue() *lifecycle.DeadLetterQueue {
	return o.infraDeps.DeadLetterQueue
}

// memberSessions returns the Wire-injected MemberSessionV2Repo, or nil when
// not configured. Used by PauseSession/ResumeSession to sync v2 status.
func (o *ChatOrchestrator) memberSessions() biz.MemberSessionV2Repo {
	return o.infraDeps.MemberSessions
}

// Sub-manager accessors delegate to the composite interfaces.
func (o *ChatOrchestrator) sessionStateMgr() sessionStateTransitor { return o.turnLC }
func (o *ChatOrchestrator) turnMetrics() turnRecorder              { return o.turnLC }
func (o *ChatOrchestrator) eventPublisher() turnEventPublisher     { return o.turnLC }
func (o *ChatOrchestrator) runStatus() runStatusTracker            { return o.runMgr }
func (o *ChatOrchestrator) pendingQ() pendingQueueManager          { return o.runMgr }
func (o *ChatOrchestrator) awaitCoord() awaitCoordinator           { return o.runMgr }
func (o *ChatOrchestrator) sessionRunLC() sessionRunLifecycle      { return o.runMgr }

// ChatTurnDeps groups turn execution lifecycle dependencies: session pipeline,
// run registry, runtime tooling, admission control, and turn timeout.
type ChatTurnDeps struct {
	rt.TurnDeps
	Runs         *rt.RunRegistry
	PendingQueue *rt.PendingMessageQueue
	RT           RuntimeTooling
	TurnTimeout  time.Duration
	Admission    *biz.TurnAdmissionUsecase
	StepReader   biz.StepV2Reader
	StepWriter   biz.StepV2Writer
	// TaskV2 backs interrupted-task resume (L3): GetTask pre-check +
	// ResumeInterruptedTask CAS. Nil = resume unavailable (Internal error).
	TaskV2 biz.TaskV2Repo
}

// ChatUsageDeps groups usage tracking, monitoring, artifact, and analytics dependencies.
type ChatUsageDeps struct {
	Usage        *biz.UsageUsecase
	Monitor      *biz.MonitorUsecase
	Artifacts    *biz.ArtifactUsecase
	SkillStats   biz.SkillInvocationStatsReader
	ExpAnalytics *biz.ExperienceAnalyticsUsecase
}

// ChatTeamDeps groups team orchestration, graph execution, and spirit assembly dependencies.
type ChatTeamDeps struct {
	Team            TeamOrchestrationDeps
	TeamStarter     biz.TeamStarterPort
	GraphExec       biz.GraphExecutor
	SpiritAssembler *SpiritTeamAssembler
	SpiritSynthesis *SpiritSynthesisService
}

// ChatEvolutionDeps groups skill evolution and agent evolution dependencies.
type ChatEvolutionDeps struct {
	// SkillIntel is the ADR-3 unified evolution-suggestion entry point
	// (skills_butler tools route here; the legacy SkillEvolutionUsecase
	// proposal view was retired).
	SkillIntel *biz.SkillIntelligenceUsecase
	Evolution  *biz.EvolutionUsecase
}

// ChatInfraDeps groups cross-cutting infrastructure dependencies: logging,
// orchestration cache, A2A, MCP, outbound routing, sub-agent service, and
// the biz-layer turn lifecycle usecase.
type ChatInfraDeps struct {
	LG              loggateway.Logger
	OrchCache       *biz.OrchestrationCache
	A2AUC           *biz.A2AUsecase
	MCPServers      *biz.MCPServerUsecase
	OutboundRouter  *outbound.Router
	SubAgentService *subagenttool.Service
	TurnLifecycle   *biz.TurnLifecycleUsecase
	// HeartbeatEmitter publishes run_heartbeat events so the frontend can
	// detect stale runs within 30s (P1-7). When nil, no heartbeats are
	// emitted and stale detection degrades gracefully.
	HeartbeatEmitter *RunHeartbeatEmitter
	// DeadLetterQueue holds pending-queue messages that failed processing
	// (A4). When nil, failed messages are only logged (legacy behavior).
	DeadLetterQueue *lifecycle.DeadLetterQueue
	// ProfileResolver resolves the active runtime profile for an agent
	// and converts it to framework RunOptions applied per turn. When nil,
	// no profile overrides are applied (graceful degradation).
	ProfileResolver *chatagent.ProfileResolver
	// V2ProjectorFactory creates per-turn v2 ActivityProjector instances.
	// Each turn (spirit + each team member) gets its own instance, isolating
	// per-turn streaming state. The factory shares the singleton Sequencer +
	// SeqAssigner so Seq allocation remains globally monotonic per spirit
	// session. Wired via Wire DI; nil = v2 disabled.
	V2ProjectorFactory *v2.ProjectorFactory
	// MemberSessions looks up/upserts v2 MemberSession rows (PauseSession /
	// ResumeSession sync). Optional: nil disables member pause projection.
	MemberSessions biz.MemberSessionV2Repo
	// MemoryConsolidationWriter persists facts to memory_fact. Used to create
	// ImmediateFactWriter for <fact> tag extraction. When nil, immediate fact
	// extraction is disabled (graceful degradation).
	MemoryConsolidationWriter biz.MemoryConsolidationWriter
	// FactIndexSync 即时事实写入后的 embedding 索引回采（P2-2，对齐 auto_memory
	// 范式）。nil 时跳过即时同步，由 reconciler cron 最终一致兜底。
	FactIndexSync biz.MemoryFactIndexSyncer
	// SkillEmbedder 供 memory_butler selective_remember 语义判重（P2-3）等
	// 场景使用（生产为 MultiProviderEmbedder）。nil 时相关工具降级字符串判重。
	SkillEmbedder biz.SkillEmbedder
	// MemoryConflictDetector arbitrates governable fact kinds for the
	// memory_remember explicit-memory tool (FR-M4). Optional: nil disables
	// conflict governance (writes still succeed).
	MemoryConflictDetector biz.MemoryConflictDetector
	// MemoryConflictStore applies supersede/mark-conflict decisions for the
	// memory_remember tool (FR-M4). Optional: nil skips governance application.
	MemoryConflictStore biz.L3ConflictStore
	// VoiceDelegation 是语音委派登记表单例（M74 V9，设计 74 §15.4-C）。
	// Wire 提供；voiceButlerTools 注入 delegate_to_spirit 工具。
	// nil = 委派工具不挂载（语音助手退化为纯快答）。
	VoiceDelegation *voice.DelegationRegistry
}

// ChatOrchestratorDeps groups all dependencies for ChatOrchestrator construction.
// Sub-aggregates (ChatTurnDeps, ChatUsageDeps, ChatTeamDeps, ChatEvolutionDeps,
// ChatInfraDeps, ChatChannelDeps) reduce the flat parameter count and make
// responsibility boundaries explicit.
type ChatOrchestratorDeps struct {
	Turn      ChatTurnDeps
	Usage     ChatUsageDeps
	Channel   ChatChannelDeps
	Team      ChatTeamDeps
	Evolution ChatEvolutionDeps
	Infra     ChatInfraDeps
}

func coalesceRunRegistry(r *rt.RunRegistry) *rt.RunRegistry {
	if r != nil {
		return r
	}
	return rt.NewRunRegistry()
}

func coalescePendingQueue(q *rt.PendingMessageQueue) *rt.PendingMessageQueue {
	if q != nil {
		return q
	}
	return rt.NewPendingMessageQueue()
}

func NewChatOrchestrator(deps ChatOrchestratorDeps) *ChatOrchestrator {
	runs := coalesceRunRegistry(deps.Turn.Runs)
	pending := coalescePendingQueue(deps.Turn.PendingQueue)
	sessionLocks := biz.NewSessionLockManager()
	var v2Seq rt.EventPublisher
	if pf := deps.Infra.V2ProjectorFactory; pf != nil {
		v2Seq = pf.Seq()
	}
	chatUC := NewChatUsecaseFromDeps(runs, pending, sessionLocks, deps.Turn.Sessions, deps.Turn.Pipeline.EventBus, v2Seq, deps.Infra.LG)
	// Wire provider/model resolution ports (BA4): biz-layer methods need
	// access to RefineLLM config, LLM catalog, and session updates.
	chatUC.SetRefineLLMLookup(deps.Turn.ReadDeps.Settings)
	chatUC.SetModelLister(deps.Turn.ReadDeps.LLM)
	chatUC.SetSessionUpdater(deps.Turn.Sessions)

	// Build individual sub-managers first.
	stateMgr := sessionStateTransitor(deps.Infra.TurnLifecycle)
	metrics := turnRecorder(newChatTurnMetrics(deps.Turn.Sessions, deps.Usage.Usage, deps.Usage.Monitor, deps.Infra.LG))
	evtPub := turnEventPublisher(newChatTurnEventPublisher(deps.Turn.Sessions, deps.Turn.Pipeline.EventBus, v2Seq, deps.Infra.LG))
	rStatus := runStatusTracker(newChatRunStatusTracker(runs, deps.Turn.Sessions, deps.Turn.Pipeline.EventBus, deps.Infra.LG))
	pendQ := pendingQueueManager(newChatPendingQueueManager(chatUC))
	awaitCoord := awaitCoordinator(newChatAwaitCoordinator(chatAwaitCoordinatorDeps{
		ChatUC:       chatUC,
		RunStatus:    rStatus,
		SessionState: stateMgr,
		SessionRT: func() *araneasession.Runtime {
			if deps.Turn.SessionRT != nil {
				return deps.Turn.SessionRT
			}
			if deps.Turn.Persist.Session == nil {
				return nil
			}
			return araneasession.NewRuntime(deps.Turn.Persist.Session, deps.Infra.LG)
		},
		EventBus: deps.Turn.Pipeline.EventBus,
		Seq:      v2Seq,
		Logger:   deps.Infra.LG,
	}))
	sessRunLC := sessionRunLifecycle(newChatSessionRunLifecycle(chatSessionRunLifecycleDeps{
		SessionRuns:  deps.Channel.ChJobs.SessionRuns,
		Channels:     deps.Channel.ChJobs.Channels,
		Sessions:     deps.Turn.Sessions,
		RunStatus:    rStatus,
		SessionState: stateMgr,
		Runs:         runs,
		Escalation:   deps.Channel.ChNotify.RunEscalation,
		Logger:       deps.Infra.LG,
	}))

	turnTimeout := deps.Turn.TurnTimeout
	if turnTimeout <= 0 {
		turnTimeout = chatagent.DefaultTurnTimeout
	}

	o := &ChatOrchestrator{
		core: chatTurnCoreDeps{
			TD:          deps.Turn.TurnDeps,
			RT:          deps.Turn.RT,
			Admission:   deps.Turn.Admission,
			TurnTimeout: turnTimeout,
			StepReader:  deps.Turn.StepReader,
			StepWriter:  deps.Turn.StepWriter,
			TaskV2:      deps.Turn.TaskV2,
		},
		channelDeps:         deps.Channel,
		usageDeps:           deps.Usage,
		teamExecDeps:        deps.Team,
		evoDeps:             deps.Evolution,
		infraDeps:           deps.Infra,
		runs:                runs,
		chatUC:              chatUC,
		v2Seq:               v2Seq,
		immediateFactWriter: biz.NewImmediateFactWriter(deps.Infra.MemoryConsolidationWriter, deps.Infra.FactIndexSync, deps.Infra.LG),
		turnLC: &chatTurnLifecycleImpl{
			sessionStateTransitor: stateMgr,
			turnRecorder:          metrics,
			turnEventPublisher:    evtPub,
		},
		runMgr: &chatRunManagerImpl{
			runStatusTracker:    rStatus,
			pendingQueueManager: pendQ,
			awaitCoordinator:    awaitCoord,
			sessionRunLifecycle: sessRunLC,
		},
		cmdSafetyChecker: security.NewCommandSafetyPermissionChecker(deps.Infra.LG),
	}
	o.agentBuild = newChatAgentBuildDirector(chatAgentBuildDirectorDeps{
		TurnDeps:       deps.Turn.TurnDeps,
		RT:             deps.Turn.RT,
		AwaitCoord:     awaitCoord,
		SubAgentSvc:    deps.Infra.SubAgentService,
		OutboundRouter: deps.Infra.OutboundRouter,
		A2AEnabled:     deps.Infra.A2AUC != nil,
		CustomToolFunc: func(ctx context.Context, ag biz.Agent) []trpctool.Tool {
			var tools []trpctool.Tool
			tools = append(tools, o.cliAdminTools(ctx, ag)...)
			tools = append(tools, o.spiritCustomTools(ag)...)
			tools = append(tools, o.skillsButlerTools(ctx, ag)...)
			tools = append(tools, o.memoryButlerTools(ctx, ag)...)
			tools = append(tools, o.voiceButlerTools(ctx, ag)...)
			tools = append(tools, o.memoryRememberTools(ag)...)
			tools = append(tools, o.deliverableReaderTools()...)
			tools = append(tools, o.memberFSDeptMailTools(ag)...)
			tools = append(tools, o.sessionAccessTools(ag)...)
			return tools
		},
		Logger: deps.Infra.LG,
	})
	o.core.AdmitGate = newTurnAdmissionGate(turn.RunRegistryAdapter{Registry: runs}, chatUC, pendQ.SessionPendingMergeFollowup)

	// Wire the threshold resolver so that TurnAdmissionUsecase.EvaluateContextPressure
	// uses the orchestrator's channel-aware threshold lookup policy.
	if o.admission() != nil {
		o.admission().SetThresholdResolver(biz.ThresholdResolverFunc(o.resolveContextAdmissionThresholdForSession))
		// Wire the channel config resolver so channel entry points use the
		// long-task config threshold directly instead of the agent L0 threshold.
		if o.sessionRunLC() != nil {
			o.admission().SetChannelConfigResolver(biz.ChannelLongTaskConfigResolverFunc(o.sessionRunLC().ResolveChannelLongTaskConfig))
		}
	}

	if deps.Team.Team.TeamsNative != nil {
		deps.Team.Team.TeamsNative.SetAwaitHookProvider(func(runCtx context.Context, sessionID, runID string) biz.AwaitReplyFunc {
			return o.awaitCoord().MakeAwaitReplyFunc(runCtx, sessionID, runID)
		})
		// 2026-07-28 修复3：装配 runner 侧真实产出闸门——DAG 团队无
		// set_deliverable 交付物时 veto run success（FSM 终态不可逆，
		// 必须在 success 转换前拦截）。与 HandleTeamTurnResult 的
		// service 闸门互为双保险。
		if deps.Team.Team.SpiritUC != nil {
			deps.Team.Team.TeamsNative.SetDeliverableGate(deps.Team.Team.SpiritUC.HasRealDeliverable)
			// 2026-08-08 问题3c：装配上游交付物种子——DAG 下游团队 turn 启动时
			// 注入 graph 初始 state，成员 get_deliverable 直接读上游 topic。
			deps.Team.Team.TeamsNative.SetUpstreamDeliverableSeed(deps.Team.Team.SpiritUC.UpstreamDeliverableSeed)
			// G3+G4（ADR-G 2026-08-14）：装配交付物质量门 + 修订 followup 通道。
			// 质量门在二元门之后评估内容质量（pass/revise，J2 充分性 / J3 占位
			// 拒答 / J4 成员异常）；打回反馈经 P2-3 followup 路基入队，当前 turn
			// 结束后作为新 turn 输入驱动团队修订。拒收（无活动 run/队列满）转为
			// error → runner 侧 fail-open 放行（不得出现「打回了却没人修」）。
			deps.Team.Team.TeamsNative.SetQualityGate(deps.Team.Team.SpiritUC.EvaluateDeliverableQuality)
			deps.Team.Team.TeamsNative.SetRevisionEnqueuer(func(_ context.Context, sessionID, content string) error {
				accepted, _, _, reason, err := chatUC.EnqueueUserMessageWithKind(sessionID, content, biz.ChatEnqueueKindFollowup, false)
				if err != nil {
					return err
				}
				if !accepted {
					return fmt.Errorf("质量门修订 followup 入队被拒: %s", reason)
				}
				return nil
			})
		}
		if deps.Team.Team.TeamMediator != nil {
			deps.Team.Team.TeamsNative.SetMediator(deps.Team.Team.TeamMediator)
			deps.Team.Team.TeamMediator.SetFinisher(deps.Team.Team.TeamsNative)
			if deps.Team.Team.TeamGraphCoord != nil {
				deps.Team.Team.TeamGraphCoord.SetFinisher(deps.Team.Team.TeamMediator)
				// Runner → Mediator → Coordinator → Mediator.SetCoordinator
				// （team/runner.go 约定顺序）：缺这一步会导致 RegisterTeamGraphExecution
				// 等协调调用全部跳过，graph_executions 不落库。
				deps.Team.Team.TeamMediator.SetCoordinator(deps.Team.Team.TeamGraphCoord)
				deps.Team.Team.TeamGraphCoord.RecoverSessions(context.Background())
			}
		}
	}

	o.sweepStop = make(chan struct{})
	safego.GoBackground("orch-map-sweep", o.sweepLoop)
	return o
}

// Compile-time interface assertions.
var (
	_ biz.TurnExecutor          = (*ChatOrchestrator)(nil)
	_ biz.ChannelTurnGateway    = (*ChatService)(nil)
	_ biz.TurnExecutorGateway   = (*ChatService)(nil)
	_ biz.TurnRunControlGateway = (*ChatService)(nil)
	_ biz.TurnGateway           = (*ChatService)(nil)
	_ biz.TurnControlGateway    = (*ChatService)(nil)
	_ biz.DurableResumeGateway  = (*ChatService)(nil)
	_ biz.PendingQueueGateway   = (*ChatService)(nil)
	_ biz.A2ARunnerFactory      = (*ChatService)(nil)
)

// Execute implements biz.TurnExecutor — the shared entry point for all turn
// execution paths (Web, WS, Channel, Cron, A2A).
//
// 澄清等待态的自由回复拦截在 runNativeAgentTurnBody（Sessions.Get 之后）：
// 内存 cache 命中走热路径；重启/其他副本 cache 缺失时用会话状态门闩 +
// 持久化 clarify Step 信封重建，避免每个 turn 额外查库。
func (o *ChatOrchestrator) Execute(ctx context.Context, input biz.TurnInput) (biz.TurnResult, error) {
	return o.RunNativeAgentTurnWithOutcome(ctx, input)
}

// SetPlanBoardOrch injects the PlanBoard-backed progress/cancel fallback (C-18).
// Called from ProvideChatService after PlanExecutor construction.
func (o *ChatOrchestrator) SetPlanBoardOrch(f tools.PlanBoardOrchFallback) {
	if o == nil {
		return
	}
	o.planBoardOrch = f
}

func (o *ChatOrchestrator) planBoardOrchFallback() tools.PlanBoardOrchFallback {
	if o == nil {
		return nil
	}
	return o.planBoardOrch
}

// SetVoiceDelegationGateway 回填 delegate_to_spirit 的 turn 提交网关
// （M74 V9）。ProvideChatService 构造后调用，打破 Wire 环（SetPlanBoardOrch 同款）。
func (o *ChatOrchestrator) SetVoiceDelegationGateway(gw biz.TurnExecutorGateway) {
	if o == nil {
		return
	}
	o.voiceDelegationGw = gw
}

func (o *ChatOrchestrator) voiceDelegationGateway() biz.TurnExecutorGateway {
	if o == nil {
		return nil
	}
	return o.voiceDelegationGw
}

// RunGateway exposes the shared session run registry.
func (o *ChatOrchestrator) RunGateway() rt.RunGateway {
	return o.runs
}

// HasActiveRun reports whether a session has an in-flight run.
func (o *ChatOrchestrator) HasActiveRun(sessionID string) bool {
	return o.runs.HasActive(sessionID)
}

// HasActiveRunner reports whether the active run has a live trpc runner (steer-ready).
func (o *ChatOrchestrator) HasActiveRunner(sessionID string) bool {
	if o == nil || o.runs == nil {
		return false
	}
	_, _, ok := o.runs.ActiveRunner(sessionID)
	return ok
}

// CancelRun stops the active run for a session.
func (o *ChatOrchestrator) CancelRun(ctx context.Context, sessionID string) bool {
	// P0-01 fix: verify session ownership before cancelling.
	authUserID := ctxuser.FromContext(ctx)
	if authUserID != ctxuser.DefaultUserID {
		if sess, err := o.td().Sessions.Get(ctx, sessionID); err == nil && sess.UserID != "" && authUserID != sess.UserID {
			o.lg().Warn("cancel denied: session ownership mismatch",
				loggateway.StepID("chat.cancel_ownership"),
				loggateway.SessionID(sessionID),
				loggateway.Str("auth_user", authUserID),
				loggateway.Str("session_user", sess.UserID))
			return false
		}
	}
	return o.cancelActiveRun(ctx, sessionID)
}

// LastPendingMessageID returns the most recently enqueued pending message id.
func (o *ChatOrchestrator) LastPendingMessageID(sessionID string) string {
	return o.pendingQ().LastPendingMessageID(sessionID)
}

// GetPendingMessages returns pending messages for a session.
func (o *ChatOrchestrator) GetPendingMessages(sessionID string) []biz.PendingQueueEntry {
	return o.pendingQ().GetPendingMessages(sessionID)
}

// CancelPendingMessage cancels a pending message.
func (o *ChatOrchestrator) CancelPendingMessage(sessionID, pendingID string) bool {
	return o.pendingQ().CancelPendingMessage(sessionID, pendingID)
}

// UpdatePendingMessage updates a pending message's content.
func (o *ChatOrchestrator) UpdatePendingMessage(sessionID, pendingID, content string) bool {
	return o.pendingQ().UpdatePendingMessage(sessionID, pendingID, content)
}

// EnqueueUserMessage enqueues a user message when a turn is active.
func (o *ChatOrchestrator) EnqueueUserMessage(sessionID, content string) (accepted, queued bool, pendingID, rejectReason string, err error) {
	return o.pendingQ().EnqueueUserMessage(sessionID, content)
}

// EnqueueUserMessageWithKind enqueues with an explicit injection level (P2-3:
// steer / followup / inject). Callers that do not care should use
// EnqueueUserMessage (defaults to steer-first auto behavior).
func (o *ChatOrchestrator) EnqueueUserMessageWithKind(sessionID, content, kind string) (accepted, queued bool, pendingID, rejectReason string, err error) {
	return o.pendingQ().EnqueueUserMessageWithKind(sessionID, content, kind)
}

// SetSessionPendingMergeFollowup toggles followup merge for pending queue enqueues (CH-BOR-01).
func (o *ChatOrchestrator) SetSessionPendingMergeFollowup(sessionID string, merge bool) {
	o.pendingQ().SetSessionPendingMergeFollowup(sessionID, merge)
}

// InterruptAndSendMessage promotes a pending message to the front, marks it high priority,
// and cancels the current turn so the pending queue processor picks it up next.
func (o *ChatOrchestrator) InterruptAndSendMessage(ctx context.Context, sessionID, pendingEntryID string) error {
	if o == nil || o.chatUC == nil {
		return nil
	}
	return o.chatUC.InterruptAndSendMessage(ctx, sessionID, pendingEntryID)
}

// DequeuePendingMessage dequeues the next pending message.
func (o *ChatOrchestrator) DequeuePendingMessage(sessionID string) (biz.PendingQueueEntry, bool) {
	return o.pendingQ().DequeuePendingMessage(sessionID)
}

// GetRunStatus returns the current run lifecycle state for a session.
func (o *ChatOrchestrator) GetRunStatus(ctx context.Context, sessionID string) (runID, status, errMsg string, updatedAt string, ok bool) {
	return o.runStatus().GetRunStatus(ctx, sessionID)
}

// ActiveRunner returns the active runner for a session, if any.
func (o *ChatOrchestrator) ActiveRunner(sessionID string) (runner trpcrunner.Runner, requestID string, active bool) {
	return o.runs.ActiveRunner(sessionID)
}

func (o *ChatOrchestrator) transitionSessionStatus(ctx context.Context, sessionID string, targetStatus sessstatus.SessionStatus, reason sessstatus.SessionStatusReason) {
	o.sessionStateMgr().TransitionStatus(ctx, sessionID, targetStatus, reason)
}

// cancelActiveRun cancels the active run for a session.
// C-10: Cancel sets in-memory cancelled status before the turn's Finish defer
// runs; we then persist+publish cancelled so a racing failed path is refused.
func (o *ChatOrchestrator) cancelActiveRun(ctx context.Context, sessionID string) bool {
	if o == nil || sessionID == "" {
		return false
	}
	stopped, runID := o.runs.Cancel(sessionID, "user_cancel")
	if !stopped {
		return false
	}
	// Persist+publish cancelled while the status entry still exists (before
	// the turn defer calls Finish). Prefer a non-cancelled ctx so terminal
	// persist is not aborted by the turn cancellation itself.
	persistCtx := ctx
	if persistCtx == nil || persistCtx.Err() != nil {
		persistCtx = context.Background()
	}
	if err := o.runStatus().SetRunStatus(persistCtx, sessionID, runID, biz.SessionRunPhaseCancelled, ""); err != nil {
		o.lg().Warn("set run status failed on cancel",
			loggateway.StepID("chat.cancel_run"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
	}
	o.transitionSessionStatus(persistCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonUserCancelled)
	if _, err := chatactivity.CancelRunningActivityMessages(persistCtx, o.stepReader(), o.stepWriter(), sessionID, o.lg()); err != nil {
		o.lg().Warn("取消执行卡片查询失败",
			loggateway.StepID("chat.activity.cancel"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
	}
	return true
}

// setRunStatus delegates to the runStatus sub-manager.
func (o *ChatOrchestrator) setRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	if err := o.runStatus().SetRunStatus(ctx, sessionID, runID, status, errMsg); err != nil {
		o.lg().Warn("set run status failed",
			loggateway.StepID("chat.set_run_status"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Str("status", status),
			loggateway.Err(err))
	}
}

// publishTurnTimeoutNotification pushes a timeout alert via WS so the user knows
// the turn is taking longer than expected, without failing the turn.
func (o *ChatOrchestrator) publishTurnTimeoutNotification(ctx context.Context, sessionID, runID string, timeout time.Duration) {
	monitorBus := o.td().Pipeline.MonitorEventBus
	if monitorBus == nil {
		return
	}
	ev := contract.NewMonitorEvent(contract.MonitorEventTypeAlertNotify, "chat-service")
	ev.SessionID = sessionID
	ev.Metadata = map[string]any{
		"alert_kind":    "turn_timeout",
		"timeout":       timeout.String(),
		"message":       "对话响应时间较长，请耐心等待或手动停止",
		"i18n_key":      "chat.turn.timeout_notify",
		"request_id":    sessionID,
		"invocation_id": runID,
	}
	monitorBus.Publish(ctx, ev)
}

func (o *ChatOrchestrator) lockSession(sessionID string) func() {
	return o.chatUC.LockSession(sessionID)
}

// AttachNativeTurnAfterHook sets the post-turn hook.
func (o *ChatOrchestrator) AttachNativeTurnAfterHook(hook biz.NativeTurnAfterHook) {
	if o == nil || hook == nil {
		return
	}
	o.core.TD.AfterTurn = hook
}

// AwaitChannel operations delegate to awaitCoord.
func (o *ChatOrchestrator) RegisterAwaitChannel(sessionID string, ch biz.AwaitChannel) {
	o.awaitCoord().RegisterAwaitChannel(sessionID, ch)
}

func (o *ChatOrchestrator) DeleteAwaitChannel(sessionID string) {
	o.awaitCoord().DeleteAwaitChannel(sessionID)
}

func (o *ChatOrchestrator) LoadAwaitChannel(sessionID string) (biz.AwaitChannel, bool) {
	return o.awaitCoord().LoadAwaitChannel(sessionID)
}

func (o *ChatOrchestrator) TrySendAwaitChannel(sessionID string, msg biz.AwaitReplyMsg) bool {
	return o.awaitCoord().TrySendAwaitChannel(sessionID, msg)
}

// hydrateRunStatusFromSession delegates to the runStatus sub-manager.
func (o *ChatOrchestrator) hydrateRunStatusFromSession(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	return o.runStatus().HydrateRunStatusFromSession(ctx, sessionID)
}

func (o *ChatOrchestrator) resolveAwaitMeta(ctx context.Context, sessionID, status string) biz.ChatAwaitMeta {
	return o.runStatus().ResolveAwaitMeta(ctx, sessionID, status)
}

func (o *ChatOrchestrator) Close() {
	if o == nil {
		return
	}
	if o.sweepStop != nil {
		select {
		case o.sweepStop <- struct{}{}:
		default:
		}
	}
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
