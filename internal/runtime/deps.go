// Package runtime provides the consolidated dependency set for agent/team chat turns.
// It replaces the legacy runtimedeps package (now a deprecated alias layer).
package runtime

import (
	"context"
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/event"
	memtrpc "aranea-agents/internal/memory/trpc"
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
	Session  trpcsession.Service  // SQLite-backed or in-memory session service
	Memory   MemorySet            // TRPC memory service + L0–L4 admin port
	AgentMCP *biz.AgentMCPTooling // per-agent MCP tool configuration
	Artifact trpcartifact.Service // optional; wired from biz.ArtifactUsecase adapter
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
	LLMHTTP   *http.Client
	Compress  biz.NativeTurnCompressor
	AfterTurn biz.NativeTurnAfterHook
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
	var mem MemorySet
	if store != nil {
		mem = MemorySet{
			TRPC:  memtrpc.NewSQLiteMemoryService(store),
			Admin: newSessionAdminStoreAdapter(store),
		}
	}
	return PersistenceSet{Session: sess, Memory: mem, AgentMCP: mcp, Artifact: artifact}
}

// sessionAdminStoreAdapter wraps *sessionmemory.Store to implement biz.SessionAdminStore
// without requiring biz to import internal/data/sessionmemory.
type sessionAdminStoreAdapter struct {
	inner *sessionmemory.Store
}

func newSessionAdminStoreAdapter(store *sessionmemory.Store) biz.SessionAdminStore {
	if store == nil {
		return nil
	}
	return &sessionAdminStoreAdapter{inner: store}
}

func (a *sessionAdminStoreAdapter) ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error) {
	return a.inner.ListL0SnapshotRows(ctx, sessionID, limit)
}

func (a *sessionAdminStoreAdapter) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	return a.inner.ListL1TaskRows(ctx, sessionID, agentID, status, includeEnded)
}

func (a *sessionAdminStoreAdapter) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error) {
	return a.inner.ListL1FieldRows(ctx, taskID, includeInternal)
}

func (a *sessionAdminStoreAdapter) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	return a.inner.ListFactRows(ctx, scopeType, scopeID, kind, status, keyword, limit, offset)
}

func (a *sessionAdminStoreAdapter) ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error) {
	return a.inner.ListEntityRows(ctx, scopeType, scopeID, workspaceID, userID, entityType, status, keyword, limit, offset)
}

func (a *sessionAdminStoreAdapter) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32) ([]byte, error) {
	return a.inner.NeighborhoodJSON(ctx, centerID, hops, maxNodes)
}

func (a *sessionAdminStoreAdapter) AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error) {
	return a.inner.AgentIdentityJSON(ctx, agentID)
}

func (a *sessionAdminStoreAdapter) AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error) {
	return a.inner.AgentStrategyJSON(ctx, agentID)
}

func (a *sessionAdminStoreAdapter) EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	return a.inner.EvolutionProposalRows(ctx, agentID, status, limit)
}

func (a *sessionAdminStoreAdapter) EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error) {
	return a.inner.EvolutionEventRows(ctx, agentID, limit)
}

func (a *sessionAdminStoreAdapter) EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error) {
	return a.inner.EvolutionMetricsJSON(ctx, agentID)
}

func (a *sessionAdminStoreAdapter) UpsertFactRow(ctx context.Context, in biz.FactUpsert) ([]byte, error) {
	return a.inner.UpsertFactRow(ctx, factUpsertToStore(in))
}

func (a *sessionAdminStoreAdapter) InsertEvolutionEventRow(ctx context.Context, in biz.EvolutionEventInsert) ([]byte, error) {
	return a.inner.InsertEvolutionEventRow(ctx, evolutionEventInsertToStore(in))
}

func (a *sessionAdminStoreAdapter) DeleteSessionEventEntities(ctx context.Context, sessionID string) error {
	return a.inner.DeleteSessionEventEntities(ctx, sessionID)
}

func factUpsertToStore(in biz.FactUpsert) sessionmemory.MemoryFactUpsert {
	return sessionmemory.MemoryFactUpsert{
		ID:                    in.ID,
		ScopeType:             in.ScopeType,
		ScopeID:               in.ScopeID,
		WorkspaceID:           in.WorkspaceID,
		UserID:                in.UserID,
		TeamID:                in.TeamID,
		AgentID:               in.AgentID,
		Statement:             in.Statement,
		Fingerprint:           in.Fingerprint,
		DetailsMarkdown:       in.DetailsMarkdown,
		FactKind:              in.FactKind,
		TagsJSON:              in.TagsJSON,
		Confidence:            in.Confidence,
		Importance:            in.Importance,
		UseCount:              in.UseCount,
		HitCount:              in.HitCount,
		PositiveFeedbackCount: in.PositiveFeedbackCount,
		NegativeFeedbackCount: in.NegativeFeedbackCount,
		ConflictCount:         in.ConflictCount,
		SourceKind:            in.SourceKind,
		SourceEpisodeID:       in.SourceEpisodeID,
		SourceSessionID:       in.SourceSessionID,
		SourceMessageID:       in.SourceMessageID,
		SourceExternal:        in.SourceExternal,
		Version:               in.Version,
		Status:                in.Status,
		PIIFlag:               in.PIIFlag,
		MetadataJSON:          in.MetadataJSON,
		CreatedAt:             in.CreatedAt,
		UpdatedAt:             in.UpdatedAt,
	}
}

func evolutionEventInsertToStore(in biz.EvolutionEventInsert) sessionmemory.EvolutionEventInsert {
	return sessionmemory.EvolutionEventInsert{
		AgentID:       in.AgentID,
		WorkspaceID:   in.WorkspaceID,
		EventKind:     in.EventKind,
		Kind:          in.Kind,
		TargetField:   in.TargetField,
		Reason:        in.Reason,
		TriggerKind:   in.TriggerKind,
		TriggerSource: in.TriggerSource,
		MetadataJSON:  in.MetadataJSON,
	}
}

// NewRunnerManagerFromPersist builds a RunnerManager from a wired PersistenceSet.
func NewRunnerManagerFromPersist(persist PersistenceSet) *RunnerManager {
	return NewRunnerManager(RunnerFactoryDeps{Persist: persist})
}

// CoalesceRunnerManager returns the wired RunnerManager or builds one from Persist.
// Mutates TurnDeps when RunnerMgr was nil (tests and legacy constructors).
func (d *TurnDeps) CoalesceRunnerManager() *RunnerManager {
	if d.RunnerMgr != nil {
		return d.RunnerMgr
	}
	d.RunnerMgr = NewRunnerManagerFromPersist(d.Persist)
	return d.RunnerMgr
}
