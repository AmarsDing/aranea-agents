package agent

import (
	"testing"

	"aranea-agents/pkg/loggateway"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	loggateway.SetGlobal(loggateway.NewNoop())
	// T6.2: goleak detection — verify no goroutine leaks across all agent
	// package tests. IgnoreCurrent excludes goroutines spawned by the test
	// harness itself (loggateway, etc.) so we only detect new leaks from
	// ActivityProjector, EventProjector, and related components.
	// VerifyTestMain runs m.Run() internally and exits on leak detection.
	goleak.VerifyTestMain(m,
		goleak.IgnoreCurrent(),
	)
}
