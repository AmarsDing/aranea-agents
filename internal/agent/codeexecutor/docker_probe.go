package codeexecutor

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

// dockerProbeTTL bounds how long an availability result is cached, so a
// daemon started after this process becomes visible without a restart.
const dockerProbeTTL = 30 * time.Second

var (
	dockerProbeMu sync.Mutex
	dockerOK      bool
	dockerProbeAt time.Time
)

// Probe hooks are package vars so tests can stub process execution and time.
var dockerProbeRun = func() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	return cmd.Run() == nil
}

var dockerNow = time.Now

// DockerAvailable reports whether the docker CLI responds within timeout.
// The result is cached for dockerProbeTTL to avoid spawning a process per call.
func DockerAvailable() bool {
	dockerProbeMu.Lock()
	defer dockerProbeMu.Unlock()
	if !dockerProbeAt.IsZero() && dockerNow().Sub(dockerProbeAt) < dockerProbeTTL {
		return dockerOK
	}
	dockerOK = dockerProbeRun()
	dockerProbeAt = dockerNow()
	return dockerOK
}

// ResetDockerProbe clears the cached probe result (tests only).
func ResetDockerProbe() {
	dockerProbeMu.Lock()
	defer dockerProbeMu.Unlock()
	dockerProbeAt = time.Time{}
	dockerOK = false
}
