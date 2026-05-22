package service

import (
	"testing"
	"time"
)

func TestRuntimeReloadInterval(t *testing.T) {
	t.Setenv("CHANNEL_RUNTIME_RELOAD_INTERVAL", "30s")
	if got := RuntimeReloadInterval(); got != 30*time.Second {
		t.Fatalf("got %v want 30s", got)
	}
	t.Setenv("CHANNEL_RUNTIME_RELOAD_INTERVAL", "0")
	if got := RuntimeReloadInterval(); got != 0 {
		t.Fatalf("got %v want 0", got)
	}
}
