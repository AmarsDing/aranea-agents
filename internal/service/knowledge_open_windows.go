//go:build windows

package service

import "golang.org/x/sys/windows"

// launchDefaultApp 用 ShellExecute 以默认程序打开文件。不经 cmd /c start：
// 文件名里的 & | ^ 会被 cmd 当成命令分隔符。
func launchDefaultApp(abs string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
