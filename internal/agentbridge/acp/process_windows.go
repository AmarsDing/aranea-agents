//go:build windows

package acp

import (
	"errors"
	"os/exec"
	"strconv"
	"syscall"
)

// setProcAttrs 让子进程归属新进程组，便于整组终止。
func setProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killProcessTree 用 taskkill 终止整棵进程树（npx 场景下 agent 是 node 孙进程）。
func killProcessTree(cmd *exec.Cmd) {
	pid := cmd.Process.Pid
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}

// ReapOrphanPID M4 P0-4（S4）：服务重启后按任务行 child_pid（进程组根）收割
// 孤儿子进程树——整树 taskkill /T /F，覆盖 npx→node 孙进程。尽力语义：
// PID 已退出（taskkill 退出码 128 / not found）视为成功；pid<=0 no-op。
func ReapOrphanPID(pid int) error {
	if pid <= 0 {
		return nil
	}
	err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	if err == nil {
		return nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 128 {
		return nil
	}
	return err
}

