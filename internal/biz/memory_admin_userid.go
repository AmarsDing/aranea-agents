package biz

import (
	"context"
	"encoding/json"
)

// SessionUserIDResolver resolves the owning user for a session (MEM-01).
// L1 task rows have no user_id column; Path B entity writes need a reliable scope.
type SessionUserIDResolver func(ctx context.Context, sessionID string) (string, error)

// SetSessionUserIDResolver injects session->user lookup for L1 archive Path B (MEM-01).
func (uc *MemoryAdminUsecase) SetSessionUserIDResolver(fn SessionUserIDResolver) {
	if uc != nil {
		uc.resolveUserID = fn
	}
}

// extractUserIDFromTaskMeta attempts to extract user_id from the L1 task row's
// metadata_json (best-effort; the L1 task table does not have a dedicated user_id column).
func extractUserIDFromTaskMeta(taskMap map[string]any) string {
	if taskMap == nil {
		return ""
	}
	metaStr, _ := taskMap["metadata_json"].(string)
	if metaStr == "" {
		return ""
	}
	var meta map[string]any
	if json.Unmarshal([]byte(metaStr), &meta) != nil {
		return ""
	}
	uid, _ := meta["user_id"].(string)
	return uid
}
