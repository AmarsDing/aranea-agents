package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func stopAll(root string, log func(string, ...any)) error {
	mode := readModeFile(root)
	_ = killImage("AraneaAgents.exe")
	_ = killImage("aranea-server.exe")

	if mode.StartedBundledRedis || mode.RedisMode == "bundled" {
		_ = killImage("redis-server.exe")
		log("stopped bundled redis")
	} else {
		log("skip system redis stop")
	}
	if mode.StartedBundledPG || mode.PGMode == "bundled" {
		pgCtl := filepath.Join(root, "postgres", "bin", "pg_ctl.exe")
		pgData := filepath.Join(root, "postgres", "data")
		if _, err := os.Stat(pgCtl); err == nil {
			if _, err := os.Stat(filepath.Join(pgData, "PG_VERSION")); err == nil {
				cmd := hiddenCmd(pgCtl, "stop", "-D", pgData, "-m", "fast")
				_ = cmd.Run()
				log("stopped bundled postgres")
			}
		}
	} else {
		log("skip system postgres stop")
	}
	log("all aranea-managed services stopped")
	return nil
}

func hiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	hideConsoleWindow(cmd)
	return cmd
}

func guiCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	showGUIWindow(cmd)
	return cmd
}

func processRunning(image string) bool {
	cmd := hiddenCmd("tasklist", "/FI", "IMAGENAME eq "+image, "/NH")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(image))
}

func killImage(image string) error {
	cmd := hiddenCmd("taskkill", "/IM", image, "/F", "/T")
	_ = cmd.Run()
	return nil
}

func acquireSingleInstance() (release func(), alreadyRunning bool) {
	if runtime.GOOS != "windows" {
		return func() {}, false
	}
	return acquireMutexWindows(mutexName)
}

func showError(title, msg string) {
	if runtime.GOOS == "windows" {
		messageBoxWindows(appTitle+" — "+title, msg, true)
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, msg)
}

func showInfo(title, msg string) {
	if runtime.GOOS == "windows" {
		messageBoxWindows(appTitle+" — "+title, msg, false)
		return
	}
	fmt.Fprintf(os.Stdout, "%s: %s\n", title, msg)
}

func execLookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func readLogTail(path string, maxLines int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(无法读取 " + path + ")"
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	s := strings.TrimSpace(strings.Join(lines, "\n"))
	if s == "" {
		return "(日志为空)"
	}
	if len(s) > 1500 {
		return s[len(s)-1500:]
	}
	return s
}

// Windows-specific helpers live in main_windows.go / main_stub.go.
var (
	acquireMutexWindows func(name string) (func(), bool)
	messageBoxWindows   func(title, msg string, isError bool)
)
