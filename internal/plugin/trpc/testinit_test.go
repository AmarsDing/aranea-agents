package plugintrpc

import (
	"os"
	"testing"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func TestMain(m *testing.M) {
	loggateway.SetGlobal(loggateway.NewNoop())
	InitHookLogger(event.NewBus(nil), nil, loggateway.NewNoop())
	os.Exit(m.Run())
}
