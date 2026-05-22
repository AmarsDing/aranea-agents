package service

import (
	"context"
	"testing"
	"time"
)

func TestApplyChannelTurnTimeout_respectsShorterParent(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer parentCancel()
	ctx, cancel := applyChannelTurnTimeout(parent, 900)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	if time.Until(deadline) > 3*time.Second {
		t.Fatalf("should keep parent deadline, got %v", time.Until(deadline))
	}
}

func TestFirstByteTimeoutFromContext(t *testing.T) {
	ctx := WithChannelTurnDeadlines(context.Background(), ChannelTurnDeadlines{
		FirstByteTimeout: 120 * time.Second,
	})
	d, ok := firstByteTimeoutFromContext(ctx)
	if !ok || d != 120*time.Second {
		t.Fatalf("got %v ok=%v", d, ok)
	}
}
