//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	autostartTaskName = "AraneaAgents"
	serviceName       = "AraneaAgents"
)

// installAutostart registers machine-boot auto-start and returns the kind used.
//
// Judgment (bundled PostgreSQL refuses to run as SYSTEM/administrator):
//   - system PG + system Redis → Windows service (delayed-auto)
//   - anything bundled         → scheduled task at user logon (non-elevated)
//
// Falls back to the scheduled task when service registration is denied
// (e.g. not elevated).
func installAutostart(root string, st *setupState, log func(string, ...any)) (string, error) {
	exe := filepath.Join(root, "AraneaLauncher.exe")
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("missing %s", exe)
	}
	if st != nil && st.PGMode == "system" && st.RedisMode == "system" {
		if err := installService(exe, log); err != nil {
			log("service install failed (%v); falling back to logon task", err)
		} else {
			return "service", nil
		}
	}
	if err := installLogonTask(exe); err != nil {
		return "", err
	}
	return "task", nil
}

// installService creates (or updates) and starts the AraneaAgents Windows service.
// The launcher binary detects the service context via svc.IsWindowsService.
func installService(exe string, log func(string, ...any)) error {
	bin := fmt.Sprintf("\"%s\"", exe)
	create := hiddenCmd("sc.exe", "create", serviceName,
		"binPath=", bin, "start=", "delayed-auto", "DisplayName=", "Aranea-Agents")
	out, err := create.CombinedOutput()
	if err != nil && strings.Contains(string(out), "1073") { // ERROR_SERVICE_EXISTS
		cfg := hiddenCmd("sc.exe", "config", serviceName,
			"binPath=", bin, "start=", "delayed-auto", "DisplayName=", "Aranea-Agents")
		out, err = cfg.CombinedOutput()
	}
	if err != nil {
		return fmt.Errorf("sc create: %v: %s", err, strings.TrimSpace(string(out)))
	}
	desc := hiddenCmd("sc.exe", "description", serviceName,
		"Aranea-Agents backend (system PostgreSQL/Redis mode)")
	_ = desc.Run()
	start := hiddenCmd("sc.exe", "start", serviceName)
	if out, err := start.CombinedOutput(); err != nil {
		// 1056 = already running — not a failure.
		if !strings.Contains(string(out), "1056") {
			log("sc start warn: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// installLogonTask registers a per-user scheduled task that starts the stack
// headless at logon. Runs non-elevated so bundled PostgreSQL is allowed.
func installLogonTask(exe string) error {
	tr := fmt.Sprintf("\"%s\" -headless", exe)
	cmd := hiddenCmd("schtasks", "/Create", "/TN", autostartTaskName,
		"/SC", "ONLOGON", "/TR", tr, "/RL", "LIMITED", "/F")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks create: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// uninstallAutostart removes both registration kinds (best-effort).
func uninstallAutostart(log func(string, ...any)) {
	for _, args := range [][]string{
		{"stop", serviceName},
		{"delete", serviceName},
	} {
		if out, err := hiddenCmd("sc.exe", args...).CombinedOutput(); err == nil {
			log("sc %s %s: %s", args[0], serviceName, strings.TrimSpace(string(out)))
		}
	}
	if out, err := hiddenCmd("schtasks", "/Delete", "/TN", autostartTaskName, "/F").CombinedOutput(); err == nil {
		log("schtasks delete: %s", strings.TrimSpace(string(out)))
	}
}
