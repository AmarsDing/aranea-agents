package contracts

import (
	"arenea/backend/internal/domain"
	mem "arenea/backend/internal/memory/domain"
)

// Store 是遗留单体应用使用的统一持久化端口，由 SQLiteRepository 实现。
// 核心聚合类型在 [domain]；L0–L4 等记忆 DTO 在 [mem]。随各 Context 落地，方法会拆到
// <context>/ports 与细粒度 contracts 接口（见 0 main design.md §5）。
type Store interface {
	Migrate() error
	Close() error
	ListAgents() ([]domain.Agent, error)
	SearchAgents(query domain.AgentListQuery) (domain.AgentListResult, error)
	GetAgentByID(id string) (domain.Agent, error)
	GetAgentByKey(key string) (domain.Agent, error)
	CreateAgent(a domain.Agent) (domain.Agent, error)
	UpdateAgent(a domain.Agent) (domain.Agent, error)
	GetAgentRuntimeSettings(agentID string) (domain.AgentRuntimeSettings, error)
	UpsertAgentRuntimeSettings(settings domain.AgentRuntimeSettings) (domain.AgentRuntimeSettings, error)
	ListAgentPromptFiles(agentID string) ([]domain.AgentPromptFile, error)
	ReplaceAgentPromptFiles(agentID string, files []domain.AgentPromptFile) ([]domain.AgentPromptFile, error)
	DeleteAgent(id string) error
	ListTeams() ([]domain.Team, error)
	GetTeamByID(id string) (domain.Team, error)
	CreateTeam(t domain.Team) (domain.Team, error)
	UpdateTeam(t domain.Team) (domain.Team, error)
	DeleteTeam(id string) error
	AddTeamRun(run domain.TeamRun) (domain.TeamRun, error)
	UpdateTeamRun(run domain.TeamRun) (domain.TeamRun, error)
	AddTeamRunStep(step domain.TeamRunStep) (domain.TeamRunStep, error)
	ListTeamRuns(teamID string, limit int) ([]domain.TeamRun, error)
	ListTeamRunSteps(runID string) ([]domain.TeamRunStep, error)
	CreateSession(s domain.Session) (domain.Session, error)
	GetSessionByID(id string) (domain.Session, error)
	SearchSessions(query domain.SessionSearchQuery) (domain.SessionListResult, error)
	ListSessions(agentID string) ([]domain.Session, error)
	ListTeamSessions(teamID string) ([]domain.Session, error)
	UpdateSessionTitle(id string, title string) (domain.Session, error)
	UpdateSessionContextUsedRatio(sessionID string, ratio float64) error
	UpdateSessionL0Context(sessionID string, promptTokens int, contextWindow int, ratio float64) error
	ArchiveSession(id string) error
	DeleteSession(id string) error
	DeleteSessionsByAgentID(agentID string) error
	AddMessage(m domain.Message) (domain.Message, error)
	ListMessages(sessionID string) ([]domain.Message, error)
	ListLatestMessagesByTokens(sessionID string, maxTokens int, hardCap int) ([]domain.Message, error)
	ListSessionSummaries(sessionID string, limit int) ([]mem.SessionSummary, error)
	AddSessionSummary(summary mem.SessionSummary) (mem.SessionSummary, error)
	InsertL0AssemblySnapshot(snap mem.L0AssemblySnapshot) error
	UpdateL0AssemblySnapshotActualTokens(snapshotID string, actualPromptTokens int, usedRatio float64) error
	GetL0AssemblySnapshotByID(id string) (mem.L0AssemblySnapshot, error)
	ListL0AssemblySnapshotsBySession(sessionID string, limit int) ([]mem.L0AssemblySnapshot, error)
	ListL0AssemblySnapshotsBySpan(spanID string) ([]mem.L0AssemblySnapshot, error)
	GetActiveModelPricingRule(provider string, model string, at string) (domain.ModelPricingRule, error)
	UpsertModelPricingRule(rule domain.ModelPricingRule) (domain.ModelPricingRule, error)
	AddModelTokenUsageEvent(event domain.ModelTokenUsageEvent) (domain.ModelTokenUsageEvent, error)
	UpsertModelTokenUsageDaily(event domain.ModelTokenUsageEvent) error
	GetModelUsageSummary(query domain.ModelUsageQuery) (domain.ModelUsageSummary, error)
	ListModelUsageTrends(query domain.ModelUsageQuery) ([]domain.ModelUsageTrendPoint, error)
	ListTopModelUsage(query domain.ModelUsageQuery) ([]domain.ModelUsageBreakdownRow, error)
	ListTopAgentUsage(query domain.ModelUsageQuery) ([]domain.ModelUsageBreakdownRow, error)
	ListModelUsageEvents(query domain.ModelUsageQuery) ([]domain.ModelTokenUsageEvent, error)
	ListChatOptions(optionType string) ([]domain.ChatOption, error)
	SearchTools(query domain.ToolListQuery) (domain.ToolListResult, error)
	GetToolByID(id string) (domain.Tool, error)
	CreateTool(input domain.ToolUpsertInput) (domain.Tool, error)
	UpdateTool(id string, input domain.ToolUpsertInput) (domain.Tool, error)
	DeleteTool(id string) error
	UpdateToolEnabled(id string, enabled bool) (domain.Tool, error)
	SearchToolInvocations(query domain.ToolRunQuery) (domain.ToolRunResult, error)
	SearchSkills(query domain.SkillListQuery) (domain.SkillListResult, error)
	GetSkillByID(id string) (domain.Skill, error)
	UpdateSkillEnabled(id string, enabled bool) (domain.Skill, error)
	DuplicateSkill(id string) (domain.Skill, error)
	DeleteSkill(id string) error
	SearchSkillInvocations(query domain.SkillRunQuery) (domain.SkillRunResult, error)
	ListSkillSimilaritySources() ([]domain.SkillSimilaritySource, error)
	CreateSkillWithVersion(input domain.SkillCreateInput) (domain.Skill, error)
	GetSkillStorageDir(id string) (string, error)
	ListPlatformResources(resource string) ([]domain.PlatformResource, error)
	GetProviderModel(provider string, model string) (domain.PlatformResource, error)
	GetPlatformResource(resource string, id string) (domain.PlatformResource, error)
	CreatePlatformResource(v domain.PlatformResource) (domain.PlatformResource, error)
	UpdatePlatformResource(v domain.PlatformResource) (domain.PlatformResource, error)
	DeletePlatformResource(resource string, id string) error
	ListCronTaskRuns(query domain.CronTaskRunQuery) ([]domain.CronTaskRun, error)
	AddCronTaskRun(run domain.CronTaskRun) (domain.CronTaskRun, error)
	UpdateCronTaskRun(run domain.CronTaskRun) (domain.CronTaskRun, error)
	ListChannelCredentials(channelID string) ([]domain.ChannelCredential, error)
	UpsertChannelCredential(credential domain.ChannelCredential) (domain.ChannelCredential, error)
	DeleteChannelCredential(channelID string, credentialKey string) error
	AddChannelDelivery(delivery domain.ChannelDelivery) (domain.ChannelDelivery, error)
	ListChannelDeliveries(channelID string, limit int) ([]domain.ChannelDelivery, error)
	ListEnabledChannelRuntimeConfigs() ([]domain.ChannelRuntimeConfig, error)
	SearchPlugins(query domain.PluginListQuery) (domain.PluginListResult, error)
	UpsertPlugin(plugin domain.Plugin) (domain.Plugin, error)
	UpdatePluginEnabled(id string, enabled bool) (domain.Plugin, error)
	UpdatePluginConfig(id string, configJSON string) (domain.Plugin, error)
	ListEnabledPluginKeys() ([]string, error)
	ListAvatarAssets(scope string, workspaceID string, ownerUserID string) ([]domain.AvatarAsset, error)
	GetAvatarImage(id string, thumbnail bool) (domain.AvatarImage, error)
	CreateAvatarAsset(asset domain.AvatarAsset, image []byte, thumbnail []byte) (domain.AvatarAsset, error)
	ValidateProviderModel(provider string, model string) (bool, error)
	AddAuditLog(l domain.AuditLog) error
	ListAuditLogs(limit int) ([]domain.AuditLog, error)

	// L1 工作记忆（aranea/docs/13 memory-L1-working.md §4.2）。方法均为同步；
	// 服务层将其包装为 ChatService 与 HTTP 层使用的高层生命周期钩子。
	CreateL1Task(t mem.MemoryL1Task) (mem.MemoryL1Task, error)
	UpdateL1TaskStatus(taskID string, status mem.L1TaskStatus, endedAt string, archivedAt string) error
	UpdateL1TaskUsedTokens(taskID string, usedTokens int) error
	UpdateL1TaskShared(taskID string, shared []mem.L1FieldShare) error
	UpdateL1TaskBudget(taskID string, budgetTokens int) error
	GetL1TaskByID(taskID string) (mem.MemoryL1Task, error)
	GetL1TaskByKey(sessionID, taskKey, agentID string) (mem.MemoryL1Task, error)
	ListL1TasksBySession(query mem.L1TaskListQuery) ([]mem.MemoryL1Task, error)
	ArchiveIdleL1Tasks(before string) (int, error)

	UpsertL1Field(f mem.MemoryL1Field, history mem.MemoryL1FieldHistory, keepRevisions int) (mem.MemoryL1Field, error)
	GetL1Field(taskID, fieldPath string) (mem.MemoryL1Field, error)
	GetL1FieldByID(fieldID string) (mem.MemoryL1Field, error)
	ListL1FieldsByTask(taskID string, includeInternal bool) ([]mem.MemoryL1Field, error)
	DeleteL1Field(fieldID string) error
	BumpL1FieldRead(fieldID string, atISO string) error

	ListL1FieldHistory(fieldID string, limit int) ([]mem.MemoryL1FieldHistory, error)
	GetL1FieldHistory(fieldID string, revision int) (mem.MemoryL1FieldHistory, error)

	UpsertL1Schema(s mem.MemoryL1Schema) (mem.MemoryL1Schema, error)
	ListL1Schemas(scopeType, scopeID string) ([]mem.MemoryL1Schema, error)
	GetL1SchemaByID(id string) (mem.MemoryL1Schema, error)
	DeleteL1Schema(id string) error

	// L2 情景记忆（aranea/docs/14 memory-L2-episodic.md §4.2）。
	CreateEpisode(e mem.MemoryEpisode) (mem.MemoryEpisode, error)
	UpdateEpisode(e mem.MemoryEpisode) error
	GetEpisode(id string) (mem.MemoryEpisode, error)
	ListEpisodes(sessionID, kind string, limit, offset int) ([]mem.MemoryEpisode, int, error)
	ListPendingConsolidation(minImportance float64, limit int) ([]mem.MemoryEpisode, error)
	UpdateEpisodeConsolidationStatus(id, status string, l3Count, l4Count int) error
	UpdateEpisodeEmbedding(id, status, model string, dim int, norm float64) error
	SoftDeleteEpisode(id string) error
	UpsertL2Index(entry mem.MemoryL2IndexEntry, text string) error
	DeleteL2Index(episodeID string) error
	SearchL2BM25(sessionID, query string, minImportance float64, limit int) ([]mem.MemoryL2RecallResult, error)
	UpsertEventMark(m mem.MemoryEventMark) (mem.MemoryEventMark, error)
	SoftDeleteEventMark(id string) error
	ListEventMarks(sessionID, markType string, limit int) ([]mem.MemoryEventMark, error)
	ListMarksForEpisode(episodeID string) ([]mem.MemoryEventMark, error)
	ListL2Events(q mem.MemoryL2EventQuery) ([]mem.MemoryL2Event, int, error)
	ArchiveEpisodesBeforeDate(sessionID, before string) (int, error)
	DeleteArchivedEpisodesBefore(before string) (int, error)
	CountAgentEpisodesSince(agentID, since string) (int, error)
	InsertToolInvocation(t domain.ToolInvocation) (domain.ToolInvocation, error)

	// L3 语义记忆（aranea/docs/15 memory-L3-semantic.md §4.2）。
	CreateFact(f mem.MemoryFact) (mem.MemoryFact, error)
	UpdateFact(f mem.MemoryFact) error
	GetFact(id string) (mem.MemoryFact, error)
	GetFactByFingerprint(scopeType mem.ScopeType, scopeID, fp string) (mem.MemoryFact, error)
	ListFacts(q FactListQuery) ([]mem.MemoryFact, int, error)
	UpdateFactConfidence(id string, newConfidence float64, hitInc, posInc, negInc int) error
	UpdateFactStatus(id, status, supersededBy, archivedAt string) error
	BumpFactUseStat(id string, hit bool, atISO string) error

	InsertFactVersion(fv mem.FactVersion) error
	ListFactVersions(factID string, limit int) ([]mem.FactVersion, error)
	GetFactVersion(factID string, version int) (mem.FactVersion, error)

	InsertFactFeedback(fb mem.FactFeedback) (mem.FactFeedback, error)
	ListFactFeedback(factID string, limit int) ([]mem.FactFeedback, error)
	CountRecentFactFeedback(factID, feedbackType string, limit int) (int, error)
	CountAgentFactFeedbackSince(agentID string, feedbackTypes []string, since string) (int, error)

	UpsertFactConflict(c mem.FactConflict) (mem.FactConflict, error)
	GetFactConflict(id string) (mem.FactConflict, error)
	ListOpenFactConflicts(scope mem.ScopeType, scopeID string, limit int) ([]mem.FactConflict, error)
	UpdateFactConflictResolution(id, status, resolution, by, resolvedAt string) error

	UpsertFactEmbedding(id, model string, dim int, blob []byte, norm float64) error
	UpsertFactsFTS(factID string, scopeType mem.ScopeType, scopeID, kind, text string) error
	DeleteFactIndex(factID string) error
	SearchFactsBM25(scopes []mem.ScopeType, scopeIDs []string, query string, limit int) ([]mem.FactRecallHit, error)
	SearchFactsVector(scopes []mem.ScopeType, scopeIDs []string, q []float32, limit int) ([]mem.FactRecallHit, error)

	ListFactsDueForDecay(before string, limit int) ([]mem.MemoryFact, error)
	ApplyFactDecay(factID string, factor float64, nextAt string) error
	ArchiveFactsBelowConfidence(threshold float64, limit int) (int, error)
	CountFactsByStatus(scope mem.ScopeType, scopeID string) (map[string]int, error)

	// L4 持久 / 演化记忆
	//（aranea/docs/16 memory-L4-persistent.md §5.1）。
	UpsertEntity(e mem.MemoryEntity) (mem.MemoryEntity, error)
	GetEntity(id string) (mem.MemoryEntity, error)
	GetEntityByName(scope mem.ScopeType, scopeID string, t mem.EntityType, normalized string) (mem.MemoryEntity, error)
	ListEntities(q EntityListQuery) ([]mem.MemoryEntity, int, error)
	UpdateEntityStatus(id, status, mergedInto, archivedAt, deletedAt string) error
	UpdateEntityName(id, name, normalized string) error
	UpsertEntityFact(entityID, factID string, weight float64) error
	ListFactsForEntity(entityID string, limit int) ([]mem.MemoryEntityFactLink, error)
	InsertEntityVersion(v mem.MemoryEntityVersion) error
	ListEntityVersions(entityID string, limit int) ([]mem.MemoryEntityVersion, error)
	BumpEntityUseCount(id string, atISO string) error

	UpsertRelation(r mem.MemoryRelation) (mem.MemoryRelation, error)
	GetRelation(id string) (mem.MemoryRelation, error)
	ListRelationsForNode(nodeID string, limit int) ([]mem.MemoryRelation, error)
	UpdateRelationStatus(id, status, archivedAt, deletedAt string) error
	BumpRelationUseCount(id string, atISO string) error

	GetNeighborhood(centerID string, hops, maxNodes int) (mem.GraphNeighborhood, error)

	// Agent 演化（§5.3）。
	GetAgentIdentity(agentID string) (domain.AgentIdentity, error)
	UpsertAgentIdentity(id domain.AgentIdentity) (domain.AgentIdentity, error)
	GetAgentStrategyProfile(agentID string) (domain.AgentStrategyProfile, error)
	UpsertAgentStrategyProfile(p domain.AgentStrategyProfile) (domain.AgentStrategyProfile, error)

	InsertEvolutionEvent(e domain.EvolutionEvent) (domain.EvolutionEvent, error)
	GetEvolutionEvent(id string) (domain.EvolutionEvent, error)
	ListEvolutionEvents(q EvolutionEventQuery) ([]domain.EvolutionEvent, int, error)
	MarkEvolutionEventReverted(id, byEventID, atISO string) error

	InsertEvolutionProposal(p domain.EvolutionProposal) (domain.EvolutionProposal, error)
	GetEvolutionProposal(id string) (domain.EvolutionProposal, error)
	ListEvolutionProposals(q EvolutionProposalQuery) ([]domain.EvolutionProposal, int, error)
	UpdateEvolutionProposalStatus(id, status, by, eventID, atISO string) error
	SupersedeProposalsByTarget(agentID, targetField, sinceISO string) (int, error)

	UpsertAgentSkillStat(s domain.AgentSkillStat) (domain.AgentSkillStat, error)
	GetAgentSkillStat(agentID, scope, scopeValue, toolKey string) (domain.AgentSkillStat, error)
	ListAgentSkillStats(agentID string, limit int) ([]domain.AgentSkillStat, error)
}

