package monitor

import (
	"github.com/google/wire"
)

// ProvideSelfCheckers returns the list of all registered SelfChecker implementations.
// Add new checkers here as they are implemented.
func ProvideSelfCheckers() []SelfChecker {
	return []SelfChecker{}
}

// ProvideSelfCheckRepairers returns the list of all registered SelfCheckRepairer implementations.
// Add new repairers here as they are implemented.
func ProvideSelfCheckRepairers() []SelfCheckRepairer {
	return []SelfCheckRepairer{}
}

// WireProviderSet exports all monitor sub-package Wire providers.
var WireProviderSet = wire.NewSet(
	NewSelfCheckScheduler,
	NewAlertMetricRegistry,
	ProvideSelfCheckers,
	ProvideSelfCheckRepairers,
)
