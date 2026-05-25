package modelcatalog

import "time"

// MigrationCheckpoint records the last successful provider binding migration (not user-editable rules).
type MigrationCheckpoint struct {
	AppliedAt string              `json:"applied_at"`
	Version   string              `json:"version"`
	Stats     ApplyMigrationStats `json:"stats,omitempty"`
}

func NewMigrationCheckpoint(stats ApplyMigrationStats) MigrationCheckpoint {
	return MigrationCheckpoint{
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
		Version:   ProviderMigrationVersion,
		Stats:     stats,
	}
}
