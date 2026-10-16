package xiaohongshu

import (
	"context"
	"errors"
	"testing"

	conn "aranea-agents/internal/connector/publish"
)

func TestStubNotImplemented(t *testing.T) {
	var a Adapter
	_, err := a.Publish(context.Background(), conn.Request{Title: "x"})
	if !errors.Is(err, conn.ErrNotImplemented) {
		t.Fatalf("got %v", err)
	}
	if a.Capabilities().SupportsAPI {
		t.Fatal("API must be false in MVP")
	}
}
