package runtime

import (
	"testing"
)

func TestNewTRPCSessionServiceNilDB(t *testing.T) {
	svc := NewTRPCSessionService(nil)
	if svc == nil {
		t.Fatal("expected in-memory fallback session service")
	}
}

func TestNewGraphCheckpointSaverNilDB(t *testing.T) {
	_, err := NewGraphCheckpointSaver(nil)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}
