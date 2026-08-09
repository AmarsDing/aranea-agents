// Package runtime provides the consolidated dependency set for agent/team chat turns.
// It replaces the legacy runtimedeps package (now a deprecated alias layer).
package runtime

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"aranea-agents/internal/biz"
	bizmedia "aranea-agents/internal/biz/media"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/provider"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/pkg/loggateway"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// TurnReadDeps provides read-only access to biz repositories and use-cases
// needed during each chat turn (agent lookup, tool registry, model catalog, …).
// Usecase fields use narrow interfaces so consumers (team, agent, service)
// depend only on the methods they actually call.
type TurnReadDeps struct {
	Agents          biz.AgentRepository
	AgentsUC        biz.TeamAgentLookup
	CLIAdminAgentUC biz.CLIAdminAgentLister
	Tools           biz.ToolRegistryReader
	ToolUC          biz.TeamToolLookup
	LLM             biz.TeamModelCatalog
	SkillUC         biz.TeamSkillLookup
	CLIAdminSkillUC biz.CLIAdminSkillLister
	Settings        biz.SystemSettingRepo
	// MediaProviders resolves media generation provider configs
	// (media_providers catalog) for media tool assembly. Optional: when nil,
	// media generation tools are unavailable.
	MediaProviders bizmedia.ProviderReader
}

// PersistenceSet groups the session and memory persistence services for a turn.
type PersistenceSet struct {
	Session        trpcsession.Service        // primary-DB-backed or in-memory session service
	Memory         MemorySet                  // TRPC memory service + L0–L4 admin port
	AgentMCP       *biz.AgentMCPTooling       // per-agent MCP tool configuration
	Artifact       trpcartifact.Service       // optional; wired from biz.ArtifactUsecase adapter
	ArtifactUC     *biz.ArtifactUsecase       // optional; attachment ref resolution for turns
	RunnerRollback RunnerSessionRollbackStore // optional framework-session rollback boundary store
}

// EventPipeline wraps the event buses used for projecting runtime events
// to WebSocket subscribers and internal consumers.
//
// Phase 5 Blocker F Stage 1 removed the legacy Envelope Bus field.
// Blocker D removed the session-revision envelope publishers
// (event.BumpAndPublishSessionRevision* / PublishSessionRevisionEnvelope /
// NotifySessionRevisionSync). Blocker F Stage 1 then removed the dead
// parameter chain: NewTraceEmitter.bus, ConsumeWithFirstByteGuard.eventBus,
// configureMCPObserve, and EventPipeline.Bus itself.
//
// Buffer (the legacy replay buffer) was removed in Phase 5 Blocker E:
// Blocker A deleted the WS replay path (event.Buffer.Replay is no longer
// called); the buffer was write-only with no reader. All Append callsites
// (FlowTracker.emit / EventBusConsumer.handleEnvelope) were dead writes.
//
// MonitorEventBus is the typed contract.MonitorBus carrying
// contract.MonitorEvent for monitor-channel events (alerts, logs, etc.).
// Chat/team/graph realtime uses EventBus / Sequencer only.
type EventPipeline struct {
	EventBus        biz.EventBus
	MonitorEventBus contract.MonitorBus
	// Sequencer is the v2 publish-only entry point for typed events + notices.
	Sequencer EventPublisher
}

// EventPublisher is the publish-only subset of biz.EventBus.
// *v2.Sequencer implements this interface (Publish without Subscribe).
// Defined here (runtime pkg) so team/service packages can depend on it
// without importing internal/agent/v2 (which would reverse the dependency).
type EventPublisher interface {
	Publish(ctx context.Context, e biz.Event)
}

// TurnDeps is the consolidated dependency set threaded through each chat turn.
// Fields are grouped into four sub-aggregates (ReadDeps, Persist, Pipeline)
// plus three utility scalars (Sessions, LLMHTTP, Compress).
type TurnDeps struct {
	ReadDeps TurnReadDeps
	Persist  PersistenceSet
	Pipeline EventPipeline

	Sessions  biz.SessionTurnManager
	SessionRT *araneasession.Runtime
	LLMHTTP   *http.Client
	Compress  biz.NativeTurnCompressor
	AfterTurn biz.NativeTurnAfterHook
	RunnerMgr *RunnerManager
	// LearningLoop records tool_call observations into the learning loop
	// (Observation → Pattern → Proposal pipeline). Optional: when nil,
	// observation recording is skipped.
	LearningLoop biz.ObservationRecorder
	// MsgHistory loads recent chat messages (chronological order) for
	// intent-pass history injection. Optional: nil disables injection.
	MsgHistory biz.SessionRecentMessageLister
	Lg         loggateway.Logger
}

