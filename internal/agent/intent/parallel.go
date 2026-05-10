package intent

import (
	"os"
	"strings"
)

// AllowAutoParallel is false unless ARANEA_PARALLEL_AUTO=1|true|on (experimental).
// Full parallel_candidates scheduling is not wired; this gates future auto-fan-out.
func AllowAutoParallel() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ARANEA_PARALLEL_AUTO")))
	switch v {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}
