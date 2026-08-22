package codeexecutor

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

// localProbeInterpreters lists the binaries the framework local executor
// shells out to (trpc-agent-go codeexecutor/local buildCommandArgs).
var localProbeInterpreters = []string{"python3", "bash"}

// localProbeTTL mirrors dockerProbeTTL: probing spawns one process per binary.
const localProbeTTL = 30 * time.Second

// localUnavailableReason explains why the local backend is not selectable.
const localUnavailableReason = "no runnable python3/bash interpreter"

var (
	localProbeMu sync.Mutex
	localProbeAt time.Time
	localProbeOK bool
)

// localProbeRun actually executes `<name> --version`: LookPath alone is not
// sufficient because Windows Store alias stubs (python3.exe) and WSL bash.exe
// sit in PATH yet fail to run.
var localProbeRun = func(name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, "--version").Run() == nil
}

var localNow = time.Now

// LocalAvailable reports whether at least one interpreter required by the
// framework local executor (python3 / bash) actually runs. The result is
// cached for localProbeTTL; the second return value is the reason when false.
func LocalAvailable() (bool, string) {
	localProbeMu.Lock()
	defer localProbeMu.Unlock()
	if !localProbeAt.IsZero() && localNow().Sub(localProbeAt) < localProbeTTL {
		return localProbeResult()
	}
	ok := false
	for _, bin := range localProbeInterpreters {
		if localProbeRun(bin) {
			ok = true
			break
		}
	}
	localProbeOK = ok
	localProbeAt = localNow()
	return localProbeResult()
}

func localProbeResult() (bool, string) {
	if localProbeOK {
		return true, ""
	}
	return false, localUnavailableReason
}

// ResetLocalProbe clears the cached probe result (tests only).
func ResetLocalProbe() {
	localProbeMu.Lock()
	defer localProbeMu.Unlock()
	localProbeAt = time.Time{}
	localProbeOK = false
}
