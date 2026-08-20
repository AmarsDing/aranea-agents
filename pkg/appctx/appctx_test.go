package appctx

import (
	"context"
	"testing"
	"time"
)

type detachTestKey struct{}

func TestDetach_SurvivesParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), detachTestKey{}, "trace-123"))
	cancel() // 父 ctx 已取消（模拟客户端断连/用户取消 turn）

	detached, done := Detach(parent)
	defer done()

	if err := detached.Err(); err != nil {
		t.Fatalf("detached ctx should survive parent cancel, got %v", err)
	}
	if got := detached.Value(detachTestKey{}); got != "trace-123" {
		t.Fatalf("detached ctx lost values, got %v", got)
	}
	if _, ok := detached.Deadline(); !ok {
		t.Fatal("detached ctx should carry a bounded timeout")
	}
}

func TestDetach_NilSafe(t *testing.T) {
	detached, done := Detach(nil)
	defer done()
	if detached == nil || detached.Err() != nil {
		t.Fatal("Detach(nil) should return a live background ctx")
	}
}

func TestDetach_TimeoutFires(t *testing.T) {
	detached, done := Detach(context.Background())
	defer done()
	deadline, _ := detached.Deadline()
	if d := time.Until(deadline); d <= 0 || d > 30*time.Second {
		t.Fatalf("unexpected deadline delta %v", d)
	}
}
