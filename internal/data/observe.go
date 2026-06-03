package data

import (
	"time"

	"aranea-agents/pkg/loggateway"
)

const slowQueryThreshold = 100 * time.Millisecond

// observeQuery wraps a database operation with latency tracking and slow query logging.
// It records the query duration in the aranea_db_query_duration_seconds histogram
// and logs a warning if the operation exceeds 100ms.
func observeQuery(lg loggateway.Logger, repo, operation string, fn func() error) error {
	start := time.Now()
	err := fn()
	elapsed := time.Since(start)

	observeDBQuery(repo, operation, elapsed.Seconds(), err)

	if elapsed > slowQueryThreshold {
		lg.Warn("slow query detected",
			loggateway.StepID("data.slow_query"),
			loggateway.Str("repo", repo),
			loggateway.Str("operation", operation),
			loggateway.Int("duration_ms", int(elapsed.Milliseconds())),
		)
	}

	return err
}
