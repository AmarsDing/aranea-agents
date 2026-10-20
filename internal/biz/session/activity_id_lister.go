package session

import "context"

// ActivityIDLister is an optional extension of ActivityLister that loads
// activities by primary key without a full session scan (COMPRESS-IO / W1-5).
type ActivityIDLister interface {
	ListByIDs(ctx context.Context, sessionID string, ids []string) ([]ActivityEntry, error)
}
