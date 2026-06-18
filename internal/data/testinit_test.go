package data

import (
	"testing"

	"aranea-agents/pkg/loggateway"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	loggateway.SetGlobal(loggateway.NewNoop())
	// T6.2: goleak detection — verify no goroutine leaks across all data
	// package tests. IgnoreCurrent excludes goroutines spawned by the test
	// harness itself (loggateway, SQLite driver, etc.) so we only detect
	// new leaks from Repo implementations and migration runners.
	goleak.VerifyTestMain(m,
		goleak.IgnoreCurrent(),
	)
}