// RoundTrip returns a provider.RoundTrip backed by the LLMHTTP client.
func (d TurnDeps) RoundTrip() *provider.RoundTrip {
	return &provider.RoundTrip{HTTP: d.LLMHTTP}
}

// RoundTripForSession returns a provider.RoundTrip with an OnRetry callback
// that publishes llm_retry system.notice events. sessionID is captured in the
// callback so events carry the correct session for frontend routing. If the
// EventBus is nil, the callback is not set (retry still works, just no event
// is published).
func (d TurnDeps) RoundTripForSession(sessionID string) *provider.RoundTrip {
	rt := d.RoundTrip()
	if d.Pipeline.EventBus == nil {
		return rt
	}
	bus := d.Pipeline.EventBus
	lg := d.Logger()
	rt.OnRetry = func(req *http.Request, attempt, maxRetries int, err error, delay time.Duration) {
		maxLabel := "∞"
		if maxRetries > 0 {
			maxLabel = fmt.Sprintf("%d", maxRetries)
		}
		msg := fmt.Sprintf("LLM 调用失败，正在重试（第 %d 次，上限 %s），%v 后重试", attempt, maxLabel, delay)
		meta := map[string]any{
			"attempt":     attempt,
			"max_retries": maxRetries,
			"delay_ms":    delay.Milliseconds(),
			"error":       err.Error(),
			"message":     msg,
			"agent_key":   "provider",
		}
		ctx := req.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		bus.Publish(ctx, biz.NewSystemNoticeEvent(sessionID, "llm_retry", msg, meta))
		if lg != nil {
			lg.Warn("LLM 重试事件已发布",
				loggateway.StepID("provider.llm_retry"),
				loggateway.SessionID(sessionID),
				loggateway.Int("attempt", attempt),
				loggateway.Int("max_retries", maxRetries),
				loggateway.Int64("delay_ms", delay.Milliseconds()))
		}
	}
	return rt
}

// NewRunnerManagerFromPersist builds a RunnerManager from a wired PersistenceSet.
func NewRunnerManagerFromPersist(persist PersistenceSet, lg loggateway.Logger) *RunnerManager {
	return NewRunnerManager(RunnerFactoryDeps{Persist: persist}, lg)
}

// CoalesceRunnerManager returns the wired RunnerManager or builds one from Persist.
// Mutates TurnDeps when RunnerMgr was nil (tests and legacy constructors).
func (d *TurnDeps) CoalesceRunnerManager() *RunnerManager {
	if d.RunnerMgr != nil {
		return d.RunnerMgr
	}
	lg := d.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	d.RunnerMgr = NewRunnerManagerFromPersist(d.Persist, lg)
	return d.RunnerMgr
}

func (d *TurnDeps) Logger() loggateway.Logger {
	if d.Lg == nil {
		return loggateway.NewNoop()
	}
	return d.Lg
}

// NewEmptyPersistenceSet creates a PersistenceSet with nil memory for tests.
func NewEmptyPersistenceSet(sess trpcsession.Service, mcp *biz.AgentMCPTooling, artifact trpcartifact.Service) PersistenceSet {
	return PersistenceSet{Session: sess, Memory: MemorySet{}, AgentMCP: mcp, Artifact: artifact}
}

// NewEmptyPersistenceSetWithUC is like NewEmptyPersistenceSet but includes ArtifactUsecase for tests.
func NewEmptyPersistenceSetWithUC(sess trpcsession.Service, mcp *biz.AgentMCPTooling, artifact trpcartifact.Service, uc *biz.ArtifactUsecase) PersistenceSet {
	return PersistenceSet{Session: sess, Memory: MemorySet{}, AgentMCP: mcp, Artifact: artifact, ArtifactUC: uc}
}
