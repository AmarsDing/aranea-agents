//go:build !windows

package main

import "fmt"

func installAutostart(root string, st *setupState, log func(string, ...any)) (string, error) {
	return "", fmt.Errorf("auto-start registration is only supported on Windows")
}

func uninstallAutostart(log func(string, ...any)) {}
