package data

// SQL fragments shared by usage aggregates and quota SUM.

const (
	sqlUsageStatusSuccess = `status IN ('success', 'ok')`
	sqlUsageStatusFailed  = `status IN ('failed', 'timeout', 'error')`

	// sqlUsageBillableKind excludes team_turn from platform/agent aggregates.
	// Team billable usage is attributed via team_member rows; team_turn is run-level reconciliation only.
	sqlUsageBillableKind = `(usage_kind IS NULL OR usage_kind = '' OR usage_kind <> 'team_turn')`
)
