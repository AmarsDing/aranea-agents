package biz

import "testing"

func TestIsChannelTurnJobIdempotentLockedStatus(t *testing.T) {
	if !IsChannelTurnJobIdempotentLockedStatus(ChannelTurnJobStatusAsyncQueued) {
		t.Fatal("async_queued should be idempotent-locked")
	}
	if IsChannelTurnJobIdempotentLockedStatus(ChannelTurnJobStatusRunning) {
		t.Fatal("running should not be idempotent-locked")
	}
}
