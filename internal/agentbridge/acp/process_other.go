//go:build !windows

package acp

import (
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
