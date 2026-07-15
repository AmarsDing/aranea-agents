//go:build !windows

package main

import "os/exec"

func hideConsoleWindow(cmd *exec.Cmd) {}

func init() {
	acquireMutexWindows = func(name string) (func(), bool) {
		return func() {}, false
	}
	messageBoxWindows = func(title, msg string) {}
}
