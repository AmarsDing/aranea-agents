package biz

import (
	"context"
	"testing"
)

func TestForkMemoryPrivateCtx(t *testing.T) {
	if ForkMemoryPrivateFromContext(context.Background()) {
		t.Fatal("empty ctx must not be private")
	}
	ctx := WithForkMemoryPrivate(context.Background())
	if !ForkMemoryPrivateFromContext(ctx) {
		t.Fatal("fork ctx must be private")
	}
}
