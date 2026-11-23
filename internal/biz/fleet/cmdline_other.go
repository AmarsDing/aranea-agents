//go:build !windows

package fleet

import "os/exec"

// setRawCmdLine 只有 Windows 需要（.cmd 垫片）；其他系统直接执行工具本身。
func setRawCmdLine(*exec.Cmd, string) {}
