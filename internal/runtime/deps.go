// Package runtime provides the consolidated dependency set for agent/team chat turns.
// It replaces the legacy runtimedeps package (now a deprecated alias layer).
package runtime

import (
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// Catalog provides read-only access to biz repositories and use-cases
// needed during each chat turn (agent lookup, tool catalog, model catalog, …).
type Catalog struct {
	Agents   biz.AgentRepository
	AgentsUC *biz.AgentUsecase
	Tools    biz.ToolCatalogReader
	ToolUC   *biz.ToolUsecase
	LLM      *biz.LlmProviderModelUsecase
	SkillUC  *biz.SkillUsecase
	Settings biz.SystemSettingRepo
}

// PersistenceSet groups the session and memory persistence services for a turn.
type PersistenceSet struct {
	Session    trpcsession.Service  // SQLite-backed or in-memory session service
	Memory     MemorySet            // TRPC memory service + L0–L4 admin port
	AgentMCP   *biz.AgentMCPTooling // per-agent MCP tool configuration
	Artifact   trpcartifact.Service // optional; wired from biz.ArtifactUsecase adapter
	ArtifactUC *biz.ArtifactUsecase // optional; attachment ref resolution for turns
	RunnerRollback RunnerSessionRollbackStore // optional framework-session rollback boundary store
}

// EventPipeline wraps the event bus used for projecting runtime events
// to WebSocket subscribers and internal consumers.
type EventPipeline struct {
	Bus    event.Bus
	Buffer *event.Buffer
}

// TurnDeps is the consolidated dependency set threaded through each chat turn.
// Fields are grouped into four sub-aggregates (Catalog, Persist, Pipeline)
// plus three utility scalars (Sessions, LLMHTTP, Compress).
type TurnDeps struct {
	Catalog  Catalog
	Persist  PersistenceSet
	Pipeline EventPipeline

	Sessions  *biz.SessionUsecase
	SessionRT *araneasession.Runtime
	LLMHTTP   *http.Client
	Compress  biz.NativeTurnCompressor
	AfterTurn biz.NativeTurnAfterHook
	RunnerMgr *RunnerManager
	lg        loggateway.Logger
}

// RoundTrip returns a provider.RoundTrip backed by the LLMHTTP client.
func (d TurnDeps) RoundTrip() *provider.RoundTrip {
	return &provider.RoundTrip{HTTP: d.LLMHTTP}
}

// SQLiteSessionMemory reports whether the turn has an active SQLite memory store.
func (d TurnDeps) SQLiteSessionMemory() bool {
	return d.Persist.Memory.Available()
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
	lg := d.lg
	if lg == nil {
		lg = loggateway.Global()
	}
	d.RunnerMgr = NewRunnerManagerFromPersist(d.Persist, lg)
	return d.RunnerMgr
}

func (d *TurnDeps) SetLogger(lg loggateway.Logger) {
	d.lg = lg
}

func (d *TurnDeps) Logger() loggateway.Logger {
	if d.lg == nil {
		return loggateway.Global()
	}
	return d.lg
}

// NewEmptyPersistenceSet creates a PersistenceSet with nil memory for tests.
func NewEmptyPersistenceSet(sess trpcsession.Service, mcp *biz.AgentMCPTooling, artifact trpcartifact.Service) PersistenceSet {
	return PersistenceSet{Session: sess, Memory: MemorySet{}, AgentMCP: mcp, Artifact: artifact}
}

// NewEmptyPersistenceSetWithUC is like NewEmptyPersistenceSet but includes ArtifactUsecase for tests.
func NewEmptyPersistenceSetWithUC(sess trpcsession.Service, mcp *biz.AgentMCPTooling, artifact trpcartifact.Service, uc *biz.ArtifactUsecase) PersistenceSet {
	return PersistenceSet{Session: sess, Memory: MemorySet{}, AgentMCP: mcp, Artifact: artifact, ArtifactUC: uc}
}
