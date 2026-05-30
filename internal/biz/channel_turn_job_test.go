package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestNormalizeChannelTurnJobListLimit(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{"zero returns default", 0, 50},
		{"negative returns default", -1, 50},
		{"within range", 100, 100},
		{"at max", biz.MaxChannelTurnJobListLimit, biz.MaxChannelTurnJobListLimit},
		{"over max capped", 300, biz.MaxChannelTurnJobListLimit},
		{"one", 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.NormalizeChannelTurnJobListLimit(tc.limit)
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNormalizeChannelTurnJobStatus(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   string
	}{
		{"running", "running", biz.ChannelTurnJobStatusRunning},
		{"queued", "queued", biz.ChannelTurnJobStatusQueued},
		{"completed", "completed", biz.ChannelTurnJobStatusCompleted},
		{"failed", "failed", biz.ChannelTurnJobStatusFailed},
		{"timeout", "timeout", biz.ChannelTurnJobStatusTimeout},
		{"cancelled", "cancelled", biz.ChannelTurnJobStatusCancelled},
		{"async_queued", "async_queued", biz.ChannelTurnJobStatusAsyncQueued},
		{"unknown defaults accepted", "unknown", biz.ChannelTurnJobStatusAccepted},
		{"empty defaults accepted", "", biz.ChannelTurnJobStatusAccepted},
		{"uppercase normalized", "RUNNING", biz.ChannelTurnJobStatusRunning},
		{"with spaces", "  completed  ", biz.ChannelTurnJobStatusCompleted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.NormalizeChannelTurnJobStatus(tc.status)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsChannelTurnJobTerminalStatus(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   bool
	}{
		{"completed is terminal", biz.ChannelTurnJobStatusCompleted, true},
		{"failed is terminal", biz.ChannelTurnJobStatusFailed, true},
		{"timeout is terminal", biz.ChannelTurnJobStatusTimeout, true},
		{"cancelled is terminal", biz.ChannelTurnJobStatusCancelled, true},
		{"running is not terminal", biz.ChannelTurnJobStatusRunning, false},
		{"queued is not terminal", biz.ChannelTurnJobStatusQueued, false},
		{"accepted is not terminal", biz.ChannelTurnJobStatusAccepted, false},
		{"async_queued is not terminal", biz.ChannelTurnJobStatusAsyncQueued, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.IsChannelTurnJobTerminalStatus(tc.status)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsChannelTurnJobIdempotentLockedStatus(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   bool
	}{
		{"completed is locked", biz.ChannelTurnJobStatusCompleted, true},
		{"failed is locked", biz.ChannelTurnJobStatusFailed, true},
		{"timeout is locked", biz.ChannelTurnJobStatusTimeout, true},
		{"cancelled is locked", biz.ChannelTurnJobStatusCancelled, true},
		{"queued is locked", biz.ChannelTurnJobStatusQueued, true},
		{"async_queued is locked", biz.ChannelTurnJobStatusAsyncQueued, true},
		{"running is not locked", biz.ChannelTurnJobStatusRunning, false},
		{"accepted is not locked", biz.ChannelTurnJobStatusAccepted, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.IsChannelTurnJobIdempotentLockedStatus(tc.status)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
