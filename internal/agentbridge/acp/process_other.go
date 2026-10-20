//go:build !windows

package acp

import (
	"errors"
	"os/exec"
	"syscall"
)

// setProcAttrs 让子进程成为新进程组组长。
func setProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessTree 向整个进程组发 SIGKILL。
func killProcessTree(cmd *exec.Cmd) {
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

// ReapOrphanPID M4 P0-4（S4）：服务重启后按任务行 child_pid（进程组根）收割
// 孤儿子进程树——负 pgid SIGKILL 整组终止。尽力语义：ESRCH（已退出）视为成功；
// pid<=0 no-op。
func ReapOrphanPID(pid int) error {
	if pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	return nil
}

