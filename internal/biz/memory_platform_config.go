package biz

import (
	"context"
	"os"
	"strings"
)

// MemoryPlatformSetting holds platform-wide memory worker / policy toggles (system_settings).
type MemoryPlatformSetting struct {
	PolicyStrict            bool
	EpisodeBackfillDisabled bool
}

func envTruthy(name string) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return raw == "1" || raw == "true" || raw == "yes"
}

// MemoryPlatformEnvOverrides reports whether env vars override DB for each toggle.
func MemoryPlatformEnvOverrides() (policyStrict, episodeBackfillDisabled bool) {
	return envTruthy("MEMORY_POLICY_STRICT"), envTruthy("MEMORY_EPISODE_BACKFILL_DISABLED")
}

// ResolvePolicyStrict returns true when policy audit failures must block writes.
// Precedence: MEMORY_POLICY_STRICT env > system_settings.memory_policy_strict.
func ResolvePolicyStrict(ctx context.Context, sys SystemSettingRepo) bool {
	if envTruthy("MEMORY_POLICY_STRICT") {
		return true
	}
	if sys == nil {
		return false
	}
	row, err := sys.Get(ctx)
	if err != nil {
		return false
	}
	return row.MemoryPlatform.PolicyStrict
}

// ResolveEpisodeBackfillDisabled returns true when the episode embedding backfill worker should skip runs.
// Precedence: MEMORY_EPISODE_BACKFILL_DISABLED env > system_settings.memory_episode_backfill_disabled.
func ResolveEpisodeBackfillDisabled(ctx context.Context, sys SystemSettingRepo) bool {
	if envTruthy("MEMORY_EPISODE_BACKFILL_DISABLED") {
		return true
	}
	if sys == nil {
		return false
	}
	row, err := sys.Get(ctx)
	if err != nil {
		return false
	}
	return row.MemoryPlatform.EpisodeBackfillDisabled
}
