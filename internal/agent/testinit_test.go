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
	// ActivityProjector and related components.
	// VerifyTestMain runs m.Run() internally and exits on leak detection.
	goleak.VerifyTestMain(m,
		goleak.IgnoreCurrent(),
		// HTTP/2 client read loops from the shared OAuth2 client may still be
		// winding down when tests finish; they are not production leaks.
		goleak.IgnoreAnyFunction("net/http.(*http2ClientConn).readLoop"),
	)
}
