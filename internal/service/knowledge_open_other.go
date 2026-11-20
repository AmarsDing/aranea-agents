//go:build !windows

package service

import (
	"os/exec"
	"runtime"
)

// launchDefaultApp macOS 用 open；其余（容器里的 Linux 一般没有桌面）报不支持，前端改成下载。
func launchDefaultApp(abs string) error {
	if runtime.GOOS != "darwin" {
		return errOpenUnsupported
	}
	return exec.Command("open", abs).Start()
}
