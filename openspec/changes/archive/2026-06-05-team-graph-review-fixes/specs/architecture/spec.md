## MODIFIED Requirements

### Requirement: loggateway.Global() prohibited in new code
All new code SHALL use constructor-injected `loggateway.Logger` instead of `loggateway.Global()`. The `wsTurnExecutorAdapter` in `wire.go` SHALL be updated to accept `loggateway.Logger` via constructor injection.

#### Scenario: wsTurnExecutorAdapter uses injected logger
- **WHEN** `wsTurnExecutorAdapter.ExecuteTurn` needs to log
- **THEN** it SHALL use its injected `loggateway.Logger` field
- **AND** it SHALL NOT call `loggateway.Global()`

### Requirement: No global side effects in Wire provider functions
Wire provider functions SHALL NOT set package-level global variables. `biztool.SetGlobalWebResearchChecker` SHALL be removed; the checker SHALL be passed via constructor injection.

#### Scenario: ToolUsecase receives WebResearchChecker via constructor
- **WHEN** `NewToolUsecase` is called
- **THEN** it SHALL accept `WebResearchReadinessChecker` as a constructor parameter
- **AND** `biztool.SetGlobalWebResearchChecker` SHALL NOT be called

#### Scenario: No package-level global variable for WebResearchChecker
- **WHEN** the codebase is searched for `SetGlobalWebResearchChecker`
- **THEN** no calls SHALL exist in production code

### Requirement: Ent Schema aligned with hand-written DDL
The Ent Schema for `compiled_teams` table SHALL be consistent with the hand-written SQL DDL. Specifically: `id` field MaxLen SHALL be 192 (not 64), and `updated_at` SHALL NOT be `Optional().Nillable()`.

#### Scenario: compiled_teams id field MaxLen matches actual ID format
- **WHEN** the Ent Schema for `compiled_teams` defines the `id` field
- **THEN** `MaxLen` SHALL be 192 (accommodating `teamID:graphID` format up to 64+1+64=129 chars with margin)

#### Scenario: compiled_teams updated_at matches DDL behavior
- **WHEN** the Ent Schema for `compiled_teams` defines the `updated_at` field
- **THEN** it SHALL NOT be `Optional().Nillable()` (the Save method always writes a value)

### Requirement: CompiledTeamRepo SQL uses ON CONFLICT instead of INSERT OR REPLACE
`CompiledTeamRepo.Save` SHALL use `INSERT ... ON CONFLICT(id) DO UPDATE SET` syntax instead of `INSERT OR REPLACE`, to preserve the original `created_at` value.

#### Scenario: Save preserves created_at on conflict
- **WHEN** `Save` is called for a CompiledTeam that already exists
- **THEN** `created_at` SHALL retain its original value
- **AND** only `updated_at` SHALL be updated to the current time

### Requirement: LoadForSession validates sessionID is non-empty
`CompiledTeamRepo.LoadForSession` SHALL return an error when `sessionID` is empty, rather than silently skipping validation.

#### Scenario: LoadForSession rejects empty sessionID
- **WHEN** `LoadForSession` is called with an empty `sessionID`
- **THEN** it SHALL return a `kerrors.BadRequest` error
- **AND** it SHALL NOT return the CompiledTeam

### Requirement: CompiledTeamRepo has unit test coverage
`CompiledTeamRepo` SHALL have unit tests covering Save/Load round-trip, LoadForSession validation, and Delete operations.

#### Scenario: Save and Load round-trip test
- **WHEN** a `CompiledTeam` is saved and then loaded
- **THEN** the loaded value SHALL equal the saved value

#### Scenario: LoadForSession rejects empty sessionID test
- **WHEN** `LoadForSession` is called with empty sessionID
- **THEN** the test SHALL verify a `kerrors.BadRequest` error is returned
