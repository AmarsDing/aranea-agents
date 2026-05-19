package memory

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/data/sessionmemory"
)

func TestL4GraphWriter_WriteFromUserText(t *testing.T) {
	// Smoke test with nil store (no-op).
	w := NewL4GraphWriter(nil)
	n, err := w.WriteFromUserText(context.Background(), "ag1", "u1", "My name is Alice")
	if err != nil || n != 0 {
		t.Fatalf("nil store: n=%d err=%v", n, err)
	}
	_ = sessionmemory.EventEntityParams{}
	_ = strings.ToLower("Alice")
}
