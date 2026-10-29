package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

// finishExecuteErrorLifecycleSpy 记录 FinishSessionRunLifecycle 调用。
type finishExecuteErrorLifecycleSpy struct {
	calls       int
	sessionID   string
	sessionRun  string
	turnErr     error
	beginErr    error
	escalateHit bool
}

func (s *finishExecuteErrorLifecycleSpy) BeginSessionRunLifecycle(ctx context.Context, _ SessionRunStartParams) (context.Context, string, error) {
	return ctx, "", s.beginErr
}

func (s *finishExecuteErrorLifecycleSpy) FinishSessionRunLifecycle(_ context.Context, sessionID, sessionRunID string, turnErr error) {
	s.calls++
	s.sessionID = sessionID
	s.sessionRun = sessionRunID
	s.turnErr = turnErr
}

func (s *finishExecuteErrorLifecycleSpy) CompleteHeldSessionRun(context.Context, string) {}

func (s *finishExecuteErrorLifecycleSpy) EscalateToDurableByUser(context.Context, string, string) {
	s.escalateHit = true
}

func (s *finishExecuteErrorLifecycleSpy) EscalateToDurableOnShutdown(context.Context, string, string) bool {
	s.escalateHit = true
	return true
}

func (s *finishExecuteErrorLifecycleSpy) ResolveChannelLongTaskConfig(context.Context, biz.Session) biz.ChannelLongTaskConfig {
	return biz.ChannelLongTaskConfig{}
}

// EXECUTE 失败路径必须终结 session run（2026-10-29 活跃 run 泄漏修复）：
// 此前仅成功路径注册 Finish defer，失败分支直接 return，session_runs 残留
// interactive 相位，活跃唯一索引使后续所有 turn fail-closed CONFLICT。
func TestFinishSessionRunOnExecuteError_DelegatesWithRunID(t *testing.T) {
	spy := &finishExecuteErrorLifecycleSpy{}
	turnErr := errors.New("first byte timeout after 3m33s")
	finishSessionRunOnExecuteError(context.Background(), spy, "sess-1", "srid-1", turnErr)
	if spy.calls != 1 {
		t.Fatalf("FinishSessionRunLifecycle calls = %d, want 1", spy.calls)
	}
	if spy.sessionID != "sess-1" || spy.sessionRun != "srid-1" {
		t.Fatalf("delegated ids = (%q, %q), want (sess-1, srid-1)", spy.sessionID, spy.sessionRun)
	}
	if !errors.Is(spy.turnErr, turnErr) {
		t.Fatalf("delegated turnErr = %v, want %v", spy.turnErr, turnErr)
	}
}

// 空 runID（Begin 未成功/未挂绑）必须静默跳过，禁止以空串终结。
func TestFinishSessionRunOnExecuteError_EmptyRunIDSkips(t *testing.T) {
	spy := &finishExecuteErrorLifecycleSpy{}
	finishSessionRunOnExecuteError(context.Background(), spy, "sess-1", "", errors.New("boom"))
	if spy.calls != 0 {
		t.Fatalf("FinishSessionRunLifecycle calls = %d, want 0 for empty runID", spy.calls)
	}
}

// lifecycle 缺失（nil）必须静默跳过，不得 panic。
func TestFinishSessionRunOnExecuteError_NilLifecycleSkips(t *testing.T) {
	finishSessionRunOnExecuteError(context.Background(), nil, "sess-1", "srid-1", errors.New("boom"))
}
