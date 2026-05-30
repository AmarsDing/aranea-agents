package service

import (
	"context"
	"testing"
)

func TestWithChannelTurnJob_bothSet(t *testing.T) {
	ctx := context.Background()
	ctx = withChannelTurnJob(ctx, "job-1", "sess-1")
	jobID, sessionID := channelTurnJobFromContext(ctx)
	if jobID != "job-1" {
		t.Errorf("jobID = %q, want %q", jobID, "job-1")
	}
	if sessionID != "sess-1" {
		t.Errorf("sessionID = %q, want %q", sessionID, "sess-1")
	}
}

func TestWithChannelTurnJob_onlyJobID(t *testing.T) {
	ctx := context.Background()
	ctx = withChannelTurnJob(ctx, "job-2", "")
	jobID, sessionID := channelTurnJobFromContext(ctx)
	if jobID != "job-2" {
		t.Errorf("jobID = %q, want %q", jobID, "job-2")
	}
	if sessionID != "" {
		t.Errorf("sessionID = %q, want empty", sessionID)
	}
}

func TestWithChannelTurnJob_onlySessionID(t *testing.T) {
	ctx := context.Background()
	ctx = withChannelTurnJob(ctx, "", "sess-2")
	jobID, sessionID := channelTurnJobFromContext(ctx)
	if jobID != "" {
		t.Errorf("jobID = %q, want empty", jobID)
	}
	if sessionID != "sess-2" {
		t.Errorf("sessionID = %q, want %q", sessionID, "sess-2")
	}
}

func TestWithChannelTurnJob_bothEmpty(t *testing.T) {
	ctx := context.Background()
	result := withChannelTurnJob(ctx, "", "")
	if result != ctx {
		t.Error("both empty should return original context")
	}
}

func TestWithChannelTurnJob_whitespaceTrimmed(t *testing.T) {
	ctx := context.Background()
	ctx = withChannelTurnJob(ctx, "  job-3  ", "  sess-3  ")
	jobID, sessionID := channelTurnJobFromContext(ctx)
	if jobID != "job-3" {
		t.Errorf("jobID = %q, want %q", jobID, "job-3")
	}
	if sessionID != "sess-3" {
		t.Errorf("sessionID = %q, want %q", sessionID, "sess-3")
	}
}

func TestWithChannelTurnJobID_preservesSessionID(t *testing.T) {
	ctx := context.Background()
	ctx = withChannelTurnJob(ctx, "old-job", "sess-keep")
	ctx = withChannelTurnJobID(ctx, "new-job")
	jobID, sessionID := channelTurnJobFromContext(ctx)
	if jobID != "new-job" {
		t.Errorf("jobID = %q, want %q", jobID, "new-job")
	}
	if sessionID != "sess-keep" {
		t.Errorf("sessionID = %q, want %q", sessionID, "sess-keep")
	}
}

func TestWithChannelTurnJobID_noExistingContext(t *testing.T) {
	ctx := context.Background()
	ctx = withChannelTurnJobID(ctx, "job-new")
	jobID, sessionID := channelTurnJobFromContext(ctx)
	if jobID != "job-new" {
		t.Errorf("jobID = %q, want %q", jobID, "job-new")
	}
	if sessionID != "" {
		t.Errorf("sessionID = %q, want empty", sessionID)
	}
}

func TestChannelTurnJobFromContext_nilContext(t *testing.T) {
	jobID, sessionID := channelTurnJobFromContext(nil)
	if jobID != "" {
		t.Errorf("jobID = %q, want empty for nil context", jobID)
	}
	if sessionID != "" {
		t.Errorf("sessionID = %q, want empty for nil context", sessionID)
	}
}

func TestChannelTurnJobFromContext_noValue(t *testing.T) {
	ctx := context.Background()
	jobID, sessionID := channelTurnJobFromContext(ctx)
	if jobID != "" {
		t.Errorf("jobID = %q, want empty with no value set", jobID)
	}
	if sessionID != "" {
		t.Errorf("sessionID = %q, want empty with no value set", sessionID)
	}
}

func TestChannelTurnJobIDFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"with_job", withChannelTurnJob(context.Background(), "j1", "s1"), "j1"},
		{"no_job", context.Background(), ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := channelTurnJobIDFromContext(tt.ctx)
			if got != tt.want {
				t.Errorf("channelTurnJobIDFromContext() = %q, want %q", got, tt.want)
			}
		})
	}
}
