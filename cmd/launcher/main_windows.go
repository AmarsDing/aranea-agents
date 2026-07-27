//go:build windows

package main

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// hideConsoleWindow hides console-subsystem tools (psql/pg_ctl/redis/server).
// Do NOT use this for GUI apps (Tauri/desktop) — HideWindow makes the window invisible.
func hideConsoleWindow(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// showGUIWindow starts a GUI process normally (visible window).
func showGUIWindow(cmd *exec.Cmd) {
	// Leave SysProcAttr nil / default so the window is shown.
	cmd.SysProcAttr = &syscall.SysProcAttr{}
}

func init() {
	acquireMutexWindows = func(name string) (func(), bool) {
		n, err := syscall.UTF16PtrFromString(name)
		if err != nil {
			return func() {}, false
		}
		handle, err := windows.CreateMutex(nil, false, n)
		if err == windows.ERROR_ALREADY_EXISTS {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
			return func() {}, true
		}
		if err != nil {
			return func() {}, false
		}
		return func() {
			_ = windows.ReleaseMutex(handle)
			_ = windows.CloseHandle(handle)
		}, false
	}

	messageBoxWindows = func(title, msg string, isError bool) {
		t, _ := syscall.UTF16PtrFromString(title)
		m, _ := syscall.UTF16PtrFromString(msg)
		user32 := windows.NewLazySystemDLL("user32.dll")
		proc := user32.NewProc("MessageBoxW")
		const mbOK = 0x00000000
		const mbIconError = 0x00000010
		const mbIconInfo = 0x00000040
		// MB_TOPMOST | MB_SETFOREGROUND | MB_TASKMODAL: 保证 MessageBox 永远在前台可见。
		// 否则在 NSIS 后台调用或 start.bat 启动时，窗口会被遮盖，用户找不到导致"卡住"。
		const mbTopmost = 0x00040000
		const mbSetForeground = 0x00010000
		const mbTaskModal = 0x00002000
		flags := uintptr(mbOK | mbIconInfo | mbTopmost | mbSetForeground | mbTaskModal)
		if isError {
			flags = uintptr(mbOK | mbIconError | mbTopmost | mbSetForeground | mbTaskModal)
		}
		_, _, _ = proc.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	}
}
