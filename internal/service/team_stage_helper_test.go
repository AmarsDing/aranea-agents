package service

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// levelCountingLogger 按级别计数，用于断言降噪后的日志级别。
type levelCountingLogger struct {
	mu         sync.Mutex
	debugCount int
	warnCount  int
}

func (l *levelCountingLogger) Debug(string, ...loggateway.Field) {
	l.mu.Lock()
	l.debugCount++
	l.mu.Unlock()
}
func (l *levelCountingLogger) Info(string, ...loggateway.Field) {}
func (l *levelCountingLogger) Warn(string, ...loggateway.Field) {
	l.mu.Lock()
	l.warnCount++
	l.mu.Unlock()
}
func (l *levelCountingLogger) Error(string, ...loggateway.Field)          {}
func (l *levelCountingLogger) With(...loggateway.Field) loggateway.Logger { return l }

func (l *levelCountingLogger) counts() (debug, warn int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.debugCount, l.warnCount
}

// 00:52 会话复测取证：团队重复启动事件（from=running event=start）触发
// 「transition rejected by state machine」Warn 噪音。幂等重复投递（事件
// 目标态 == 当前态）应静默跳过（Debug）；终态冲突（completed→fail 等
// P0-2 取消竞态防护路径）必须保持 Warn。
func TestResolveTeamStageUpdate_IdempotentDuplicateIsQuiet(t *testing.T) {
	stage := biz.TeamStage{
		ID: "ts-1", TeamID: "team-1", SessionID: "sess-1",
		Status: biz.TeamStageStatusRunning, Stage: biz.TeamStageStageExecuting,
		Version: 3,
	}
	reader := &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{"ts-1": stage}}
	lg := &levelCountingLogger{}

	newStatus, newVersion, ok := resolveTeamStageUpdate(
		context.Background(), reader, biz.NewTeamStageStateMachine(),
		"ts-1", biz.TeamStageEventStart, biz.TeamStageStatusRunning, lg)

	if ok {
		t.Fatal("duplicate start on running stage must still skip publish (ok=false)")
	}
	if newStatus != biz.TeamStageStatusRunning || newVersion != 3 {
		t.Fatalf("duplicate start must return current state unchanged, got status=%s version=%d", newStatus, newVersion)
	}
	_, warn := lg.counts()
	if warn != 0 {
		t.Fatalf("idempotent duplicate (target==current) must not log Warn, got %d", warn)
	}
}

func TestResolveTeamStageUpdate_TerminalConflictStillWarns(t *testing.T) {
	stage := biz.TeamStage{
		ID: "ts-1", TeamID: "team-1", SessionID: "sess-1",
		Status: biz.TeamStageStatusCompleted, Stage: biz.TeamStageStageExecuting,
		Version: 5,
	}
	reader := &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{"ts-1": stage}}
	lg := &levelCountingLogger{}

	// P0-2 取消竞态防护：终态权威，迟到的冲突事件（目标态 != 当前终态）必须 Warn。
	_, _, ok := resolveTeamStageUpdate(
		context.Background(), reader, biz.NewTeamStageStateMachine(),
		"ts-1", biz.TeamStageEventFail, biz.TeamStageStatusFailed, lg)

	if ok {
		t.Fatal("fail on completed stage must skip publish (ok=false)")
	}
	_, warn := lg.counts()
	if warn != 1 {
		t.Fatalf("terminal conflict (target!=current) must log Warn, got %d", warn)
	}
}

func TestResolveTeamStageUpdate_NormalTransitionUnchanged(t *testing.T) {
	stage := biz.TeamStage{
		ID: "ts-1", TeamID: "team-1", SessionID: "sess-1",
		Status: biz.TeamStageStatusPending, Stage: biz.TeamStageStageExecuting,
		Version: 1,
	}
	reader := &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{"ts-1": stage}}
	lg := &levelCountingLogger{}

	newStatus, newVersion, ok := resolveTeamStageUpdate(
		context.Background(), reader, biz.NewTeamStageStateMachine(),
		"ts-1", biz.TeamStageEventStart, biz.TeamStageStatusRunning, lg)

	if !ok || newStatus != biz.TeamStageStatusRunning || newVersion != 2 {
		t.Fatalf("normal pending→start transition broken: ok=%v status=%s version=%d", ok, newStatus, newVersion)
	}
	_, warn := lg.counts()
	if warn != 0 {
		t.Fatalf("normal transition must not log Warn, got %d", warn)
	}
}
