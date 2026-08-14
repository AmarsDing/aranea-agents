package monitor

import (
	"aranea-agents/internal/biz/monitor/alert"
)

// DEV-05: the memory-canary alert metric lives in the alert subpackage;
// aliases keep the historical monitor.* API surface intact.

type (
	// CanaryFailureReader is a narrow port for reading the memory canary's
	// consecutive failure streak. Implemented by *biz.MemoryCanaryStatus.
	CanaryFailureReader = alert.CanaryFailureReader
	MemoryCanaryMetric  = alert.MemoryCanaryMetric
)

var NewMemoryCanaryMetric = alert.NewMemoryCanaryMetric
