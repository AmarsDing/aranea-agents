package modelregistry

import "time"

type MigrationCheckpoint struct {
	AppliedAt      string              `json:"applied_at"`
	Version        string              `json:"version"`
	Stats          ApplyMigrationStats `json:"stats,omitempty"`
	CompletedRules []string            `json:"completed_rules,omitempty"`
}

func NewMigrationCheckpoint(stats ApplyMigrationStats) MigrationCheckpoint {
	return MigrationCheckpoint{
		AppliedAt: nowRFC3339(),
		Version:   ProviderMigrationVersion,
		Stats:     stats,
	}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
