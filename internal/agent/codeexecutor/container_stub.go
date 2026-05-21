//go:build !codeexec_container

package codeexecutor

import (
	"fmt"

	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

func tryNewContainerExecutor() (trpcagentcodeexec.CodeExecutor, error) {
	return nil, fmt.Errorf("container: build without codeexec_container tag")
}

// ContainerBuildEnabled reports whether the container backend was compiled in.
func ContainerBuildEnabled() bool {
	return false
}
