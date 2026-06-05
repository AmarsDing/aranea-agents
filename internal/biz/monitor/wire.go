package monitor

import (
	"database/sql"

	"github.com/google/wire"
)

// ProvideSelfCheckers returns the list of all registered SelfChecker implementations.
func ProvideSelfCheckers(
	db *sql.DB,
	projector *TraceProjector,
	worker *AlertEvalWorker,
	bus EventBusHealthChecker,
	counter WSConnectionCounter,
	appender *FlowFileAppender,
) []SelfChecker {
	var checkers []SelfChecker
	if db != nil {
		checkers = append(checkers, NewDBHealthChecker(db))
	}
	if projector != nil {
		checkers = append(checkers, NewTraceProjectorChecker(projector))
	}
	if worker != nil {
		checkers = append(checkers, NewAlertEvalChecker(worker))
	}
	if bus != nil {
		checkers = append(checkers, NewEventBusChecker(bus))
	}
	if counter != nil {
		checkers = append(checkers, NewWebSocketChecker(counter))
	}
	if appender != nil {
		checkers = append(checkers, NewFlowFileChecker(appender, ""))
	}
	return checkers
}

// ProvideSelfCheckRepairers returns the list of all registered SelfCheckRepairer implementations.
func ProvideSelfCheckRepairers(
	appender *FlowFileAppender,
	projector *TraceProjector,
	worker *AlertEvalWorker,
	bus EventBusResubscriber,
) []SelfCheckRepairer {
	var repairers []SelfCheckRepairer
	if appender != nil {
		repairers = append(repairers, NewFlowFileRepairer(appender))
	}
	if projector != nil {
		repairers = append(repairers, NewTraceProjectorRepairer(projector))
	}
	if worker != nil {
		repairers = append(repairers, NewAlertEvalRepairer(worker))
	}
	if bus != nil {
		repairers = append(repairers, NewEventBusRepairer(bus))
	}
	return repairers
}

// WireProviderSet exports all monitor sub-package Wire providers.
var WireProviderSet = wire.NewSet(
	NewAlertMetricRegistry,
	ProvideSelfCheckers,
	ProvideSelfCheckRepairers,
)
