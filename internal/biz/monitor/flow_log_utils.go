package monitor

import (
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// metaStr extracts a trimmed string value from a metadata map. Shared by the
// rca/heal domain files remaining in this package (the trace subpackage keeps
// its own private copy).
func metaStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// ParseFlowLogTimeBounds parses and validates since/until time bounds from RFC3339 strings.
func ParseFlowLogTimeBounds(sinceRaw, untilRaw string) (since, until time.Time, err error) {
	if s := strings.TrimSpace(sinceRaw); s != "" {
		since, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			if since, err = time.Parse(time.RFC3339, s); err != nil {
				return time.Time{}, time.Time{}, apierror.BadRequest("MONITOR", "invalid since: %s", err.Error())
			}
		}
		since = since.UTC()
	}
	if u := strings.TrimSpace(untilRaw); u != "" {
		until, err = time.Parse(time.RFC3339Nano, u)
		if err != nil {
			if until, err = time.Parse(time.RFC3339, u); err != nil {
				return time.Time{}, time.Time{}, apierror.BadRequest("MONITOR", "invalid until: %s", err.Error())
			}
		}
		until = until.UTC()
	}
	if !since.IsZero() && !until.IsZero() && until.Before(since) {
		return time.Time{}, time.Time{}, apierror.BadRequest("MONITOR", "until must be after since")
	}
	return since, until, nil
}
