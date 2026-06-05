## ADDED Requirements

### Requirement: Build functions accept GraphNodeResolverSet instead of BuildDeps
All graph build functions (`BuildStateGraph`, `BuildStateGraphWithRegistry`, `BuildStateGraphWithRegistryAndLogger`, `BuildStateGraphWithAgents`, `buildFromResolved`) SHALL accept `*GraphNodeResolverSet` instead of `*BuildDeps`. The `BuildDeps` struct and `ToBuildDeps()`/`ToBuildDepsPtr()` conversion methods SHALL be deleted.

#### Scenario: BuildStateGraphWithAgents uses GraphNodeResolverSet
- **WHEN** `BuildStateGraphWithAgents` is called
- **THEN** its `deps` parameter SHALL be of type `*GraphNodeResolverSet`
- **AND** it SHALL NOT reference `BuildDeps`

#### Scenario: BuildDeps struct removed
- **WHEN** the codebase is searched for `BuildDeps`
- **THEN** no struct definition or usage SHALL exist
- **AND** `ToBuildDeps()`/`ToBuildDepsPtr()` SHALL be deleted

### Requirement: wireNode consumes deps.Functions for function node resolution
`wireNode` SHALL use `deps.Functions.ResolveFunction(ctx, funcRef)` to resolve function nodes, replacing the current `FuncRef` string-only path.

#### Scenario: function node resolved via FunctionResolver
- **WHEN** `wireNode` processes a node with `Type == "function"`
- **THEN** it SHALL call `deps.Functions.ResolveFunction(ctx, node.FuncRef)` to obtain the `NodeFunc`
- **AND** if resolution fails, it SHALL return an error

#### Scenario: function node FuncRef fallback
- **WHEN** `deps.Functions` is nil or `ResolveFunction` returns nil
- **THEN** `wireNode` SHALL fall back to the current `FuncRef`-based resolution as a graceful degradation

### Requirement: GraphNodeResolverSet.Subgraphs field removed or implemented
The `Subgraphs SubgraphResolver` field in `GraphNodeResolverSet` SHALL either be removed (if no near-term consumer) or have a concrete implementation provided.

#### Scenario: Subgraphs field removed
- **WHEN** no SubgraphResolver implementation exists
- **THEN** the `Subgraphs` field and `SubgraphResolver` interface SHALL be removed from `build_deps.go`

#### Scenario: Subgraphs field implemented
- **WHEN** a `CatalogSubgraphResolver` implementation is provided
- **THEN** `wireNode` SHALL use `deps.Subgraphs.ResolveSubgraph(ctx, graphID)` for subgraph node resolution
