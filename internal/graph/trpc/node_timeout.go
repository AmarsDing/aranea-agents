package graph

import (
	"time"

	"aranea-agents/internal/biz"
)

// MaxNodeTimeout returns the max NodeDef.TimeoutSeconds as a duration.
// Used to configure executor-level WithNodeTimeout (trpc graph has no per-node timeout option).
func MaxNodeTimeout(nodes []biz.NodeDef) time.Duration {
	maxSec := 0
	for _, n := range nodes {
		if n.TimeoutSeconds > maxSec {
			maxSec = n.TimeoutSeconds
		}
	}
	if maxSec <= 0 {
		return 0
	}
	return time.Duration(maxSec) * time.Second
}
