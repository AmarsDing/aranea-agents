//go:build windows

package acp

import (
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
