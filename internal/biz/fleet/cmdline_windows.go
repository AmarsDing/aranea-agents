//go:build windows

package fleet

import (
	"os/exec"
	"syscall"
)

// setRawCmdLine 让 cmd.exe 收到原样命令行（Go 默认的参数转义会被 cmd 的引号规则拆坏）。
func setRawCmdLine(cmd *exec.Cmd, line string) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
}
