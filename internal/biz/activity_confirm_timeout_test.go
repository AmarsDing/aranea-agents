package biz

import (
	"context"
	"testing"
)

func TestConfirmTimeoutRetryingCtx(t *testing.T) {
	if ConfirmTimeoutRetrying(context.Background()) {
		t.Fatal("plain ctx must not be retrying")
	}
	if !ConfirmTimeoutRetrying(WithConfirmTimeoutRetrying(context.Background())) {
		t.Fatal("wrapped ctx must report retrying")
	}
	if !ConfirmTimeoutRetrying(WithConfirmTimeoutRetrying(nil)) {
		t.Fatal("nil ctx wrap must still report retrying")
	}
}
