//go:build windows

package main

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// statusConsole is a visible UTF-16 console panel for startup guidance.
// Uses AllocConsole + WriteConsoleW so Chinese never depends on OEM code page.
type statusConsole struct {
	mu     sync.Mutex
	out    windows.Handle
	closed bool
}

func openStatusConsole(title string) *statusConsole {
	k32 := windows.NewLazySystemDLL("kernel32.dll")
	alloc := k32.NewProc("AllocConsole")
	_, _, _ = alloc.Call()

	_ = setConsoleCP(65001)
	_ = setConsoleOutputCP(65001)

	if title != "" {
		t, err := syscall.UTF16PtrFromString(title)
		if err == nil {
			setTitle := k32.NewProc("SetConsoleTitleW")
			_, _, _ = setTitle.Call(uintptr(unsafe.Pointer(t)))
		}
	}

	h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil || h == 0 || h == windows.InvalidHandle {
		return &statusConsole{closed: true}
	}
	c := &statusConsole{out: h}
	c.Println(appTitle + " - startup console")
	c.Println("Encoding: WriteConsoleW (UTF-16).")
	c.Println("")
	return c
}

func (c *statusConsole) Println(s string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.out == 0 {
		return
	}
	writeConsoleW(c.out, s+"\r\n")
}

func (c *statusConsole) Printf(format string, args ...any) {
	c.Println(fmt.Sprintf(format, args...))
}

func (c *statusConsole) WaitDismiss(prompt string) {
	if c == nil {
		return
	}
	if prompt == "" {
		prompt = "Press Enter to close..."
	}
	c.Println("")
	c.Println(prompt)
	in, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil || in == 0 || in == windows.InvalidHandle {
		return
	}
	var buf [1]uint16
	var read uint32
	for {
		err := windows.ReadConsole(in, &buf[0], 1, &read, nil)
		if err != nil || read == 0 {
			return
		}
		if buf[0] == '\r' || buf[0] == '\n' {
			return
		}
	}
}

func (c *statusConsole) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	k32 := windows.NewLazySystemDLL("kernel32.dll")
	free := k32.NewProc("FreeConsole")
	_, _, _ = free.Call()
}

func writeConsoleW(h windows.Handle, s string) {
	u, err := syscall.UTF16FromString(s)
	if err != nil || len(u) == 0 {
		return
	}
	n := uint32(len(u) - 1)
	if n == 0 {
		return
	}
	var written uint32
	_ = windows.WriteConsole(h, &u[0], n, &written, nil)
}

func setConsoleCP(cp uint32) error {
	k32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := k32.NewProc("SetConsoleCP")
	r, _, err := proc.Call(uintptr(cp))
	if r == 0 {
		return err
	}
	return nil
}

func setConsoleOutputCP(cp uint32) error {
	k32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := k32.NewProc("SetConsoleOutputCP")
	r, _, err := proc.Call(uintptr(cp))
	if r == 0 {
		return err
	}
	return nil
}
