## ADDED Requirements

### Requirement: UsageUsecase team member quota check
UsageUsecase SHALL provide a `CheckTeamMemberQuotas` method that validates quota for all enabled team members.

#### Scenario: All members within quota
- **WHEN** `CheckTeamMemberQuotas` is called for a team where all enabled members are within quota
- **THEN** it returns nil

#### Scenario: Member exceeds quota
- **WHEN** `CheckTeamMemberQuotas` is called for a team where an enabled member exceeds quota
- **THEN** it returns the quota enforcement error

#### Scenario: Empty team ID
- **WHEN** `CheckTeamMemberQuotas` is called with an empty team ID
- **THEN** it returns nil

### Requirement: UsageUsecase turn usage recording
UsageUsecase SHALL provide a `RecordTurnUsage` method that constructs a TokenUsageEvent, records it, accumulates session metrics, publishes the usage envelope, and links runner completion.

#### Scenario: Successful usage recording
- **WHEN** `RecordTurnUsage` is called with valid input
- **THEN** a TokenUsageEvent is constructed with correct TotalTokens, DateKey, HourKey, and persisted

#### Scenario: Nil usage usecase
- **WHEN** `RecordTurnUsage` is called on a nil UsageUsecase
- **THEN** it returns without error
