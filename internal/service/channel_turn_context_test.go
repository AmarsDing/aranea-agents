package service

import (
	"context"
	"testing"
	"time"
)

func TestWithChannelTurnDeadlines(t *testing.T) {
	ctx := context.Background()

	zeroCtx := WithChannelTurnDeadlines(ctx, ChannelTurnDeadlines{})
	if zeroCtx != ctx {
		t.Fatal("zero deadlines should return original ctx")
	}

	d := ChannelTurnDeadlines{TurnTimeout: 30 * time.Second}
	newCtx := WithChannelTurnDeadlines(ctx, d)
	if newCtx == ctx {
		t.Fatal("non-zero deadlines should return new ctx")
	}
	got, ok := channelTurnDeadlinesFromContext(newCtx)
	if !ok || got.TurnTimeout != d.TurnTimeout {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}

func TestChannelTurnDeadlinesFromContext_NilCtx(t *testing.T) {
	_, ok := channelTurnDeadlinesFromContext(nil)
	if ok {
		t.Fatal("nil ctx should return false")
	}
}

func TestApplyChannelTurnTimeout(t *testing.T) {
	parent := context.Background()

	ctx, cancel := applyChannelTurnTimeout(parent, 0)
	defer cancel()
	if ctx != parent {
		t.Fatal("zero turnSec should return parent ctx")
	}

	ctx, cancel = applyChannelTurnTimeout(parent, 30)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	if time.Until(deadline) > 30*time.Second || time.Until(deadline) < 28*time.Second {
		t.Fatalf("deadline too far: %v", time.Until(deadline))
	}
}

func TestApplyChannelTurnTimeout_ParentAlreadyShorter(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer parentCancel()

	ctx, cancel := applyChannelTurnTimeout(parent, 300)
	defer cancel()
	if ctx != parent {
		t.Fatal("should reuse parent when parent deadline is shorter")
	}
}

func TestFirstByteTimeoutFromContext(t *testing.T) {
	_, ok := firstByteTimeoutFromContext(context.Background())
	if ok {
		t.Fatal("empty ctx should not have first byte timeout")
	}

	ctx := WithChannelTurnDeadlines(context.Background(), ChannelTurnDeadlines{FirstByteTimeout: 5 * time.Second})
	d, ok := firstByteTimeoutFromContext(ctx)
	if !ok || d != 5*time.Second {
		t.Fatalf("got %v ok=%v", d, ok)
	}
}
