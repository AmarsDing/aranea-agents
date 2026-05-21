package codeexecutor

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

var (
	dockerProbeMu sync.Mutex
	dockerOK      bool
	dockerProbed  bool
)

// DockerAvailable reports whether the docker CLI responds within timeout.
func DockerAvailable() bool {
	dockerProbeMu.Lock()
	defer dockerProbeMu.Unlock()
	if dockerProbed {
		return dockerOK
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	err := cmd.Run()
	dockerOK = err == nil
	dockerProbed = true
	return dockerOK
}

// ResetDockerProbe clears the cached probe result (tests only).
func ResetDockerProbe() {
	dockerProbeMu.Lock()
	defer dockerProbeMu.Unlock()
	dockerProbed = false
	dockerOK = false
}
