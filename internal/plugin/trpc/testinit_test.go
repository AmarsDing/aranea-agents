package plugintrpc

import (
	"os"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestMain(m *testing.M) {
	loggateway.SetGlobal(loggateway.NewNoop())
	InitHookLogger(nil)
	os.Exit(m.Run())
}
