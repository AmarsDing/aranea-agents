package data

import "testing"

// TestMigrationVersionsGloballyUnique guards the schema_migrations version
// namespace: DDL migrations (ddlMigrations), data migrations (Migration*
// constants) and seed versions (Seed* constants) share one table whose
// version is the PRIMARY KEY — whichever runs first claims the version and
// the others are silently skipped forever. 2026-07-30 audit found five such
// collisions in production (memory_episodes missing its partial unique
// indexes, cascade_saga_steps stuck with INTEGER id, audit action normalize
// never applied, etc.), all fixed by renumbering the loser.
//
// When adding a new Migration*/Seed* constant, add it to the list below.
func TestMigrationVersionsGloballyUnique(t *testing.T) {
	type owner struct {
		kind, name string
	}
	owners := map[int][]owner{}
	add := func(kind, name string, version int) {
		owners[version] = append(owners[version], owner{kind, name})
	}

	for _, m := range ddlMigrations {
		add("ddl", m.Name, m.Version)
	}
	for _, dm := range []struct {
		name    string
		version int
	}{
		{"legacy_trpc_memory_facts", MigrationLegacyTRPCMemoryFacts},
		{"turn_index_to_turn_id", MigrationTurnIndexToTurnID},
		{"session_status_active_to_idle", MigrationSessionStatusIdle},
		{"session_turn_number_backfill", MigrationSessionTurnNumberBackfill},
		{"session_turn_number_rebackfill", MigrationSessionTurnNumberRebackfill},
		{"team_copy_ownership_to_user", MigrationTeamCopyOwnership},
		{"audit_action_verb_first_normalize", MigrationAuditActionNormalize},
		{"monitor_trace_interrupted_backfill", MigrationMonitorTraceInterruptedBackfill},
		{"avatar_image_repair", MigrationAvatarImageRepair},
		{"organization_redesign", MigrationOrganizationRedesign},
	} {
		add("data", dm.name, dm.version)
	}
	for _, sv := range []struct {
		name    string
		version int
	}{
		{"SeedPackBuiltinV1", SeedPackBuiltinV1},
		{"SeedPackBuiltinV2", SeedPackBuiltinV2},
		{"SeedCleanupNonSystemV1", SeedCleanupNonSystemV1},
		{"SeedPackAgencyV1", SeedPackAgencyV1},
		{"SeedPackItOpsV1", SeedPackItOpsV1},
	} {
		add("seed", sv.name, sv.version)
	}

	for version, list := range owners {
		if len(list) > 1 {
			t.Errorf("schema_migrations version %d claimed by %d migrations: %v", version, len(list), list)
		}
	}
}
