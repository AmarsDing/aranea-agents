package data

import (
	"context"
	"strings"
)

// LookupSessionUserID returns sessions.user_id for MEM-01 L1 archive scoping.
func LookupSessionUserID(ctx context.Context, d *Data, sessionID string) (string, error) {
	if d == nil {
		return "", nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", nil
	}
	row, err := d.RW().Read(ctx).Session.Get(ctx, sessionID)
	if err != nil {
		return "", entErrToBizErr(err, "SESSION")
	}
	return row.UserID, nil
}
