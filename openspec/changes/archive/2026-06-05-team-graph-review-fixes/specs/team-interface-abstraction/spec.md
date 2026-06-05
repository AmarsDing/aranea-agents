## ADDED Requirements

### Requirement: NewRunner accepts narrow interfaces instead of concrete Usecase types
`NewRunner` SHALL accept narrow biz interfaces instead of `*biz.XxxUsecase` concrete types for the following parameters: usage, sessions, agentsUC, toolUC, catalog, skillUC.

#### Scenario: NewRunner accepts TeamUsageQuerier instead of *biz.UsageUsecase
- **WHEN** `NewRunner` is constructed
- **THEN** the `usage` parameter SHALL be of type `biz.TeamUsageQuerier` (or similar narrow interface)
- **AND** the interface SHALL have ≤5 methods covering only the methods Runner actually calls

#### Scenario: NewRunner accepts TeamSessionManager instead of *biz.SessionUsecase
- **WHEN** `NewRunner` is constructed
- **THEN** the `sessions` parameter SHALL be of type `biz.TeamSessionManager` (or similar narrow interface)

#### Scenario: NewRunner accepts TeamAgentLookup instead of *biz.AgentUsecase
- **WHEN** `NewRunner` is constructed
- **THEN** the `agentsUC` parameter SHALL be of type `biz.TeamAgentLookup` (or similar narrow interface)

#### Scenario: Wire binds concrete Usecase to narrow interface
- **WHEN** Wire constructs the Runner
- **THEN** `wire.Bind(new(biz.TeamUsageQuerier), new(*biz.UsageUsecase))` (and similar) SHALL be declared in `wire.go`

### Requirement: KnowledgeFacade holds interfaces instead of concrete types
`KnowledgeFacade` SHALL hold interfaces (defined in biz) instead of `*knowledge.Retriever`, `*knowledge.AdaptiveRouter`, `*knowledge.FederatedRetriever`, `*knowledge.RetrievalEvaluator` concrete types.

#### Scenario: KnowledgeFacade.Retriever is an interface
- **WHEN** `KnowledgeFacade` is defined
- **THEN** `Retriever` SHALL be of type `biz.KnowledgeRetriever` (or similar interface)
- **AND** the interface SHALL cover only the methods Runner/Coordinator actually call

#### Scenario: KnowledgeFacade constructed with interface implementations
- **WHEN** `KnowledgeFacade` is constructed in Wire
- **THEN** concrete `*knowledge.Xxx` instances SHALL be bound to their biz interfaces via `wire.Bind`

### Requirement: RunnerConfig PluginRT and PluginManager use interfaces
`RunnerConfig.PluginRT` and `RunnerConfig.PluginManager` SHALL hold biz interfaces instead of `*plugintrpc.Runtime` and `*plugintrpc.Manager` concrete types.

#### Scenario: RunnerConfig.PluginRT is an interface
- **WHEN** `RunnerConfig` is defined
- **THEN** `PluginRT` SHALL be of type `biz.PluginRuntime` (or similar interface)

#### Scenario: RunnerConfig.PluginManager is an interface
- **WHEN** `RunnerConfig` is defined
- **THEN** `PluginManager` SHALL be of type `biz.PluginManagerAccess` (or similar interface)