// EntityListQuery 在仓储层筛选知识图谱节点。
type EntityListQuery struct {
	ScopeType   mem.ScopeType
	ScopeID     string
	WorkspaceID string
	UserID      string
	EntityType  mem.EntityType
	Status      string
	Keyword     string
	Limit       int
	Offset      int
}

// EvolutionEventQuery 为列表接口筛选 EvolutionEvent 行。
type EvolutionEventQuery struct {
	AgentID     string
	WorkspaceID string
	Kind        string
	TriggerKind string
	Reverted    *bool
	Limit       int
	Offset      int
}

// EvolutionProposalQuery 为列表接口筛选 EvolutionProposal 行。
type EvolutionProposalQuery struct {
	AgentID     string
	WorkspaceID string
	Status      string
	RiskLevel   string
	Source      string
	TargetField string
	Limit       int
	Offset      int
}

// FactListQuery 在仓储层筛选事实。空字段会被忽略，使同一结构体同时适用于
// 「展示全部」的管理端接口与限定范围的 Agent 界面。
type FactListQuery struct {
	ScopeType   mem.ScopeType
	ScopeID     string
	WorkspaceID string
	UserID      string
	TeamID      string
	AgentID     string
	Status      string
	Kind        mem.FactKind
	Tags        []string
	Keyword     string
	Limit       int
	Offset      int
}
