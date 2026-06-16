package runtime

import (
	"testing"

	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/pkg/loggateway"
)

func TestNewTRPCSessionServiceNilDB(t *testing.T) {
	svc := NewTRPCSessionService(nil, loggateway.NewNoop(), sessiontrpc.SummarizerConfig{})
	if svc == nil {
		t.Fatal("expected in-memory fallback session service")
	}
}

func TestNewGraphCheckpointSaverNilDB(t *testing.T) {
	_, err := NewGraphCheckpointSaver(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}
