package service

import (
	"os"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestMain(m *testing.M) {
	loggateway.SetGlobal(loggateway.NewNoop())
	os.Exit(m.Run())
}
