//go:build codeexec_container

package codeexecutor

import (
	containerexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/container"

	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

func tryNewContainerExecutor() (trpcagentcodeexec.CodeExecutor, error) {
	return containerexec.New()
}

// ContainerBuildEnabled reports whether the container backend was compiled in.
func ContainerBuildEnabled() bool {
	return true
}
