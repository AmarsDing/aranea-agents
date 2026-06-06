package monitor

import (
	"strings"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ParseFlowLogTimeBounds parses and validates since/until time bounds from RFC3339 strings.
func ParseFlowLogTimeBounds(sinceRaw, untilRaw string) (since, until time.Time, err error) {
	if s := strings.TrimSpace(sinceRaw); s != "" {
		since, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			if since, err = time.Parse(time.RFC3339, s); err != nil {
				return time.Time{}, time.Time{}, kerrors.BadRequest("MONITOR", "invalid since: "+err.Error())
			}
		}
		since = since.UTC()
	}
	if u := strings.TrimSpace(untilRaw); u != "" {
		until, err = time.Parse(time.RFC3339Nano, u)
		if err != nil {
			if until, err = time.Parse(time.RFC3339, u); err != nil {
				return time.Time{}, time.Time{}, kerrors.BadRequest("MONITOR", "invalid until: "+err.Error())
			}
		}
		until = until.UTC()
	}
	if !since.IsZero() && !until.IsZero() && until.Before(since) {
		return time.Time{}, time.Time{}, kerrors.BadRequest("MONITOR", "until must be after since")
	}
	return since, until, nil
}
