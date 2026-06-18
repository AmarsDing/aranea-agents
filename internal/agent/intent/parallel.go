package intent

import (
	"os"
	"strings"
)

// AllowAutoParallel controls whether the parallel_candidates field returned by
// the LLM is automatically scheduled by ParallelToolExecutor.
//
// P1 fix (2026-06-18): Auto-parallel is now enabled by default to release the
// Cursor-level parallel tool calling capability. Set ARANEA_PARALLEL_AUTO=0|false|off
// to disable for regression testing or unstable model behavior.
func AllowAutoParallel() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ARANEA_PARALLEL_AUTO")))
	switch v {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}
