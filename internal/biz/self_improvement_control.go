package biz

import (
	"errors"
	"sync"

	"aranea-agents/pkg/apierror"
)

// ── User intervention control plane (73-self-iteration-v3, T3.6 / D5) ───────
//
// 用户介入指令（暂停/跳过重试/强制回滚）经 chat 命令解析后 Issue 进控制面；
// 正在执行的流水线在阶段边界 Poll 消费：
//
//	pause      → 流水线以 ErrSIRunPaused 退出，run 停留当前非终态（恢复入口属 Phase 4）
//	skip_retry → 下一次 verify 失败不再回 Patcher，直接 verify_failed 终态
//	rollback   → pre-apply 中止：run → rejected（applied/observing 的回滚属 Phase 4 Applier）
//
// 控制面为内存态（单进程流水线与 chat 命令同进程）；nil 控制面时流水线行为不变。

// ErrSIRunPaused is returned by Execute when a user pause command is consumed.
// The run stays in its current non-terminal status for later resumption.
var ErrSIRunPaused = errors.New("self-improvement run paused by user")

// SIControlCommand is a user intervention command for one run.
type SIControlCommand string

const (
	SIControlPause     SIControlCommand = "pause"
	SIControlSkipRetry SIControlCommand = "skip_retry"
	SIControlRollback  SIControlCommand = "rollback"
)

// ParseSIControlCommand parses a chat-command token into an SIControlCommand.
func ParseSIControlCommand(s string) (SIControlCommand, error) {
	switch cmd := SIControlCommand(s); cmd {
	case SIControlPause, SIControlSkipRetry, SIControlRollback:
		return cmd, nil
	default:
		return "", apierror.BadRequest(apierror.DomainSkill, "unknown self-improvement control command %q (want pause|skip_retry|rollback)", s)
	}
}

// SIControlPlane holds pending per-run user intervention commands.
// Safe for concurrent use (chat parser goroutine vs pipeline goroutine).
type SIControlPlane struct {
	mu      sync.Mutex
	pending map[string]SIControlCommand
}

// NewSIControlPlane creates an empty control plane.
func NewSIControlPlane() *SIControlPlane {
	return &SIControlPlane{pending: map[string]SIControlCommand{}}
}

// Issue enqueues a command for a run; a newer command replaces the pending one.
func (c *SIControlPlane) Issue(runID string, cmd SIControlCommand) error {
	if runID == "" {
		return errors.New("si control: empty runID")
	}
	if _, err := ParseSIControlCommand(string(cmd)); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending[runID] = cmd
	return nil
}

// Poll consumes the pending command of a run. Second call returns false.
func (c *SIControlPlane) Poll(runID string) (SIControlCommand, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cmd, ok := c.pending[runID]
	if ok {
		delete(c.pending, runID)
	}
	return cmd, ok
}

// Clear drops the pending command of a run (e.g. run reached a terminal state).
func (c *SIControlPlane) Clear(runID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, runID)
}
