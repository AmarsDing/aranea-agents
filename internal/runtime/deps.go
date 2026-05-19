// Package runtime provides the consolidated dependency set for agent/team chat turns.
// It replaces the legacy runtimedeps package (now a deprecated alias layer).
package runtime

import (
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/event"
	aramemory "aranea-agents/internal/memory"
	"aranea-agents/internal/provider"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// Catalog provides read-only access to biz repositories and use-cases
// needed during each chat turn (agent lookup, tool catalog, model catalog, …).
type Catalog struct {
	Agents   biz.AgentRepository
	AgentsUC *biz.AgentUsecase
	Tools    biz.ToolRepo
	ToolUC   *biz.ToolUsecase
	LLM      *biz.LlmProviderModelUsecase
	SkillUC  *biz.SkillUsecase
	Settings biz.SystemSettingRepo
}

// PersistenceSet groups the session and memory persistence services for a turn.
type PersistenceSet struct {
	Session  trpcsession.Service    // SQLite-backed or in-memory session service
	Memory   aramemory.RuntimeSet   // TRPC memory service + L0–L4 admin port
	AgentMCP *biz.AgentMCPTooling   // per-agent MCP tool configuration
	Artifact trpcartifact.Service   // optional; wired from biz.ArtifactUsecase adapter
}

// EventPipeline wraps the event bus used for projecting runtime events
// to WebSocket subscribers and internal consumers.
type EventPipeline struct {
	Bus event.Bus
}

// TurnDeps is the consolidated dependency set threaded through each chat turn.
// Fields are grouped into four sub-aggregates (Catalog, Persist, Pipeline)
// plus three utility scalars (Sessions, LLMHTTP, Compress).
type TurnDeps struct {
	Catalog  Catalog
	Persist  PersistenceSet
	Pipeline EventPipeline

	Sessions  *biz.SessionUsecase
	LLMHTTP   *http.Client
	Compress  biz.NativeTurnCompressor
	RunnerMgr *RunnerManager
}

// RoundTrip returns a provider.RoundTrip backed by the LLMHTTP client.
func (d TurnDeps) RoundTrip() *provider.RoundTrip {
	return &provider.RoundTrip{HTTP: d.LLMHTTP}
}

// SQLiteSessionMemory reports whether the turn has an active SQLite memory store.
func (d TurnDeps) SQLiteSessionMemory() bool {
	return d.Persist.Memory.Available()
}

// NewPersistenceSet constructs a PersistenceSet from its three components.
// Used by Wire when wiring the dependency graph.
func NewPersistenceSet(store *sessionmemory.Store, mcp *biz.AgentMCPTooling, sess trpcsession.Service, artifact trpcartifact.Service) PersistenceSet {
	return PersistenceSet{Session: sess, Memory: aramemory.NewRuntimeSet(store), AgentMCP: mcp, Artifact: artifact}
}

// NewRunnerManagerFromPersist builds a RunnerManager from a wired PersistenceSet.
func NewRunnerManagerFromPersist(persist PersistenceSet) *RunnerManager {
	return NewRunnerManager(RunnerFactoryDeps{Persist: persist})
}
